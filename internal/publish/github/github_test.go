package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish"
)

// fakeGitHub is enough of the REST API for the target: labels, issue create
// and patch, search by body, sub-issues and blocked-by, recording each call.
type fakeGitHub struct {
	mu       sync.Mutex
	t        *testing.T
	labels   []string
	issues   map[int]*fakeIssue
	next     int
	subs     map[int][]int64 // parent number -> child ids
	blocked  map[int][]int64 // issue number -> blocker ids
	calls    []string
	failSubs bool
	token    string
}

type fakeIssue struct {
	ID     int64    `json:"id"`
	Number int      `json:"number"`
	URL    string   `json:"html_url"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	State  string   `json:"state"`
	Labels []string `json:"-"`
}

func (f *fakeIssue) json() map[string]any {
	labels := make([]map[string]string, 0, len(f.Labels))
	for _, l := range f.Labels {
		labels = append(labels, map[string]string{"name": l})
	}
	return map[string]any{"id": f.ID, "number": f.Number, "html_url": f.URL, "title": f.Title, "body": f.Body, "state": f.State, "labels": labels}
}

var (
	issueRE   = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)$`)
	subRE     = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)/sub_issues$`)
	blockedRE = regexp.MustCompile(`^/repos/acme/demo/issues/(\d+)/dependencies/blocked_by$`)
)

func newFake(t *testing.T) (*fakeGitHub, *Target) {
	f := &fakeGitHub{t: t, labels: []string{"epic", "bug"}, issues: map[int]*fakeIssue{}, next: 1, subs: map[int][]int64{}, blocked: map[int][]int64{}, token: "tok"}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	target := New("acme", "demo", "tok")
	target.API = srv.URL
	return f, target
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer "+f.token {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	write := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	path := r.URL.Path
	switch {
	case path == "/repos/acme/demo/labels" && r.Method == http.MethodGet:
		out := []map[string]string{}
		for _, l := range f.labels {
			out = append(out, map[string]string{"name": l})
		}
		write(200, out)
	case path == "/repos/acme/demo/issues" && r.Method == http.MethodPost:
		n := f.next
		f.next++
		is := &fakeIssue{ID: int64(1000 + n), Number: n, URL: fmt.Sprintf("https://github.com/acme/demo/issues/%d", n), Title: body["title"].(string), Body: body["body"].(string), State: "open"}
		if raw, ok := body["labels"].([]any); ok {
			for _, l := range raw {
				is.Labels = append(is.Labels, l.(string))
			}
		}
		f.issues[n] = is
		write(201, is.json())
	case issueRE.MatchString(path) && r.Method == http.MethodPatch:
		n, _ := strconv.Atoi(issueRE.FindStringSubmatch(path)[1])
		is, ok := f.issues[n]
		if !ok {
			write(404, map[string]string{"message": "Not Found"})
			return
		}
		if t, ok := body["title"].(string); ok {
			is.Title = t
		}
		if b, ok := body["body"].(string); ok {
			is.Body = b
		}
		write(200, is.json())
	case subRE.MatchString(path) && r.Method == http.MethodPost:
		if f.failSubs {
			write(http.StatusUnprocessableEntity, map[string]string{"message": "sub-issues disabled"})
			return
		}
		n, _ := strconv.Atoi(subRE.FindStringSubmatch(path)[1])
		f.subs[n] = append(f.subs[n], int64(body["sub_issue_id"].(float64)))
		write(201, map[string]any{})
	case blockedRE.MatchString(path) && r.Method == http.MethodPost:
		n, _ := strconv.Atoi(blockedRE.FindStringSubmatch(path)[1])
		f.blocked[n] = append(f.blocked[n], int64(body["issue_id"].(float64)))
		write(201, map[string]any{})
	case path == "/search/issues" && r.Method == http.MethodGet:
		q := r.URL.Query().Get("q")
		m := regexp.MustCompile(`"artifact_id=([^"]+)"`).FindStringSubmatch(q)
		require.NotNil(f.t, m, "search query carries the artifact_id: %s", q)
		items := []map[string]any{}
		for n := 1; n < f.next; n++ {
			is := f.issues[n]
			if strings.Contains(is.Body, "artifact_id="+m[1]+" ") {
				items = append(items, is.json())
			}
		}
		write(200, map[string]any{"total_count": len(items), "items": items})
	default:
		write(404, map[string]string{"message": "no route " + r.Method + " " + path})
	}
}

