package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// usageEntry mirrors one row of `knowledge usage`.
type usageEntry struct {
	Tier         string   `json:"tier"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	Created      string   `json:"created"`
	TrackedSince string   `json:"tracked_since"`
	Tracked      bool     `json:"tracked"`
	Discoveries  usageLog `json:"discoveries"`
	Retrievals   usageLog `json:"retrievals"`
}

type usageLog struct {
	Count  int     `json:"count"`
	Last   *string `json:"last"`
	Recent []struct {
		At       string `json:"at"`
		Workflow string `json:"workflow"`
		Spec     string `json:"spec"`
	} `json:"recent"`
}

// fixKnowledgeClock pins the clock usage is recorded against, restoring it
// afterwards.
func fixKnowledgeClock(t *testing.T, at time.Time) {
	t.Helper()
	old := knowledgeNow
	knowledgeNow = func() time.Time { return at }
	t.Cleanup(func() { knowledgeNow = old })
}

// readUsage runs `knowledge usage` over every store and returns its rows
// keyed by "<name>:<path>".
func readUsage(t *testing.T) map[string]usageEntry {
	t.Helper()
	stdout, _, err := runKnowledge(t, "usage")
	require.NoError(t, err)
	var result struct {
		Entries []usageEntry `json:"entries"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	rows := make(map[string]usageEntry, len(result.Entries))
	for _, e := range result.Entries {
		rows[e.Name+":"+e.Path] = e
	}
	return rows
}

// seedWorkflowState writes an in-progress workflow state file at path.
func seedWorkflowState(t *testing.T, path, kind, name string) {
	t.Helper()
	body := map[string]any{
		"kind":            kind,
		"current_step":    "discovery",
		"completed_steps": []string{"overview"},
		"data":            map[string]any{"name": name},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, raw, 0o644))
}

func requireUsageTime(t *testing.T, want time.Time, got string) {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, got)
	require.NoError(t, err, "time %q", got)
	require.True(t, want.Equal(parsed), "want %s, got %s", want, got)
}

// Criterion 1: a search that returns entries records one discovery per
// returned hit, at the search's time, and no retrieval.
func TestKnowledgeSearch_RecordsADiscoveryPerHit(t *testing.T) {
	twoScopeProject(t)
	at := time.Date(2026, time.October, 24, 9, 12, 0, 0, time.UTC)
	fixKnowledgeClock(t, at)

	_, _, err := runKnowledge(t, "search", "compass")
	require.NoError(t, err)

	rows := readUsage(t)
	for _, key := range []string{"testproj:readme.md", "team:guidelines.md"} {
		row := rows[key]
		require.True(t, row.Tracked, key)
		require.Equal(t, 1, row.Discoveries.Count, key)
		require.NotNil(t, row.Discoveries.Last, key)
		requireUsageTime(t, at, *row.Discoveries.Last)
		require.Equal(t, 0, row.Retrievals.Count, key)
	}
	require.Equal(t, 0, rows["testproj:architecture/initial-idea.md"].Discoveries.Count,
		"an entry the search did not return gains no discovery")

	_, _, err = runKnowledge(t, "search", "compass")
	require.NoError(t, err)
	require.Equal(t, 2, readUsage(t)["testproj:readme.md"].Discoveries.Count)
}

// Criterion 2: a read, an always-applied load and a conventions load each
// record one retrieval per returned entry, at the read's time.
func TestKnowledgeRead_AndAlwaysAppliedRecordRetrievals(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	seedKnowledgeFile(t, projectLoc, "conventions/naming.md", "name things plainly\n")
	at := time.Date(2026, time.October, 24, 9, 13, 0, 0, time.UTC)
	fixKnowledgeClock(t, at)

	_, _, err := runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`)
	require.NoError(t, err)
	rows := readUsage(t)
	require.Equal(t, 1, rows["testproj:readme.md"].Retrievals.Count)
	requireUsageTime(t, at, *rows["testproj:readme.md"].Retrievals.Last)
	require.Equal(t, 0, rows["testproj:readme.md"].Discoveries.Count)

	_, _, err = runKnowledge(t, "always-applied")
	require.NoError(t, err)
	require.Equal(t, 1, readUsage(t)["testproj:conventions/naming.md"].Retrievals.Count)

	_, _, err = runKnowledge(t, "conventions")
	require.NoError(t, err)
	rows = readUsage(t)
	require.Equal(t, 2, rows["testproj:conventions/naming.md"].Retrievals.Count)
	require.Equal(t, 1, rows["testproj:readme.md"].Retrievals.Count,
		"loading the conventions retrieves nothing else")
}

// Criterion 2: with exactly one workflow in progress, a read that names no
// spec is attributed to that workflow and its spec.
func TestKnowledgeRead_NamesTheSingleRunningWorkflow(t *testing.T) {
	root, _, _ := twoScopeProject(t)
	seedWorkflowState(t, filepath.Join(root, ".spektacular", "state.json"), "plan", "000099_solo")

	_, _, err := runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`)
	require.NoError(t, err)

	recent := readUsage(t)["testproj:readme.md"].Retrievals.Recent
	require.Len(t, recent, 1)
	require.Equal(t, "plan", recent[0].Workflow)
	require.Equal(t, "000099_solo", recent[0].Spec)
}

