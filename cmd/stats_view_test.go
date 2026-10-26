package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/runview"
	"github.com/stretchr/testify/require"
)

// decodePage reads the page data out of the page written at path.
func decodePage(t *testing.T, path string) runview.Model {
	t.Helper()
	m, err := runview.Decode(readBytes(t, path))
	require.NoError(t, err)
	return m
}

func TestStatsViewWritesTheLatestRunToTheDefaultFile(t *testing.T) {
	dir := statsProject(t, "standard")
	out := stats(t, "view")

	file := filepath.Join(dir, "spektacular-run-44444444.html")
	require.Equal(t, "Wrote plan 000002_other (run 44444444, 6 spans) to "+file+"\n", out)
	m := decodePage(t, file)
	require.Equal(t, statsOpenPlan, m.Run.RunID, "no selection renders the run that started last")
	require.Len(t, m.Rows.SpanID, 6, "one bar per span and command")
}

func TestStatsViewSelectsTheRunStatsRunReports(t *testing.T) {
	statsProject(t, "standard")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"by name", []string{"--name", "000001_demo"}, statsImplRun},
		{"by full run id", []string{"--run", statsPlanRun}, statsPlanRun},
		{"by run id prefix", []string{"--run", "33333333"}, statsImplRun},
		{"by kind", []string{"--kind", "spec"}, statsSpecRun},
		{"by agent", []string{"--agent", "claude"}, statsPlanRun},
		{"several runs render the first", []string{"--run", statsSpecRun, "--run", statsPlanRun}, statsSpecRun},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRootCmd(t)
			reported := statsJSON(t, append([]string{"run"}, tc.args...)...)["run"].(map[string]any)["run_id"]
			require.Equal(t, tc.want, reported)

			file := filepath.Join(t.TempDir(), "page.html")
			resetRootCmd(t)
			stats(t, append([]string{"view", "--out", file}, tc.args...)...)
			require.Equal(t, tc.want, decodePage(t, file).Run.RunID)
		})
	}
}

func TestStatsViewOutNamesAFileOrADirectory(t *testing.T) {
	dir := statsProject(t, "standard")

	stats(t, "view", "--run", statsSpecRun, "--out", "named.html")
	require.Equal(t, statsSpecRun, decodePage(t, filepath.Join(dir, "named.html")).Run.RunID, "a relative --out is taken from the current directory")

	pages := t.TempDir()
	resetRootCmd(t)
	stats(t, "view", "--run", statsPlanRun, "--out", pages)
	require.Equal(t, statsPlanRun, decodePage(t, filepath.Join(pages, "spektacular-run-abcdef01.html")).Run.RunID,
		"a directory gets the default file name inside it")

	// Rendering again replaces the page, and leaves no temporary file.
	resetRootCmd(t)
	stats(t, "view", "--run", statsImplRun, "--out", filepath.Join(pages, "spektacular-run-abcdef01.html"))
	require.Equal(t, statsImplRun, decodePage(t, filepath.Join(pages, "spektacular-run-abcdef01.html")).Run.RunID)
	entries, err := os.ReadDir(pages)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestStatsViewJSONResult(t *testing.T) {
	dir := statsProject(t, "standard")
	file := filepath.Join(t.TempDir(), "run.html")
	m := statsJSON(t, "view", "--run", statsImplRun, "--out", file)

	info, err := os.Stat(file)
	require.NoError(t, err)
	require.Equal(t, file, m["file"])
	require.Equal(t, float64(info.Size()), m["bytes"])
	require.Equal(t, float64(len(decodePage(t, file).Rows.SpanID)), m["rows"])
	require.Equal(t, float64(7), m["rows"])
	require.Equal(t, float64(300000), m["gap_threshold_ms"])
	require.Equal(t, logsDir(dir), m["log_dir"])
	require.Equal(t, float64(0), m["skipped_lines"])
	run := m["run"].(map[string]any)
	require.Equal(t, statsImplRun, run["run_id"])
	require.Equal(t, "implement", run["kind"])
	require.Equal(t, "000001_demo", run["name"])
	require.Equal(t, "codex", run["agent"])

	resetRootCmd(t)
	out := stats(t, "view", "--run", statsImplRun, "--out", file, "--format", "json", "--fields", `["file"]`)
	var only map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &only))
	require.Equal(t, []string{"file"}, dataKeys(only))
	require.Equal(t, file, only["file"])
}

