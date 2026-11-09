// Package lineartest is a fake Linear GraphQL API for tests of the publish
// linear target and of the publish command.
package lineartest

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

	"github.com/stretchr/testify/assert"

	"github.com/hivecommons/spektacular/internal/publish/linear"
)

// What the fake knows: one team with three states, two team labels, one
// workspace label and one user.
const (
	APIKey         = "lin_api_test"
	Team           = "ENG"
	TeamID         = "team-uuid-eng"
	Triage         = "Triage"
	Backlog        = "Backlog"
	Todo           = "Todo"
	LabelOne       = "spek"
	LabelTwo       = "epic"
	WorkspaceLabel = "platform"
	UserEmail      = "dev@acme.test"
	UserName       = "Dev One"
	UserID         = "user-uuid-dev"
)

// Server is enough of the GraphQL API for the target: teams (+ workspace
// issueLabels), users, issues by description with paging and archived
// filtering, issueCreate, issueUpdate and issueRelationCreate, recording
// each operation by the name of its first field.
//
// Descriptions are stored normalised (trailing whitespace stripped, one
// trailing newline), as a Markdown store that does not return bytes
// verbatim; the target compares digests, not stored text. Assertions in
// handlers use assert, not require: FailNow cannot stop a test from the
// server's goroutine.
type Server struct {
	mu   sync.Mutex
	t    *testing.T
	URL  string
	next int
	// Issues by UUID, in creation order via Order.
	Issues map[string]*Issue
	Order  []string
	// Relations records "BLOCKER blocks BLOCKED" by identifier.
	Relations     []string
	FailRelations bool
	// NoTodo drops the Todo state, for the first-unstarted fallback.
	NoTodo bool
	calls  []string
}

// Issue is one issue the fake holds.
type Issue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	ParentID    string
	LabelIDs    []string
	StateID     string
	StateType   string
	AssigneeID  string
	// Archived hides the issue from an issues query without
	// includeArchived, as Linear does once a completed issue is archived.
	Archived bool
}

