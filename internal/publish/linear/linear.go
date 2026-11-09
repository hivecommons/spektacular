// Package linear files a publication into a Linear team through the
// GraphQL API: the RFC as an issue, each child as a sub-issue of it, and
// each depends_on edge as a "blocks" relation. Linear stores Markdown; the
// marker is the last line of every description, which is also how a
// re-publish finds what it filed, and the line before it carries a digest
// of the body as rendered, so a re-publish compares digests rather than
// trusting Linear to hand the Markdown back byte for byte.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/publish"
)

// DefaultEndpoint is Linear's GraphQL API.
const DefaultEndpoint = "https://api.linear.app/graphql"

// RelationBlocks is the relation type filed for a depends_on edge, read
// from the blocker: the blocker "blocks" the dependant.
const RelationBlocks = "blocks"

// DefaultPageSize is how many nodes each list query asks for; Find pages
// through the rest.
const DefaultPageSize = 250

// DigestPrefix opens the HTML comment that carries the body digest, on the
// line before the marker.
const DigestPrefix = "<!-- spek:digest "

// digestLineRE matches the digest line Find strips before comparing.
var digestLineRE = regexp.MustCompile(`(?m)^<!-- spek:digest ([0-9a-f]+) -->\n`)

// Target files into one Linear team.
type Target struct {
	// Endpoint is the GraphQL URL; DefaultEndpoint when empty.
	Endpoint string
	// TeamKey is the team's key, the prefix of its identifiers: ENG in ENG-12.
	TeamKey string
	// APIKey is sent bare in the Authorization header, as Linear expects
	// for personal API keys.
	APIKey string
	// State, when set, is the name of the workflow state new issues are
	// created in. Unset, the team's "Todo" state is used when it has one,
	// else its first unstarted state, else the team default: a hive reads
	// Todo, not Backlog or Triage.
	State string
	// Assignee, when set, is the email, display name or user name of the
	// user every filed issue is assigned to.
	Assignee string
	// PageSize is how many nodes each list query asks for; DefaultPageSize
	// when 0.
	PageSize int
	// HTTPClient defaults to one with a 30s timeout.
	HTTPClient *http.Client
}

