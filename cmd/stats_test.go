package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// The stats group carries the logging-exempt annotation, so no `stats`
// command, bare or mistyped, ever adds a record or a file to the logs it
// reads, at the standard or the verbose level; the next ordinary command is
// still logged, with the same output and exit code as before.
func TestStatsWritesNoRecord(t *testing.T) {
	useConfiguredLogLevel(t)
	require.Equal(t, logExempt, statsCmd.Annotations[logAnnotation])
	for _, level := range []string{"standard", "verbose"} {
		t.Run(level, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  level: "+level+"\n")
			resetRootCmd(t)

			runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			listOut, listCode := runLogged(t, "spec", "file", "list")
			require.Equal(t, 0, listCode)
			require.Equal(t, []string{"spec new", "spec file list"}, recordNames(allRecords(t, logsDir(dir))))
			before := rawLogSnapshot(t, logsDir(dir))

			for _, args := range [][]string{{"stats"}, {"stats", "nope"}} {
				stdout, code := runLogged(t, args...)
				require.Equal(t, 1, code, "%v", args)
				var env map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &env), stdout)
				require.Equal(t, "unknown_subcommand", env["code"], stdout)
			}
			// Driven through SetArgs alone, so only the post-command check
			// sees the command is stats.
			_, _, code := runRootCmd(t, "stats")
			require.Equal(t, 1, code)
			require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "stats adds no record and no file")

			stdout, code := runLogged(t, "spec", "file", "list")
			require.Equal(t, listCode, code)
			require.Equal(t, listOut, stdout, "the next command's output is unchanged")
			require.Equal(t, []string{"spec new", "spec file list", "spec file list"}, recordNames(allRecords(t, logsDir(dir))))
		})
	}
}

// Fixture runs, each with a fixed id and fixed times on 2026-10-25. Ids 1
// and 2 share the prefix "abcdef01", for the ambiguous-prefix refusal.
const (
	statsSpecRun  = "abcdef01110000000000000000000001" // spec 000001_demo, claude, 08:00, finished
	statsPlanRun  = "abcdef01220000000000000000000002" // plan 000001_demo, claude, 09:00, finished
	statsImplRun  = "3333333300000000000000000000000c" // implement 000001_demo, codex, 10:00, finished
	statsOpenPlan = "4444444400000000000000000000000d" // plan 000002_other, codex, 12:00, in progress
)

func statsAt(h, m int) time.Time { return time.Date(2026, 10, 25, h, m, 0, 0, time.UTC) }

// statsFixture builds one run's records. Each enters "first" at +1m and moves
// to "second" at +10m; a finished run reaches "finished" at +end.
type statsFixture struct {
	trace, kind, name, agent string
	start                    time.Time
}

func (f statsFixture) attrs(typ string) map[string]any {
	return map[string]any{
		tracelog.AttrSpanType: typ, tracelog.AttrAgent: f.agent,
		tracelog.AttrWorkflowKind: f.kind, tracelog.AttrWorkflowName: f.name,
	}
}

func (f statsFixture) cmd(name, parent string, from, to time.Duration) tracelog.Record {
	return tracelog.Record{
		Record: tracelog.RecordSpan, TraceID: f.trace, SpanID: fmt.Sprintf("%s%04d", f.trace[:12], int(from.Seconds())),
		ParentSpanID: parent, Name: name, Kind: tracelog.KindInternal,
		StartTime: f.start.Add(from), EndTime: f.start.Add(to),
		Status: tracelog.Status{Code: tracelog.StatusOK}, Attributes: f.attrs(tracelog.TypeCommand),
	}
}

func (f statsFixture) span(id, name, typ, parent string, from, to time.Duration, reason string) []tracelog.Record {
	start := tracelog.Record{
		Record: tracelog.RecordSpanStart, TraceID: f.trace, SpanID: id, ParentSpanID: parent, Name: name,
		Kind: tracelog.KindInternal, StartTime: f.start.Add(from), Attributes: f.attrs(typ),
	}
	if reason == "" {
		return []tracelog.Record{start}
	}
	end := start
	end.Record = tracelog.RecordSpanEnd
	end.EndTime = f.start.Add(to)
	end.Status = tracelog.ReasonStatus(reason)
	end.Attributes = f.attrs(typ)
	end.Attributes[tracelog.AttrEndReason] = reason
	return []tracelog.Record{start, end}
}

// records is the run: a new command, a first step with one command and a
// helper, a second step with two commands; finished runs end at +end.
func (f statsFixture) records(end time.Duration, finished bool) []tracelog.Record {
	p := f.trace[:4]
	min := time.Minute
	rs := []tracelog.Record{f.cmd(f.kind+" new", "", 0, 30*time.Second)}
	rs = append(rs, f.span(p+"000000000001", "first", tracelog.TypeStep, "", min, 10*min, tracelog.EndMoved)...)
	rs = append(rs, f.span(p+"0000000000a1", "research", tracelog.TypeSubAgent, p+"000000000001", 2*min, 5*min, tracelog.EndStopped)...)
	rs = append(rs, f.cmd(f.kind+" goto", p+"000000000001", 9*min, 10*min))
	if !finished {
		rs = append(rs, f.span(p+"000000000002", "second", tracelog.TypeStep, "", 10*min, 0, "")...)
		rs = append(rs, f.cmd(f.kind+" goto", p+"000000000002", 30*min, 31*min))
		return rs
	}
	rs = append(rs, f.span(p+"000000000002", "second", tracelog.TypeStep, "", 10*min, end, tracelog.EndMoved)...)
	rs = append(rs, f.cmd(f.kind+" goto", p+"000000000002", end-min, end))
	rs = append(rs, f.span(p+"000000000003", "finished", tracelog.TypeStep, "", end, end, tracelog.EndFinished)...)
	return rs
}

// fileName is the run's log file, named as the file logger names it.
func (f statsFixture) fileName() string {
	return f.start.Format("20060102T150405.000000000Z") + "_" + f.agent + "_" + f.kind + "_" + f.name + ".jsonl"
}

// seedLogs writes three finished runs (spec, plan, implement, by two
// agents), one plan run in progress, and a day file of standalone
// commands, into the logs directory dir.
func seedLogs(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	write := func(name string, rs []tracelog.Record) {
		var b bytes.Buffer
		for _, r := range rs {
			line, err := json.Marshal(r)
			require.NoError(t, err)
			b.Write(line)
			b.WriteByte('\n')
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0o644))
	}
	for _, f := range []struct {
		fx       statsFixture
		end      time.Duration
		finished bool
	}{
		{statsFixture{statsSpecRun, "spec", "000001_demo", "claude", statsAt(8, 0)}, 30 * time.Minute, true},
		{statsFixture{statsPlanRun, "plan", "000001_demo", "claude", statsAt(9, 0)}, 45 * time.Minute, true},
		{statsFixture{statsImplRun, "implement", "000001_demo", "codex", statsAt(10, 0)}, 60 * time.Minute, true},
		{statsFixture{statsOpenPlan, "plan", "000002_other", "codex", statsAt(12, 0)}, 0, false},
	} {
		write(f.fx.fileName(), f.fx.records(f.end, f.finished))
	}
	alone := tracelog.Record{
		Record: tracelog.RecordSpan, TraceID: "5555555500000000000000000000000e", SpanID: "5555555500000001",
		Name: "spec file list", Kind: tracelog.KindInternal, StartTime: statsAt(7, 0), EndTime: statsAt(7, 1),
		Status: tracelog.Status{Code: tracelog.StatusOK}, Attributes: map[string]any{tracelog.AttrSpanType: tracelog.TypeCommand, tracelog.AttrAgent: "claude"},
	}
	write("standalone_20261025.jsonl", []tracelog.Record{alone})
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644))
}