// Normalize is what the fake does to a description it stores.
func Normalize(d string) string {
	lines := strings.Split(d, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// operationRE takes the first field of a GraphQL document as its name.
var operationRE = regexp.MustCompile(`^\s*(?:query|mutation)\b[^{]*\{\s*(\w+)`)

// New starts the fake and stops it with the test.
func New(t *testing.T) *Server {
	t.Helper()
	f := &Server{t: t, Issues: map[string]*Issue{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

// Target is a target pointed at the fake with its credentials.
func (f *Server) Target() *linear.Target {
	t := linear.New(Team, APIKey)
	t.Endpoint = f.URL
	return t
}

// LabelID and StateID are the ids the fake gives its labels and states.
func LabelID(name string) string { return "label-" + name }
func StateID(name string) string { return "state-" + name }

func (f *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	write := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	if r.Method != http.MethodPost {
		write(http.StatusMethodNotAllowed, map[string]any{"errors": []map[string]string{{"message": "POST only"}}})
		return
	}
	if r.Header.Get("Authorization") != APIKey {
		write(http.StatusUnauthorized, map[string]any{"errors": []map[string]string{{"message": "Authentication required, not authenticated"}}})
		return
	}
	var req struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	data := func(v any) { write(http.StatusOK, map[string]any{"data": v}) }
	gqlErr := func(msg string) {
		write(http.StatusOK, map[string]any{"data": nil, "errors": []map[string]string{{"message": msg}}})
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		gqlErr("bad request body: " + err.Error())
		return
	}
	m := operationRE.FindStringSubmatch(req.Query)
	if m == nil {
		gqlErr("unparseable document: " + req.Query)
		return
	}
	op := m[1]
	f.calls = append(f.calls, op)
	input, _ := req.Variables["input"].(map[string]any)
	switch op {
	case "teams":
		key, _ := req.Variables["key"].(string)
		if !strings.EqualFold(key, Team) {
			data(map[string]any{"teams": map[string]any{"nodes": []any{}}, "issueLabels": map[string]any{"nodes": []any{}}})
			return
		}
		states := []any{
			map[string]any{"id": StateID(Triage), "name": Triage, "type": "triage"},
			map[string]any{"id": StateID(Backlog), "name": Backlog, "type": "backlog"},
		}
		if !f.NoTodo {
			states = append(states, map[string]any{"id": StateID(Todo), "name": Todo, "type": "unstarted"})
		}
		states = append(states, map[string]any{"id": StateID("In Progress"), "name": "In Progress", "type": "started"})
		data(map[string]any{
			"teams": map[string]any{"nodes": []any{map[string]any{
				"id": TeamID, "key": Team,
				"states": map[string]any{"nodes": states},
				"labels": map[string]any{"nodes": []any{
					map[string]any{"id": LabelID(LabelOne), "name": LabelOne},
					map[string]any{"id": LabelID(LabelTwo), "name": LabelTwo},
				}},
			}}},
			"issueLabels": map[string]any{"nodes": []any{
				map[string]any{"id": LabelID(WorkspaceLabel), "name": WorkspaceLabel},
			}},
		})
	case "users":
		q, _ := req.Variables["q"].(string)
		nodes := []any{}
		if strings.EqualFold(q, UserEmail) || strings.EqualFold(q, UserName) || strings.EqualFold(q, "dev") {
			nodes = append(nodes, map[string]any{"id": UserID, "name": "dev", "displayName": UserName, "email": UserEmail})
		}
		data(map[string]any{"users": map[string]any{"nodes": nodes}})
	case "issues":
		needle, _ := req.Variables["needle"].(string)
		team, _ := req.Variables["team"].(string)
		includeArchived := strings.Contains(req.Query, "includeArchived: true")
		first := len(f.Order)
		if n, ok := req.Variables["first"].(float64); ok && n > 0 {
			first = int(n)
		}
		from := 0
		if c, ok := req.Variables["after"].(string); ok && c != "" {
			from, _ = strconv.Atoi(c)
		}
		var matching []*Issue
		for _, id := range f.Order {
			is := f.Issues[id]
			if !strings.EqualFold(team, Team) || (is.Archived && !includeArchived) || !strings.Contains(is.Description, needle) {
				continue
			}
			matching = append(matching, is)
		}
		nodes := []any{}
		end := from + first
		if end > len(matching) {
			end = len(matching)
		}
		for i := from; i < end; i++ {
			nodes = append(nodes, f.node(matching[i]))
		}
		data(map[string]any{"issues": map[string]any{
			"nodes":    nodes,
			"pageInfo": map[string]any{"hasNextPage": end < len(matching), "endCursor": strconv.Itoa(end)},
		}})
	case "issueCreate":
		if !assert.Equal(f.t, TeamID, input["teamId"]) {
			gqlErr("unknown team")
			return
		}
		title, _ := input["title"].(string)
		desc, _ := input["description"].(string)
		is := &Issue{Title: title, Description: Normalize(desc), StateType: "backlog"}
		if p, ok := input["parentId"].(string); ok {
			if _, exists := f.Issues[p]; !assert.True(f.t, exists, "parentId must name an existing issue") {
				gqlErr("Entity not found: Issue")
				return
			}
			is.ParentID = p
		}
		if s, ok := input["stateId"].(string); ok {
			is.StateID = s
			if s == StateID(Todo) {
				is.StateType = "unstarted"
			}
		}
		if a, ok := input["assigneeId"].(string); ok {
			is.AssigneeID = a
		}
		if ls, ok := input["labelIds"].([]any); ok {
			for _, l := range ls {
				is.LabelIDs = append(is.LabelIDs, l.(string))
			}
		}
		f.add(is)
		data(map[string]any{"issueCreate": map[string]any{"success": true, "issue": f.node(is)}})
	case "issueUpdate":
		id, _ := req.Variables["id"].(string)
		is, ok := f.Issues[id]
		if !ok {
			gqlErr("Entity not found: Issue")
			return
		}
		if title, ok := input["title"].(string); ok {
			is.Title = title
		}
		if d, ok := input["description"].(string); ok {
			is.Description = Normalize(d)
		}
		if p, ok := input["parentId"].(string); ok {
			if _, exists := f.Issues[p]; !exists {
				gqlErr("Entity not found: Issue")
				return
			}
			is.ParentID = p
		}
		data(map[string]any{"issueUpdate": map[string]any{"success": true}})
	case "issueRelationCreate":
		if f.FailRelations {
			gqlErr("relation refused")
			return
		}
		assert.Equal(f.t, linear.RelationBlocks, input["type"])
		blockerID, _ := input["issueId"].(string)
		blockedID, _ := input["relatedIssueId"].(string)
		blocker, ok1 := f.Issues[blockerID]
		blocked, ok2 := f.Issues[blockedID]
		if !assert.True(f.t, ok1 && ok2, "relation between unknown issues") {
			gqlErr("Entity not found: Issue")
			return
		}
		f.Relations = append(f.Relations, blocker.Identifier+" blocks "+blocked.Identifier)
		data(map[string]any{"issueRelationCreate": map[string]any{"success": true}})
	default:
		gqlErr("unsupported operation: " + op)
	}
}

func (f *Server) node(is *Issue) map[string]any {
	n := map[string]any{
		"id": is.ID, "identifier": is.Identifier, "url": "https://linear.app/acme/issue/" + is.Identifier,
		"title": is.Title, "description": is.Description, "state": map[string]any{"type": is.StateType},
		"parent": nil,
	}
	if is.ParentID != "" {
		n["parent"] = map[string]any{"id": is.ParentID}
	}
	inverse := []any{}
	for _, rel := range f.Relations {
		blocker, blocked, _ := strings.Cut(rel, " blocks ")
		if blocked == is.Identifier {
			if b := f.byIdentifier(blocker); b != nil {
				inverse = append(inverse, map[string]any{"type": linear.RelationBlocks, "issue": map[string]any{"id": b.ID}})
			}
		}
	}
	n["inverseRelations"] = map[string]any{"nodes": inverse}
	return n
}

func (f *Server) add(is *Issue) {
	f.next++
	if is.ID == "" {
		is.ID = fmt.Sprintf("uuid-%d", f.next)
	}
	if is.Identifier == "" {
		is.Identifier = fmt.Sprintf("%s-%d", Team, f.next)
	}
	if is.StateType == "" {
		is.StateType = "backlog"
	}
	f.Issues[is.ID] = is
	f.Order = append(f.Order, is.ID)
}

// Add seeds an issue as if filed earlier; its description is stored as
// given, not normalised, so a test can seed exactly what it wants read back.
func (f *Server) Add(is *Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(is)
}

// Count is how many times an operation (teams, issues, issueCreate,
// issueUpdate, issueRelationCreate) was called.
func (f *Server) Count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if c == op {
			n++
		}
	}
	return n
}

// ByIdentifier returns the issue with that identifier, or nil. The
// pointer is the fake's own, for tests that change an issue between runs.
func (f *Server) ByIdentifier(identifier string) *Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.byIdentifier(identifier)
}

// RelationList copies the "BLOCKER blocks BLOCKED" relations under the lock.
func (f *Server) RelationList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.Relations...)
}

// Update changes the issue with that identifier under the lock, for
// tests that alter the workspace's state between runs.
func (f *Server) Update(identifier string, fn func(*Issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if is := f.byIdentifier(identifier); is != nil {
		fn(is)
	}
}

func (f *Server) byIdentifier(identifier string) *Issue {
	for _, is := range f.Issues {
		if is.Identifier == identifier {
			return is
		}
	}
	return nil
}

// Snapshot copies the issues in creation order under the lock, for
// readers outside a handler.
func (f *Server) Snapshot() []Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Issue, 0, len(f.Order))
	for _, id := range f.Order {
		out = append(out, *f.Issues[id])
	}
	return out
}

// Calls is how many requests the fake has served.
func (f *Server) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}