func (f *fakeGitHub) count(call string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

func testPublication(t *testing.T) publish.Publication {
	t.Helper()
	body := []byte(`# Feature: Nightly digest

## Overview

Operators want one email per day summarising what the hive filed, merged and
parked, so they can skim the night's work without opening the dashboard.

## Requirements

- [ ] **Schedule**: send one digest per day at the configured hour, in the operator's time zone.
- [ ] **Content**: list filed, merged and parked items with links back to each.

## Acceptance Criteria

- [ ] **Schedule**: a digest arrives within five minutes of the hour.
- [ ] **Content**: every item of the last 24 hours appears exactly once.
`)
	p, err := publish.Build(publish.Source{Kind: publish.KindSpec, Name: "nightly-digest", ArtifactID: "demo/spec/nightly-digest", Target: "github", Body: body})
	require.NoError(t, err)
	// R2 depends on R1 so the blocked-by path is exercised.
	p.Children[1].DependsOn = []string{"R1"}
	require.NoError(t, publish.Validate(p))
	return p
}

func TestTarget_PublishCreatesRFCThenChildrenAndLinks(t *testing.T) {
	f, target := newFake(t)
	p := testPublication(t)

	existing, err := target.Find(context.Background(), p.ArtifactID)
	require.NoError(t, err)
	require.Nil(t, existing)

	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{ParentLabel: "epic", Labels: []string{"spek", "bug"}})
	require.NoError(t, err)

	require.Equal(t, "https://github.com/acme/demo/issues/1", res.RFCURL)
	require.Equal(t, 1, res.RFCNumber)
	require.Equal(t, []string{"rfc", "R1", "R2"}, res.Created)
	require.Equal(t, []publish.ResultChild{{ID: "R1", URL: "https://github.com/acme/demo/issues/2", Number: 2}, {ID: "R2", URL: "https://github.com/acme/demo/issues/3", Number: 3}}, res.Children)
	require.Len(t, res.Warnings, 1, "the one missing label is warned about once")
	require.Contains(t, res.Warnings[0], `"spek" does not exist`)

	rfc := f.issues[1]
	require.Equal(t, "RFC: Nightly digest", rfc.Title)
	require.Equal(t, []string{"epic", "bug"}, rfc.Labels, "existing labels applied; the missing one never created")
	require.Contains(t, rfc.Body, "- [ ] #2 — Schedule")
	require.Contains(t, rfc.Body, "- [ ] #3 — Content")
	require.True(t, strings.HasSuffix(strings.TrimSpace(rfc.Body), p.RFC.Marker))

	child := f.issues[3]
	require.Equal(t, []string{"bug"}, child.Labels, "the parent label goes on the RFC only")
	require.Contains(t, child.Body, "- Blocked by #2 — Schedule")
	require.Contains(t, child.Body, "- Part of RFC: https://github.com/acme/demo/issues/1")
	m, ok := publish.ParseMarker(child.Body)
	require.True(t, ok)
	require.Equal(t, "R2", m.Task)
	require.Equal(t, "https://github.com/acme/demo/issues/1", m.RFC)

	require.Equal(t, []int64{1002, 1003}, f.subs[1], "children are sub-issues of the RFC, by database id")
	require.Equal(t, []int64{1002}, f.blocked[3], "R2 is blocked by R1, by database id")
	require.Equal(t, 1, f.count("GET /repos/acme/demo/labels"))
	require.Equal(t, 3, f.count("POST /repos/acme/demo/issues"))
}

func TestTarget_FindThenRepublishIsIdempotent(t *testing.T) {
	f, target := newFake(t)
	p := testPublication(t)
	ctx := context.Background()
	_, err := target.Publish(ctx, p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	// A stray issue for another artifact must not be picked up.
	f.issues[99] = &fakeIssue{ID: 9999, Number: 99, URL: "u", Body: "<!-- spek:publish name=other artifact_id=demo/spec/other target=github -->"}

	existing, err := target.Find(ctx, p.ArtifactID)
	require.NoError(t, err)
	require.NotNil(t, existing)
	require.NotNil(t, existing.RFC)
	require.Equal(t, 1, existing.RFC.Number)
	require.Equal(t, []string{"R1", "R2"}, publish.SortedIDs(existing.Children))
	require.Empty(t, publish.Diff(p, existing), "what was filed matches what would be filed")

	created := f.count("POST /repos/acme/demo/issues")
	res, err := target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Empty(t, res.Created)
	require.Empty(t, res.Updated)
	require.Equal(t, []string{"R1", "R2", "rfc"}, res.Unchanged)
	require.Equal(t, created, f.count("POST /repos/acme/demo/issues"), "nothing is created twice")

	// A changed child title is patched in place; a new child is created and
	// linked; the RFC checklist grows.
	p.Children[0].Title = "Schedule the digest"
	p.Children = append(p.Children, publish.Child{})
	p.Children[2] = p.Children[1]
	p.Children[2].ID = "R3"
	p.Children[2].Title = "Opt out"
	p.Children[2].DependsOn = []string{"R2"}
	p.Children[2].Marker = strings.Replace(p.Children[1].Marker, "task=R2", "task=R3", 1)
	require.Equal(t, "rfc (body), R1 (title), R2 (body), R3 (create)", publish.DescribeChanges(publish.Diff(p, existing)))

	res, err = target.Publish(ctx, p, existing, publish.PublishOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"R3"}, res.Created)
	require.ElementsMatch(t, []string{"R1", "R2", "rfc"}, res.Updated)
	require.Empty(t, res.Unchanged)
	require.Equal(t, "Schedule the digest", f.issues[2].Title)
	require.Contains(t, f.issues[1].Body, "- [ ] #4 — Opt out")
	require.Equal(t, []int64{1002, 1003, 1004}, f.subs[1])
	require.Equal(t, []int64{1003}, f.blocked[4])
}

func TestTarget_RelationFailuresAreWarnings(t *testing.T) {
	f, target := newFake(t)
	f.failSubs = true
	p := testPublication(t)
	res, err := target.Publish(context.Background(), p, nil, publish.PublishOptions{})
	require.NoError(t, err)
	require.Len(t, res.Children, 2)
	require.Len(t, res.Warnings, 2)
	require.Contains(t, res.Warnings[0], "could not add #2 as a sub-issue of #1")
	require.Contains(t, res.Warnings[0], "HTTP 422: sub-issues disabled")
}

func TestTarget_BadCredentialsFail(t *testing.T) {
	_, target := newFake(t)
	target.Token = "wrong"
	_, err := target.Find(context.Background(), "demo/spec/x")
	require.Error(t, err)
	require.Contains(t, err.Error(), "HTTP 401: Bad credentials")
}