// statsProject makes a project, with logging on at level, whose logs
// directory holds the seeded runs, and changes into it.
func statsProject(t *testing.T, level string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: "+level+"\n")
	seedLogs(t, logsDir(dir))
	resetRootCmd(t)
	return dir
}

// stats runs a stats command through runRoot with os.Args matching, and
// requires it to succeed.
func stats(t *testing.T, args ...string) string {
	t.Helper()
	stdout, code := runLogged(t, append([]string{"stats"}, args...)...)
	require.Equalf(t, 0, code, "stats %v: %s", args, stdout)
	return stdout
}

func statsJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	out := stats(t, append(args, "--format", "json")...)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m), out)
	require.Equal(t, false, m["error"])
	return m
}

func listedIDs(t *testing.T, m map[string]any) []string {
	t.Helper()
	var ids []string
	for _, r := range m["runs"].([]any) {
		ids = append(ids, r.(map[string]any)["run_id"].(string))
	}
	return ids
}

func TestStatsListShowsEveryRunNewestFirst(t *testing.T) {
	dir := statsProject(t, "standard")
	m := statsJSON(t, "list")
	require.Equal(t, logsDir(dir), m["log_dir"])
	require.Equal(t, float64(0), m["skipped_lines"])
	require.Equal(t, []string{statsOpenPlan, statsImplRun, statsPlanRun, statsSpecRun}, listedIDs(t, m))

	runs := m["runs"].([]any)
	require.Equal(t, map[string]any{
		"run_id": statsImplRun, "kind": "implement", "name": "000001_demo", "agent": "codex",
		"log_file":   "20261025T100000.000000000Z_codex_implement_000001_demo.jsonl",
		"started_at": "2026-10-25T10:00:00Z", "ended_at": "2026-10-25T11:00:00Z",
		"wall_ms": float64(3600000), "status": "finished",
	}, runs[1])
	open := runs[0].(map[string]any)
	require.Equal(t, "in_progress", open["status"])
	require.Equal(t, float64(31*60000), open["wall_ms"])

	pretty := stats(t, "list")
	lines := strings.Split(strings.TrimSpace(pretty), "\n")
	require.Len(t, lines, 5)
	require.Equal(t, []string{"RUN", "KIND", "NAME", "AGENT", "STARTED", "WALL", "STATUS"}, strings.Fields(lines[0]))
	require.Equal(t, []string{"44444444", "plan", "000002_other", "codex", "2026-10-25", "12:00:00", "31m00s", "in_progress"}, strings.Fields(lines[1]))
	require.Equal(t, []string{"33333333", "implement", "000001_demo", "codex", "2026-10-25", "10:00:00", "1h00m00s", "finished"}, strings.Fields(lines[2]))
	require.Equal(t, []string{"abcdef01", "plan", "000001_demo", "claude", "2026-10-25", "09:00:00", "45m00s", "finished"}, strings.Fields(lines[3]))
	require.Equal(t, []string{"abcdef01", "spec", "000001_demo", "claude", "2026-10-25", "08:00:00", "30m00s", "finished"}, strings.Fields(lines[4]))
}

func TestStatsRunDefaultsToTheLatestRun(t *testing.T) {
	statsProject(t, "standard")
	m := statsJSON(t, "run")
	run := m["run"].(map[string]any)
	require.Equal(t, statsOpenPlan, run["run_id"])
	require.Equal(t, float64(300000), m["gap_threshold_ms"])
	totals := m["totals"].(map[string]any)
	require.Equal(t, float64(31*60000), totals["wall_ms"])
	require.Equal(t, float64(3*60000), totals["helper_wait_ms"])
	require.Equal(t, float64(3), totals["commands"])
}

func TestStatsSelectionsNarrowTheRuns(t *testing.T) {
	statsProject(t, "standard")
	cases := []struct {
		name   string
		args   []string
		listed []string
	}{
		{"by name", []string{"--name", "000001_demo"}, []string{statsImplRun, statsPlanRun, statsSpecRun}},
		{"by kind", []string{"--kind", "plan"}, []string{statsOpenPlan, statsPlanRun}},
		{"by agent", []string{"--agent", "codex"}, []string{statsOpenPlan, statsImplRun}},
		{"by kind and agent", []string{"--kind", "plan", "--agent", "claude"}, []string{statsPlanRun}},
		{"by full run id", []string{"--run", statsSpecRun}, []string{statsSpecRun}},
		{"by unique prefix", []string{"--run", "abcdef0122"}, []string{statsPlanRun}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRootCmd(t)
			require.Equal(t, tc.listed, listedIDs(t, statsJSON(t, append([]string{"list"}, tc.args...)...)))
			resetRootCmd(t)
			run := statsJSON(t, append([]string{"run"}, tc.args...)...)["run"].(map[string]any)
			require.Equal(t, tc.listed[0], run["run_id"], "the report covers the latest selected run")
		})
	}
}

func TestStatsFieldsFilter(t *testing.T) {
	statsProject(t, "standard")
	out := stats(t, "run", "--format", "json", "--fields", `["run"]`)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &m))
	require.Equal(t, []string{"run"}, dataKeys(m))
	require.Equal(t, statsOpenPlan, m["run"].(map[string]any)["run_id"])
}

func TestStatsPrettyShowsTheSameFigures(t *testing.T) {
	statsProject(t, "standard")
	m := statsJSON(t, "run", "--run", statsImplRun)
	resetRootCmd(t)
	pretty := stats(t, "run", "--run", statsImplRun)

	require.Contains(t, pretty, "implement 000001_demo  agent codex  run 33333333  finished")
	require.Equal(t, float64(3600000), m["totals"].(map[string]any)["wall_ms"])
	require.Regexp(t, `(?m)^Wall time\s+1h00m00s$`, pretty)
	require.Equal(t, float64(180000), m["totals"].(map[string]any)["helper_wait_ms"])
	require.Regexp(t, `(?m)^Helper wait\s+3m00s\s+\(1 helper\)$`, pretty)
	want := map[string]float64{"first": 9 * 60000, "second": 50 * 60000, "finished": 0}
	for _, s := range m["steps"].([]any) {
		row := s.(map[string]any)
		name := row["step"].(string)
		require.Equal(t, want[name], row["duration_ms"], name)
	}
	require.Regexp(t, `(?m)^first\s+1\s+9m00s\s+1\s+1m00s\s+3m00s\s+0\.0s\s+0\s+\?\s+\?\s+\?$`, pretty)
	require.Regexp(t, `(?m)^second\s+1\s+50m00s\s+1\s+1m00s\s+0\.0s\s+0\.0s\s+0\s+\?\s+\?\s+\?$`, pretty)
	require.Regexp(t, `(?m)^research\s+first\s+2026-10-25 10:02:00\s+3m00s\s+stopped\s+\?\s+\?\s+\?$`, pretty)
}