func TestStatsViewTotalsAndThresholdMatchStatsRun(t *testing.T) {
	statsProject(t, "standard")
	for _, id := range []string{statsSpecRun, statsPlanRun, statsImplRun, statsOpenPlan} {
		resetRootCmd(t)
		totals := statsJSON(t, "run", "--run", id)["totals"].(map[string]any)
		file := filepath.Join(t.TempDir(), "run.html")
		resetRootCmd(t)
		stats(t, "view", "--run", id, "--out", file)
		got := decodePage(t, file).Totals
		require.Equal(t, totals["wall_ms"], float64(got.WallMS), id)
		require.Equal(t, totals["working_ms"], float64(got.WorkingMS), id)
		require.Equal(t, totals["user_wait_ms"], float64(got.UserWaitMS), id)
		require.Equal(t, totals["helper_wait_ms"], float64(got.HelperWaitMS), id)
	}

	file := filepath.Join(t.TempDir(), "run.html")
	resetRootCmd(t)
	m := statsJSON(t, "view", "--run", statsImplRun, "--out", file, "--gap-threshold", "1m")
	require.Equal(t, float64(60000), m["gap_threshold_ms"])
	require.Equal(t, int64(60000), decodePage(t, file).GapThresholdMS, "--gap-threshold is the page's default threshold")
}

func TestStatsViewFromACopiedLogsDirectoryIsIdentical(t *testing.T) {
	dir := statsProject(t, "standard")
	inProject := filepath.Join(t.TempDir(), "in-project.html")
	stats(t, "view", "--run", statsImplRun, "--out", inProject)

	copied := copyLogs(t, logsDir(dir))
	elsewhere := t.TempDir()
	t.Chdir(elsewhere) // outside any project
	resetRootCmd(t)
	stats(t, "view", "--dir", copied, "--run", statsImplRun, "--out", "a.html")
	require.Equal(t, readBytes(t, inProject), readBytes(t, filepath.Join(elsewhere, "a.html")),
		"the page does not depend on where the logs were read")
}

func TestStatsViewLeavesTheLogsByteIdentical(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := statsProject(t, "verbose")
	before := rawLogSnapshot(t, logsDir(dir))
	for _, args := range [][]string{
		{"view"}, {"view", "--format", "json"}, {"view", "--kind", "implement", "--gap-threshold", "1m"},
	} {
		resetRootCmd(t)
		stats(t, args...)
	}
	require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "no file added, removed or changed")
}

func TestStatsViewRefusals(t *testing.T) {
	dir := statsProject(t, "standard")
	logs := logsDir(dir)
	cases := []struct {
		name string
		args []string
		code string
	}{
		{"out inside the logs directory", []string{"stats", "view", "--out", filepath.Join(logs, "x.html")}, "stats_view_out_in_logs"},
		{"out is the logs directory", []string{"stats", "view", "--out", logs}, "stats_view_out_in_logs"},
		{"out under a missing directory", []string{"stats", "view", "--out", filepath.Join(dir, "nonexistent", "dir", "x.html")}, "stats_view_write_failed"},
		{"no matching run", []string{"stats", "view", "--name", "000009_none"}, "stats_no_runs"},
		{"unknown run id", []string{"stats", "view", "--run", "ffffffff"}, "stats_run_not_found"},
		{"bad gap threshold", []string{"stats", "view", "--gap-threshold", "0s"}, "stats_bad_option"},
		{"unsupported format", []string{"stats", "view", "--format", "html"}, "stats_format_unsupported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := rawLogSnapshot(t, logs)
			resetRootCmd(t)
			er := runRefused(t, tc.args...)
			require.Equal(t, tc.code, er.Code)
			require.NotEmpty(t, er.NextAction)
			require.Equal(t, before, rawLogSnapshot(t, logs), "nothing is written into the logs")
		})
	}

	resetRootCmd(t)
	er := runRefused(t, "stats", "view", "--out", filepath.Join(logs, "x.html"))
	require.Contains(t, er.NextAction, "--out")
	require.NoFileExists(t, filepath.Join(logs, "x.html"))

	resetRootCmd(t)
	er = runRefused(t, "stats", "view", "--out", filepath.Join(dir, "nonexistent", "dir", "x.html"))
	require.Contains(t, er.NextAction, filepath.Join(dir, "nonexistent", "dir"))
	require.NoDirExists(t, filepath.Join(dir, "nonexistent"))

	// A copied logs directory is refused the same way.
	copied := copyLogs(t, logs)
	t.Chdir(t.TempDir())
	resetRootCmd(t)
	er = runRefused(t, "stats", "view", "--dir", copied, "--out", filepath.Join(copied, "x.html"))
	require.Equal(t, "stats_view_out_in_logs", er.Code)
	require.NoFileExists(t, filepath.Join(copied, "x.html"))
}

func TestStatsViewSchema(t *testing.T) {
	t.Chdir(t.TempDir())
	resetRootCmd(t)
	out := runOK(t, "stats", "view", "--schema")
	var s commandSchema
	require.NoError(t, json.Unmarshal([]byte(out), &s), out)
	for _, k := range []string{"log_dir", "skipped_lines", "gap_threshold_ms", "run", "file", "rows", "bytes"} {
		require.Contains(t, s.Output.Properties, k)
	}
	for _, k := range []string{"out", "dir", "run", "name", "kind", "agent", "gap-threshold", "format"} {
		require.Contains(t, s.Flags, k)
	}
}
