// Package lineartest is a fake Linear GraphQL API for tests of the publish
// linear target and of the publish command.
package lineartest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hivecommons/spektacular/internal/publish/linear"
)

// What the fake knows: one team with two states and two labels.
const (
	APIKey   = "lin_api_test"
	Team     = "ENG"
	TeamID   = "team-uuid-eng"
	Backlog  = "Backlog"
	Todo     = "Todo"
	LabelOne = "spek"
	LabelTwo = "epic"
)

// Server is enough of the GraphQL API for the target: teams, issues by
// description, issueCreate, issueUpdate and issueRelationCreate, recording
// each operation.
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
	calls         []string
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
}

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
	require.NoError(f.t, json.NewDecoder(r.Body).Decode(&req))
	data := func(v any) { write(http.StatusOK, map[string]any{"data": v}) }
	gqlErr := func(msg string) {
		write(http.StatusOK, map[string]any{"data": nil, "errors": []map[string]string{{"message": msg}}})
	}
	input, _ := req.Variables["input"].(map[string]any)
	switch {
	case strings.Contains(req.Query, "teams("):
		f.calls = append(f.calls, "teams")
		if req.Variables["key"] != Team {
			data(map[string]any{"teams": map[string]any{"nodes": []any{}}})
			return
		}
		data(map[string]any{"teams": map[string]any{"nodes": []any{map[string]any{
			"id": TeamID, "key": Team,
			"states": map[string]any{"nodes": []any{
				map[string]any{"id": StateID(Backlog), "name": Backlog, "type": "backlog"},
				map[string]any{"id": StateID(Todo), "name": Todo, "type": "unstarted"},
			}},
			"labels": map[string]any{"nodes": []any{
				map[string]any{"id": LabelID(LabelOne), "name": LabelOne},
				map[string]any{"id": LabelID(LabelTwo), "name": LabelTwo},
			}},
		}}}})
	case strings.Contains(req.Query, "issues("):
		f.calls = append(f.calls, "issues")
		needle, _ := req.Variables["needle"].(string)
		nodes := []any{}
		for _, id := range f.Order {
			if req.Variables["team"] != Team {
				break
			}
			is := f.Issues[id]
			if strings.Contains(is.Description, needle) {
				nodes = append(nodes, is.node())
			}
		}
		data(map[string]any{"issues": map[string]any{"nodes": nodes}})
	case strings.Contains(req.Query, "issueCreate("):
		f.calls = append(f.calls, "issueCreate")
		require.Equal(f.t, TeamID, input["teamId"])
		f.next++
		is := &Issue{
			ID:          fmt.Sprintf("uuid-%d", f.next),
			Identifier:  fmt.Sprintf("%s-%d", Team, f.next),
			Title:       input["title"].(string),
			Description: input["description"].(string),
			StateType:   "backlog",
		}
		if p, ok := input["parentId"].(string); ok {
			require.Contains(f.t, f.Issues, p, "parentId must name an existing issue")
			is.ParentID = p
		}
		if s, ok := input["stateId"].(string); ok {
			is.StateID = s
		}
		if ls, ok := input["labelIds"].([]any); ok {
			for _, l := range ls {
				is.LabelIDs = append(is.LabelIDs, l.(string))
			}
		}
		f.Issues[is.ID] = is
		f.Order = append(f.Order, is.ID)
		data(map[string]any{"issueCreate": map[string]any{"success": true, "issue": is.node()}})
	case strings.Contains(req.Query, "issueUpdate("):
		f.calls = append(f.calls, "issueUpdate")
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
			is.Description = d
		}
		data(map[string]any{"issueUpdate": map[string]any{"success": true}})
	case strings.Contains(req.Query, "issueRelationCreate("):
		f.calls = append(f.calls, "issueRelationCreate")
		if f.FailRelations {
			gqlErr("relation refused")
			return
		}
		require.Equal(f.t, linear.RelationBlocks, input["type"])
		blocker, ok1 := f.Issues[input["issueId"].(string)]
		blocked, ok2 := f.Issues[input["relatedIssueId"].(string)]
		require.True(f.t, ok1 && ok2, "relation between unknown issues")
		f.Relations = append(f.Relations, blocker.Identifier+" blocks "+blocked.Identifier)
		data(map[string]any{"issueRelationCreate": map[string]any{"success": true}})
	default:
		gqlErr("unsupported operation: " + req.Query)
	}
}

func (is *Issue) node() map[string]any {
	return map[string]any{
		"id": is.ID, "identifier": is.Identifier, "url": "https://linear.app/acme/issue/" + is.Identifier,
		"title": is.Title, "description": is.Description, "state": map[string]any{"type": is.StateType},
	}
}

// Add seeds an issue as if filed earlier.
func (f *Server) Add(is *Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

// ByIdentifier returns the issue with that identifier, or nil.
func (f *Server) ByIdentifier(identifier string) *Issue {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, is := range f.Issues {
		if is.Identifier == identifier {
			return is
		}
	}
	return nil
}