// copyLogs copies the regular files of src into a new directory.
func copyLogs(t *testing.T, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "collected", "logs")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		require.NoError(t, os.WriteFile(filepath.Join(dst, e.Name()), readBytes(t, filepath.Join(src, e.Name())), 0o644))
	}
	return dst
}

func TestStatsDirOnACopyGivesTheSameFigures(t *testing.T) {
	dir := statsProject(t, "standard")
	inProject := map[string]map[string]any{}
	for _, sub := range []string{"list", "run"} {
		resetRootCmd(t)
		inProject[sub] = statsJSON(t, sub)
	}

	copied := copyLogs(t, logsDir(dir))
	t.Chdir(t.TempDir()) // outside any project
	for _, sub := range []string{"list", "run"} {
		resetRootCmd(t)
		got := statsJSON(t, sub, "--dir", copied)
		require.Equal(t, copied, got["log_dir"])
		got["log_dir"] = inProject[sub]["log_dir"]
		require.Equal(t, inProject[sub], got, sub)
	}

	// A relative --dir is made absolute.
	t.Chdir(filepath.Dir(copied))
	resetRootCmd(t)
	require.Equal(t, copied, statsJSON(t, "list", "--dir", "logs")["log_dir"])
}

func TestStatsSkipsAMalformedLine(t *testing.T) {
	dir := statsProject(t, "standard")
	clean := statsJSON(t, "run", "--run", statsPlanRun)

	f, err := os.OpenFile(filepath.Join(logsDir(dir), "20261025T090000.000000000Z_claude_plan_000001_demo.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("{this is not a record\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	resetRootCmd(t)
	damaged := statsJSON(t, "run", "--run", statsPlanRun)
	require.Equal(t, float64(1), damaged["skipped_lines"])
	damaged["skipped_lines"] = clean["skipped_lines"]
	require.Equal(t, clean, damaged, "every figure is unchanged")

	resetRootCmd(t)
	require.Contains(t, stats(t, "run", "--run", statsPlanRun), "Skipped lines: 1")
	resetRootCmd(t)
	require.Equal(t, float64(1), statsJSON(t, "list")["skipped_lines"])
}

func TestStatsListAndRunLeaveTheLogsByteIdentical(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := statsProject(t, "verbose")
	before := rawLogSnapshot(t, logsDir(dir))
	for _, args := range [][]string{
		{"list"}, {"list", "--format", "json"},
		{"run"}, {"run", "--format", "json"}, {"run", "--kind", "implement", "--gaps", "3"},
	} {
		resetRootCmd(t)
		stats(t, args...)
	}
	require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "no file added, removed or changed")
}

func TestStatsRefusals(t *testing.T) {
	dir := statsProject(t, "standard")
	cases := []struct {
		name string
		args []string
		code string
	}{
		{"unsupported format", []string{"stats", "list", "--format", "xml"}, "stats_format_unsupported"},
		{"missing directory", []string{"stats", "run", "--dir", filepath.Join(dir, "nope")}, "stats_log_dir_not_found"},
		{"no matching run", []string{"stats", "run", "--name", "000009_none"}, "stats_no_runs"},
		{"unknown run id", []string{"stats", "run", "--run", "ffffffff"}, "stats_run_not_found"},
		{"ambiguous run id", []string{"stats", "run", "--run", "abcdef01"}, "stats_run_ambiguous"},
		{"malformed run id", []string{"stats", "list", "--run", "xyz"}, "stats_bad_selection"},
		{"bad kind", []string{"stats", "list", "--kind", "epic"}, "stats_bad_selection"},
		{"bad gap count", []string{"stats", "run", "--gaps", "0"}, "stats_bad_option"},
		{"bad gap threshold", []string{"stats", "run", "--gap-threshold", "0s"}, "stats_bad_option"},
		{"bare stats", []string{"stats"}, "unknown_subcommand"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRootCmd(t)
			er := runRefused(t, tc.args...)
			require.Equal(t, tc.code, er.Code)
			require.NotEmpty(t, er.NextAction)
		})
	}

	resetRootCmd(t)
	er := runRefused(t, "stats", "run", "--run", "abcdef01")
	require.Contains(t, er.NextAction, statsSpecRun)
	require.Contains(t, er.NextAction, statsPlanRun)

	resetRootCmd(t)
	er = runRefused(t, "stats")
	require.Equal(t, "run one of: commands, compare, list, run, spans, summary, view", er.NextAction)

	t.Chdir(t.TempDir())
	resetRootCmd(t)
	er = runRefused(t, "stats", "list")
	require.Equal(t, "no_project", er.Code)
	require.Contains(t, er.NextAction, "--dir")
}

func TestStatsSchema(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, sub := range []string{"list", "run", "compare", "summary", "commands", "spans", "view"} {
		resetRootCmd(t)
		out := runOK(t, "stats", sub, "--schema")
		var s commandSchema
		require.NoError(t, json.Unmarshal([]byte(out), &s), out)
		require.Contains(t, s.Output.Properties, "log_dir")
		require.Contains(t, s.Output.Properties, "skipped_lines")
		require.Contains(t, s.Flags, "dir")
		require.Contains(t, s.Flags, "run")
	}
}

// seedImplementRuns adds four more finished implement runs of 000001_demo,
// by claude, at 13:00 to 16:00 and 20 to 50 minutes long, so the logs hold
// five implement runs in all.
func seedImplementRuns(t *testing.T, dir string) []string {
	t.Helper()
	ids := []string{statsImplRun}
	for i := 1; i <= 4; i++ {
		f := statsFixture{fmt.Sprintf("%d%d%d%d00000000000000000000000000%02d", 6+i, 6+i, 6+i, 6+i, i), "implement", "000001_demo", "claude", statsAt(12+i, 0)}
		var b bytes.Buffer
		for _, r := range f.records(time.Duration(10+10*i)*time.Minute, true) {
			line, err := json.Marshal(r)
			require.NoError(t, err)
			b.Write(line)
			b.WriteByte('\n')
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.fileName()), b.Bytes(), 0o644))
		ids = append(ids, f.trace)
	}
	return ids
}

func runIDs(t *testing.T, m map[string]any) []string {
	t.Helper()
	var ids []string
	for _, r := range m["runs"].([]any) {
		ids = append(ids, r.(map[string]any)["run_id"].(string))
	}
	return ids
}