// Criterion 2: with two workflows running side by side, a read that names its
// spec is attributed to that spec's workflow, and one that names none is
// attributed to no workflow rather than a guess.
func TestKnowledgeRead_NamedSpecPicksItsWorkflowAmongParallelRuns(t *testing.T) {
	root, projectLoc, _ := twoScopeProject(t)
	seedKnowledgeFile(t, projectLoc, "conventions/naming.md", "name things plainly\n")
	lanes := filepath.Join(root, ".spektacular", "workflows")
	seedWorkflowState(t, filepath.Join(lanes, "plan-000100_a.json"), "plan", "000100_a")
	seedWorkflowState(t, filepath.Join(lanes, "plan-000101_b.json"), "plan", "000101_b")

	_, _, err := runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md","spec":"000101_b"}`)
	require.NoError(t, err)
	_, _, err = runKnowledge(t, "always-applied", "--spec", "000100_a")
	require.NoError(t, err)
	_, _, err = runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`)
	require.NoError(t, err)

	rows := readUsage(t)
	readme := rows["testproj:readme.md"].Retrievals.Recent
	require.Len(t, readme, 2)
	require.Equal(t, "plan", readme[0].Workflow)
	require.Equal(t, "000101_b", readme[0].Spec)
	require.Empty(t, readme[1].Workflow, "an unnamed read among parallel runs names no workflow")
	require.Empty(t, readme[1].Spec)

	naming := rows["testproj:conventions/naming.md"].Retrievals.Recent
	require.Len(t, naming, 1)
	require.Equal(t, "plan", naming[0].Workflow)
	require.Equal(t, "000100_a", naming[0].Spec)
}

// Criterion 3: list, tags and the delete command's existence check record no
// usage, and a delete drops the removed entry's record.
func TestKnowledgeInternalReadsRecordNothing(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	seedKnowledgeFile(t, projectLoc, "gotchas/doomed.md", "---\ntags: [compass]\n---\n\na doomed note\n")

	// Something must have written the index first, or there is nothing to
	// compare against.
	_, _, err := runKnowledge(t, "write", "--data", `{"tier":"repo","name":"testproj","path":"learnings/kept.md"}`, "--file", writeTemp(t, "kept\n"))
	require.NoError(t, err)

	for _, args := range [][]string{
		{"list"},
		{"tags"},
		{"sources"},
		{"usage"},
	} {
		_, _, err := runKnowledge(t, args...)
		require.NoError(t, err, "%v", args)
	}
	_, _, err = runKnowledge(t, "delete", "--data", `{"tier":"repo","name":"testproj","path":"gotchas/doomed.md"}`)
	require.NoError(t, err)

	rows := readUsage(t)
	for key, row := range rows {
		require.Zero(t, row.Discoveries.Count, key)
		require.Zero(t, row.Retrievals.Count, key)
	}
	_, ok := rows["testproj:gotchas/doomed.md"]
	require.False(t, ok, "the deleted entry is gone from usage")

	raw, err := os.ReadFile(filepath.Join(projectLoc, knowledge.UsageIndexFile))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "gotchas/doomed.md", "the deleted entry's record is dropped from the index")
}

// Criterion 4: the index and its lock cannot be written, read or deleted
// through the entry commands, each refusal pointing at `knowledge usage`, and
// the index never appears in list or search.
func TestKnowledgeIndexPathIsRefusedAndHidden(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	// A search writes the index, so it now exists on disk.
	_, _, err := runKnowledge(t, "search", "compass")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(projectLoc, knowledge.UsageIndexFile))

	for _, p := range []string{".usage-index.json", ".usage-index.json.lock"} {
		data := `{"tier":"repo","name":"testproj","path":"` + p + `"}`
		for _, args := range [][]string{
			{"write", "--data", data, "--file", writeTemp(t, "{}\n")},
			{"read", "--data", data},
			{"delete", "--data", data},
		} {
			_, _, err := runKnowledge(t, args...)
			var refusal *output.ErrorResponse
			require.ErrorAs(t, err, &refusal, "%v", args)
			require.Equal(t, knowledge.ErrCodeIndexReserved, refusal.Code, "%v", args)
			require.Contains(t, refusal.NextAction, "knowledge usage --tier repo --filter testproj", "%v", args)
		}
	}
	require.NoFileExists(t, filepath.Join(projectLoc, ".usage-index.json.lock"))

	stdout, _, err := runKnowledge(t, "list")
	require.NoError(t, err)
	require.NotContains(t, stdout, knowledge.UsageIndexFile)

	// "readme" appears in the index as a recorded path, so a store walk would
	// match the index itself if it were not excluded.
	stdout, _, err = runKnowledge(t, "search", "readme")
	require.NoError(t, err)
	require.NotContains(t, stdout, knowledge.UsageIndexFile)
}

// An entry written and then rewritten keeps the creation date of its first
// write, and its counts survive the rewrite.
func TestKnowledgeWrite_RewriteKeepsTheFirstCreationDate(t *testing.T) {
	twoScopeProject(t)
	first := time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC)
	data := `{"tier":"repo","name":"testproj","path":"learnings/fresh.md"}`

	fixKnowledgeClock(t, first)
	_, _, err := runKnowledge(t, "write", "--data", data, "--file", writeTemp(t, "first body\n"))
	require.NoError(t, err)
	_, _, err = runKnowledge(t, "read", "--data", data)
	require.NoError(t, err)

	fixKnowledgeClock(t, first.Add(72*time.Hour))
	_, _, err = runKnowledge(t, "write", "--data", data, "--file", writeTemp(t, "second body\n"))
	require.NoError(t, err)

	row := readUsage(t)["testproj:learnings/fresh.md"]
	requireUsageTime(t, first, row.Created)
	requireUsageTime(t, first, row.TrackedSince)
	require.Equal(t, 1, row.Retrievals.Count, "the rewrite keeps the entry's counts")
}

// Criterion 6: an entry seeded on disk before any index exists can still be
// searched and read, and gains a creation date and a tracking start with no
// manual edit at the first command that writes the index.
func TestKnowledge_PreExistingEntryCarriesOver(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	old := time.Date(2025, time.March, 3, 3, 3, 3, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(projectLoc, "architecture", "initial-idea.md"), old, old))
	now := time.Date(2026, time.October, 24, 12, 0, 0, 0, time.UTC)
	fixKnowledgeClock(t, now)

	before := readUsage(t)["testproj:architecture/initial-idea.md"]
	require.False(t, before.Tracked, "nothing has been recorded yet")

	stdout, _, err := runKnowledge(t, "search", "widgets")
	require.NoError(t, err)
	require.Contains(t, stdout, "architecture/initial-idea.md")
	stdout, _, err = runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"architecture/initial-idea.md"}`)
	require.NoError(t, err)
	require.Contains(t, stdout, "widgets")

	row := readUsage(t)["testproj:architecture/initial-idea.md"]
	require.True(t, row.Tracked)
	requireUsageTime(t, old, row.Created)
	requireUsageTime(t, now, row.TrackedSince)
}