// New returns a target for teamKey.
func New(teamKey, apiKey string) *Target {
	return &Target{Endpoint: DefaultEndpoint, TeamKey: teamKey, APIKey: apiKey, HTTPClient: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements publish.Target.
func (t *Target) Name() string { return "linear" }

// Slug is the team key, what Result.Repo reports.
func (t *Target) Slug() string { return t.TeamKey }

func (t *Target) endpoint() string {
	if t.Endpoint == "" {
		return DefaultEndpoint
	}
	return t.Endpoint
}

func (t *Target) pageSize() int {
	if t.PageSize <= 0 {
		return DefaultPageSize
	}
	return t.PageSize
}

type issueNode struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       struct {
		Type string `json:"type"`
	} `json:"state"`
	Parent *struct {
		ID string `json:"id"`
	} `json:"parent"`
	InverseRelations struct {
		Nodes []struct {
			Type  string `json:"type"`
			Issue struct {
				ID string `json:"id"`
			} `json:"issue"`
		} `json:"nodes"`
	} `json:"inverseRelations"`
}

// issueFields is what every issue read or mutation returns: enough to
// build a Filed, including the parent and the relations pointing at it.
const issueFields = `id identifier url title description state { type } parent { id } inverseRelations { nodes { type issue { id } } }`

// findQuery pages through the team's issues, archived ones included: a
// completed RFC Linear has auto-archived must still be found, so it is
// refused as closed rather than filed again.
const findQuery = `query($team: String!, $needle: String!, $first: Int!, $after: String) {
  issues(filter: { team: { key: { eqIgnoreCase: $team } }, description: { contains: $needle } }, first: $first, after: $after, includeArchived: true) {
    nodes { ` + issueFields + ` }
    pageInfo { hasNextPage endCursor }
  }
}`

// Find implements publish.Target: the team's issues whose description
// carries the artifact id, each read back by its marker.
func (t *Target) Find(ctx context.Context, artifactID string) (*publish.Existing, error) {
	var nodes []issueNode
	var after any
	for {
		var out struct {
			Issues struct {
				Nodes    []issueNode `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		vars := map[string]any{"team": t.TeamKey, "needle": "artifact_id=" + artifactID, "first": t.pageSize(), "after": after}
		if err := t.query(ctx, findQuery, vars, &out); err != nil {
			return nil, fmt.Errorf("searching team %s for published items: %w", t.TeamKey, err)
		}
		nodes = append(nodes, out.Issues.Nodes...)
		if !out.Issues.PageInfo.HasNextPage || out.Issues.PageInfo.EndCursor == "" {
			break
		}
		after = out.Issues.PageInfo.EndCursor
	}
	existing := &publish.Existing{Children: map[string]publish.Filed{}}
	for _, n := range nodes {
		m, ok := publish.ParseMarker(n.Description)
		if !ok || m.ArtifactID != artifactID {
			continue
		}
		f := filed(n, m)
		if m.IsRFC() {
			if existing.RFC == nil || (existing.RFC.Closed && !f.Closed) {
				rfc := f
				existing.RFC = &rfc
			}
			continue
		}
		if prev, dup := existing.Children[m.Task]; !dup || (prev.Closed && !f.Closed) {
			existing.Children[m.Task] = f
		}
	}
	if existing.RFC == nil && len(existing.Children) == 0 {
		return nil, nil
	}
	return existing, nil
}

// filed maps an issue to a Filed: Key is Linear's UUID, what mutations
// take; Ref the identifier, what people and other bodies use. A
// description with a digest line reports BodyDigest and no Body, so the
// comparison does not depend on Linear returning the Markdown verbatim;
// one without (filed before digests) is compared as stored.
func filed(n issueNode, m publish.Marker) publish.Filed {
	f := publish.Filed{
		Key:    n.ID,
		Ref:    n.Identifier,
		URL:    n.URL,
		Title:  n.Title,
		Closed: n.State.Type == "completed" || n.State.Type == "canceled",
		Marker: m,
	}
	if dm := digestLineRE.FindStringSubmatch(n.Description); dm != nil {
		f.BodyDigest = dm[1]
	} else {
		f.Body = n.Description
	}
	if n.Parent != nil {
		f.Parent = n.Parent.ID
	}
	for _, r := range n.InverseRelations.Nodes {
		if r.Type == RelationBlocks && r.Issue.ID != "" {
			f.BlockedBy = append(f.BlockedBy, r.Issue.ID)
		}
	}
	return f
}

// description is body as stored: the digest of body on the line before
// the marker. Find strips the line again, so the digest covers exactly
// what Render produced.
func description(body string) string {
	line := DigestPrefix + publish.Digest(body) + " -->\n"
	if i := strings.LastIndex(body, "<!-- spek:publish"); i >= 0 {
		return body[:i] + line + body[i:]
	}
	return body + line
}

type team struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	States struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"nodes"`
	} `json:"states"`
	Labels struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

// teamQuery reads the team (key compared case-insensitively, as people
// type it), its states and labels, and the workspace labels every team
// shares, in one round trip.
const teamQuery = `query($key: String!, $first: Int!) {
  teams(filter: { key: { eqIgnoreCase: $key } }, first: 1) {
    nodes {
      id key
      states(first: $first) { nodes { id name type } }
      labels(first: $first) { nodes { id name } }
    }
  }
  issueLabels(filter: { team: { null: true } }, first: $first) { nodes { id name } }
}`

// team reads the team's id, states and labels; an unknown key is an error
// before anything is filed. Workspace labels are appended to the team's.
func (t *Target) team(ctx context.Context) (team, error) {
	var out struct {
		Teams struct {
			Nodes []team `json:"nodes"`
		} `json:"teams"`
		IssueLabels struct {
			Nodes []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"nodes"`
		} `json:"issueLabels"`
	}
	if err := t.query(ctx, teamQuery, map[string]any{"key": t.TeamKey, "first": t.pageSize()}, &out); err != nil {
		return team{}, fmt.Errorf("reading team %s: %w", t.TeamKey, err)
	}
	if len(out.Teams.Nodes) == 0 {
		return team{}, fmt.Errorf("team %q does not exist or the API key cannot see it", t.TeamKey)
	}
	tm := out.Teams.Nodes[0]
	tm.Labels.Nodes = append(tm.Labels.Nodes, out.IssueLabels.Nodes...)
	return tm, nil
}

// stateID resolves the state new issues are created in: the configured
// name, else "Todo" when the team has it, else the first unstarted state,
// else "" for the team default.
func (t *Target) stateID(tm team) (string, error) {
	if t.State != "" {
		for _, s := range tm.States.Nodes {
			if strings.EqualFold(s.Name, t.State) {
				return s.ID, nil
			}
		}
		return "", fmt.Errorf("team %s has no workflow state named %q", t.TeamKey, t.State)
	}
	for _, s := range tm.States.Nodes {
		if strings.EqualFold(s.Name, "Todo") {
			return s.ID, nil
		}
	}
	for _, s := range tm.States.Nodes {
		if s.Type == "unstarted" {
			return s.ID, nil
		}
	}
	return "", nil
}

const usersQuery = `query($q: String!) {
  users(filter: { or: [ { email: { eqIgnoreCase: $q } }, { displayName: { eqIgnoreCase: $q } }, { name: { eqIgnoreCase: $q } } ] }, first: 2) {
    nodes { id name displayName email }
  }
}`

// assigneeID resolves Assignee to a user id; "" when none is configured.
// A name matching two users is an error, not a guess.
func (t *Target) assigneeID(ctx context.Context) (string, error) {
	if t.Assignee == "" {
		return "", nil
	}
	var out struct {
		Users struct {
			Nodes []struct {
				ID string `json:"id"`
			} `json:"nodes"`
		} `json:"users"`
	}
	if err := t.query(ctx, usersQuery, map[string]any{"q": t.Assignee}, &out); err != nil {
		return "", fmt.Errorf("resolving assignee %q: %w", t.Assignee, err)
	}
	switch len(out.Users.Nodes) {
	case 0:
		return "", fmt.Errorf("no Linear user has the email, display name or name %q", t.Assignee)
	case 1:
		return out.Users.Nodes[0].ID, nil
	default:
		return "", fmt.Errorf("more than one Linear user matches %q; use the email", t.Assignee)
	}
}

// filing is what creating an issue needs, read lazily so a run that
// creates nothing makes no team, label or user query.
type filing struct {
	team        team
	stateID     string
	assigneeID  string
	rfcLabels   []string
	childLabels []string
}

func (t *Target) prepare(ctx context.Context, opts publish.PublishOptions, res *publish.Result) (*filing, error) {
	tm, err := t.team(ctx)
	if err != nil {
		return nil, err
	}
	stateID, err := t.stateID(tm)
	if err != nil {
		return nil, err
	}
	assigneeID, err := t.assigneeID(ctx)
	if err != nil {
		return nil, err
	}
	return &filing{
		team:        tm,
		stateID:     stateID,
		assigneeID:  assigneeID,
		rfcLabels:   t.pickLabels(tm, res, opts.ParentLabel, opts.Labels...),
		childLabels: t.pickLabels(tm, res, "", opts.Labels...),
	}, nil
}

// pickLabels keeps the wanted labels that exist in the team, warning once
// per missing one. Labels are never created.
func (t *Target) pickLabels(tm team, res *publish.Result, parent string, wanted ...string) []string {
	have := map[string]string{}
	for _, l := range tm.Labels.Nodes {
		have[strings.ToLower(l.Name)] = l.ID
	}
	var out []string
	add := func(name, role string) {
		if name == "" {
			return
		}
		if id, ok := have[strings.ToLower(name)]; ok {
			out = append(out, id)
			return
		}
		w := fmt.Sprintf("%s label %q does not exist in team %s; not applied (publish never creates labels)", role, name, t.TeamKey)
		if !contains(res.Warnings, w) {
			res.Warnings = append(res.Warnings, w)
		}
	}
	add(parent, "parent")
	for _, l := range wanted {
		add(l, "configured")
	}
	return out
}

// Publish implements publish.Target in the same order as the GitHub
// target: the RFC first, then each child as its sub-issue, then the blocks
// relations, then the RFC again with the children's identifiers in its
// checklist. Every child's parent and relations are compared with what
// Find read back and repaired when missing, whichever run filed the
// child; those repairs are best effort and reported as Linked, a failure
// as a warning.
func (t *Target) Publish(ctx context.Context, p publish.Publication, existing *publish.Existing, opts publish.PublishOptions) (publish.Result, error) {
	res := publish.Result{Target: t.Name(), Repo: t.Slug(), Created: []string{}, Updated: []string{}, Unchanged: []string{}, Warnings: []string{}, Children: []publish.ResultChild{}}
	if existing == nil {
		existing = &publish.Existing{Children: map[string]publish.Filed{}}
	}
	var prepared *filing
	ready := func() (*filing, error) {
		if prepared == nil {
			f, err := t.prepare(ctx, opts, &res)
			if err != nil {
				return nil, err
			}
			prepared = f
		}
		return prepared, nil
	}

	refs := map[string]publish.Ref{}
	for id, f := range existing.Children {
		refs[id] = publish.Ref{Ref: f.Ref, URL: f.URL}
	}

	var rfc publish.Filed
	if existing.RFC == nil {
		fl, err := ready()
		if err != nil {
			return res, err
		}
		created, err := t.createIssue(ctx, fl, p.RFC.Title, p.RFCBody(refs), fl.rfcLabels, "")
		if err != nil {
			return res, fmt.Errorf("creating the RFC issue: %w", err)
		}
		rfc = created
		res.Created = append(res.Created, "rfc")
	} else {
		rfc = *existing.RFC
	}
	res.RFCURL = rfc.URL
	res.RFCKey = rfc.Ref

	filedChildren := map[string]publish.Filed{}
	for _, c := range p.Children {
		body := c.Render(p, rfc.URL, refs)
		if f, ok := existing.Children[c.ID]; ok {
			if f.Title != c.Title || !f.SameAsFiled(body) {
				if err := t.updateIssue(ctx, f.Key, c.Title, body); err != nil {
					return res, fmt.Errorf("updating child %s (%s): %w", c.ID, f.Ref, err)
				}
				f.Body, f.BodyDigest = "", publish.Digest(body)
				res.Updated = append(res.Updated, c.ID)
			} else {
				res.Unchanged = append(res.Unchanged, c.ID)
			}
			filedChildren[c.ID] = f
		} else {
			fl, err := ready()
			if err != nil {
				return res, err
			}
			created, err := t.createIssue(ctx, fl, c.Title, body, fl.childLabels, rfc.Key)
			if err != nil {
				return res, fmt.Errorf("creating child %s: %w", c.ID, err)
			}
			filedChildren[c.ID] = created
			refs[c.ID] = publish.Ref{Ref: created.Ref, URL: created.URL}
			res.Created = append(res.Created, c.ID)
		}
		f := filedChildren[c.ID]
		res.Children = append(res.Children, publish.ResultChild{ID: c.ID, URL: f.URL, Key: f.Ref})
	}

	// Second pass: bodies that named children by ID before those existed
	// get their identifiers, and parent and blocks relations are
	// reconciled with what the issues carry.
	for _, c := range p.Children {
		f := filedChildren[c.ID]
		body := c.Render(p, rfc.URL, refs)
		wasCreated := contains(res.Created, c.ID)
		if !f.SameAsFiled(body) {
			if err := t.updateIssue(ctx, f.Key, c.Title, body); err != nil {
				return res, fmt.Errorf("updating child %s (%s): %w", c.ID, f.Ref, err)
			}
			if !wasCreated && !contains(res.Updated, c.ID) {
				res.Updated = append(res.Updated, c.ID)
				res.Unchanged = remove(res.Unchanged, c.ID)
			}
		}
		if f.Parent != rfc.Key {
			t.setParent(ctx, f, rfc, &res)
		}
		for _, dep := range c.DependsOn {
			blocker, ok := filedChildren[dep]
			if !ok || contains(f.BlockedBy, blocker.Key) {
				continue
			}
			t.blockedBy(ctx, f, blocker, &res)
		}
	}

	rfcBody := p.RFCBody(refs)
	if existing.RFC == nil {
		if err := t.updateIssue(ctx, rfc.Key, p.RFC.Title, rfcBody); err != nil {
			return res, fmt.Errorf("updating the RFC checklist (%s): %w", rfc.Ref, err)
		}
	} else if rfc.Title != p.RFC.Title || !rfc.SameAsFiled(rfcBody) {
		if err := t.updateIssue(ctx, rfc.Key, p.RFC.Title, rfcBody); err != nil {
			return res, fmt.Errorf("updating the RFC (%s): %w", rfc.Ref, err)
		}
		res.Updated = append(res.Updated, "rfc")
	} else {
		res.Unchanged = append(res.Unchanged, "rfc")
	}
	return res, nil
}

const createMutation = `mutation($input: IssueCreateInput!) {
  issueCreate(input: $input) { success issue { ` + issueFields + ` } }
}`

const updateMutation = `mutation($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) { success }
}`

const relationMutation = `mutation($input: IssueRelationCreateInput!) {
  issueRelationCreate(input: $input) { success }
}`

func (t *Target) createIssue(ctx context.Context, fl *filing, title, body string, labelIDs []string, parentID string) (publish.Filed, error) {
	input := map[string]any{"teamId": fl.team.ID, "title": title, "description": description(body)}
	if len(labelIDs) > 0 {
		input["labelIds"] = labelIDs
	}
	if fl.stateID != "" {
		input["stateId"] = fl.stateID
	}
	if fl.assigneeID != "" {
		input["assigneeId"] = fl.assigneeID
	}
	if parentID != "" {
		input["parentId"] = parentID
	}
	var out struct {
		IssueCreate struct {
			Success bool      `json:"success"`
			Issue   issueNode `json:"issue"`
		} `json:"issueCreate"`
	}
	if err := t.query(ctx, createMutation, map[string]any{"input": input}, &out); err != nil {
		return publish.Filed{}, err
	}
	if !out.IssueCreate.Success || out.IssueCreate.Issue.ID == "" {
		return publish.Filed{}, errors.New("issueCreate reported no issue")
	}
	m, _ := publish.ParseMarker(body)
	f := filed(out.IssueCreate.Issue, m)
	f.Body, f.BodyDigest = "", publish.Digest(body)
	return f, nil
}

func (t *Target) updateIssue(ctx context.Context, id, title, body string) error {
	var out struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	input := map[string]any{"title": title, "description": description(body)}
	if err := t.query(ctx, updateMutation, map[string]any{"id": id, "input": input}, &out); err != nil {
		return err
	}
	if !out.IssueUpdate.Success {
		return errors.New("issueUpdate reported failure")
	}
	return nil
}

// setParent makes issue a sub-issue of rfc again, best effort.
func (t *Target) setParent(ctx context.Context, issue, rfc publish.Filed, res *publish.Result) {
	var out struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	err := t.query(ctx, updateMutation, map[string]any{"id": issue.Key, "input": map[string]any{"parentId": rfc.Key}}, &out)
	if err == nil && !out.IssueUpdate.Success {
		err = errors.New("issueUpdate reported failure")
	}
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not put %s under %s: %v", issue.Ref, rfc.Ref, err))
		return
	}
	res.Linked = append(res.Linked, issue.Ref+" under "+rfc.Ref)
}

// blockedBy files blocker → issue as a blocks relation, best effort.
func (t *Target) blockedBy(ctx context.Context, issue, blocker publish.Filed, res *publish.Result) {
	input := map[string]any{"issueId": blocker.Key, "relatedIssueId": issue.Key, "type": RelationBlocks}
	var out struct {
		IssueRelationCreate struct {
			Success bool `json:"success"`
		} `json:"issueRelationCreate"`
	}
	err := t.query(ctx, relationMutation, map[string]any{"input": input}, &out)
	if err == nil && !out.IssueRelationCreate.Success {
		err = errors.New("issueRelationCreate reported failure")
	}
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not mark %s as blocked by %s: %v", issue.Ref, blocker.Ref, err))
		return
	}
	res.Linked = append(res.Linked, issue.Ref+" blocked by "+blocker.Ref)
}

// apiError is a non-2xx response or a GraphQL errors array.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("linear API %d: %s", e.Status, e.Message)
	}
	return "linear API: " + e.Message
}

// query posts one GraphQL document and decodes data into out.
func (t *Target) query(ctx context.Context, doc string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": doc, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", t.APIKey)
	client := t.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &apiError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decoding linear response: %w", err)
	}
	if len(env.Errors) > 0 {
		msgs := make([]string, 0, len(env.Errors))
		for _, e := range env.Errors {
			msgs = append(msgs, e.Message)
		}
		return &apiError{Message: strings.Join(msgs, "; ")}
	}
	if out != nil && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return nil
}

func errorMessage(raw []byte) string {
	var env struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &env) == nil {
		if len(env.Errors) > 0 {
			return env.Errors[0].Message
		}
		if env.Message != "" {
			return env.Message
		}
	}
	return strings.TrimSpace(string(raw))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func remove(list []string, s string) []string {
	out := list[:0]
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}