// figure finds a figure row by name in a list of figure rows.
func figure(t *testing.T, rows any, name string) map[string]any {
	t.Helper()
	for _, r := range rows.([]any) {
		if m := r.(map[string]any); m["figure"] == name {
			return m
		}
	}
	t.Fatalf("no figure %s", name)
	return nil
}

func stepFigures(t *testing.T, m map[string]any, step string) any {
	t.Helper()
	for _, s := range m["steps"].([]any) {
		if sm := s.(map[string]any); sm["step"] == step {
			return sm["figures"]
		}
	}
	t.Fatalf("no step %s", step)
	return nil
}

func TestStatsCompareDefaultsAndNamedRuns(t *testing.T) {
	statsProject(t, "standard")
	// The latest run is a plan run: the two latest plan runs, oldest first.
	m := statsJSON(t, "compare")
	require.Equal(t, []string{statsPlanRun, statsOpenPlan}, runIDs(t, m))
	require.Equal(t, float64(300000), m["gap_threshold_ms"])
	require.Equal(t, map[string]any{
		"figure": "wall_ms", "values": []any{float64(45 * 60000), float64(31 * 60000)},
		"delta": []any{float64(0), float64(-14 * 60000)}, "delta_pct": []any{float64(0), -31.1},
	}, figure(t, m["totals"], "wall_ms"))
	// The open run never reached finished: absent, not zero.
	fin := figure(t, stepFigures(t, m, "finished"), "duration_ms")
	require.Equal(t, []any{float64(0), nil}, fin["values"])
	require.Equal(t, []any{nil, nil}, fin["delta_pct"])

	// Named runs compare in the order given, across kinds.
	resetRootCmd(t)
	m = statsJSON(t, "compare", "--run", statsImplRun, "--run", statsSpecRun)
	require.Equal(t, []string{statsImplRun, statsSpecRun}, runIDs(t, m))
	second := figure(t, stepFigures(t, m, "second"), "duration_ms")
	require.Equal(t, []any{float64(50 * 60000), float64(20 * 60000)}, second["values"])
	require.Equal(t, []any{float64(0), float64(-30 * 60000)}, second["delta"])
	require.Equal(t, []any{float64(0), -60.0}, second["delta_pct"])

	resetRootCmd(t)
	pretty := stats(t, "compare", "--run", statsImplRun, "--run", statsSpecRun)
	require.Regexp(t, `(?m)^second\s+50m00s\s+20m00s \(-30m00s, -60\.0%\)$`, pretty)
	require.Regexp(t, `(?m)^wall_ms\s+1h00m00s\s+30m00s \(-30m00s, -50\.0%\)$`, pretty)
}

func TestStatsCompareNeedsTwoRuns(t *testing.T) {
	statsProject(t, "standard")
	for _, args := range [][]string{
		{"stats", "compare", "--run", statsSpecRun},
		{"stats", "compare", "--kind", "spec"},
		{"stats", "compare", "--kind", "plan", "--run", statsSpecRun, "--run", statsImplRun},
	} {
		resetRootCmd(t)
		er := runRefused(t, args...)
		require.Equal(t, "stats_compare_needs_two", er.Code, "%v", args)
		require.Contains(t, er.NextAction, "--run <id> --run <id>")
	}
}

func TestStatsSummaryDefaultsAndKind(t *testing.T) {
	dir := statsProject(t, "standard")
	m := statsJSON(t, "summary")
	require.Equal(t, []string{statsPlanRun, statsOpenPlan}, runIDs(t, m), "every run of the latest run's kind, oldest first")
	require.Equal(t, map[string]any{"figure": "wall_ms", "count": float64(2), "median": float64(38 * 60000), "p90": float64(45 * 60000)},
		figure(t, m["totals"], "wall_ms"))
	require.Equal(t, float64(1), figure(t, stepFigures(t, m, "finished"), "duration_ms")["count"], "only one plan run finished")

	ids := seedImplementRuns(t, logsDir(dir))
	resetRootCmd(t)
	m = statsJSON(t, "summary", "--kind", "implement")
	require.ElementsMatch(t, ids, runIDs(t, m))
	// second lasts 50, 10, 20, 30 and 40 minutes.
	require.Equal(t, map[string]any{"figure": "duration_ms", "count": float64(5), "median": float64(30 * 60000), "p90": float64(50 * 60000)},
		figure(t, stepFigures(t, m, "second"), "duration_ms"))
	for _, f := range m["totals"].([]any) {
		require.Equal(t, float64(5), f.(map[string]any)["count"])
	}

	resetRootCmd(t)
	pretty := stats(t, "summary", "--kind", "implement")
	require.Contains(t, pretty, "Runs: 5\n")
	require.Regexp(t, `(?m)^second\s+5\s+30m00s\s+50m00s$`, pretty)

	resetRootCmd(t)
	m = statsJSON(t, "summary", "--agent", "codex", "--kind", "implement")
	require.Equal(t, []string{statsImplRun}, runIDs(t, m), "the same selection as the other commands")

	resetRootCmd(t)
	er := runRefused(t, "stats", "summary", "--name", "000009_none")
	require.Equal(t, "stats_no_runs", er.Code)
}

func TestStatsCompareAndSummaryLeaveTheLogsByteIdentical(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := statsProject(t, "verbose")
	before := rawLogSnapshot(t, logsDir(dir))
	for _, args := range [][]string{
		{"compare"}, {"compare", "--format", "json"}, {"compare", "--run", statsSpecRun, "--run", statsImplRun},
		{"summary"}, {"summary", "--format", "json"}, {"summary", "--kind", "spec"},
	} {
		resetRootCmd(t)
		stats(t, args...)
	}
	require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "no file added, removed or changed")
}

// The compare and summary JSON keys, written by hand from the contract in
// the plan's Data Structures section (reproduced on statsCompareResult).
func TestStatsCompareAndSummaryFieldNames(t *testing.T) {
	statsProject(t, "standard")
	top := []string{"error", "gap_threshold_ms", "log_dir", "runs", "skipped_lines", "steps", "totals"}

	m := statsJSON(t, "compare")
	require.Equal(t, top, dataKeys(m))
	require.Equal(t, []string{"agent", "ended_at", "kind", "log_file", "name", "run_id", "started_at", "status", "wall_ms"},
		dataKeys(m["runs"].([]any)[0].(map[string]any)))
	var order []string
	for _, f := range m["totals"].([]any) {
		require.Equal(t, []string{"delta", "delta_pct", "figure", "values"}, dataKeys(f.(map[string]any)))
		order = append(order, f.(map[string]any)["figure"].(string))
	}
	require.Equal(t, []string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "gap_ms"}, order)
	step := m["steps"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"figures", "step"}, dataKeys(step))
	order = nil
	for _, f := range step["figures"].([]any) {
		order = append(order, f.(map[string]any)["figure"].(string))
	}
	require.Equal(t, []string{"duration_ms", "entries", "commands", "command_ms", "helper_wait_ms", "user_wait_ms"}, order)

	resetRootCmd(t)
	m = statsJSON(t, "summary")
	require.Equal(t, top, dataKeys(m))
	for _, f := range m["totals"].([]any) {
		require.Equal(t, []string{"count", "figure", "median", "p90"}, dataKeys(f.(map[string]any)))
	}
	step = m["steps"].([]any)[0].(map[string]any)
	require.Equal(t, []string{"figures", "step"}, dataKeys(step))
	require.Equal(t, []string{"count", "figure", "median", "p90"}, dataKeys(step["figures"].([]any)[0].(map[string]any)))
}

