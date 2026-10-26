package runview

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/runstats"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files")

// t0 is the fixed moment every fixture is built around. No test reads the
// wall clock.
var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// at is t0 plus m minutes.
func at(m float64) time.Time { return t0.Add(time.Duration(m * float64(time.Minute))) }

const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"

// fx builds the records of one implement run.
type fx struct{ trace, kind, name, agent string }

var run1 = fx{trace: traceID, kind: "implement", name: "000077_html-log-viewer", agent: "claude"}

func (f fx) attrs(typ string, extra map[string]any) map[string]any {
	a := map[string]any{
		tracelog.AttrSpanType:     typ,
		tracelog.AttrAgent:        f.agent,
		tracelog.AttrWorkflowKind: f.kind,
		tracelog.AttrWorkflowName: f.name,
	}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

// cmd is a command record under parent, run in step, exiting with code.
func (f fx) cmd(id, name, parent, step string, start, end time.Time, code int, extra ...map[string]any) tracelog.Record {
	x := map[string]any{
		tracelog.AttrStep:        step,
		tracelog.AttrCommandArgs: append([]string{}, strings.Fields(name)...),
		tracelog.AttrExitCode:    code,
	}
	for _, e := range extra {
		for k, v := range e {
			x[k] = v
		}
	}
	status := tracelog.Status{Code: tracelog.StatusOK}
	if code != 0 {
		status = tracelog.Status{Code: tracelog.StatusError, Message: "exit status"}
	}
	return tracelog.Record{
		Record: tracelog.RecordSpan, TraceID: f.trace, SpanID: id, ParentSpanID: parent,
		Name: name, Kind: tracelog.KindInternal, StartTime: start, EndTime: end,
		Status: status, Attributes: f.attrs(tracelog.TypeCommand, x),
	}
}

// open is a span_start record.
func (f fx) open(id, name, typ, parent string, start time.Time, extra map[string]any) tracelog.Record {
	return tracelog.Record{
		Record: tracelog.RecordSpanStart, TraceID: f.trace, SpanID: id, ParentSpanID: parent,
		Name: name, Kind: tracelog.KindInternal, StartTime: start, Attributes: f.attrs(typ, extra),
	}
}

// closeSpan is the span_end record closing start at end for reason.
func closeSpan(start tracelog.Record, end time.Time, reason string) tracelog.Record {
	r := start
	r.Record = tracelog.RecordSpanEnd
	r.EndTime = end
	r.Status = tracelog.ReasonStatus(reason)
	r.Attributes = map[string]any{}
	for k, v := range start.Attributes {
		r.Attributes[k] = v
	}
	r.Attributes[tracelog.AttrEndReason] = reason
	return r
}

// span is a span's start and end records.
func (f fx) span(id, name, typ, parent string, start, end time.Time, reason string, extra map[string]any) []tracelog.Record {
	s := f.open(id, name, typ, parent, start, extra)
	return []tracelog.Record{s, closeSpan(s, end, reason)}
}

func recs(parts ...any) []tracelog.Record {
	var out []tracelog.Record
	for _, p := range parts {
		switch v := p.(type) {
		case tracelog.Record:
			out = append(out, v)
		case []tracelog.Record:
			out = append(out, v...)
		default:
			panic("recs: unexpected part")
		}
	}
	return out
}

// assemble builds the one run in rs.
func assemble(t *testing.T, rs []tracelog.Record) *runstats.Run {
	t.Helper()
	runs := runstats.Assemble(rs)
	require.Len(t, runs, 1)
	return runs[0]
}

func step(name string) map[string]any { return map[string]any{tracelog.AttrStep: name} }

// treeRecords is a task holding two steps, each with commands, a helper
// holding a nested work span, a wait on the user, and the instant terminal
// step.
func treeRecords() []tracelog.Record {
	f := run1
	task := map[string]any{tracelog.AttrTaskID: "9d0f8c1e"}
	analyze := map[string]any{tracelog.AttrStep: "analyze", tracelog.AttrTaskID: "9d0f8c1e"}
	impl := map[string]any{tracelog.AttrStep: "implement", tracelog.AttrTaskID: "9d0f8c1e"}
	return recs(
		f.span("task1", "Build the page data", tracelog.TypeTask, "", at(0), at(30), tracelog.EndMoved, task),
		f.span("step1", "analyze", tracelog.TypeStep, "task1", at(0), at(10), tracelog.EndMoved, analyze),
		f.cmd("cmd1", "implement goto", "step1", "analyze", at(1), at(1).Add(200*time.Millisecond), 0),
		f.span("agent1", "research", tracelog.TypeSubAgent, "step1", at(2), at(8), tracelog.EndStopped, step("analyze")),
		f.span("work1", "scan files", tracelog.TypeWork, "agent1", at(3), at(4), tracelog.EndStopped, step("analyze")),
		f.span("step2", "implement", tracelog.TypeStep, "task1", at(10), at(30), tracelog.EndMoved, impl),
		f.span("wait1", "which approach?", tracelog.TypeUserWait, "step2", at(12), at(15), tracelog.EndStopped, step("implement")),
		f.cmd("cmd2", "implement goto", "step2", "implement", at(29), at(29).Add(100*time.Millisecond), 0),
		f.span("step3", "finished", tracelog.TypeStep, "", at(30), at(30), tracelog.EndFinished, step("finished")),
	)
}

func build(t *testing.T, rs []tracelog.Record, threshold time.Duration) (Model, *runstats.Run) {
	t.Helper()
	r := assemble(t, rs)
	return Build(r, Options{GapThreshold: threshold}), r
}

// rowOf is the row index of the span with id.
func rowOf(t *testing.T, m Model, id string) int {
	t.Helper()
	for i, s := range m.Rows.SpanID {
		if s == id {
			return i
		}
	}
	t.Fatalf("no row for span %q", id)
	return -1
}

// str is the string at index i, or "" for -1.
func str(m Model, i int) string {
	if i < 0 {
		return ""
	}
	return m.Strings[i]
}

func TestBuildOneRowPerSpanInTreeOrder(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)

	require.Equal(t,
		[]string{"task1", "step1", "cmd1", "agent1", "work1", "step2", "wait1", "cmd2", "step3"},
		m.Rows.SpanID, "one row per span and command, each parent before its children, siblings by start")
	require.Equal(t, []int{0, 1, 2, 2, 3, 1, 2, 2, 0}, m.Rows.Depth)
	require.Equal(t, []int{-1, 0, 1, 1, 3, 0, 5, 5, -1}, m.Rows.Parent)
	require.Equal(t,
		[]int64{0, 0, 60_000, 120_000, 180_000, 600_000, 720_000, 1_740_000, 1_800_000},
		m.Rows.StartMS, "each row starts at its recorded start, from the run's start")
	require.Equal(t,
		[]int64{1_800_000, 600_000, 200, 360_000, 60_000, 1_200_000, 180_000, 100, 0},
		m.Rows.DurationMS, "each row runs to its recorded end; the terminal step is instant")

	typ := func(id string) string { return m.Types[m.Rows.Type[rowOf(t, m, id)]] }
	require.Equal(t, tracelog.TypeTask, typ("task1"))
	require.Equal(t, tracelog.TypeStep, typ("step1"))
	require.Equal(t, tracelog.TypeCommand, typ("cmd1"))
	require.Equal(t, tracelog.TypeSubAgent, typ("agent1"))
	require.Equal(t, tracelog.TypeWork, typ("work1"))
	require.Equal(t, tracelog.TypeUserWait, typ("wait1"))
}

