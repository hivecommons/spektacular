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
	// EpicLinkField, when set, receives the Epic's key on each child instead
	// of parent, for Data Center sites without parent support.
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
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Summary     string          `json:"summary"`
		Description json.RawMessage `json:"description"`
		Status      struct {
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
	} `json:"fields"`
}

var searchFields = []string{"summary", "description", "status"}

// Find implements publish.Target. It searches the project by the issue
// property first and falls back to the description text when the site
// cannot search properties, then reads each hit's marker from the property,
// or from the description when the property is missing.
func (t *Target) Find(ctx context.Context, artifactID string) (*publish.Existing, error) {
	byProperty := fmt.Sprintf(`project = %s AND issue.property[%s].artifact_id = %q`, t.ProjectKey, PropertyKey, artifactID)
	byText := fmt.Sprintf(`project = %s AND description ~ %q`, t.ProjectKey, "artifact_id="+artifactID)
	issues, err := t.search(ctx, byProperty)
	if err != nil {
		var apiErr *apiError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
			return nil, fmt.Errorf("searching %s for published items: %w", t.ProjectKey, err)
		}
		if issues, err = t.search(ctx, byText); err != nil {
			return nil, fmt.Errorf("searching %s for published items: %w", t.ProjectKey, err)
		}
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

func (t *Target) search(ctx context.Context, jql string) ([]searchIssue, error) {
	var page struct {
		Issues []searchIssue `json:"issues"`
	}
	if t.DataCenter {
		q := url.Values{}
		q.Set("jql", jql)
		q.Set("fields", strings.Join(searchFields, ","))
		q.Set("maxResults", "100")
		if err := t.do(ctx, http.MethodGet, t.restPath("search")+"?"+q.Encode(), nil, &page); err != nil {
			return nil, err
		}
		return page.Issues, nil
	}
	payload := map[string]any{"jql": jql, "fields": searchFields, "maxResults": 100}
	if err := t.do(ctx, http.MethodPost, t.restPath("search/jql"), payload, &page); err != nil {
		return nil, err
	}
	return page.Issues, nil
}

// filedFrom turns a search hit into a Filed, reading its marker from the
// property or, failing that, the description text.
func (t *Target) filedFrom(ctx context.Context, it searchIssue) (publish.Filed, bool) {
	f := publish.Filed{
		Key:    it.Key,
		URL:    t.browseURL(it.Key),
		Ref:    it.Key,
		Title:  it.Fields.Summary,
		Closed: it.Fields.Status.StatusCategory.Key == "done",
	}
	var prop struct {
		Value property `json:"value"`
	}
	if err := t.do(ctx, http.MethodGet, t.restPath("issue/"+url.PathEscape(it.Key)+"/properties/"+PropertyKey), nil, &prop); err == nil && prop.Value.ArtifactID != "" {
		f.Marker = prop.Value.marker()
		f.BodyDigest = prop.Value.BodyDigest
		return f, true
	}
	text := t.descriptionText(it.Fields.Description)
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

// description renders a Markdown body in the site's document format.
func (t *Target) description(md string) any {
	if t.DataCenter {
		return toWiki(md)
	}
	return toADF(md)
}

// Publish implements publish.Target. Creation order mirrors the GitHub
// target: the Epic first (its URL goes into every child's marker), then the
// children under it, then the Blocks links, then the Epic's description
// again with the children's keys in its checklist. Links are best effort: a
// failure becomes a warning, never an error.
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

	refs := map[string]publish.Ref{}
	for id, f := range existing.Children {
		refs[id] = publish.Ref{Ref: f.Ref, URL: f.URL}
	}

	var rfc publish.Filed
	if existing.RFC == nil {
		created, err := t.createIssue(ctx, t.ParentIssueType, p.RFC.Title, p.RFCBody(refs), labels, "", nil)
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
			created, err := t.createIssue(ctx, t.ChildIssueType, c.Title, body, labels, rfc.Key, &c)
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
	// get their keys, and the Blocks links are filed.
	for _, c := range p.Children {
		f := filedChildren[c.ID]
		body := c.Render(p, rfc.URL, refs)
		wasCreated := contains(res.Created, c.ID)
		if !f.SameAsFiled(body) {
			if err := t.updateIssue(ctx, f.Key, c.Title, body, c); err != nil {
				return res, fmt.Errorf("updating child %s (%s): %w", c.ID, f.Key, err)
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
	}
}

// createIssue files one issue and writes its property. parentKey, when
// set, makes it a child of that Epic; child carries the acceptance
// criteria for the configured field.
func (t *Target) createIssue(ctx context.Context, issueType, title, body string, labels []string, parentKey string, child *publish.Child) (publish.Filed, error) {
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
		if t.EpicLinkField != "" {
			fields[t.EpicLinkField] = parentKey
		} else {
			fields["parent"] = map[string]string{"key": parentKey}
		}
	}
	if child != nil {
		t.addAcceptanceCriteria(fields, *child)
	}
	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := t.do(ctx, http.MethodPost, t.restPath("issue"), map[string]any{"fields": fields}, &created); err != nil {
		return publish.Filed{}, err
	}
	if created.Key == "" {
		return publish.Filed{}, fmt.Errorf("Jira returned no issue key")
	}
	m, _ := publish.ParseMarker(body)
	f := publish.Filed{Key: created.Key, URL: t.browseURL(created.Key), Ref: created.Key, Title: title, BodyDigest: publish.Digest(body), Marker: m}
	if err := t.writeProperty(ctx, f.Key, m, title, f.BodyDigest); err != nil {
		return f, fmt.Errorf("recording the marker on %s: %w", f.Key, err)
	}
	return f, nil
}

func (t *Target) updateIssue(ctx context.Context, key, title, body string, child publish.Child) error {
	fields := map[string]any{"summary": title, "description": t.description(body)}
	if child.ID != "" {
		t.addAcceptanceCriteria(fields, child)
	}
	if err := t.do(ctx, http.MethodPut, t.restPath("issue/"+url.PathEscape(key)), map[string]any{"fields": fields}, nil); err != nil {
		return err
	}
	m, _ := publish.ParseMarker(body)
	return t.writeProperty(ctx, key, m, title, publish.Digest(body))
}

func (t *Target) addAcceptanceCriteria(fields map[string]any, c publish.Child) {
	if t.AcceptanceCriteriaField == "" || len(c.AcceptanceCriteria) == 0 {
		return
	}
	var b strings.Builder
	for _, ac := range c.AcceptanceCriteria {
		b.WriteString("- " + ac + "\n")
	}
	fields[t.AcceptanceCriteriaField] = t.description(b.String())
}

func (t *Target) writeProperty(ctx context.Context, key string, m publish.Marker, title, digest string) error {
	prop := property{Name: m.Name, ArtifactID: m.ArtifactID, Target: m.Target, Task: m.Task, RFC: m.RFC, Title: title, BodyDigest: digest}
	return t.do(ctx, http.MethodPut, t.restPath("issue/"+url.PathEscape(key)+"/properties/"+PropertyKey), prop, nil)
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