// Records an epic orchestrator wrote outside any run, at 06:00 to 06:30:
// an `epic order` command, a helper span it stopped, and one it never did.
const (
	statsOrchOrder = "6666666600000000000000000000000a"
	statsOrchSpan  = "6666666600000000000000000000000b"
	statsOrchOpen  = "6666666600000000000000000000000c"
)

// seedOrchestrator adds the orchestrator's day file to the logs directory.
func seedOrchestrator(t *testing.T, dir string) {
	t.Helper()
	attrs := func(typ string) map[string]any {
		return map[string]any{tracelog.AttrSpanType: typ, tracelog.AttrAgent: "claude"}
	}
	order := tracelog.Record{
		Record: tracelog.RecordSpan, TraceID: statsOrchOrder, SpanID: "6666666600000001", Name: "epic order",
		Kind: tracelog.KindInternal, StartTime: statsAt(6, 0), EndTime: statsAt(6, 1),
		Status: tracelog.Status{Code: tracelog.StatusOK}, Attributes: attrs(tracelog.TypeCommand),
	}
	order.Attributes[tracelog.AttrCommandArgs] = []string{"epic", "order", "--data", `{"name":"e"}`}
	order.Attributes[tracelog.AttrExitCode] = 0
	start := tracelog.Record{
		Record: tracelog.RecordSpanStart, TraceID: statsOrchSpan, SpanID: "6666666600000002", Name: "plan 000001_demo",
		Kind: tracelog.KindInternal, StartTime: statsAt(6, 5), Attributes: attrs(tracelog.TypeSubAgent),
	}
	end := start
	end.Record = tracelog.RecordSpanEnd
	end.EndTime = statsAt(6, 30)
	end.Status = tracelog.Status{Code: tracelog.StatusOK}
	end.Attributes = attrs(tracelog.TypeSubAgent)
	end.Attributes[tracelog.AttrEndReason] = tracelog.EndStopped
	open := tracelog.Record{
		Record: tracelog.RecordSpanStart, TraceID: statsOrchOpen, SpanID: "6666666600000003", Name: "plan 000002_other",
		Kind: tracelog.KindInternal, StartTime: statsAt(6, 10), Attributes: attrs(tracelog.TypeSubAgent),
	}
	var b bytes.Buffer
	for _, r := range []tracelog.Record{order, start, open, end} {
		line, err := json.Marshal(r)
		require.NoError(t, err)
		b.Write(line)
		b.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "standalone_20261024.jsonl"), b.Bytes(), 0o644))
}

// listingProject is statsProject with the orchestrator's records added.
func listingProject(t *testing.T, level string) string {
	t.Helper()
	dir := statsProject(t, level)
	seedOrchestrator(t, logsDir(dir))
	return dir
}

func rows(t *testing.T, m map[string]any, key string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range m[key].([]any) {
		out = append(out, r.(map[string]any))
	}
	return out
}

func TestStatsCommandsListsEveryCommandInBothForms(t *testing.T) {
	dir := listingProject(t, "standard")
	m := statsJSON(t, "commands")
	require.Equal(t, logsDir(dir), m["log_dir"])
	require.Equal(t, float64(0), m["skipped_lines"])
	cmds := rows(t, m, "commands")
	// 1 orchestrator command, 1 standalone command and 3 commands in each
	// of the four runs.
	require.Len(t, cmds, 14)
	order := cmds[0]
	require.Equal(t, "epic order", order["command"])
	require.Nil(t, order["run_id"])
	require.Nil(t, order["step"])
	require.Equal(t, []any{"epic", "order", "--data", `{"name":"e"}`}, order["args"])
	require.Equal(t, float64(0), order["exit_code"])
	require.Equal(t, float64(60000), order["duration_ms"])
	require.Equal(t, "spec file list", cmds[1]["command"])
	require.Equal(t, "spec new", cmds[2]["command"])
	require.Equal(t, statsSpecRun, cmds[2]["run_id"])
	require.Equal(t, "spec", cmds[2]["kind"])
	require.Equal(t, "000001_demo", cmds[2]["name"])

	resetRootCmd(t)
	pretty := stats(t, "commands")
	lines := strings.Split(strings.TrimSpace(pretty), "\n")
	require.Len(t, lines, 15)
	require.Equal(t, []string{"TIME", "RUN", "STEP", "COMMAND", "EXIT", "DURATION"}, strings.Fields(lines[0]))
	require.Equal(t, []string{"2026-10-25", "06:00:00", "-", "-", "epic", "order", "0", "1m00s"}, strings.Fields(lines[1]))
	require.Equal(t, []string{"2026-10-25", "08:00:00", "abcdef01", "-", "spec", "new", "0", "30.0s"}, strings.Fields(lines[3]))

	resetRootCmd(t)
	out := stats(t, "commands", "--format", "json", "--fields", `["commands"]`)
	var f map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &f))
	require.Equal(t, []string{"commands"}, dataKeys(f))
	require.Len(t, f["commands"], 14)
}

func TestStatsSpansListsHelpersInAndOutsideRuns(t *testing.T) {
	listingProject(t, "standard")
	m := statsJSON(t, "spans")
	spans := rows(t, m, "spans")
	// The two orchestrator spans, then one helper in each of four runs.
	require.Len(t, spans, 6)
	stopped, open := spans[0], spans[1]
	require.Equal(t, "plan 000001_demo", stopped["name"])
	require.Equal(t, "sub-agent", stopped["type"])
	require.Equal(t, statsOrchSpan, stopped["trace_id"])
	require.Nil(t, stopped["run_id"])
	require.Nil(t, stopped["workflow"])
	require.Equal(t, true, stopped["complete"])
	require.Equal(t, float64(25*60000), stopped["duration_ms"])
	require.Equal(t, "open", open["end_reason"])
	require.Equal(t, false, open["complete"])
	require.Equal(t, float64(0), open["duration_ms"])

	helper := spans[2]
	require.Equal(t, statsSpecRun, helper["run_id"])
	require.Equal(t, "spec", helper["kind"])
	require.Equal(t, "000001_demo", helper["workflow"])
	require.Equal(t, "research", helper["name"])
	require.Equal(t, "first", helper["step"])
	require.Equal(t, float64(180000), helper["duration_ms"])

	resetRootCmd(t)
	pretty := stats(t, "spans")
	require.Regexp(t, regexp.MustCompile(`(?m)^TIME\s+RUN\s+TYPE\s+NAME\s+STEP\s+DURATION\s+END$`), pretty)
	require.Regexp(t, regexp.MustCompile(`(?m)^2026-10-25 06:05:00\s+-\s+sub-agent\s+plan 000001_demo\s+-\s+25m00s\s+stopped$`), pretty)
	require.Regexp(t, regexp.MustCompile(`(?m)^2026-10-25 06:10:00\s+-\s+sub-agent\s+plan 000002_other\s+-\s+0\.0s\s+open\*$`), pretty)
	require.Regexp(t, regexp.MustCompile(`(?m)^2026-10-25 08:02:00\s+abcdef01\s+sub-agent\s+research\s+first\s+3m00s\s+stopped$`), pretty)

	resetRootCmd(t)
	out := stats(t, "spans", "--format", "json", "--fields", `["spans"]`)
	var f map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &f))
	require.Equal(t, []string{"spans"}, dataKeys(f))
}