func TestBuildTypesAreFixed(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)
	require.Equal(t, []string{"command", "step", "task", "sub-agent", "user-wait", "work"}, m.Types)
	require.Equal(t, []string{"", "failed", "interrupted", "abandoned", "open"}, m.Problems)
	require.Equal(t, 1, m.Version)
}

func TestBuildSiblingsAtTheSameMomentPutContainersFirst(t *testing.T) {
	f := run1
	rs := recs(
		f.span("s1", "discovery", tracelog.TypeStep, "", at(0), at(5), tracelog.EndMoved, step("discovery")),
		f.cmd("c0", "spec new", "", "", at(0), at(0).Add(time.Second), 0),
		f.span("t1", "task", tracelog.TypeTask, "", at(0), at(5), tracelog.EndMoved, nil),
	)
	m, _ := build(t, rs, 5*time.Minute)
	require.Equal(t, []string{"t1", "s1", "c0"}, m.Rows.SpanID)
}

func TestBuildOrphanParentGoesToTheTopLevel(t *testing.T) {
	f := run1
	rs := recs(
		f.span("s1", "discovery", tracelog.TypeStep, "", at(0), at(10), tracelog.EndMoved, step("discovery")),
		f.span("a1", "lost helper", tracelog.TypeSubAgent, "not-in-this-run", at(2), at(4), tracelog.EndStopped, step("discovery")),
	)
	m, _ := build(t, rs, 5*time.Minute)
	i := rowOf(t, m, "a1")
	require.Equal(t, -1, m.Rows.Parent[i])
	require.Equal(t, 0, m.Rows.Depth[i])
}

