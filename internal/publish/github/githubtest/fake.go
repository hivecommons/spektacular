// Package githubtest is a fake GitHub REST API for tests of the publish
// github target and of the publish command.
package githubtest

import (
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

	"github.com/hivecommons/spektacular/internal/publish/github"
)

// Server is enough of the REST API for the target: labels, issue create
// and patch, search by body, sub-issues and blocked-by, recording each call.
type Server struct {
	mu  sync.Mutex
	t   *testing.T
	URL string
	// Labels the repo has; publish applies only these.
	Labels   []string
	Issues   map[int]*Issue
	next     int
	Subs     map[int][]int64 // parent number -> child ids
	Blocked  map[int][]int64 // issue number -> blocker ids
	calls    []string
	FailSubs bool
}

// Token is the credential the fake accepts, as "Bearer Token".
const Token = "tok"

// Owner and Repo are the one repository the fake serves.
const (
	Owner = "acme"
	Repo  = "demo"
)

type Issue struct {
	ID     int64    `json:"id"`
	Number int      `json:"number"`
	URL    string   `json:"html_url"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	State  string   `json:"state"`
	Labels []string `json:"-"`
}

func (f *Issue) json() map[string]any {
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

// New starts the fake with the labels epic and bug and stops it with the
// test.
func New(t *testing.T) *Server {
	t.Helper()
	f := &Server{t: t, Labels: []string{"epic", "bug"}, Issues: map[int]*Issue{}, next: 1, Subs: map[int][]int64{}, Blocked: map[int][]int64{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Target is a target pointed at the fake with its credential.
func (f *Server) Target() *github.Target {
	target := github.New(Owner, Repo, Token)
	target.API = f.URL
	return target
}

func (f *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer "+Token {
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
		for _, l := range f.Labels {
			out = append(out, map[string]string{"name": l})
		}
		write(200, out)
	case path == "/repos/acme/demo/issues" && r.Method == http.MethodPost:
		n := f.next
		f.next++
		is := &Issue{ID: int64(1000 + n), Number: n, URL: fmt.Sprintf("https://github.com/acme/demo/issues/%d", n), Title: body["title"].(string), Body: body["body"].(string), State: "open"}
		if raw, ok := body["labels"].([]any); ok {
			for _, l := range raw {
				is.Labels = append(is.Labels, l.(string))
			}
		}
		f.Issues[n] = is
		write(201, is.json())
	case issueRE.MatchString(path) && r.Method == http.MethodPatch:
		n, _ := strconv.Atoi(issueRE.FindStringSubmatch(path)[1])
		is, ok := f.Issues[n]
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
		if f.FailSubs {
			write(http.StatusUnprocessableEntity, map[string]string{"message": "sub-issues disabled"})
			return
		}
		n, _ := strconv.Atoi(subRE.FindStringSubmatch(path)[1])
		f.Subs[n] = append(f.Subs[n], int64(body["sub_issue_id"].(float64)))
		write(201, map[string]any{})
	case blockedRE.MatchString(path) && r.Method == http.MethodPost:
		n, _ := strconv.Atoi(blockedRE.FindStringSubmatch(path)[1])
		f.Blocked[n] = append(f.Blocked[n], int64(body["issue_id"].(float64)))
		write(201, map[string]any{})
	case path == "/search/issues" && r.Method == http.MethodGet:
		q := r.URL.Query().Get("q")
		m := regexp.MustCompile(`"artifact_id=([^"]+)"`).FindStringSubmatch(q)
		require.NotNil(f.t, m, "search query carries the artifact_id: %s", q)
		items := []map[string]any{}
		for n := 1; n < f.next; n++ {
			is := f.Issues[n]
			if strings.Contains(is.Body, "artifact_id="+m[1]+" ") {
				items = append(items, is.json())
			}
		}
		write(200, map[string]any{"total_count": len(items), "items": items})
	default:
		write(404, map[string]string{"message": "no route " + r.Method + " " + path})
	}
}

// Count is how many times "METHOD /path" was called.
func (f *Server) Count(call string) int {
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