func TestStatsListingsSelectionAndOutsideRuns(t *testing.T) {
	listingProject(t, "standard")
	cmds := rows(t, statsJSON(t, "commands", "--run", statsImplRun), "commands")
	require.Len(t, cmds, 3)
	for _, c := range cmds {
		require.Equal(t, statsImplRun, c["run_id"])
	}

	resetRootCmd(t)
	outside := rows(t, statsJSON(t, "commands", "--outside-runs"), "commands")
	require.Len(t, outside, 2)
	require.Equal(t, "epic order", outside[0]["command"])
	require.Equal(t, "spec file list", outside[1]["command"])

	resetRootCmd(t)
	spans := rows(t, statsJSON(t, "spans", "--kind", "plan"), "spans")
	require.Len(t, spans, 2)
	for _, s := range spans {
		require.Equal(t, "plan", s["kind"])
	}

	resetRootCmd(t)
	spans = rows(t, statsJSON(t, "spans", "--outside-runs"), "spans")
	require.Len(t, spans, 2)
	for _, s := range spans {
		require.Nil(t, s["run_id"])
	}
}

func TestStatsListingsRefusals(t *testing.T) {
	dir := listingProject(t, "standard")
	for _, sub := range []string{"commands", "spans"} {
		for _, tc := range []struct {
			name string
			args []string
			code string
		}{
			{"selection with outside runs", []string{"--outside-runs", "--kind", "plan"}, "stats_bad_selection"},
			{"run with outside runs", []string{"--outside-runs", "--run", statsPlanRun}, "stats_bad_selection"},
			{"unsupported format", []string{"--format", "xml"}, "stats_format_unsupported"},
			{"missing directory", []string{"--dir", filepath.Join(dir, "nope")}, "stats_log_dir_not_found"},
			{"bad kind", []string{"--kind", "epic"}, "stats_bad_selection"},
			{"unknown run id", []string{"--run", "ffffffff"}, "stats_run_not_found"},
			{"ambiguous run id", []string{"--run", "abcdef01"}, "stats_run_ambiguous"},
		} {
			t.Run(sub+" "+tc.name, func(t *testing.T) {
				resetRootCmd(t)
				er := runRefused(t, append([]string{"stats", sub}, tc.args...)...)
				require.Equal(t, tc.code, er.Code)
				require.NotEmpty(t, er.NextAction)
			})
		}
		resetRootCmd(t)
		er := runRefused(t, "stats", sub, "--outside-runs", "--name", "000001_demo")
		require.Equal(t, "use --outside-runs on its own, or drop it and select runs with --name, --kind, --agent or --run", er.NextAction)
		require.Contains(t, er.Message, "--outside-runs")
	}
}

func TestStatsListingsDirOnACopyGiveTheSameOutput(t *testing.T) {
	dir := listingProject(t, "standard")
	inProject := map[string]map[string]any{}
	for _, sub := range []string{"commands", "spans"} {
		resetRootCmd(t)
		inProject[sub] = statsJSON(t, sub)
	}
	copied := copyLogs(t, logsDir(dir))
	t.Chdir(t.TempDir()) // outside any project
	for _, sub := range []string{"commands", "spans"} {
		resetRootCmd(t)
		got := statsJSON(t, sub, "--dir", copied)
		require.Equal(t, copied, got["log_dir"])
		got["log_dir"] = inProject[sub]["log_dir"]
		require.Equal(t, inProject[sub], got, sub)
	}
}

func TestStatsListingsLeaveTheLogsByteIdentical(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := listingProject(t, "verbose")
	before := rawLogSnapshot(t, logsDir(dir))
	for _, args := range [][]string{
		{"commands"}, {"commands", "--format", "json"}, {"commands", "--outside-runs"},
		{"spans"}, {"spans", "--format", "json"}, {"spans", "--kind", "plan"},
	} {
		resetRootCmd(t)
		stats(t, args...)
	}
	require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "no file added, removed or changed")
}

// Token use (spec 000078). The token fixtures are a logs directory whose
// command records carry their arguments, as real ones do, and a Claude Code
// projects directory holding the session that drove the spec run.

// tokenPrices is a stats.prices table pricing opus only.
const tokenPrices = `stats:
  prices:
    claude-opus-5-5:
      input: 5
      output: 25
      cache_read: 0.5
      cache_write: 6.25
`

// withArgs gives every command record its process.command_args, the words
// of its name, as the logger records them.
func withArgs(rs []tracelog.Record) []tracelog.Record {
	for i := range rs {
		if rs[i].Record == tracelog.RecordSpan && rs[i].Attributes[tracelog.AttrSpanType] == tracelog.TypeCommand {
			rs[i].Attributes[tracelog.AttrCommandArgs] = strings.Fields(rs[i].Name)
		}
	}
	return rs
}

// seedTokenLogs writes the spec run (claude) and the implement run (codex)
// of seedLogs, with their command arguments, into the logs directory dir.
func seedTokenLogs(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []struct {
		fx  statsFixture
		end time.Duration
	}{
		{statsFixture{statsSpecRun, "spec", "000001_demo", "claude", statsAt(8, 0)}, 30 * time.Minute},
		{statsFixture{statsImplRun, "implement", "000001_demo", "codex", statsAt(10, 0)}, 60 * time.Minute},
	} {
		var b bytes.Buffer
		for _, r := range withArgs(f.fx.records(f.end, true)) {
			line, err := json.Marshal(r)
			require.NoError(t, err)
			b.Write(line)
			b.WriteByte('\n')
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, f.fx.fileName()), b.Bytes(), 0o644))
	}
}

// claudeLine is one Claude Code transcript record.
func claudeLine(t *testing.T, rec map[string]any) string {
	t.Helper()
	b, err := json.Marshal(rec)
	require.NoError(t, err)
	return string(b) + "\n"
}

func claudeTime(at time.Time) string { return at.UTC().Format("2006-01-02T15:04:05.000Z") }