func TestBuildParentCycleStillGivesEveryRowOnce(t *testing.T) {
	f := run1
	rs := recs(
		f.span("s1", "discovery", tracelog.TypeStep, "", at(0), at(10), tracelog.EndMoved, step("discovery")),
		f.span("a", "a", tracelog.TypeWork, "b", at(1), at(2), tracelog.EndStopped, nil),
		f.span("b", "b", tracelog.TypeWork, "a", at(3), at(4), tracelog.EndStopped, nil),
	)
	m, _ := build(t, rs, 5*time.Minute)
	ids := append([]string{}, m.Rows.SpanID...)
	sort.Strings(ids)
	require.Equal(t, []string{"a", "b", "s1"}, ids)
	require.Equal(t, -1, m.Rows.Parent[rowOf(t, m, "a")], "the first of a cycle is placed at the top level")
}

func TestBuildUnfinishedSpansEndWhereTheyWereCutOff(t *testing.T) {
	f := run1
	rs := recs(
		f.span("s1", "implement", tracelog.TypeStep, "", at(0), at(10), tracelog.EndMoved, step("implement")),
		// Never closed: it runs to the run's last event.
		f.open("open1", "still running", tracelog.TypeSubAgent, "s1", at(5), step("implement")),
		// Closed a day later by a restart's sweep: capped at the run's end.
		f.span("ab1", "abandoned work", tracelog.TypeWork, "s1", at(2), at(24*60), tracelog.EndAbandoned, step("implement")),
		f.cmd("c1", "implement goto", "s1", "implement", at(9), at(10), 0),
	)
	m, r := build(t, rs, 5*time.Minute)

	open := rowOf(t, m, "open1")
	require.Equal(t, int64(300_000), m.Rows.StartMS[open])
	require.Equal(t, int64(300_000), m.Rows.DurationMS[open], "an open span ends at the run's last event")
	ab := rowOf(t, m, "ab1")
	require.Equal(t, int64(120_000), m.Rows.StartMS[ab])
	require.Equal(t, int64(480_000), m.Rows.DurationMS[ab], "an abandoned span is capped at the run's end, not its recorded end")

	for _, s := range r.Spans {
		i := rowOf(t, m, s.ID)
		require.Equal(t, s.End.Sub(s.Start).Milliseconds(), m.Rows.DurationMS[i], "row %s uses the span's effective end", s.ID)
	}
}

func TestBuildProblemFlags(t *testing.T) {
	f := run1
	failed := f.open("failedHelper", "broken helper", tracelog.TypeSubAgent, "s1", at(1), step("implement"))
	failedEnd := closeSpan(failed, at(2), tracelog.EndStopped)
	failedEnd.Status = tracelog.Status{Code: tracelog.StatusError, Message: "helper crashed"}
	interrupted := f.open("s2", "verify", tracelog.TypeStep, "", at(10), step("verify"))
	rs := recs(
		f.span("s1", "implement", tracelog.TypeStep, "", at(0), at(10), tracelog.EndMoved, step("implement")),
		failed, failedEnd,
		f.span("okHelper", "fine helper", tracelog.TypeSubAgent, "s1", at(3), at(4), tracelog.EndStopped, step("implement")),
		f.span("ab1", "abandoned", tracelog.TypeWork, "s1", at(5), at(6), tracelog.EndAbandoned, nil),
		f.open("open1", "never closed", tracelog.TypeWork, "s1", at(7), nil),
		f.cmd("bad", "implement goto", "s1", "implement", at(8), at(8).Add(time.Second), 2),
		f.cmd("good", "implement goto", "s1", "implement", at(9), at(9).Add(time.Second), 0),
		interrupted, closeSpan(interrupted, at(12), tracelog.EndInterrupted),
		f.span("w", "finished work", tracelog.TypeWork, "s2", at(11), at(11.5), tracelog.EndStopped, nil),
	)
	m, _ := build(t, rs, 5*time.Minute)
	problem := func(id string) string { return m.Problems[m.Rows.Problem[rowOf(t, m, id)]] }

	require.Equal(t, "failed", problem("failedHelper"), "a helper stopped with an error is failed")
	require.Equal(t, "failed", problem("bad"), "a command with a non-zero exit is failed")
	require.Equal(t, "interrupted", problem("s2"))
	require.Equal(t, "abandoned", problem("ab1"))
	require.Equal(t, "open", problem("open1"))
	for _, id := range []string{"s1", "okHelper", "good", "w"} {
		require.Equal(t, "", problem(id), "finished work %s is unmarked", id)
	}

	require.Equal(t, StatusError, m.Rows.Status[rowOf(t, m, "failedHelper")])
	require.Equal(t, "helper crashed", str(m, m.Rows.Message[rowOf(t, m, "failedHelper")]))
	require.Equal(t, StatusNone, m.Rows.Status[rowOf(t, m, "open1")], "an open span has no status")
	require.Equal(t, StatusOK, m.Rows.Status[rowOf(t, m, "okHelper")])
	require.Equal(t, 2, *m.Rows.ExitCode[rowOf(t, m, "bad")])
}

