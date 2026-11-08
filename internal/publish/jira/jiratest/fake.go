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

	"github.com/stretchr/testify/assert"

	"github.com/hivecommons/spektacular/internal/publish/jira"
)

// Server is enough of the REST API for the target, for Cloud (v3: ADF,
// POST search/jql) and Data Center (v2: wiki text, GET search): issue
// create and update with entity properties, issue read, issue links, the
// field list and JQL text search on the description, recording each call.
// Like a real site it refuses the issue.property[...] JQL (no app has
// indexed the property) and, on Data Center, parent on anything but a
// sub-task.
//
// Assertions inside handlers use assert, not require: a handler runs on
// the server's goroutine, where FailNow cannot stop the test.
type Server struct {
	mu         sync.Mutex
	t          *testing.T
	URL        string
	dataCenter bool
	Issues     map[string]*Issue
	next       int
	Links      []string // "BLOCKER blocks BLOCKED"
	calls      []string
	FailLinks  bool
	// FailCreateAfter makes the n-th POST issue (counting from 1) answer
	// 500 after n-1 succeeded, for a publish that fails part way; 0 is off.
	FailCreateAfter int
	// SearchLag models the eventually consistent search index: when set,
	// a search sees only the issues that existed at the last Reindex.
	SearchLag bool
	indexed   int
	auth      string
}

// EpicLinkField is the Epic Link custom field the Data Center fake has,
// the one GET field reports and children must use instead of parent.
const EpicLinkField = "customfield_10014"

// Issue is one issue the fake holds.
type Issue struct {
	ID          string
	Key         string
	Type        string
	Summary     string
	Description any
	Labels      []string
	// Parent is the Epic's key however it was set: parent on Cloud, the
	// Epic Link field on Data Center.
	Parent   string
	Fields   map[string]any
	Property map[string]any
	Done     bool
}

func (f *Server) issueJSON(is *Issue) map[string]any {
	category := "new"
	if is.Done {
		category = "done"
	}
	fields := map[string]any{
		"summary": is.Summary, "description": is.Description,
		"status": map[string]any{"statusCategory": map[string]any{"key": category}},
	}
	if is.Parent != "" {
		if f.dataCenter {
			fields[EpicLinkField] = is.Parent
		} else {
			fields["parent"] = map[string]any{"key": is.Parent}
		}
	}
	links := []any{}
	for _, l := range f.Links {
		blocker, blocked, _ := strings.Cut(l, " blocks ")
		switch is.Key {
		case blocked:
			links = append(links, map[string]any{"type": map[string]any{"name": jira.BlocksLinkType}, "outwardIssue": map[string]any{"key": blocker}})
		case blocker:
			links = append(links, map[string]any{"type": map[string]any{"name": jira.BlocksLinkType}, "inwardIssue": map[string]any{"key": blocked}})
		}
	}
	fields["issuelinks"] = links
	return map[string]any{"id": is.ID, "key": is.Key, "fields": fields}
}

var (
	issuePathRE    = regexp.MustCompile(`^/rest/api/[23]/issue/([A-Z]+-\d+)$`)
	propertyPathRE = regexp.MustCompile(`^/rest/api/[23]/issue/([A-Z]+-\d+)/properties/spek\.publish$`)
	tokenJQLRE     = regexp.MustCompile(`^project = DEMO AND description ~ "(spekpub[0-9a-f]+)"$`)
)

// Credentials the fake accepts: Basic Email/APIToken on Cloud, Bearer PAT
// on Data Center.
const (
	Email    = "me@acme.test"
	APIToken = "tok"
	PAT      = "pat"
	Project  = "DEMO"
)

