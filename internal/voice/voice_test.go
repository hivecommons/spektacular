package voice

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeMCP answers initialize and tools/call over pipes; tools maps a tool
// name to a handler that returns the text content.
func fakeMCP(t *testing.T, tools map[string]func(args map[string]any) (string, bool)) Dialer {
	t.Helper()
	return func(ctx context.Context) (MCPClient, error) {
		clientIn, serverOut := io.Pipe()
		serverIn, clientOut := io.Pipe()
		go func() {
			defer serverOut.Close()
			sc := bufio.NewScanner(serverIn)
			sc.Buffer(make([]byte, 64<<10), 8<<20)
			for sc.Scan() {
				var req struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					} `json:"params"`
				}
				if err := json.Unmarshal(sc.Bytes(), &req); err != nil || req.ID == nil {
					continue
				}
				var result any
				switch req.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": mcpProtocolVersion, "serverInfo": map[string]any{"name": "fake"}}
				case "tools/call":
					h, ok := tools[req.Params.Name]
					if !ok {
						result = map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "unknown tool " + req.Params.Name}}}
						break
					}
					text, isErr := h(req.Params.Arguments)
					result = map[string]any{"isError": isErr, "content": []map[string]any{{"type": "text", "text": text}}}
				default:
					result = map[string]any{}
				}
				out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
				if _, err := serverOut.Write(append(out, '\n')); err != nil {
					return
				}
			}
		}()
		return NewMCPSession(ctx, clientIn, clientOut, func() error {
			_ = clientOut.Close()
			return nil
		})
	}
}

func plaudTools(files string, notes, transcripts map[string]string) map[string]func(map[string]any) (string, bool) {
	return map[string]func(map[string]any) (string, bool){
		"list_files":     func(map[string]any) (string, bool) { return files, false },
		"get_note":       func(a map[string]any) (string, bool) { return notes[a["file_id"].(string)], false },
		"get_transcript": func(a map[string]any) (string, bool) { return transcripts[a["file_id"].(string)], false },
	}
}

const plaudFiles = `{"files":[
 {"id":"a1","name":"Cluster drift alerts","created_at":"2026-05-02T09:00:00Z"},
 {"id":"b2","name":"Grocery list","created_at":"2026-05-03T09:00:00Z"}]}`

func TestPlaudFetchesTheLatestRecordingAsNoteAndTranscript(t *testing.T) {
	notes := map[string]string{"b2": `{"note_list":[{"title":"Summary","content":"Buy milk."}]}`}
	transcripts := map[string]string{"b2": `{"source_list":[{"start_time":1500,"speaker":"Andy","text":"Buy milk."}]}`}
	src := newPlaud(Options{Dial: fakeMCP(t, plaudTools(plaudFiles, notes, transcripts))})
	n, err := src.Fetch(context.Background(), Query{Latest: true})
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Grocery list" || n.URI != "plaud://recordings/b2" {
		t.Fatalf("picked %q %q", n.Title, n.URI)
	}
	if !strings.Contains(n.Summary, "### Summary") || !strings.Contains(n.Summary, "Buy milk.") {
		t.Fatalf("summary: %q", n.Summary)
	}
	if n.Transcript != "[00:01] Andy: Buy milk." {
		t.Fatalf("transcript: %q", n.Transcript)
	}
}

func TestPlaudMatchesATitleCaseInsensitively(t *testing.T) {
	notes := map[string]string{"a1": "## Requirements\n- alert on drift"}
	src := newPlaud(Options{Dial: fakeMCP(t, plaudTools(plaudFiles, notes, map[string]string{}))})
	n, err := src.Fetch(context.Background(), Query{Title: "cluster DRIFT"})
	if err != nil {
		t.Fatal(err)
	}
	if n.URI != "plaud://recordings/a1" || n.Summary != "## Requirements\n- alert on drift" {
		t.Fatalf("got %+v", n)
	}
}

func TestPlaudReportsWhatWasAvailableWhenNothingMatches(t *testing.T) {
	src := newPlaud(Options{Dial: fakeMCP(t, plaudTools(plaudFiles, nil, nil))})
	_, err := src.Fetch(context.Background(), Query{Title: "nope"})
	var nf *NotFoundError
	if !errors.As(err, &nf) || !strings.Contains(err.Error(), "Cluster drift alerts") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlaudPointsAtLoginWhenUnauthenticated(t *testing.T) {
	tools := map[string]func(map[string]any) (string, bool){
		"list_files": func(map[string]any) (string, bool) { return "HTTP 401 Unauthorized", true },
	}
	src := newPlaud(Options{Dial: fakeMCP(t, tools)})
	_, err := src.Fetch(context.Background(), Query{Latest: true})
	if err == nil || !strings.Contains(err.Error(), "@plaud-ai/mcp@latest install") {
		t.Fatalf("err = %v", err)
	}
}

func TestMCPSessionFailsCleanlyWhenTheServerExits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, w := io.Pipe()
	_ = w.Close()
	_, err := NewMCPSession(ctx, r, io.Discard, nil)
	if err == nil {
		t.Fatal("expected an error when the server closes before answering")
	}
}