func TestBuildHelperAndCommandDetails(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)

	i := rowOf(t, m, "agent1")
	require.Equal(t, "research", str(m, m.Rows.Name[i]))
	require.Equal(t, "sub-agent", m.Types[m.Rows.Type[i]])
	require.Equal(t, int64(120_000), m.Rows.StartMS[i])
	require.Equal(t, int64(360_000), m.Rows.DurationMS[i])
	require.Equal(t, StatusOK, m.Rows.Status[i])
	require.Equal(t, "stopped", str(m, m.Rows.EndReason[i]))
	require.Equal(t, "analyze", str(m, m.Rows.Step[i]))
	require.Nil(t, m.Rows.ExitCode[i], "a span has no exit code")
	require.Equal(t, -1, m.Rows.Args[i])

	require.Equal(t, "9d0f8c1e", str(m, m.Rows.TaskID[rowOf(t, m, "step1")]))

	require.Equal(t, RunInfo{
		RunID: traceID, Kind: "implement", Name: "000077_html-log-viewer", Agent: "claude",
		Status: runstats.StatusFinished, StartedAt: at(0), EndedAt: at(30),
	}, m.Run, "the run carries its agent and workflow, and no path")

	c := rowOf(t, m, "cmd1")
	require.Equal(t, "implement goto", str(m, m.Rows.Name[c]))
	require.Equal(t, "implement goto", str(m, m.Rows.Args[c]))
	require.Equal(t, 0, *m.Rows.ExitCode[c])
	require.Equal(t, -1, m.Rows.EndReason[c])
}

func TestBuildCutsLongCommandLines(t *testing.T) {
	f := run1
	long := strings.Repeat("é", 1500) // 3000 bytes
	rs := recs(
		f.span("s1", "implement", tracelog.TypeStep, "", at(0), at(1), tracelog.EndMoved, step("implement")),
		f.cmd("c1", "implement goto", "s1", "implement", at(0.5), at(0.6), 0,
			map[string]any{tracelog.AttrCommandArgs: []any{"implement", "goto", "--data", long}}),
	)
	m, _ := build(t, rs, 5*time.Minute)
	args := str(m, m.Rows.Args[rowOf(t, m, "c1")])
	require.True(t, strings.HasPrefix(args, "implement goto --data é"))
	require.True(t, strings.HasSuffix(args, "…"))
	require.LessOrEqual(t, len(args), 2000+len("…"))
	require.True(t, strings.ToValidUTF8(args, "?") == args, "the cut never splits a character")
}

func TestBuildLeavesOutVerboseAttributes(t *testing.T) {
	f := run1
	rs := recs(
		f.span("s1", "implement", tracelog.TypeStep, "", at(0), at(1), tracelog.EndMoved, step("implement")),
		f.cmd("c1", "implement goto", "s1", "implement", at(0.5), at(0.6), 0, map[string]any{
			tracelog.AttrResponse:    "FULL-COMMAND-OUTPUT-XYZ",
			tracelog.AttrStateBefore: map[string]any{"snapshot": "STATE-BEFORE-XYZ"},
			tracelog.AttrStateAfter:  "STATE-AFTER-XYZ",
		}),
	)
	m, _ := build(t, rs, 5*time.Minute)
	data, err := json.Marshal(m)
	require.NoError(t, err)
	for _, s := range []string{tracelog.AttrResponse, tracelog.AttrStateBefore, tracelog.AttrStateAfter,
		"FULL-COMMAND-OUTPUT-XYZ", "STATE-BEFORE-XYZ", "STATE-AFTER-XYZ"} {
		require.NotContains(t, string(data), s)
	}
}