// New starts a fake site for project DEMO.
func New(t *testing.T, dataCenter bool) *Server {
	f := &Server{t: t, dataCenter: dataCenter, Issues: map[string]*Issue{}, next: 1}
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
	write := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if v != nil {
			_ = json.NewEncoder(w).Encode(v)
		}
	}
	if !assert.True(f.t, strings.HasPrefix(r.URL.Path, wantAPI), "wrong API version: %s", r.URL.Path) {
		write(404, nil)
		return
	}
	if r.Header.Get("Authorization") != f.auth {
		http.Error(w, `{"errorMessages":["Unauthorized"]}`, http.StatusUnauthorized)
		return
	}
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	fields, _ := body["fields"].(map[string]any)
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/field") && r.Method == http.MethodGet:
		out := []map[string]any{
			{"id": "summary", "name": "Summary", "schema": map[string]any{"type": "string", "system": "summary"}},
			{"id": "customfield_10100", "name": "Acceptance Criteria", "schema": map[string]any{"type": "string", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:textarea"}},
		}
		if f.dataCenter {
			out = append(out, map[string]any{"id": EpicLinkField, "name": "Epic Link", "schema": map[string]any{"type": "any", "custom": jira.EpicLinkFieldType}})
		}
		write(200, out)
	case strings.HasSuffix(path, "/issue") && r.Method == http.MethodPost:
		if f.FailCreateAfter > 0 && f.Count("POST "+path) >= f.FailCreateAfter {
			write(500, map[string]any{"errorMessages": []string{"Internal server error"}})
			return
		}
		if fields == nil {
			write(400, map[string]any{"errorMessages": []string{"no fields"}})
			return
		}
		n := f.next
		f.next++
		is := &Issue{ID: fmt.Sprintf("%d", 10000+n), Key: fmt.Sprintf("DEMO-%d", n), Fields: fields}
		is.Summary, _ = fields["summary"].(string)
		if it, ok := fields["issuetype"].(map[string]any); ok {
			is.Type, _ = it["name"].(string)
		}
		is.Description = fields["description"]
		f.checkDescription(is.Description)
		if raw, ok := fields["labels"].([]any); ok {
			for _, l := range raw {
				is.Labels = append(is.Labels, l.(string))
			}
		}
		if msg := f.setParent(is, fields); msg != "" {
			f.next--
			write(400, map[string]any{"errors": map[string]string{"parent": msg}})
			return
		}
		f.setProperties(is, body)
		f.Issues[is.Key] = is
		write(201, map[string]any{"id": is.ID, "key": is.Key})
	case issuePathRE.MatchString(path) && r.Method == http.MethodGet:
		is, ok := f.Issues[issuePathRE.FindStringSubmatch(path)[1]]
		if !ok {
			write(404, map[string]any{"errorMessages": []string{"Issue does not exist"}})
			return
		}
		write(200, f.issueJSON(is))
	case issuePathRE.MatchString(path) && r.Method == http.MethodPut:
		is, ok := f.Issues[issuePathRE.FindStringSubmatch(path)[1]]
		if !ok {
			write(404, map[string]any{"errorMessages": []string{"Issue does not exist"}})
			return
		}
		if s, ok := fields["summary"].(string); ok {
			is.Summary = s
		}
		if d, ok := fields["description"]; ok {
			f.checkDescription(d)
			is.Description = d
		}
		if msg := f.setParent(is, fields); msg != "" {
			write(400, map[string]any{"errors": map[string]string{"parent": msg}})
			return
		}
		for k, v := range fields {
			is.Fields[k] = v
		}
		f.setProperties(is, body)
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
		linkType, _ := body["type"].(map[string]any)
		assert.Equal(f.t, jira.BlocksLinkType, linkType["name"])
		outward, _ := body["outwardIssue"].(map[string]any)
		inward, _ := body["inwardIssue"].(map[string]any)
		blocker, _ := outward["key"].(string)
		blocked, _ := inward["key"].(string)
		if _, ok := f.Issues[blocker]; !ok || f.Issues[blocked] == nil {
			write(404, map[string]any{"errorMessages": []string{"Issue does not exist"}})
			return
		}
		f.Links = append(f.Links, blocker+" blocks "+blocked)
		write(201, nil)
	case strings.HasSuffix(path, "/search/jql") && r.Method == http.MethodPost && !f.dataCenter:
		jql, _ := body["jql"].(string)
		f.search(write, jql)
	case strings.HasSuffix(path, "/search") && r.Method == http.MethodGet && f.dataCenter:
		f.search(write, r.URL.Query().Get("jql"))
	default:
		write(404, map[string]any{"errorMessages": []string{"no route " + r.Method + " " + path}})
	}
}