func TestWisprFlowReadsTheHistoryDatabaseThroughSqlite3(t *testing.T) {
	db := filepath.Join(t.TempDir(), "flow.sqlite")
	if err := os.WriteFile(db, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var got []string
	run := func(_ context.Context, argv ...string) ([]byte, error) {
		got = argv
		return []byte(`[{"id":"h1","text":"Add drift alerts. The console should page us when a cluster drifts.","timestamp":"2026-05-03 10:00:00.000"},
		{"id":"h0","text":"older","timestamp":"2026-05-01 10:00:00.000"}]`), nil
	}
	n, err := newWisprFlow(Options{Run: run}).Fetch(context.Background(), Query{Latest: true, Path: db})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != "sqlite3" || got[1] != "-readonly" || got[3] != db {
		t.Fatalf("argv = %q", got)
	}
	if n.URI != "wisprflow://history/h1" || n.Title != "Add drift alerts" || !strings.HasPrefix(n.Transcript, "Add drift alerts.") {
		t.Fatalf("got %+v", n)
	}
}

func TestFileSourceReadsMarkdownAsSummaryAndSrtAsTranscript(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "drift.md")
	if err := os.WriteFile(md, []byte("# Cluster drift\n\n## Requirements\n- page on drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(dir, "memo.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\nWe need drift alerts.\n\n2\n00:00:02,000 --> 00:00:04,000\n<i>Page the on-call.</i>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := fileSource{}.Fetch(context.Background(), Query{Path: md})
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "Cluster drift" || !strings.Contains(n.Summary, "## Requirements") || n.Transcript != "" || !strings.HasPrefix(n.URI, "file:///") {
		t.Fatalf("md: %+v", n)
	}
	n, err = fileSource{}.Fetch(context.Background(), Query{Path: srt})
	if err != nil {
		t.Fatal(err)
	}
	if n.Transcript != "We need drift alerts. Page the on-call." || n.Title != "memo" {
		t.Fatalf("srt: %+v", n)
	}
}

func TestStdinSourceTakesPastedText(t *testing.T) {
	n, err := stdinSource{r: strings.NewReader("Build me a drift alert for every cluster, please.\n")}.Fetch(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if n.URI != "stdin://" || n.Title != "Build me a drift alert for every cluster" || n.Transcript == "" {
		t.Fatalf("got %+v", n)
	}
}

func TestDraftMapsSummaryHeadingsOntoTheScaffold(t *testing.T) {
	n := Note{Title: "Cluster drift alerts", Summary: strings.Join([]string{
		"## Summary", "Page the on-call when a cluster drifts from its desired state.",
		"## Requirements", "- Detect drift: compare live to desired every minute", "- Page on-call via the existing alert route",
		"## Action items", "- [ ] Detect drift: a drifted cluster shows in the alerts widget within 60s", "- Page on-call: an alert fires once per cluster",
		"## Out of scope", "- auto-remediation",
		"## Open questions", "- which alert route?",
	}, "\n")}
	d := DraftNote(n)
	if len(d.Missing) != 0 {
		t.Fatalf("missing %v", d.Missing)
	}
	for _, want := range []string{
		"# Feature: Cluster drift alerts",
		"## Overview\n\nPage the on-call",
		"## Requirements\n\n- [ ] **Detect drift**: compare live to desired every minute\n- [ ] **Page on-call via the existing alert route**",
		"## Acceptance Criteria\n\n- [ ] **Detect drift**: a drifted cluster shows",
		"- [ ] **Page on-call**: an alert fires once per cluster",
		"## Non-Goals\n\n- auto-remediation",
		"## Open Questions\n\n- which alert route?",
	} {
		if !strings.Contains(d.Body, want) {
			t.Fatalf("body missing %q:\n%s", want, d.Body)
		}
	}
	for _, sec := range Sections {
		if !strings.Contains(d.Body, "## "+sec+"\n") {
			t.Fatalf("scaffold section %q absent", sec)
		}
	}
}

func TestDraftFromABareTranscriptNamesWhatIsMissing(t *testing.T) {
	d := DraftNote(Note{Title: "Memo", Transcript: "We should alert on cluster drift."})
	if strings.Join(d.Missing, ",") != "Requirements,Acceptance Criteria" {
		t.Fatalf("missing = %v", d.Missing)
	}
	if !strings.Contains(d.Body, "## Overview\n\nWe should alert on cluster drift.") {
		t.Fatalf("body:\n%s", d.Body)
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("Cluster Drift Alerts!"); got != "cluster-drift-alerts" {
		t.Fatalf("got %q", got)
	}
	if got := Slug("???"); got != "voice-note" {
		t.Fatalf("got %q", got)
	}
}
