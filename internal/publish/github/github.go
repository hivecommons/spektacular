// Package github files a publication into a GitHub repository: the RFC as an
// issue, each child as a sub-issue of it, and each depends_on edge as a
// "blocked by" relation, the way hive's hive-open-issue does.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/publish"
)

// DefaultAPI is GitHub's REST endpoint; tests point the client elsewhere.
const DefaultAPI = "https://api.github.com"

const requestTimeout = 30 * time.Second

// Target is the GitHub implementation of publish.Target for one repo.
type Target struct {
	Owner string
	Repo  string
	// API is the REST base URL, DefaultAPI unless set.
	API   string
	Token string
	// HTTPClient defaults to one with requestTimeout.
	HTTPClient *http.Client
	// UserAgent identifies the client; GitHub requires one.
	UserAgent string
}

// New returns a target for owner/repo authenticated with token.
func New(owner, repo, token string) *Target {
	return &Target{Owner: owner, Repo: repo, Token: token, API: DefaultAPI, UserAgent: "spektacular-publish"}
}

// Name implements publish.Target.
func (t *Target) Name() string { return "github" }

// Slug is owner/repo.
func (t *Target) Slug() string { return t.Owner + "/" + t.Repo }

type issue struct {
	ID          int64     `json:"id"`
	Number      int       `json:"number"`
	HTMLURL     string    `json:"html_url"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	State       string    `json:"state"`
	PullRequest *struct{} `json:"pull_request,omitempty"`
	Labels      []label   `json:"labels"`
}

type label struct {
	Name string `json:"name"`
}

// Find implements publish.Target by searching issue bodies for the marker.
func (t *Target) Find(ctx context.Context, artifactID string) (*publish.Existing, error) {
	q := fmt.Sprintf(`repo:%s is:issue in:body "spek:publish" "artifact_id=%s"`, t.Slug(), artifactID)
	var page struct {
		Items []issue `json:"items"`
	}
	if err := t.do(ctx, http.MethodGet, "/search/issues?per_page=100&q="+url.QueryEscape(q), nil, &page); err != nil {
		return nil, fmt.Errorf("searching %s for published items: %w", t.Slug(), err)
	}
	existing := &publish.Existing{Children: map[string]publish.Filed{}}
	for _, it := range page.Items {
		if it.PullRequest != nil {
			continue
		}
		m, ok := publish.ParseMarker(it.Body)
		if !ok || m.ArtifactID != artifactID {
			continue
		}
		f := filed(it, m)
		if m.IsRFC() {
			// Prefer an open RFC when several match.
			if existing.RFC == nil || (existing.RFC.Closed && !f.Closed) {
				existing.RFC = &f
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

func filed(it issue, m publish.Marker) publish.Filed {
	return publish.Filed{
		ID:     it.ID,
		Number: it.Number,
		URL:    it.HTMLURL,
		Ref:    fmt.Sprintf("#%d", it.Number),
		Title:  it.Title,
		Body:   it.Body,
		Closed: it.State == "closed",
		Marker: m,
	}
}

// Publish implements publish.Target. Creation order: the RFC first (its URL
// goes into every child's marker), then the children, then the relations,
// then the RFC body again with the children's numbers in its checklist.
// Relations are best effort: a failure becomes a warning, never an error,
// because the issues exist and a re-run with --update repairs them.
func (t *Target) Publish(ctx context.Context, p publish.Publication, existing *publish.Existing, opts publish.PublishOptions) (publish.Result, error) {
	res := publish.Result{Target: t.Name(), Repo: t.Slug(), Created: []string{}, Updated: []string{}, Unchanged: []string{}, Warnings: []string{}, Children: []publish.ResultChild{}}
	if existing == nil {
		existing = &publish.Existing{Children: map[string]publish.Filed{}}
	}
	labels, err := t.labels(ctx)
	if err != nil {
		return res, err
	}
	rfcLabels := t.pickLabels(labels, &res, opts.ParentLabel, opts.Labels...)
	childLabels := t.pickLabels(labels, &res, "", opts.Labels...)

	refs := map[string]publish.Ref{}
	for id, f := range existing.Children {
		refs[id] = publish.Ref{Ref: f.Ref, URL: f.URL}
	}

	// The RFC, with whatever children refs are already known.
	var rfc publish.Filed
	if existing.RFC == nil {
		created, err := t.createIssue(ctx, p.RFC.Title, p.RFCBody(refs), rfcLabels)
		if err != nil {
			return res, fmt.Errorf("creating the RFC issue: %w", err)
		}
		rfc = created
		res.Created = append(res.Created, "rfc")
	} else {
		rfc = *existing.RFC
	}
	res.RFCURL = rfc.URL
	res.RFCNumber = rfc.Number

	// The children.
	filedChildren := map[string]publish.Filed{}
	for _, c := range p.Children {
		body := c.Render(p, rfc.URL, refs)
		if f, ok := existing.Children[c.ID]; ok {
			if f.Title != c.Title || !sameBody(f.Body, body) {
				if err := t.patchIssue(ctx, f.Number, c.Title, body, nil); err != nil {
					return res, fmt.Errorf("updating child %s (#%d): %w", c.ID, f.Number, err)
				}
				res.Updated = append(res.Updated, c.ID)
			} else {
				res.Unchanged = append(res.Unchanged, c.ID)
			}
			filedChildren[c.ID] = f
		} else {
			created, err := t.createIssue(ctx, c.Title, body, childLabels)
			if err != nil {
				return res, fmt.Errorf("creating child %s: %w", c.ID, err)
			}
			filedChildren[c.ID] = created
			refs[c.ID] = publish.Ref{Ref: created.Ref, URL: created.URL}
			res.Created = append(res.Created, c.ID)
			t.link(ctx, rfc, created, &res)
		}
		f := filedChildren[c.ID]
		res.Children = append(res.Children, publish.ResultChild{ID: c.ID, URL: f.URL, Number: f.Number})
	}

	// Second pass: bodies that referred to children by ID before those were
	// created now get their numbers, and the dependency relations are filed.
	for _, c := range p.Children {
		f := filedChildren[c.ID]
		body := c.Render(p, rfc.URL, refs)
		wasCreated := contains(res.Created, c.ID)
		if !sameBody(f.Body, body) {
			if err := t.patchIssue(ctx, f.Number, c.Title, body, nil); err != nil {
				return res, fmt.Errorf("updating child %s (#%d): %w", c.ID, f.Number, err)
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

	// The RFC body with every child referenced.
	rfcBody := p.RFCBody(refs)
	if existing.RFC == nil {
		if err := t.patchIssue(ctx, rfc.Number, p.RFC.Title, rfcBody, nil); err != nil {
			return res, fmt.Errorf("updating the RFC checklist (#%d): %w", rfc.Number, err)
		}
	} else if rfc.Title != p.RFC.Title || !sameBody(rfc.Body, rfcBody) {
		if err := t.patchIssue(ctx, rfc.Number, p.RFC.Title, rfcBody, nil); err != nil {
			return res, fmt.Errorf("updating the RFC (#%d): %w", rfc.Number, err)
		}
		res.Updated = append(res.Updated, "rfc")
	} else {
		res.Unchanged = append(res.Unchanged, "rfc")
	}
	return res, nil
}

// link makes child a sub-issue of rfc, best effort.
func (t *Target) link(ctx context.Context, rfc, child publish.Filed, res *publish.Result) {
	path := fmt.Sprintf("/repos/%s/issues/%d/sub_issues", t.Slug(), rfc.Number)
	if err := t.do(ctx, http.MethodPost, path, map[string]any{"sub_issue_id": child.ID}, nil); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not add #%d as a sub-issue of #%d: %v", child.Number, rfc.Number, err))
	}
}

// blockedBy records that issue is blocked by blocker, best effort.
func (t *Target) blockedBy(ctx context.Context, issue, blocker publish.Filed, res *publish.Result) {
	path := fmt.Sprintf("/repos/%s/issues/%d/dependencies/blocked_by", t.Slug(), issue.Number)
	if err := t.do(ctx, http.MethodPost, path, map[string]any{"issue_id": blocker.ID}, nil); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not mark #%d as blocked by #%d: %v", issue.Number, blocker.Number, err))
	}
}

// labels lists the repo's label names (first 100).
func (t *Target) labels(ctx context.Context) (map[string]bool, error) {
	var out []label
	if err := t.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/labels?per_page=100", t.Slug()), nil, &out); err != nil {
		return nil, fmt.Errorf("listing labels of %s: %w", t.Slug(), err)
	}
	names := map[string]bool{}
	for _, l := range out {
		names[strings.ToLower(l.Name)] = true
	}
	return names, nil
}

// pickLabels keeps the wanted labels that exist in the repo, warning once
// per missing one. Labels are never created.
func (t *Target) pickLabels(have map[string]bool, res *publish.Result, parent string, wanted ...string) []string {
	var out []string
	add := func(name, role string) {
		if name == "" {
			return
		}
		if have[strings.ToLower(name)] {
			out = append(out, name)
			return
		}
		w := fmt.Sprintf("%s label %q does not exist in %s; not applied (publish never creates labels)", role, name, t.Slug())
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

func (t *Target) createIssue(ctx context.Context, title, body string, labels []string) (publish.Filed, error) {
	payload := map[string]any{"title": title, "body": body}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	var it issue
	if err := t.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues", t.Slug()), payload, &it); err != nil {
		return publish.Filed{}, err
	}
	m, _ := publish.ParseMarker(body)
	return filed(it, m), nil
}

func (t *Target) patchIssue(ctx context.Context, number int, title, body string, labels []string) error {
	payload := map[string]any{"title": title, "body": body}
	if labels != nil {
		payload["labels"] = labels
	}
	return t.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/%d", t.Slug(), number), payload, nil)
}

// apiError is a non-2xx response.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("GitHub returned HTTP %d", e.Status)
	}
	return fmt.Sprintf("GitHub returned HTTP %d: %s", e.Status, e.Message)
}

func (t *Target) do(ctx context.Context, method, path string, in, out any) error {
	base := t.API
	if base == "" {
		base = DefaultAPI
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}
	if t.UserAgent != "" {
		req.Header.Set("User-Agent", t.UserAgent)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
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
		var msg struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &msg)
		return &apiError{Status: resp.StatusCode, Message: msg.Message}
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func sameBody(a, b string) bool { return publish.SameBody(a, b) }

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