// Criterion 6: in a git work tree, an entry that predates the index is dated
// from the commit that first added it.
func TestKnowledge_PreExistingEntryIsDatedFromGitHistory(t *testing.T) {
	gittest.RequireGit(t)
	root, projectLoc, _ := twoScopeProject(t)
	added := time.Date(2025, time.May, 6, 7, 8, 9, 0, time.UTC)
	git := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
			"GIT_AUTHOR_DATE="+added.Format(time.RFC3339),
			"GIT_COMMITTER_DATE="+added.Format(time.RFC3339),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "seed")

	_, _, err := runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`)
	require.NoError(t, err)
	requireUsageTime(t, added, readUsage(t)["testproj:readme.md"].Created)
	_ = projectLoc
}

// Criterion 5: when usage cannot be recorded, the search and the read still
// return their results, carrying a usage_warning.
func TestKnowledge_RecordingFailureStillReturnsTheResult(t *testing.T) {
	_, projectLoc, _ := twoScopeProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(projectLoc, knowledge.UsageIndexFile), []byte("{not json"), 0o644))

	stdout, _, err := runKnowledge(t, "read", "--data", `{"tier":"repo","name":"testproj","path":"readme.md"}`)
	require.NoError(t, err)
	var read map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &read))
	require.Equal(t, "project readme: the compass points north\n", read["content"])
	require.Contains(t, read["usage_warning"], "usage not recorded")

	stdout, _, err = runKnowledge(t, "search", "compass")
	require.NoError(t, err)
	var search map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &search))
	require.NotEmpty(t, search["hits"])
	require.Contains(t, search["usage_warning"], "usage not recorded")

	stdout, _, err = runKnowledge(t, "read", "--data", `{"tier":"project","name":"team","path":"guidelines.md"}`)
	require.NoError(t, err)
	require.NotContains(t, stdout, "usage_warning", "a store with a healthy index records normally")
}

// `knowledge usage` publishes its schema like every other knowledge command,
// and read's input schema offers the optional spec.
func TestKnowledgeUsage_Schema(t *testing.T) {
	twoScopeProject(t)

	schema, _ := knowledgeSchema(t, "usage")
	require.Nil(t, schema.Input)
	require.NotNil(t, schema.Output.Properties["entries"])
	require.Contains(t, schema.Flags, "tier")

	read, _ := knowledgeSchema(t, "read")
	require.Equal(t, []string{"tier", "name", "path"}, read.Input.Required)
	require.Equal(t, "string", read.Input.Properties["spec"].Type)

	applied, _ := knowledgeSchema(t, "always-applied")
	require.Contains(t, applied.Flags, "spec")
}

// writeTemp writes body to a temp file and returns its path, for --file.
func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "body.md")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}