// setParent applies the parent or Epic Link field of a create or edit. On
// Data Center, parent on anything but a sub-task is refused as the real
// site does; the message is what is returned under errors.parent.
func (f *Server) setParent(is *Issue, fields map[string]any) string {
	if p, ok := fields["parent"]; ok {
		if f.dataCenter && !strings.EqualFold(is.Type, "Sub-task") {
			return "Given parent work item does not belong to appropriate hierarchy."
		}
		if pm, ok := p.(map[string]any); ok {
			is.Parent, _ = pm["key"].(string)
		}
	}
	if epic, ok := fields[EpicLinkField].(string); ok && f.dataCenter {
		is.Parent = epic
	}
	return ""
}

// setProperties stores the entity properties a create or edit carried.
func (f *Server) setProperties(is *Issue, body map[string]any) {
	props, _ := body["properties"].([]any)
	for _, p := range props {
		prop, _ := p.(map[string]any)
		if prop["key"] == jira.PropertyKey {
			is.Property, _ = prop["value"].(map[string]any)
		}
	}
}

// checkDescription asserts the description is in the site's format.
func (f *Server) checkDescription(d any) {
	if f.dataCenter {
		_, ok := d.(string)
		assert.True(f.t, ok, "Data Center descriptions are wiki text, got %T", d)
		return
	}
	doc, ok := d.(map[string]any)
	if assert.True(f.t, ok, "Cloud descriptions are ADF, got %T", d) {
		assert.Equal(f.t, "doc", doc["type"])
	}
}

func (f *Server) search(write func(int, any), jql string) {
	if strings.Contains(jql, "issue.property[") {
		write(400, map[string]any{"errorMessages": []string{"Field 'issue.property[spek.publish].artifact_id' does not exist or you do not have permission to view it."}})
		return
	}
	m := tokenJQLRE.FindStringSubmatch(jql)
	if m == nil {
		f.t.Errorf("unexpected JQL: %s", jql)
		write(400, map[string]any{"errorMessages": []string{"Error in the JQL Query: " + jql}})
		return
	}
	visible := f.next
	if f.SearchLag {
		visible = f.indexed
	}
	issues := []map[string]any{}
	for n := 1; n < visible; n++ {
		is, ok := f.Issues[fmt.Sprintf("DEMO-%d", n)]
		if !ok {
			continue
		}
		text, _ := is.Description.(string)
		if !f.dataCenter {
			text = jira.ADFText(is.Description)
		}
		if strings.Contains(text, m[1]) {
			issues = append(issues, f.issueJSON(is))
		}
	}
	write(200, map[string]any{"issues": issues, "total": len(issues), "isLast": true})
}

// Reindex makes every issue filed so far visible to a search under
// SearchLag.
func (f *Server) Reindex() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.indexed = f.next
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

// Count is how many times method+path was called. It takes no lock, so a
// handler can call it; callers outside a handler are serialised by the
// client's request completing first.
func (f *Server) Count(call string) int {
	n := 0
	for _, c := range f.calls {
		if c == call {
			n++
		}
	}
	return n
}

// Calls is how many requests the fake has served.
func (f *Server) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// Snapshot copies the issues under the lock, for readers outside a
// handler.
// LinkList copies the "BLOCKER blocks BLOCKED" links under the lock.
func (f *Server) LinkList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.Links...)
}

// Update changes an issue under the lock, for tests that alter the
// site's state between runs.
func (f *Server) Update(key string, fn func(*Issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.Issues[key])
}

func (f *Server) Snapshot() map[string]Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]Issue, len(f.Issues))
	for k, is := range f.Issues {
		out[k] = *is
	}
	return out
}

// DescriptionText is the issue's description flattened to text.
func (f *Server) DescriptionText(key string) string {
	is := f.Issues[key]
	if f.dataCenter {
		return is.Description.(string)
	}
	return jira.ADFText(is.Description)
}
