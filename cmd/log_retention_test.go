package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// runLogFiles splits the log files of the project at dir into workflow-run
// files and standalone day files.
func runLogFiles(t *testing.T, dir string) (runs, days []string) {
	t.Helper()
	for _, f := range logFiles(t, logsDir(dir)) {
		if strings.HasPrefix(filepath.Base(f), "standalone_") {
			days = append(days, f)
		} else {
			runs = append(runs, f)
		}
	}
	return runs, days
}

// startRuns starts n separate spec workflow runs in the project at dir, each
// a `--force` restart of a new spec, and returns the run ids in order.
func startRuns(t *testing.T, dir string, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := range n {
		runDebugCmd(t, "spec", "new", "--force", "--data", fmt.Sprintf(`{"name":"run%02d"}`, i))
		id, _ := readStateMap(t, dir)["run_id"].(string)
		require.Regexp(t, traceIDPattern, id)
		ids = append(ids, id)
	}
	return ids
}

// runIDsOnDisk returns the trace id of each run log's records, one per file,
// in file order.
func runIDsOnDisk(t *testing.T, files []string) []string {
	t.Helper()
	ids := make([]string, 0, len(files))
	for _, f := range files {
		recs := readRecords(t, f)
		require.NotEmpty(t, recs)
		ids = append(ids, recs[0].TraceID)
	}
	return ids
}

// With the default retention, 12 separate workflow runs leave the logs of
// the 10 most recent runs only.
func TestLogRetention_DefaultKeepsTheTenMostRecentRuns(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	resetRootCmd(t)

	ids := startRuns(t, dir, 12)

	runs, _ := runLogFiles(t, dir)
	require.Len(t, runs, 10)
	require.Equal(t, ids[2:], runIDsOnDisk(t, runs), "the 10 most recent runs are kept, oldest first by name")
}

// With retention set to 3, the logs of the 3 most recent runs are kept.
func TestLogRetention_ConfiguredRetentionIsHonoured(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  retention: 3\n")
	resetRootCmd(t)

	ids := startRuns(t, dir, 6)

	runs, _ := runLogFiles(t, dir)
	require.Len(t, runs, 3)
	require.Equal(t, ids[3:], runIDsOnDisk(t, runs))
}

// Standalone commands run between workflow runs go to the day log and
// remove no workflow run's log.
func TestLogRetention_StandaloneCommandsRemoveNoRunLog(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  retention: 3\n")
	resetRootCmd(t)

	ids := startRuns(t, dir, 3)
	// With no workflow in progress, later commands are standalone: remove
	// the shared state of the last run.
	require.NoError(t, os.Remove(filepath.Join(dir, ".spektacular", "state.json")))
	for range 20 {
		runLogged(t, "knowledge", "search", "nothing-matches-this")
	}

	runs, days := runLogFiles(t, dir)
	require.Equal(t, ids, runIDsOnDisk(t, runs), "every run log survives the standalone commands")
	require.NotEmpty(t, days)
	total := 0
	for _, d := range days {
		total += len(readRecords(t, d))
	}
	require.Equal(t, 20, total)
}

// When an old log cannot be deleted while retention trims old runs, the
// command's output and exit code are identical to a run with logging off,
// and the log that could not be deleted is left in place.
func TestLogRetention_UndeletableOldLogDoesNotChangeTheCommand(t *testing.T) {
	useConfiguredLogLevel(t)
	run := func(t *testing.T, level string, stuck bool) (string, int) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  retention: 1\n  level: "+level+"\n")
		resetRootCmd(t)
		var stuckPath string
		if stuck {
			// A non-empty directory named like an old run log cannot be
			// removed by the sweep.
			stuckPath = filepath.Join(logsDir(dir), "20200101T000000.000000000Z_claude_spec_stuck.jsonl")
			require.NoError(t, os.MkdirAll(filepath.Join(stuckPath, "inner"), 0o755))
		}
		stdout, code := runLogged(t, "spec", "new", "--data", `{"name":"billing"}`)
		if stuck {
			require.DirExists(t, stuckPath)
			runs, _ := runLogFiles(t, dir)
			require.Len(t, runs, 2, "the new run is logged beside the log that could not be deleted")
		}
		return strings.ReplaceAll(stdout, dir, "TESTDIR"), code
	}

	wantOut, wantCode := run(t, "off", false)
	gotOut, gotCode := run(t, "standard", true)
	require.Equal(t, wantCode, gotCode)
	require.Equal(t, normaliseClock(wantOut), normaliseClock(gotOut))
}

// normaliseClock replaces RFC3339 clock times, which differ between two
// runs, so outputs can be compared.
func normaliseClock(s string) string {
	return timestampPattern.ReplaceAllString(s, "TIMESTAMP")
}
