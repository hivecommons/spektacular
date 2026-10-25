package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	require.Regexp(t, `(?m)^first\s+1\s+9m00s\s+1\s+1m00s\s+3m00s\s+0\.0s\s+0$`, pretty)
	require.Regexp(t, `(?m)^second\s+1\s+50m00s\s+1\s+1m00s\s+0\.0s\s+0\.0s\s+0$`, pretty)
	require.Regexp(t, `(?m)^research\s+first\s+2026-10-25 10:02:00\s+3m00s\s+stopped$`, pretty)
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
	require.Equal(t, "run one of: compare, list, run, summary", er.NextAction)

	t.Chdir(t.TempDir())
	resetRootCmd(t)
	er = runRefused(t, "stats", "list")
	require.Equal(t, "no_project", er.Code)
	require.Contains(t, er.NextAction, "--dir")
}

func TestStatsSchema(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, sub := range []string{"list", "run", "compare", "summary"} {
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