func TestBuildTotalsAreTheReports(t *testing.T) {
	m, r := build(t, treeRecords(), 5*time.Minute)
	require.Equal(t, Totals{WallMS: 1_800_000, WorkingMS: 1_620_000, UserWaitMS: 180_000, HelperWaitMS: 360_000}, m.Totals)

	rep := runstats.BuildReport(r, runstats.DefaultOptions)
	require.Equal(t, Totals{
		WallMS: rep.Totals.WallMS, WorkingMS: rep.Totals.WorkingMS,
		UserWaitMS: rep.Totals.UserWaitMS, HelperWaitMS: rep.Totals.HelperWaitMS,
	}, m.Totals)
}

// pausedRecords is one step whose commands are separated by a 20-minute, a
// 2-minute and a 500 ms pause.
func pausedRecords() []tracelog.Record {
	f := run1
	sec := func(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }
	return recs(
		f.span("s1", "discovery", tracelog.TypeStep, "", sec(0), sec(1324), tracelog.EndMoved, step("discovery")),
		f.cmd("c1", "spec new", "s1", "discovery", sec(0), sec(1), 0),
		f.cmd("c2", "spec goto", "s1", "discovery", sec(1201), sec(1202), 0),
		f.cmd("c3", "spec file read", "s1", "discovery", sec(1322), sec(1323), 0),
		f.cmd("c4", "spec goto", "s1", "discovery", sec(1323.5), sec(1324), 0),
	)
}

// over counts the gaps longer than threshold, by the report's rule.
func over(m Model, threshold time.Duration) int {
	n := 0
	for _, d := range m.Gaps.DurationMS {
		if d > threshold.Milliseconds() {
			n++
		}
	}
	return n
}

func TestBuildGaps(t *testing.T) {
	m, _ := build(t, pausedRecords(), 5*time.Minute)

	require.Equal(t, int64(300_000), m.GapThresholdMS, "the default threshold is the one passed in")
	require.Equal(t, int64(1_000), m.GapFloorMS)
	require.Equal(t, []int64{1_000, 1_202_000}, m.Gaps.StartMS, "every gap of at least a second, in time order")
	require.Equal(t, []int64{1_200_000, 120_000}, m.Gaps.DurationMS, "the 500 ms pause is under the floor")
	for i := range m.Gaps.Step {
		require.Equal(t, "discovery", str(m, m.Gaps.Step[i]))
	}
	require.Equal(t, "end of command spec new", str(m, m.Gaps.After[0]))
	require.Equal(t, "start of command spec goto", str(m, m.Gaps.Before[0]))

	require.Equal(t, 1, over(m, 5*time.Minute), "only the 20-minute pause is over the 5-minute default")
	require.Equal(t, 2, over(m, time.Minute), "both pauses are over a 1-minute threshold")

	m1, r := build(t, pausedRecords(), time.Minute)
	require.Equal(t, int64(60_000), m1.GapThresholdMS)
	rep := runstats.BuildReport(r, runstats.Options{GapThreshold: time.Minute})
	require.Equal(t, rep.Totals.GapsOverThreshold, over(m1, time.Minute), "the page's rule agrees with the report")

	m2, _ := build(t, pausedRecords(), 500*time.Millisecond)
	require.Equal(t, int64(500), m2.GapFloorMS, "a threshold under a second lowers the floor")
	require.Len(t, m2.Gaps.DurationMS, 3)
}

func TestGapFloor(t *testing.T) {
	require.Equal(t, time.Second, GapFloor(5*time.Minute))
	require.Equal(t, 200*time.Millisecond, GapFloor(200*time.Millisecond))
}

func TestDefaultFileName(t *testing.T) {
	require.Equal(t, "spektacular-run-4bf92f35.html", DefaultFileName(traceID))
	require.Equal(t, "spektacular-run-abc.html", DefaultFileName("abc"))
}

func TestBuildLongestSteps(t *testing.T) {
	f := run1
	mins := []float64{3, 1, 6, 2, 5, 4}
	var parts []any
	start := 0.0
	for i, d := range mins {
		id := string(rune('a' + i))
		parts = append(parts, f.span(id, "step-"+id, tracelog.TypeStep, "", at(start), at(start+d), tracelog.EndMoved, nil))
		start += d
	}
	m, _ := build(t, recs(parts...), 5*time.Minute)
	var got []string
	for _, i := range m.LongestSteps {
		got = append(got, m.Rows.SpanID[i])
	}
	require.Equal(t, []string{"c", "e", "f", "a", "d"}, got, "the five longest step entries, longest first")
}

