// Package jiratest is a fake Jira REST site for tests of the publish jira
// target and of the publish command.
package jiratest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish/jira"
)

// Server is enough of the REST API for the target, for Cloud (v3: ADF,
// POST search/jql) and Data Center (v2: wiki text, GET search): issue
// create and update, issue properties, issue links and JQL search on the
// property or the description text, recording each call.
type Server struct {
	mu         sync.Mutex
	t          *testing.T
	URL        string
	dataCenter bool
	// propertyJQL makes the issue.property[...] JQL work; when false the
	// site answers it with 400 as a site without indexed properties does.
	PropertyJQL bool
	Issues      map[string]*Issue
	next        int
	Links       []string // "BLOCKER blocks BLOCKED"
	calls       []string
	FailLinks   bool
	auth        string
}

// Issue is one issue the fake holds.
type Issue struct {
	ID          string
	Key         string
	Type        string
	Summary     string
	Description any
	Labels      []string
	Parent      string
	Fields      map[string]any
	Property    map[string]any
	Done        bool
}

func (f *Issue) searchJSON() map[string]any {
	category := "new"
	if f.Done {
		category = "done"
	}
	return map[string]any{"id": f.ID, "key": f.Key, "fields": map[string]any{
		"summary": f.Summary, "description": f.Description,
		"status": map[string]any{"statusCategory": map[string]any{"key": category}},
	}}
}

var (
	issuePathRE    = regexp.MustCompile(`^/rest/api/[23]/issue/([A-Z]+-\d+)$`)
	propertyPathRE = regexp.MustCompile(`^/rest/api/[23]/issue/([A-Z]+-\d+)/properties/spek\.publish$`)
	propertyJQLRE  = regexp.MustCompile(`issue\.property\[spek\.publish\]\.artifact_id = "([^"]+)"`)
	textJQLRE      = regexp.MustCompile(`description ~ "artifact_id=([^"]+)"`)
)

// Credentials the fake accepts: Basic Email/APIToken on Cloud, Bearer PAT
// on Data Center.
const (
	Email    = "me@acme.test"
	APIToken = "tok"
	PAT      = "pat"
	Project  = "DEMO"
)

// New starts a fake site for project DEMO. A Cloud fake can search the
// issue property; a Data Center fake refuses that JQL with 400, as a site
// without indexed properties does, so the text fallback is exercised.
func New(t *testing.T, dataCenter bool) *Server {
	f := &Server{t: t, dataCenter: dataCenter, PropertyJQL: !dataCenter, Issues: map[string]*Issue{}, next: 1}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	if dataCenter {
		f.auth = "Bearer " + PAT
	} else {
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth(Email, APIToken)
		f.auth = req.Header.Get("Authorization")
	}
	return f
}

// Target returns a target pointed at the fake with the matching credential.
func (f *Server) Target() *jira.Target {
	target := jira.New(f.URL, Project)
	if f.dataCenter {
		target.DataCenter = true
		target.PAT = PAT
	} else {
		target.Email, target.APIToken = Email, APIToken
	}
	return target
}

