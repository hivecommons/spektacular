// Package jira files a publication into a Jira project: the RFC as an Epic,
// each child as a Story (or the configured type) under it, and each
// depends_on edge as a "Blocks" issue link. Jira Cloud (REST v3, ADF
// descriptions) and Data Center (REST v2, wiki markup) are both supported.
//
// Hive's Jira work source enumerates `project in (<keys>) AND statusCategory
// != Done AND issuetype != Epic` by default, so the Epic is never a work
// item and every Story is.
package jira

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/publish"
)

// PropertyKey is the issue property the marker and body digest are stored
// in, so a re-publish finds what it filed without parsing the description.
const PropertyKey = "spek.publish"

// BlocksLinkType is the issue link type a depends_on edge becomes: the
// blocker is the outward issue ("blocks"), the dependant the inward one
// ("is blocked by").
const BlocksLinkType = "Blocks"

// EpicLinkFieldType is the custom-field type of Data Center's Epic Link
// field, which a Story needs to sit under an Epic there: Data Center's
// parent field is for sub-tasks only.
const EpicLinkFieldType = "com.pyxis.greenhopper.jira:gp-epic-link"

const requestTimeout = 30 * time.Second

// Target is the Jira implementation of publish.Target for one project.
type Target struct {
	// BaseURL is the site, https://acme.atlassian.net or a Data Center host.
	BaseURL    string
	ProjectKey string
	// DataCenter selects REST v2 and wiki-markup descriptions; the default
	// is Cloud's REST v3 with ADF.
	DataCenter      bool
	ParentIssueType string
	ChildIssueType  string
	// EpicLinkField is the custom field (customfield_NNNNN) that receives
	// the Epic's key on each child instead of parent. Data Center needs
	// one; when unset there, the site's Epic Link field is discovered from
	// GET field once and cached here. Cloud ignores it and uses parent.
	EpicLinkField string
	// AcceptanceCriteriaField, when set, receives the child's acceptance
	// criteria as text in addition to the description.
	AcceptanceCriteriaField string

	// Email and APIToken authenticate to Cloud with Basic auth; PAT is a
	// Data Center personal access token sent as a Bearer token.
	Email    string
	APIToken string
	PAT      string

	// HTTPClient defaults to one with requestTimeout; NewHTTPClient builds
	// one that trusts a CA bundle.
	HTTPClient *http.Client

	// knownKeys are issues the artifact's frontmatter recorded (Hint); Find
	// reads them directly when the search index has not caught up.
	knownKeys []string
}

// New returns a Cloud target for the project at baseURL.
func New(baseURL, projectKey string) *Target {
	return &Target{BaseURL: strings.TrimRight(baseURL, "/"), ProjectKey: projectKey, ParentIssueType: "Epic", ChildIssueType: "Story"}
}

// NewHTTPClient returns a client that trusts the PEM bundle at caBundle in
// addition to the system roots, or the default client when caBundle is "".
func NewHTTPClient(caBundle string) (*http.Client, error) {
	client := &http.Client{Timeout: requestTimeout}
	if caBundle == "" {
		return client, nil
	}
	pem, err := os.ReadFile(caBundle)
	if err != nil {
		return nil, fmt.Errorf("reading the CA bundle: %w", err)
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("the CA bundle %s holds no PEM certificates", caBundle)
	}
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return client, nil
}

// Name implements publish.Target.
func (t *Target) Name() string { return "jira" }

// Slug is the project key, what Result.Repo reports.
func (t *Target) Slug() string { return t.ProjectKey }

// Hint implements publish.Hinted with the browse URLs of issues filed
// earlier, so a re-run right after a partial failure finds them even
// before Jira's search index does.
func (t *Target) Hint(urls []string) {
	prefix := t.BaseURL + "/browse/"
	for _, u := range urls {
		if key := strings.TrimPrefix(u, prefix); key != u && key != "" && !strings.Contains(key, "/") {
			t.knownKeys = append(t.knownKeys, key)
		}
	}
}

func (t *Target) restPath(p string) string {
	if t.DataCenter {
		return "/rest/api/2/" + p
	}
	return "/rest/api/3/" + p
}

func (t *Target) browseURL(key string) string {
	return t.BaseURL + "/browse/" + key
}

// property is what PropertyKey holds on every filed issue.
type property struct {
	Name       string `json:"name"`
	ArtifactID string `json:"artifact_id"`
	Target     string `json:"target"`
	Task       string `json:"task,omitempty"`
	RFC        string `json:"rfc,omitempty"`
	Title      string `json:"title"`
	BodyDigest string `json:"body_digest"`
}