func TestBuildIsDeterministic(t *testing.T) {
	a, _ := build(t, treeRecords(), 5*time.Minute)
	b, _ := build(t, treeRecords(), 5*time.Minute)
	require.Equal(t, a, b)
	ja, err := json.Marshal(a)
	require.NoError(t, err)
	jb, err := json.Marshal(b)
	require.NoError(t, err)
	require.Equal(t, ja, jb)
}

func TestModelJSONFieldNames(t *testing.T) {
	m, _ := build(t, treeRecords(), 5*time.Minute)
	data, err := json.Marshal(m)
	require.NoError(t, err)
	var got map[string]map[string]any
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &top))
	require.ElementsMatch(t, []string{"version", "run", "totals", "gap_threshold_ms", "gap_floor_ms",
		"types", "problems", "strings", "rows", "gaps", "longest_steps"}, keys(top))
	got = map[string]map[string]any{}
	for _, k := range []string{"run", "totals", "rows", "gaps"} {
		var v map[string]any
		require.NoError(t, json.Unmarshal(top[k], &v))
		got[k] = v
	}
	require.ElementsMatch(t, []string{"run_id", "kind", "name", "agent", "status", "started_at", "ended_at", "skipped_lines"}, keys(got["run"]))
	require.ElementsMatch(t, []string{"wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms"}, keys(got["totals"]))
	require.ElementsMatch(t, []string{"parent", "depth", "start_ms", "duration_ms", "type", "name", "problem",
		"end_reason", "status", "message", "step", "task_id", "span_id", "args", "exit_code"}, keys(got["rows"]))
	require.ElementsMatch(t, []string{"start_ms", "duration_ms", "step", "after", "before"}, keys(got["gaps"]))
	for k, v := range got["rows"] {
		require.Len(t, v, len(m.Rows.SpanID), "rows column %s has one entry per row", k)
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestModelGolden(t *testing.T) {
	m, _ := build(t, append(treeRecords(), pausedGapRecord()...), 5*time.Minute)
	data, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	path := filepath.Join("testdata", "model.golden.json")
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test -run %s -update to write it", t.Name())
	require.Equal(t, string(want), string(data)+"\n")
}

// pausedGapRecord adds a command after a long pause past the tree's end, so
// the golden model carries a gap.
func pausedGapRecord() []tracelog.Record {
	return []tracelog.Record{run1.cmd("cmd3", "stats run", "", "", at(50), at(50).Add(time.Second), 0)}
}

func TestEmptyColumnsAreArraysNotNull(t *testing.T) {
	f := run1
	m, _ := build(t, recs(f.span("s1", "discovery", tracelog.TypeStep, "", at(0), at(0), tracelog.EndMoved, nil)), 5*time.Minute)
	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.NotContains(t, string(data), ":null", "no column or list is null")
}

// The page collapses a row by skipping the rows after it that are deeper:
// that is exactly its descendants only if every row's descendants follow it
// contiguously, each deeper than it.
func TestBuildDescendantsAreContiguousAndDeeper(t *testing.T) {
	for name, rs := range map[string][]tracelog.Record{
		"tree":      treeRecords(),
		"generated": genRun(600, 3),
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := build(t, rs, 5*time.Minute)
			rw := m.Rows
			n := len(rw.SpanID)
			for i := 0; i < n; i++ {
				// The rows after i that are deeper than it, up to the first
				// that is not.
				end := i + 1
				for end < n && rw.Depth[end] > rw.Depth[i] {
					end++
				}
				for j := 0; j < n; j++ {
					isDesc := false
					for p := rw.Parent[j]; p >= 0; p = rw.Parent[p] {
						if p == i {
							isDesc = true
							break
						}
					}
					require.Equal(t, isDesc, j > i && j < end, "row %d under row %d", j, i)
				}
				if rw.Parent[i] >= 0 {
					require.Equal(t, rw.Depth[rw.Parent[i]]+1, rw.Depth[i], "row %d is one deeper than its parent", i)
					require.Less(t, rw.Parent[i], i, "a parent comes before its child")
				}
			}
		})
	}
}
