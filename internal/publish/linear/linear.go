// Package linear files a publication into a Linear team through the
// GraphQL API: the RFC as an issue, each child as a sub-issue of it, and
// each depends_on edge as a "blocks" relation. Linear stores Markdown, so
// bodies round-trip verbatim and the marker is the last line of every
// description, which is also how a re-publish finds what it filed.
package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/publish"
)

// DefaultEndpoint is Linear's GraphQL API.
const DefaultEndpoint = "https://api.linear.app/graphql"

// RelationBlocks is the relation type filed for a depends_on edge, read
// from the blocker: the blocker "blocks" the dependant.
const RelationBlocks = "blocks"

// pageSize bounds every list the target reads; a team with more labels or
// filed items than this is not a shape publish produces.
const pageSize = 250

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
	// created in; the team's default otherwise.
	State string
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

type issueNode struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       struct {
		Type string `json:"type"`
	} `json:"state"`
}

const findQuery = `query($team: String!, $needle: String!, $first: Int!) {
  issues(filter: { team: { key: { eq: $team } }, description: { contains: $needle } }, first: $first) {
    nodes { id identifier url title description state { type } }
  }
}`

// Find implements publish.Target: the team's issues whose description
// carries the artifact id, each read back by its marker.
func (t *Target) Find(ctx context.Context, artifactID string) (*publish.Existing, error) {
	var out struct {
		Issues struct {
			Nodes []issueNode `json:"nodes"`
		} `json:"issues"`
	}
	vars := map[string]any{"team": t.TeamKey, "needle": "artifact_id=" + artifactID, "first": pageSize}
	if err := t.query(ctx, findQuery, vars, &out); err != nil {
		return nil, fmt.Errorf("searching team %s for published items: %w", t.TeamKey, err)
	}
	existing := &publish.Existing{Children: map[string]publish.Filed{}}
	for _, n := range out.Issues.Nodes {
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
// take; Ref the identifier, what people and other bodies use.
func filed(n issueNode, m publish.Marker) publish.Filed {
	return publish.Filed{
		Key:    n.ID,
		Ref:    n.Identifier,
		URL:    n.URL,
		Title:  n.Title,
		Body:   n.Description,
		Closed: n.State.Type == "completed" || n.State.Type == "canceled",
		Marker: m,
	}
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

const teamQuery = `query($key: String!, $first: Int!) {
  teams(filter: { key: { eq: $key } }, first: 1) {
    nodes {
      id key
      states(first: $first) { nodes { id name type } }
      labels(first: $first) { nodes { id name } }
    }
  }
}`

// team reads the team's id, states and labels; an unknown key is an error
// before anything is filed.
func (t *Target) team(ctx context.Context) (team, error) {
	var out struct {
		Teams struct {
			Nodes []team `json:"nodes"`
		} `json:"teams"`
	}
	if err := t.query(ctx, teamQuery, map[string]any{"key": t.TeamKey, "first": pageSize}, &out); err != nil {
		return team{}, fmt.Errorf("reading team %s: %w", t.TeamKey, err)
	}
	if len(out.Teams.Nodes) == 0 {
		return team{}, fmt.Errorf("team %q does not exist or the API key cannot see it", t.TeamKey)
	}
	return out.Teams.Nodes[0], nil
}

// stateID resolves the configured state name; "" means the team default.
func (t *Target) stateID(tm team) (string, error) {
	if t.State == "" {
		return "", nil
	}
	for _, s := range tm.States.Nodes {
		if strings.EqualFold(s.Name, t.State) {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("team %s has no workflow state named %q", t.TeamKey, t.State)
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
// checklist. Relations are best effort: a failure is a warning.
func (t *Target) Publish(ctx context.Context, p publish.Publication, existing *publish.Existing, opts publish.PublishOptions) (publish.Result, error) {
	res := publish.Result{Target: t.Name(), Repo: t.Slug(), Created: []string{}, Updated: []string{}, Unchanged: []string{}, Warnings: []string{}, Children: []publish.ResultChild{}}
	if existing == nil {
		existing = &publish.Existing{Children: map[string]publish.Filed{}}
	}
	tm, err := t.team(ctx)
	if err != nil {
		return res, err
	}
	stateID, err := t.stateID(tm)
	if err != nil {
		return res, err
	}
	rfcLabels := t.pickLabels(tm, &res, opts.ParentLabel, opts.Labels...)
	childLabels := t.pickLabels(tm, &res, "", opts.Labels...)

	refs := map[string]publish.Ref{}
	for id, f := range existing.Children {
		refs[id] = publish.Ref{Ref: f.Ref, URL: f.URL}
	}

	var rfc publish.Filed
	if existing.RFC == nil {
		created, err := t.createIssue(ctx, tm.ID, p.RFC.Title, p.RFCBody(refs), rfcLabels, stateID, "")
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
				f.Body = body
				res.Updated = append(res.Updated, c.ID)
			} else {
				res.Unchanged = append(res.Unchanged, c.ID)
			}
			filedChildren[c.ID] = f
		} else {
			created, err := t.createIssue(ctx, tm.ID, c.Title, body, childLabels, stateID, rfc.Key)
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
	// get their identifiers, and the blocks relations are filed.
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
		for _, dep := range c.DependsOn {
			blocker, ok := filedChildren[dep]
			if !ok {
				continue
			}
			if wasCreated || contains(res.Created, dep) {
				t.blockedBy(ctx, f, blocker, &res)
			}
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
  issueCreate(input: $input) { success issue { id identifier url title description state { type } } }
}`

const updateMutation = `mutation($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) { success }
}`

const relationMutation = `mutation($input: IssueRelationCreateInput!) {
  issueRelationCreate(input: $input) { success }
}`

func (t *Target) createIssue(ctx context.Context, teamID, title, body string, labelIDs []string, stateID, parentID string) (publish.Filed, error) {
	input := map[string]any{"teamId": teamID, "title": title, "description": body}
	if len(labelIDs) > 0 {
		input["labelIds"] = labelIDs
	}
	if stateID != "" {
		input["stateId"] = stateID
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
	return filed(out.IssueCreate.Issue, m), nil
}

func (t *Target) updateIssue(ctx context.Context, id, title, body string) error {
	var out struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	input := map[string]any{"title": title, "description": body}
	if err := t.query(ctx, updateMutation, map[string]any{"id": id, "input": input}, &out); err != nil {
		return err
	}
	if !out.IssueUpdate.Success {
		return errors.New("issueUpdate reported failure")
	}
	return nil
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
	}
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