func (p property) marker() publish.Marker {
	return publish.Marker{Name: p.Name, ArtifactID: p.ArtifactID, Target: p.Target, Task: p.Task, RFC: p.RFC}
}

type searchIssue struct {
	ID     string                     `json:"id"`
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

func (it searchIssue) str(field string) string {
	var v string
	_ = json.Unmarshal(it.Fields[field], &v)
	return v
}

func (it searchIssue) key(field string) string {
	var v struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(it.Fields[field], &v)
	return v.Key
}

var searchFields = []string{"summary", "description", "status", "parent", "issuelinks"}

// Find implements publish.Target. It searches the project for the
// artifact's search token (publish.SearchToken, the one word every filed
// description carries), reads any issue the frontmatter recorded that the
// search did not return, then takes each hit's marker from the spek.publish
// property, or from the description when the property is missing.
//
// The issue.property[...] JQL is not used: Cloud and Data Center both
// answer it only for properties an installed app has declared indexed, so
// a plain site would always fall through to the text search anyway.
func (t *Target) Find(ctx context.Context, artifactID string) (*publish.Existing, error) {
	fields, err := t.readFields(ctx)
	if err != nil {
		return nil, err
	}
	jql := fmt.Sprintf(`project = %s AND description ~ %q`, t.ProjectKey, publish.SearchToken(artifactID))
	issues, err := t.search(ctx, jql, fields)
	if err != nil {
		return nil, fmt.Errorf("searching %s for published items: %w", t.ProjectKey, err)
	}
	seen := map[string]bool{}
	for _, it := range issues {
		seen[it.Key] = true
	}
	for _, key := range t.knownKeys {
		if seen[key] {
			continue
		}
		it, err := t.issue(ctx, key, fields)
		if err != nil {
			var apiErr *apiError
			if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
				continue
			}
			return nil, fmt.Errorf("reading recorded issue %s: %w", key, err)
		}
		seen[key] = true
		issues = append(issues, it)
	}
	existing := &publish.Existing{Children: map[string]publish.Filed{}}
	for _, it := range issues {
		f, ok := t.filedFrom(ctx, it)
		if !ok || f.Marker.ArtifactID != artifactID {
			continue
		}
		if f.Marker.IsRFC() {
			if existing.RFC == nil || (existing.RFC.Closed && !f.Closed) {
				rfc := f
				existing.RFC = &rfc
			}
			continue
		}
		if prev, dup := existing.Children[f.Marker.Task]; !dup || (prev.Closed && !f.Closed) {
			existing.Children[f.Marker.Task] = f
		}
	}
	if existing.RFC == nil && len(existing.Children) == 0 {
		return nil, nil
	}
	return existing, nil
}

// readFields is what a search or issue read asks for: the fixed set plus
// the Epic Link field on Data Center, so a child's Epic can be read back.
func (t *Target) readFields(ctx context.Context) ([]string, error) {
	fields := append([]string{}, searchFields...)
	epic, err := t.epicLink(ctx)
	if err != nil {
		return nil, err
	}
	if epic != "" {
		fields = append(fields, epic)
	}
	return fields, nil
}

// epicLink is the field a child's Epic key goes into: EpicLinkField when
// set; on Data Center without one, the site's Epic Link custom field,
// discovered once from GET field; "" on Cloud, which takes parent.
func (t *Target) epicLink(ctx context.Context) (string, error) {
	if t.EpicLinkField != "" {
		return t.EpicLinkField, nil
	}
	if !t.DataCenter {
		return "", nil
	}
	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Schema struct {
			Custom string `json:"custom"`
		} `json:"schema"`
	}
	if err := t.do(ctx, http.MethodGet, t.restPath("field"), nil, &fields); err != nil {
		return "", fmt.Errorf("discovering the Epic Link field: %w", err)
	}
	for _, f := range fields {
		if f.Schema.Custom == EpicLinkFieldType {
			t.EpicLinkField = f.ID
			return f.ID, nil
		}
	}
	return "", fmt.Errorf("the Data Center site has no Epic Link field, which a %s needs to sit under an %s there (parent is for sub-tasks only); set publish.jira.epic_link_field to the custom field id that links them", t.ChildIssueType, t.ParentIssueType)
}