func (f *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	wantAPI := "/rest/api/3/"
	if f.dataCenter {
		wantAPI = "/rest/api/2/"
	}
	require.True(f.t, strings.HasPrefix(r.URL.Path, wantAPI), "wrong API version: %s", r.URL.Path)
	if r.Header.Get("Authorization") != f.auth {
		http.Error(w, `{"errorMessages":["Unauthorized"]}`, http.StatusUnauthorized)
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	write := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/issue") && r.Method == http.MethodPost:
		fields := body["fields"].(map[string]any)
		n := f.next
		f.next++
		is := &Issue{ID: fmt.Sprintf("%d", 10000+n), Key: fmt.Sprintf("DEMO-%d", n), Fields: fields}
		is.Summary, _ = fields["summary"].(string)
		is.Type = fields["issuetype"].(map[string]any)["name"].(string)
		is.Description = fields["description"]
		f.checkDescription(is.Description)
		if raw, ok := fields["labels"].([]any); ok {
			for _, l := range raw {
				is.Labels = append(is.Labels, l.(string))
			}
		}
		if p, ok := fields["parent"].(map[string]any); ok {
			is.Parent = p["key"].(string)
		}
		f.Issues[is.Key] = is
		write(201, map[string]any{"id": is.ID, "key": is.Key})
	case issuePathRE.MatchString(path) && r.Method == http.MethodPut:
		is, ok := f.Issues[issuePathRE.FindStringSubmatch(path)[1]]
		if !ok {
			write(404, map[string]any{"errorMessages": []string{"Issue does not exist"}})
			return
		}
		fields := body["fields"].(map[string]any)
		if s, ok := fields["summary"].(string); ok {
			is.Summary = s
		}
		if d, ok := fields["description"]; ok {
			f.checkDescription(d)
			is.Description = d
		}
		for k, v := range fields {
			is.Fields[k] = v
		}
		write(204, nil)
	case propertyPathRE.MatchString(path) && r.Method == http.MethodPut:
		is, ok := f.Issues[propertyPathRE.FindStringSubmatch(path)[1]]
		if !ok {
			write(404, nil)
			return
		}
		is.Property = body
		write(200, nil)
	case propertyPathRE.MatchString(path) && r.Method == http.MethodGet:
		is, ok := f.Issues[propertyPathRE.FindStringSubmatch(path)[1]]
		if !ok || is.Property == nil {
			write(404, map[string]any{"errorMessages": []string{"property not found"}})
			return
		}
		write(200, map[string]any{"key": jira.PropertyKey, "value": is.Property})
	case strings.HasSuffix(path, "/issueLink") && r.Method == http.MethodPost:
		if f.FailLinks {
			write(404, map[string]any{"errorMessages": []string{"No issue link type with name 'Blocks' found."}})
			return
		}
		require.Equal(f.t, jira.BlocksLinkType, body["type"].(map[string]any)["name"])
		blocker := body["outwardIssue"].(map[string]any)["key"].(string)
		blocked := body["inwardIssue"].(map[string]any)["key"].(string)
		f.Links = append(f.Links, blocker+" blocks "+blocked)
		write(201, nil)
	case strings.HasSuffix(path, "/search/jql") && r.Method == http.MethodPost && !f.dataCenter:
		f.search(write, body["jql"].(string))
	case strings.HasSuffix(path, "/search") && r.Method == http.MethodGet && f.dataCenter:
		f.search(write, r.URL.Query().Get("jql"))
	default:
		write(404, map[string]any{"errorMessages": []string{"no route " + r.Method + " " + path}})
	}
}

// checkDescription asserts the description is in the site's format.
func (f *Server) checkDescription(d any) {
	if f.dataCenter {
		_, ok := d.(string)
		require.True(f.t, ok, "Data Center descriptions are wiki text, got %T", d)
		return
	}
	doc, ok := d.(map[string]any)
	require.True(f.t, ok, "Cloud descriptions are ADF, got %T", d)
	require.Equal(f.t, "doc", doc["type"])
}

func (f *Server) search(write func(int, any), jql string) {
	require.True(f.t, strings.HasPrefix(jql, "project = DEMO AND "), jql)
	var match func(is *Issue) bool
	if m := propertyJQLRE.FindStringSubmatch(jql); m != nil {
		if !f.PropertyJQL {
			write(400, map[string]any{"errorMessages": []string{"Field 'issue.property[spek.publish].artifact_id' does not exist or you do not have permission to view it."}})
			return
		}
		match = func(is *Issue) bool { return is.Property != nil && is.Property["artifact_id"] == m[1] }
	} else if m := textJQLRE.FindStringSubmatch(jql); m != nil {
		match = func(is *Issue) bool {
			text, _ := is.Description.(string)
			if !f.dataCenter {
				text = jira.ADFText(is.Description)
			}
			return strings.Contains(text, "artifact_id="+m[1]+" ")
		}
	} else {
		f.t.Errorf("unexpected JQL: %s", jql)
		write(400, nil)
		return
	}
	issues := []map[string]any{}
	for n := 1; n < f.next; n++ {
		if is, ok := f.Issues[fmt.Sprintf("DEMO-%d", n)]; ok && match(is) {
			issues = append(issues, is.searchJSON())
		}
	}
	write(200, map[string]any{"issues": issues, "total": len(issues), "isLast": true})
}

// Add places an issue the target did not create, numbered from n onwards.
func (f *Server) Add(is *Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Issues[is.Key] = is
	var n int
	if _, err := fmt.Sscanf(is.Key, Project+"-%d", &n); err == nil && n >= f.next {
		f.next = n + 1
	}
}

// Count is how many times method+path was called.
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

// DescriptionText is the issue's description flattened to text.
func (f *Server) DescriptionText(key string) string {
	is := f.Issues[key]
	if f.dataCenter {
		return is.Description.(string)
	}
	return jira.ADFText(is.Description)
}