// claudeResponse is an assistant record of opus with usage and content.
func claudeResponse(t *testing.T, id string, at time.Time, in, out, cr, cw int, content ...map[string]any) string {
	if content == nil {
		content = []map[string]any{{"type": "text", "text": "ok"}}
	}
	return claudeLine(t, map[string]any{
		"type": "assistant", "timestamp": claudeTime(at),
		"message": map[string]any{"id": id, "model": "claude-opus-5-5", "content": content,
			"usage": map[string]any{"input_tokens": in, "output_tokens": out, "cache_read_input_tokens": cr, "cache_creation_input_tokens": cw}},
	})
}

// claudeBash is an assistant record running command, and the user record
// carrying its result.
func claudeBash(t *testing.T, id string, from, to time.Time, command string) string {
	return claudeResponse(t, "msg_"+id, from, 0, 1, 0, 0, map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": command}}) +
		claudeLine(t, map[string]any{"type": "user", "timestamp": claudeTime(to),
			"message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": id, "content": "{}"}}}})
}

// seedSessions writes a Claude Code projects directory under dir holding the
// session that drove the spec run of seedTokenLogs: Bash calls around its
// `spec new` and two `spec goto` commands; requests before its first step,
// in "first" and in "second"; a helper launched in "first" whose brief
// names the run's research span, with its own transcript; and Claude Code's
// own totals, which exceed the transcripts by 5 input and 50 output tokens.
// Every file is dated after the run started. It returns the projects
// directory.
//
// Token sums: first 2,000 + the helper's 400 output tokens; second 300 (the
// Bash responses add one output token each, the second's to first, the
// third's to second, the first's outside); outside 100 + 1 + 50 overhead.
func seedSessions(t *testing.T, dir string) string {
	t.Helper()
	projects := filepath.Join(dir, "claude", "projects")
	proj := filepath.Join(projects, "-tmp-demo")
	sub := filepath.Join(proj, "s1", "subagents")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	at := func(m, s int) time.Time { return statsAt(8, m).Add(time.Duration(s) * time.Second) }

	main := claudeBash(t, "toolu_b1", at(0, 0).Add(-200*time.Millisecond), at(0, 31), `spektacular spec new --data '{"name":"000001_demo"}'`) +
		claudeResponse(t, "msg_1", at(0, 45), 1, 100, 1000, 10) +
		claudeResponse(t, "msg_2", at(2, 0), 0, 0, 0, 0, map[string]any{"type": "tool_use", "id": "toolu_a1", "name": "Agent",
			"input": map[string]any{"description": "Research", "prompt": "Look around. Your parent span is " + statsSpecRun + "-abcd0000000000a1."}}) +
		claudeResponse(t, "msg_3", at(5, 0), 2, 2000, 5000, 20) +
		claudeBash(t, "toolu_b2", at(9, 0), at(10, 0), `spektacular spec goto --data '{"step":"second"}'`) +
		claudeResponse(t, "msg_4", at(15, 0), 3, 300, 3000, 30) +
		claudeBash(t, "toolu_b3", at(29, 0), at(30, 0), `spektacular spec goto --data '{"step":"finished"}'`) +
		claudeLine(t, map[string]any{"type": "cost-state", "modelUsage": map[string]any{"claude-opus-5-5": map[string]any{
			"inputTokens": 15, "outputTokens": 2853, "cacheReadInputTokens": 9400, "cacheCreationInputTokens": 100}}})
	helper := claudeResponse(t, "msg_h1", at(3, 0), 4, 400, 400, 40)
	files := map[string]string{
		filepath.Join(proj, "s1.jsonl"):                main,
		filepath.Join(sub, "agent-h1.jsonl"):           helper,
		filepath.Join(sub, "agent-h1.meta.json"):       `{"agentType":"Explore","description":"Research","toolUseId":"toolu_a1","spawnDepth":1}`,
		filepath.Join(proj, "unrelated-session.jsonl"): claudeResponse(t, "msg_x", at(40, 0), 9, 9, 9, 9),
	}
	for path, body := range files {
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		require.NoError(t, os.Chtimes(path, statsAt(9, 0), statsAt(9, 0)))
	}
	return projects
}

// tokenProject makes a project with the token logs and sessions, with
// config body extra (such as tokenPrices), and changes into it. It returns
// the project and the sessions directory.
func tokenProject(t *testing.T, extra string) (dir, sessions string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: standard\n"+extra)
	seedTokenLogs(t, logsDir(dir))
	resetRootCmd(t)
	return dir, seedSessions(t, t.TempDir())
}

// treeSnapshot maps every file under dir to its bytes.
func treeSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		snap[path] = string(readBytes(t, path))
		return nil
	}))
	return snap
}

// tokenFig is a token figure of a JSON report.
func tokenFig(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	require.True(t, ok, "%v is not a token figure", v)
	return m
}

func stepRow(t *testing.T, m map[string]any, step string) map[string]any {
	t.Helper()
	for _, r := range m["steps"].([]any) {
		if row := r.(map[string]any); row["step"] == step {
			return row
		}
	}
	t.Fatalf("no step %s", step)
	return nil
}

// withoutTokens is a JSON report with every token key removed.
func withoutTokens(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if k == "tokens" {
			continue
		}
		if list, ok := v.([]any); ok && (k == "steps" || k == "tasks" || k == "helpers") {
			rows := []any{}
			for _, r := range list {
				row := map[string]any{}
				for rk, rv := range r.(map[string]any) {
					if rk != "tokens" {
						row[rk] = rv
					}
				}
				rows = append(rows, row)
			}
			v = rows
		}
		out[k] = v
	}
	return out
}