func (t *Target) search(ctx context.Context, jql string, fields []string) ([]searchIssue, error) {
	var page struct {
		Issues []searchIssue `json:"issues"`
	}
	if t.DataCenter {
		q := url.Values{}
		q.Set("jql", jql)
		q.Set("fields", strings.Join(fields, ","))
		q.Set("maxResults", "100")
		if err := t.do(ctx, http.MethodGet, t.restPath("search")+"?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		return page.Issues, nil
	}
	payload := map[string]any{"jql": jql, "fields": fields, "maxResults": 100}
	if err := t.do(ctx, http.MethodPost, t.restPath("search/jql"), payload, &page); err != nil {
		return nil, err
	}
	return page.Issues, nil
}

// issue reads one issue by key with the given fields.
func (t *Target) issue(ctx context.Context, key string, fields []string) (searchIssue, error) {
	var it searchIssue
	q := url.Values{}
	q.Set("fields", strings.Join(fields, ","))
	err := t.do(ctx, http.MethodGet, t.restPath("issue/"+url.PathEscape(key))+"?"+q.Encode(), nil, &it)
	return it, err
}

// filedFrom turns a search hit into a Filed, reading its marker from the
// property or, failing that, the description text, and its parent and
// blockers from the fields.
func (t *Target) filedFrom(ctx context.Context, it searchIssue) (publish.Filed, bool) {
	var status struct {
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	_ = json.Unmarshal(it.Fields["status"], &status)
	f := publish.Filed{
		Key:    it.Key,
		URL:    t.browseURL(it.Key),
		Ref:    it.Key,
		Title:  it.str("summary"),
		Closed: status.StatusCategory.Key == "done",
		Parent: it.key("parent"),
	}
	if t.EpicLinkField != "" {
		if epic := it.str(t.EpicLinkField); epic != "" {
			f.Parent = epic
		}
	}
	var links []struct {
		Type struct {
			Name string `json:"name"`
		} `json:"type"`
		OutwardIssue *struct {
			Key string `json:"key"`
		} `json:"outwardIssue"`
	}
	_ = json.Unmarshal(it.Fields["issuelinks"], &links)
	for _, l := range links {
		if l.Type.Name == BlocksLinkType && l.OutwardIssue != nil {
			f.BlockedBy = append(f.BlockedBy, l.OutwardIssue.Key)
		}
	}
	var prop struct {
		Value property `json:"value"`
	}
	if err := t.do(ctx, http.MethodGet, t.restPath("issue/"+url.PathEscape(it.Key)+"/properties/"+PropertyKey), nil, &prop); err == nil && prop.Value.ArtifactID != "" {
		f.Marker = prop.Value.marker()
		f.BodyDigest = prop.Value.BodyDigest
		return f, true
	}
	text := t.descriptionText(it.Fields["description"])
	m, ok := publish.ParseMarker(text)
	if !ok {
		return f, false
	}
	f.Marker = m
	f.Body = text
	return f, true
}

// descriptionText flattens a description as the API returned it.
func (t *Target) descriptionText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if t.DataCenter {
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	return ADFText(doc)
}

// description renders a Markdown body in the site's document format, with
// the artifact's search token on the line before the marker: the word Find
// searches for, since the artifact id itself holds characters Jira's text
// search treats as operators.
func (t *Target) description(md string) any {
	if m, ok := publish.ParseMarker(md); ok {
		if i := strings.LastIndex(md, m.String()); i >= 0 {
			md = md[:i] + publish.SearchToken(m.ArtifactID) + "\n" + md[i:]
		}
	}
	return t.render(md)
}

// render converts Markdown to the site's document format.
func (t *Target) render(md string) any {
	if t.DataCenter {
		return toWiki(md)
	}
	return toADF(md)
}

// Publish implements publish.Target. Creation order mirrors the GitHub
// target: the Epic first (its URL goes into every child's marker), then the
// children under it, then the Blocks links, then the Epic's description
// again with the children's keys in its checklist. Every run reconciles
// each child's parent and Blocks links against the publication, so a link
// that failed or a dependency added later is filed by the next run. Links
// are best effort: a failure becomes a warning, never an error.
func (t *Target) Publish(ctx context.Context, p publish.Publication, existing *publish.Existing, opts publish.PublishOptions) (publish.Result, error) {
	res := publish.Result{Target: t.Name(), Repo: t.ProjectKey, Created: []string{}, Updated: []string{}, Unchanged: []string{}, Warnings: []string{}, Children: []publish.ResultChild{}}
	if existing == nil {
		existing = &publish.Existing{Children: map[string]publish.Filed{}}
	}
	labels := make([]string, 0, len(opts.Labels))
	for _, l := range opts.Labels {
		if jl := jiraLabel(l); jl != "" {
			labels = append(labels, jl)
		}
	}
	epicField, err := t.epicLink(ctx)
	if err != nil {
		return res, err
	}

	refs := map[string]publish.Ref{}
	for id, f := range existing.Children {
		refs[id] = publish.Ref{Ref: f.Ref, URL: f.URL}
	}

	var rfc publish.Filed
	if existing.RFC == nil {
		created, err := t.createIssue(ctx, t.ParentIssueType, p.RFC.Title, p.RFCBody(refs), labels, "", epicField, nil)
		if err != nil {
			return res, fmt.Errorf("creating the RFC epic: %w", err)
		}
		rfc = created
		res.Created = append(res.Created, "rfc")
	} else {
		rfc = *existing.RFC
	}
	res.RFCURL = rfc.URL
	res.RFCKey = rfc.Key

	filedChildren := map[string]publish.Filed{}
	for _, c := range p.Children {
		body := c.Render(p, rfc.URL, refs)
		if f, ok := existing.Children[c.ID]; ok {
			if f.Title != c.Title || !f.SameAsFiled(body) {
				if err := t.updateIssue(ctx, f.Key, c.Title, body, c); err != nil {
					return res, fmt.Errorf("updating child %s (%s): %w", c.ID, f.Key, err)
				}
				f.BodyDigest, f.Body = publish.Digest(body), ""
				res.Updated = append(res.Updated, c.ID)
			} else {
				res.Unchanged = append(res.Unchanged, c.ID)
			}
			filedChildren[c.ID] = f
		} else {
			created, err := t.createIssue(ctx, t.ChildIssueType, c.Title, body, labels, rfc.Key, epicField, &c)
			if err != nil {
				return res, fmt.Errorf("creating child %s: %w", c.ID, err)
			}
			filedChildren[c.ID] = created
			refs[c.ID] = publish.Ref{Ref: created.Ref, URL: created.URL}
			res.Created = append(res.Created, c.ID)
		}
		f := filedChildren[c.ID]
		res.Children = append(res.Children, publish.ResultChild{ID: c.ID, URL: f.URL, Key: f.Key})
	}

	// Second pass: bodies that named children by ID before those existed
	// get their keys, and each child's parent and Blocks links are
	// reconciled with what is filed.
	for _, c := range p.Children {
		f := filedChildren[c.ID]
		body := c.Render(p, rfc.URL, refs)
		if !f.SameAsFiled(body) {
			if err := t.updateIssue(ctx, f.Key, c.Title, body, c); err != nil {
				return res, fmt.Errorf("updating child %s (%s): %w", c.ID, f.Key, err)
			}
			if !contains(res.Created, c.ID) && !contains(res.Updated, c.ID) {
				res.Updated = append(res.Updated, c.ID)
				res.Unchanged = remove(res.Unchanged, c.ID)
			}
		}
		if f.Parent != rfc.Key {
			t.setParent(ctx, f, rfc, epicField, &res)
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
		if err := t.updateIssue(ctx, rfc.Key, p.RFC.Title, rfcBody, publish.Child{}); err != nil {
			return res, fmt.Errorf("updating the RFC checklist (%s): %w", rfc.Key, err)
		}
	} else if rfc.Title != p.RFC.Title || !rfc.SameAsFiled(rfcBody) {
		if err := t.updateIssue(ctx, rfc.Key, p.RFC.Title, rfcBody, publish.Child{}); err != nil {
			return res, fmt.Errorf("updating the RFC (%s): %w", rfc.Key, err)
		}
		res.Updated = append(res.Updated, "rfc")
	} else {
		res.Unchanged = append(res.Unchanged, "rfc")
	}
	return res, nil
}

// blockedBy links issue as blocked by blocker, best effort.
func (t *Target) blockedBy(ctx context.Context, issue, blocker publish.Filed, res *publish.Result) {
	payload := map[string]any{
		"type":         map[string]string{"name": BlocksLinkType},
		"inwardIssue":  map[string]string{"key": issue.Key},
		"outwardIssue": map[string]string{"key": blocker.Key},
	}
	if err := t.do(ctx, http.MethodPost, t.restPath("issueLink"), payload, nil); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not link %s as blocked by %s: %v", issue.Key, blocker.Key, err))
		return
	}
	res.Linked = append(res.Linked, issue.Key+" blocked by "+blocker.Key)
}

// setParent puts issue under rfc, best effort: parent, or the Epic Link
// field when the site takes that.
func (t *Target) setParent(ctx context.Context, issue, rfc publish.Filed, epicField string, res *publish.Result) {
	fields := map[string]any{}
	parentFields(fields, rfc.Key, epicField)
	if err := t.do(ctx, http.MethodPut, t.restPath("issue/"+url.PathEscape(issue.Key)), map[string]any{"fields": fields}, nil); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not put %s under %s: %v", issue.Key, rfc.Key, err))
		return
	}
	res.Linked = append(res.Linked, issue.Key+" under "+rfc.Key)
}

// parentFields sets the field that makes an issue a child of parentKey.
func parentFields(fields map[string]any, parentKey, epicField string) {
	if epicField != "" {
		fields[epicField] = parentKey
	} else {
		fields["parent"] = map[string]string{"key": parentKey}
	}
}

// createIssue files one issue with its property written in the same
// request, so no issue ever exists without its marker. parentKey, when
// set, makes it a child of that Epic; child carries the acceptance
// criteria for the configured field.
func (t *Target) createIssue(ctx context.Context, issueType, title, body string, labels []string, parentKey, epicField string, child *publish.Child) (publish.Filed, error) {
	fields := map[string]any{
		"project":     map[string]string{"key": t.ProjectKey},
		"summary":     title,
		"issuetype":   map[string]string{"name": issueType},
		"description": t.description(body),
	}
	if len(labels) > 0 {
		fields["labels"] = labels
	}
	if parentKey != "" {
		parentFields(fields, parentKey, epicField)
	}
	if child != nil {
		t.addAcceptanceCriteria(fields, *child)
	}
	m, _ := publish.ParseMarker(body)
	digest := publish.Digest(body)
	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	payload := map[string]any{"fields": fields, "properties": properties(m, title, digest)}
	if err := t.do(ctx, http.MethodPost, t.restPath("issue"), payload, &created); err != nil {
		return publish.Filed{}, err
	}
	if created.Key == "" {
		return publish.Filed{}, fmt.Errorf("Jira returned no issue key")
	}
	return publish.Filed{Key: created.Key, URL: t.browseURL(created.Key), Ref: created.Key, Title: title, BodyDigest: digest, Marker: m, Parent: parentKey}, nil
}

func (t *Target) updateIssue(ctx context.Context, key, title, body string, child publish.Child) error {
	fields := map[string]any{"summary": title, "description": t.description(body)}
	if child.ID != "" {
		t.addAcceptanceCriteria(fields, child)
	}
	m, _ := publish.ParseMarker(body)
	payload := map[string]any{"fields": fields, "properties": properties(m, title, publish.Digest(body))}
	return t.do(ctx, http.MethodPut, t.restPath("issue/"+url.PathEscape(key)), payload, nil)
}

func (t *Target) addAcceptanceCriteria(fields map[string]any, c publish.Child) {
	if t.AcceptanceCriteriaField == "" || len(c.AcceptanceCriteria) == 0 {
		return
	}
	var b strings.Builder
	for _, ac := range c.AcceptanceCriteria {
		b.WriteString("- " + ac + "\n")
	}
	fields[t.AcceptanceCriteriaField] = t.render(b.String())
}

// properties is the entity-properties list a create or edit request
// carries: the marker and body digest under PropertyKey.
func properties(m publish.Marker, title, digest string) []map[string]any {
	prop := property{Name: m.Name, ArtifactID: m.ArtifactID, Target: m.Target, Task: m.Task, RFC: m.RFC, Title: title, BodyDigest: digest}
	return []map[string]any{{"key": PropertyKey, "value": prop}}
}

// apiError is a non-2xx response.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Jira returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("Jira returned HTTP %d: %s", e.Status, e.Message)
}

func (t *Target) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, t.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch {
	case t.PAT != "":
		req.Header.Set("Authorization", "Bearer "+t.PAT)
	case t.Email != "" || t.APIToken != "":
		req.SetBasicAuth(t.Email, t.APIToken)
	}
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// errorMessage reads Jira's error envelope: errorMessages[] and errors{}.
func errorMessage(raw []byte) string {
	var env struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return strings.TrimSpace(string(raw))
	}
	parts := append([]string{}, env.ErrorMessages...)
	for k, v := range env.Errors {
		parts = append(parts, k+": "+v)
	}
	return strings.Join(parts, "; ")
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