// Token criterion 1: a run driven by Claude Code, with no change to how the
// agent works, gets token, cost and time-per-token figures in both forms.
func TestStatsRunGivesTokenUseForAClaudeRun(t *testing.T) {
	_, sessions := tokenProject(t, tokenPrices)
	m := statsJSON(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)

	tk := m["tokens"].(map[string]any)
	require.Equal(t, "known", tk["status"])
	require.Nil(t, tk["reason_code"])
	sess := tk["sessions"].([]any)
	require.Len(t, sess, 1)
	require.Equal(t, "s1", sess[0].(map[string]any)["session_id"])
	require.Equal(t, "main", sess[0].(map[string]any)["transcript"])

	total := tokenFig(t, tk["total"])
	require.Equal(t, float64(15), total["input"])
	require.Equal(t, float64(2853), total["output"])
	require.Equal(t, float64(9400), total["cache_read"])
	require.Equal(t, float64(100), total["cache_write"])
	// (15*5 + 2853*25 + 9400*0.5 + 100*6.25) / 1e6
	require.Equal(t, 0.0767250, total["cost"])
	require.Equal(t, true, total["cost_complete"])
	require.Equal(t, []any{}, total["unpriced_models"])
	overhead := tokenFig(t, tk["session_overhead"])
	require.Equal(t, float64(5), overhead["input"])
	require.Equal(t, float64(50), overhead["output"])

	first := tokenFig(t, stepRow(t, m, "first")["tokens"])
	require.Equal(t, float64(2000+400+1), first["output"], "the step's requests and its helper")
	// 9m of working time over 2,401 output tokens.
	require.Equal(t, float64(224906), first["working_ms_per_1k_output"])
	require.Equal(t, float64(301), tokenFig(t, stepRow(t, m, "second")["tokens"])["output"])
	require.Equal(t, float64(151), tokenFig(t, tk["outside_steps"])["output"])
	require.Nil(t, tokenFig(t, stepRow(t, m, "finished")["tokens"])["working_ms_per_1k_output"], "no output tokens: not applicable")

	helper := m["helpers"].([]any)[0].(map[string]any)
	require.Equal(t, "research", helper["name"])
	ht := tokenFig(t, helper["tokens"])
	require.Equal(t, float64(400), ht["output"])
	require.Equal(t, float64(450000), ht["working_ms_per_1k_output"], "3m over 400 output tokens")

	models := tk["models"].([]any)
	require.Len(t, models, 1)
	require.Equal(t, "claude-opus-5-5", models[0].(map[string]any)["model"])

	resetRootCmd(t)
	pretty := stats(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	require.Regexp(t, `(?m)^first\s+1\s+9m00s\s+.*\s2,401\s+0\.06\s+3m44s$`, pretty)
	require.Regexp(t, `(?m)^research\s+first\s+.*stopped\s+400\s+0\.01\s+7m30s$`, pretty)
	require.Contains(t, pretty, "\nTokens\nRead from session s1 (main)\n")
	require.Regexp(t, `(?m)^TOTAL\s+15\s+2,853\s+9,400\s+100\s+0\.08$`, pretty)
	require.Regexp(t, `(?m)^claude-opus-5-5\s+15\s+2,853\s+9,400\s+100\s+0\.08$`, pretty)
}

// Token criterion 2: costs use the project's price table; with no table,
// or outside a project, every cost is unknown and the table says why.
func TestStatsRunCostsComeFromTheProjectsPrices(t *testing.T) {
	dir, sessions := tokenProject(t, "")
	m := statsJSON(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	total := tokenFig(t, m["tokens"].(map[string]any)["total"])
	require.Equal(t, float64(2853), total["output"])
	require.Nil(t, total["cost"])
	require.Equal(t, false, total["cost_complete"])
	require.Equal(t, []any{"claude-opus-5-5"}, total["unpriced_models"])

	resetRootCmd(t)
	pretty := stats(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	require.Regexp(t, `(?m)^first\s+1\s+9m00s\s+.*\s2,401\s+\?\s+3m44s$`, pretty)
	require.Contains(t, pretty, "incomplete, no price for claude-opus-5-5 (prices per million tokens go under stats.prices in .spektacular/config.yaml)")

	// Outside a project, reading the same logs with --dir.
	t.Chdir(t.TempDir())
	resetRootCmd(t)
	m = statsJSON(t, "run", "--dir", logsDir(dir), "--run", statsSpecRun, "--sessions-dir", sessions)
	total = tokenFig(t, m["tokens"].(map[string]any)["total"])
	require.Equal(t, float64(2853), total["output"])
	require.Nil(t, total["cost"])
}

// Token criterion 3: a run driven by another agent has unknown token use,
// with the reason, in both forms.
func TestStatsRunOtherAgentIsUnknown(t *testing.T) {
	_, sessions := tokenProject(t, tokenPrices)
	m := statsJSON(t, "run", "--run", statsImplRun, "--sessions-dir", sessions)
	tk := m["tokens"].(map[string]any)
	require.Equal(t, "unknown", tk["status"])
	require.Equal(t, "agent_unsupported", tk["reason_code"])
	require.Contains(t, tk["reason"], "codex")
	require.Nil(t, tk["total"])
	require.Nil(t, stepRow(t, m, "first")["tokens"])

	resetRootCmd(t)
	pretty := stats(t, "run", "--run", statsImplRun, "--sessions-dir", sessions)
	require.Contains(t, pretty, "\nTokens\nunknown (agent_unsupported): token use is read only for Claude Code sessions; this run's agent is codex\n")
}

// Token criterion 4: with the agent's session removed, token use is
// unknown with the reason, and every timing equals the report with tokens.
func TestStatsRunWithTheSessionRemovedIsUnknownWithTheSameTimings(t *testing.T) {
	_, sessions := tokenProject(t, tokenPrices)
	known := statsJSON(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	require.NoError(t, os.Remove(filepath.Join(sessions, "-tmp-demo", "s1.jsonl")))

	resetRootCmd(t)
	gone := statsJSON(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	tk := gone["tokens"].(map[string]any)
	require.Equal(t, "unknown", tk["status"])
	require.Equal(t, "session_not_found", tk["reason_code"])
	require.Contains(t, tk["reason"], sessions)
	require.Equal(t, withoutTokens(known), withoutTokens(gone))
}

// Token criterion 5: no log file and no agent session file changes.
func TestStatsRunLeavesTheLogsAndSessionsByteIdentical(t *testing.T) {
	dir, sessions := tokenProject(t, tokenPrices)
	logsBefore, sessionsBefore := rawLogSnapshot(t, logsDir(dir)), treeSnapshot(t, sessions)
	for _, args := range [][]string{
		{"run", "--run", statsSpecRun, "--sessions-dir", sessions},
		{"run", "--run", statsSpecRun, "--sessions-dir", sessions, "--format", "json"},
		{"run", "--sessions-dir", sessions},
	} {
		resetRootCmd(t)
		stats(t, args...)
	}
	require.Equal(t, logsBefore, rawLogSnapshot(t, logsDir(dir)))
	require.Equal(t, sessionsBefore, treeSnapshot(t, sessions))
}

// Token criterion 6: the cmd tests never read the developer's own Claude
// Code sessions: without --sessions-dir a report reads the empty directory
// TestMain sets, and the flag does not outlive the command it was given to.
func TestStatsRunNeverReadsTheDevelopersSessions(t *testing.T) {
	_, sessions := tokenProject(t, tokenPrices)
	require.NotEmpty(t, os.Getenv("CLAUDE_CONFIG_DIR"))
	home, err := os.UserHomeDir()
	if err == nil {
		require.NotEqual(t, filepath.Join(home, ".claude"), os.Getenv("CLAUDE_CONFIG_DIR"))
	}

	m := statsJSON(t, "run", "--run", statsSpecRun, "--sessions-dir", sessions)
	require.Equal(t, "known", m["tokens"].(map[string]any)["status"])

	resetRootCmd(t)
	m = statsJSON(t, "run", "--run", statsSpecRun)
	tk := m["tokens"].(map[string]any)
	require.Equal(t, "unknown", tk["status"])
	require.Equal(t, "sessions_dir_missing", tk["reason_code"], "the flag was reset with the command tree")
	require.Contains(t, tk["reason"], filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "projects"))
}

func TestStatsRunSchemaListsTokens(t *testing.T) {
	t.Chdir(t.TempDir())
	resetRootCmd(t)
	out, _, code := runRootCmd(t, "stats", "run", "--schema")
	require.Equal(t, 0, code)
	require.Contains(t, out, `"tokens"`)
	require.Contains(t, out, `"sessions-dir"`)
}
