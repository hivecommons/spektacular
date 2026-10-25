package runstats

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite golden files")

// str is a pointer to s.
func str(s string) *string { return &s }

// reportOf assembles records into their one run and reports it.
func reportOf(t *testing.T, o Options, rs []tracelog.Record) Report {
	t.Helper()
	runs := Assemble(rs)
	require.Len(t, runs, 1)
	return BuildReport(runs[0], o)
}

func TestDefaultOptions(t *testing.T) {
	require.Equal(t, Options{Gaps: 10, GapThreshold: 5 * time.Minute}, DefaultOptions)
}

// knownWaits is a 20-minute run with user waits of 3 and 2 minutes and one
// 4-minute helper, none overlapping.
func knownWaits() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000003_known", agent: "claude"}
	return recs(
		a.cmd("spec new", "", at(0), at(1)),
		a.span("s1", "interview", tracelog.TypeStep, "", at(1), at(20), tracelog.EndMoved),
		a.span("u1", "question", tracelog.TypeUserWait, "s1", at(2), at(5), tracelog.EndStopped),
		a.span("h1", "research", tracelog.TypeSubAgent, "s1", at(6), at(10), tracelog.EndStopped),
		a.span("u2", "approval", tracelog.TypeUserWait, "s1", at(12), at(14), tracelog.EndStopped),
		a.cmd("spec goto", "s1", at(19.5), at(20)),
	)
}

func TestReportKnownWaits(t *testing.T) {
	rep := reportOf(t, DefaultOptions, knownWaits())
	require.Equal(t, Totals{
		WallMS:       1200000,
		WorkingMS:    900000, // 20 minutes less 5 minutes of user waits
		UserWaitMS:   300000,
		HelperWaitMS: 240000,
		CommandMS:    90000,
		Commands:     2,
		Helpers:      1,
		UserWaits:    2,
		// The gaps are 1, 3, 1, 4, 2, 2 and 5.5 minutes: only the last is
		// over 5 minutes.
		GapMS:             330000,
		GapsOverThreshold: 1,
	}, rep.Totals)
	require.Equal(t, rep.Totals.WallMS-rep.Totals.UserWaitMS, rep.Totals.WorkingMS)
	require.Len(t, rep.Steps, 1)
	require.EqualValues(t, 240000, rep.Steps[0].HelperWaitMS)
	require.EqualValues(t, 300000, rep.Steps[0].UserWaitMS)
}

// knownWaitsOverlappingHelpers has two helpers, 10:00 to 10:04 and 10:02 to
// 10:06, running at the same time.
func knownWaitsOverlappingHelpers() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000003_known", agent: "claude"}
	return recs(
		a.cmd("spec new", "", at(59), at(59.5)),
		a.span("h1", "research", tracelog.TypeSubAgent, "", at(60), at(64), tracelog.EndStopped),
		a.span("h2", "review", tracelog.TypeSubAgent, "", at(62), at(66), tracelog.EndStopped),
		a.cmd("spec status", "", at(70), at(71)),
	)
}

func TestReportOverlappingHelpersCountOnce(t *testing.T) {
	rep := reportOf(t, DefaultOptions, knownWaitsOverlappingHelpers())
	require.EqualValues(t, 360000, rep.Totals.HelperWaitMS, "6 minutes, not 8")
	require.EqualValues(t, 720000, rep.Totals.WallMS)
	require.EqualValues(t, 720000, rep.Totals.WorkingMS, "helper waits are not taken from working time")
	require.Zero(t, rep.Totals.UserWaitMS)
	require.Equal(t, 2, rep.Totals.Helpers)
	require.Len(t, rep.Helpers, 2)
	for _, h := range rep.Helpers {
		require.EqualValues(t, 240000, h.DurationMS, "each helper is listed with its own duration")
	}
}

// stepTwice enters interview twice, with three commands each time, and
// approach between.
func stepTwice() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000004_twice", agent: "claude"}
	return recs(
		a.cmd("spec new", "", at(0), at(1)),
		a.span("s1", "interview", tracelog.TypeStep, "", at(1), at(10), tracelog.EndMoved),
		a.cmd("spec status", "s1", at(2), at(2.5)),
		a.span("u1", "question", tracelog.TypeUserWait, "s1", at(3), at(4), tracelog.EndStopped),
		a.cmd("spec status", "s1", at(5), at(5.5)),
		a.cmd("spec goto", "s1", at(9), at(10)),
		a.span("s2", "approach", tracelog.TypeStep, "", at(10), at(15), tracelog.EndMoved),
		a.cmd("spec goto", "s2", at(14), at(15)),
		a.span("s3", "interview", tracelog.TypeStep, "", at(15), at(25), tracelog.EndMoved),
		a.cmd("spec status", "s3", at(16), at(16.25)),
		a.span("h1", "research", tracelog.TypeSubAgent, "s3", at(17), at(19), tracelog.EndStopped),
		a.cmd("spec status", "s3", at(20), at(20.25)),
		a.cmd("spec goto", "s3", at(24), at(25)),
	)
}

func TestReportStepEnteredTwice(t *testing.T) {
	rep := reportOf(t, DefaultOptions, stepTwice())
	require.Equal(t, []StepRow{
		{Step: "interview", Entries: 2, DurationMS: 1140000, Commands: 6, CommandMS: 210000, HelperWaitMS: 120000, UserWaitMS: 60000},
		{Step: "approach", Entries: 1, DurationMS: 300000, Commands: 1, CommandMS: 60000},
	}, rep.Steps, "one row per step, in order of first entry")
	require.Equal(t, []StepEntry{
		{Step: "interview", SpanID: "s1", StartedAt: at(1), EndedAt: at(10), DurationMS: 540000, EndReason: tracelog.EndMoved, Complete: true},
		{Step: "approach", SpanID: "s2", StartedAt: at(10), EndedAt: at(15), DurationMS: 300000, EndReason: tracelog.EndMoved, Complete: true},
		{Step: "interview", SpanID: "s3", StartedAt: at(15), EndedAt: at(25), DurationMS: 600000, EndReason: tracelog.EndMoved, Complete: true},
	}, rep.StepEntries)
}

func TestReportStepWaitsAreClippedToTheStep(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	rep := reportOf(t, DefaultOptions, recs(
		a.span("s1", "interview", tracelog.TypeStep, "", at(1), at(10), tracelog.EndMoved),
		a.span("h1", "research", tracelog.TypeSubAgent, "s1", at(8), at(13), tracelog.EndStopped),
		a.span("s2", "approach", tracelog.TypeStep, "", at(10), at(20), tracelog.EndMoved),
	))
	require.EqualValues(t, 120000, rep.Steps[0].HelperWaitMS)
	require.EqualValues(t, 180000, rep.Steps[1].HelperWaitMS)
	require.EqualValues(t, 300000, rep.Totals.HelperWaitMS)
}

// twoTasks is an implement run of a two-task plan, with analyze and verify
// steps under each task, a user wait in task 1 and a helper in task 2.
func twoTasks() []tracelog.Record {
	a := fx{trace: traceA, kind: "implement", name: "000005_tasks", agent: "claude"}
	task1 := map[string]any{tracelog.AttrTaskID: "task-1"}
	task2 := map[string]any{tracelog.AttrTaskID: "task-2"}
	return recs(
		a.cmd("implement new", "", at(0), at(1)),
		a.span("t1", "Trace log package", tracelog.TypeTask, "", at(1), at(11), tracelog.EndMoved, task1),
		a.span("a1", "analyze", tracelog.TypeStep, "t1", at(1), at(5), tracelog.EndMoved),
		a.cmd("implement goto", "a1", at(4.5), at(5)),
		a.span("b1", "verify", tracelog.TypeStep, "t1", at(5), at(11), tracelog.EndMoved),
		a.span("u1", "approval", tracelog.TypeUserWait, "b1", at(6), at(8), tracelog.EndStopped),
		a.cmd("implement goto", "b1", at(10.5), at(11)),
		a.span("t2", "Report builder", tracelog.TypeTask, "", at(11), at(30), tracelog.EndMoved, task2),
		a.span("a2", "analyze", tracelog.TypeStep, "t2", at(11), at(20), tracelog.EndMoved),
		a.span("h1", "research", tracelog.TypeSubAgent, "a2", at(12), at(17), tracelog.EndStopped),
		a.cmd("implement goto", "a2", at(19.5), at(20)),
		a.span("b2", "verify", tracelog.TypeStep, "t2", at(20), at(30), tracelog.EndMoved),
		a.cmd("implement goto", "b2", at(29.5), at(30)),
	)
}

func TestReportTwoTasks(t *testing.T) {
	rep := reportOf(t, DefaultOptions, twoTasks())
	require.Equal(t, []TaskRow{
		{TaskID: "task-1", Name: "Trace log package", SpanID: "t1", StartedAt: at(1), EndedAt: at(11),
			DurationMS: 600000, HelperWaitMS: 0, UserWaitMS: 120000, EndReason: tracelog.EndMoved, Complete: true},
		{TaskID: "task-2", Name: "Report builder", SpanID: "t2", StartedAt: at(11), EndedAt: at(30),
			DurationMS: 1140000, HelperWaitMS: 300000, UserWaitMS: 0, EndReason: tracelog.EndMoved, Complete: true},
	}, rep.Tasks)

	var entryTasks []string
	for _, e := range rep.StepEntries {
		require.NotNil(t, e.TaskID, e.SpanID)
		entryTasks = append(entryTasks, e.SpanID+"="+*e.TaskID)
	}
	require.Equal(t, []string{"a1=task-1", "b1=task-1", "a2=task-2", "b2=task-2"}, entryTasks)

	require.Equal(t, []StepRow{
		{Step: "analyze", Entries: 2, DurationMS: 780000, Commands: 2, CommandMS: 60000, HelperWaitMS: 300000},
		{Step: "verify", Entries: 2, DurationMS: 960000, Commands: 2, CommandMS: 60000, UserWaitMS: 120000},
	}, rep.Steps)

	require.Len(t, rep.Helpers, 1)
	require.Equal(t, str("task-2"), rep.Helpers[0].TaskID)
	require.Equal(t, str("analyze"), rep.Helpers[0].Step)
	require.Len(t, rep.UserWaits, 1)
	require.Equal(t, str("task-1"), rep.UserWaits[0].TaskID)
}

// threeHelpers launches three helpers: one named by its step attribute
// before the step's span starts, one under a step, one under another step.
func threeHelpers() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000006_helpers", agent: "claude"}
	return recs(
		a.cmd("spec new", "", at(0), at(1)),
		a.span("h1", "research", tracelog.TypeSubAgent, "", at(1), at(2), tracelog.EndStopped,
			map[string]any{tracelog.AttrStep: "interview"}),
		a.span("s1", "interview", tracelog.TypeStep, "", at(2), at(20), tracelog.EndMoved),
		a.span("h2", "review", tracelog.TypeSubAgent, "s1", at(5), at(8), tracelog.EndStopped),
		a.cmd("spec goto", "s1", at(19.5), at(20)),
		a.span("s2", "approach", tracelog.TypeStep, "", at(20), at(30), tracelog.EndMoved),
		a.span("h3", "draft", tracelog.TypeSubAgent, "s2", at(21), at(22.5), tracelog.EndStopped),
		a.cmd("spec goto", "s2", at(29.5), at(30)),
	)
}

func TestReportThreeHelpers(t *testing.T) {
	rep := reportOf(t, DefaultOptions, threeHelpers())
	type row struct {
		name, step string
		ms         int64
	}
	var got []row
	for _, h := range rep.Helpers {
		require.NotNil(t, h.Step, h.Name)
		require.True(t, h.Complete, h.Name)
		got = append(got, row{h.Name, *h.Step, h.DurationMS})
	}
	require.Equal(t, []row{
		{"research", "interview", 60000},
		{"review", "interview", 180000},
		{"draft", "approach", 90000},
	}, got)
	require.Equal(t, 3, rep.Totals.Helpers)
	require.EqualValues(t, 330000, rep.Totals.HelperWaitMS)
	require.Nil(t, rep.Helpers[0].ParentSpanID)
	require.Equal(t, str("s1"), rep.Helpers[1].ParentSpanID)
	require.Empty(t, rep.UserWaits)
}

// interrupted cuts interview off at its last command; under it a helper is
// never closed and a user wait is abandoned on resume.
func interrupted() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000008_cut", agent: "claude"}
	s1 := a.open("s1", "interview", tracelog.TypeStep, "", at(1))
	u1 := a.open("u1", "question", tracelog.TypeUserWait, "s1", at(4))
	return recs(
		a.cmd("spec new", "", at(0), at(1)),
		s1,
		a.cmd("spec status", "s1", at(2), at(3)),
		a.open("h1", "research", tracelog.TypeSubAgent, "s1", at(3)),
		u1,
		a.cmd("spec status", "s1", at(5), at(6)),
		closeSpan(s1, at(6), tracelog.EndInterrupted),
		closeSpan(u1, at(30), tracelog.EndAbandoned),
		a.span("s1b", "interview", tracelog.TypeStep, "", at(30), at(40), tracelog.EndMoved),
		a.cmd("spec goto", "s1b", at(39), at(40)),
	)
}

func TestReportInterrupted(t *testing.T) {
	rep := reportOf(t, DefaultOptions, interrupted())
	require.Equal(t, Totals{
		WallMS:       2400000,
		WorkingMS:    2400000,
		UserWaitMS:   0, // the abandoned wait is left out
		HelperWaitMS: 0, // the open helper is left out
		CommandMS:    240000,
		Commands:     4,
		Helpers:      1,
		UserWaits:    1,
		// Gaps of 1, 1, 1, 24 and 9 minutes.
		GapMS:             1980000,
		GapsOverThreshold: 2,
	}, rep.Totals)

	require.Equal(t, []StepRow{
		{Step: "interview", Entries: 2, DurationMS: 900000, Commands: 3, CommandMS: 180000, Incomplete: 1},
	}, rep.Steps, "the cut-off entry counts up to its last command, and neither unfinished wait is in the step's waits")
	require.Equal(t, []StepEntry{
		{Step: "interview", SpanID: "s1", StartedAt: at(1), EndedAt: at(6), DurationMS: 300000, EndReason: tracelog.EndInterrupted, Complete: false},
		{Step: "interview", SpanID: "s1b", StartedAt: at(30), EndedAt: at(40), DurationMS: 600000, EndReason: tracelog.EndMoved, Complete: true},
	}, rep.StepEntries)

	require.Equal(t, []Wait{{
		SpanID: "h1", ParentSpanID: str("s1"), Name: "research", Step: str("interview"),
		StartedAt: at(3), EndedAt: at(40), DurationMS: 2220000, EndReason: EndOpen, Complete: false,
	}}, rep.Helpers, "an open helper runs to the run's last event")
	require.Equal(t, []Wait{{
		SpanID: "u1", ParentSpanID: str("s1"), Name: "question", Step: str("interview"),
		StartedAt: at(4), EndedAt: at(30), DurationMS: 1560000, EndReason: tracelog.EndAbandoned,
		Status: str(tracelog.StatusError), Message: str(tracelog.EndAbandoned), Complete: false,
	}}, rep.UserWaits, "an abandoned wait runs to where it was closed")
}

// investigationRow is one discovery step with three commands, a helper and
// a user wait: the investigation's per-step figures.
func investigationRow() []tracelog.Record {
	a := fx{trace: traceA, kind: "plan", name: "000009_investigation", agent: "claude"}
	return recs(
		a.cmd("plan new", "", at(0), at(0.5)),
		a.span("s1", "discovery", tracelog.TypeStep, "", at(0.5), at(10.5), tracelog.EndMoved),
		a.cmd("plan status", "s1", at(1), at(1.5)),
		a.span("h1", "research", tracelog.TypeSubAgent, "s1", at(2), at(4), tracelog.EndStopped),
		a.cmd("plan status", "s1", at(4), at(4.25)),
		a.span("u1", "question", tracelog.TypeUserWait, "s1", at(5), at(8), tracelog.EndStopped),
		a.cmd("plan goto", "s1", at(10), at(10.5)),
	)
}

func TestReportInvestigationRow(t *testing.T) {
	rep := reportOf(t, DefaultOptions, investigationRow())
	require.Equal(t, []StepRow{
		{Step: "discovery", Entries: 1, DurationMS: 600000, Commands: 3, CommandMS: 75000, HelperWaitMS: 120000, UserWaitMS: 180000},
	}, rep.Steps)

	// The same figures, read from the structured form.
	data, err := json.Marshal(rep)
	require.NoError(t, err)
	var doc struct {
		Steps []map[string]any `json:"steps"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Len(t, doc.Steps, 1)
	row := doc.Steps[0]
	require.Equal(t, "discovery", row["step"])
	require.Equal(t, float64(600000), row["duration_ms"])
	require.Equal(t, float64(75000), row["command_ms"])
	require.Equal(t, float64(120000), row["helper_wait_ms"])
	require.Equal(t, float64(180000), row["user_wait_ms"])
}

// goldenRun is an implement run that fills every report field: a step
// outside any task, two tasks, a helper and a user wait (one each with no
// parent), an interrupted step, an open helper, and gaps both inside and
// outside a step.
func goldenRun() []tracelog.Record {
	a := fx{trace: "9a1b2c3d000000000000000000000075", kind: "implement", name: "000075_run-stats-command", agent: "claude"}
	task1 := map[string]any{tracelog.AttrTaskID: "df1bade8-0000-4000-8000-000000000001"}
	task2 := map[string]any{tracelog.AttrTaskID: "df1bade8-0000-4000-8000-000000000002"}
	return recs(
		a.cmd("implement new", "", at(0), at(0.5)),
		a.span("p0", "setup", tracelog.TypeStep, "", at(1), at(2), tracelog.EndMoved),
		a.cmd("implement goto", "p0", at(1.5), at(2)),
		a.span("t1", "Trace log package", tracelog.TypeTask, "", at(2), at(20), tracelog.EndMoved, task1),
		a.span("a1", "analyze", tracelog.TypeStep, "t1", at(2), at(8), tracelog.EndMoved),
		a.span("h1", "research", tracelog.TypeSubAgent, "a1", at(3), at(6), tracelog.EndStopped),
		a.cmd("implement goto", "a1", at(7.5), at(8)),
		a.span("b1", "verify", tracelog.TypeStep, "t1", at(8), at(20), tracelog.EndMoved),
		a.cmd("implement status", "b1", at(9), at(9.5)),
		a.span("u1", "approval", tracelog.TypeUserWait, "", at(10), at(12), tracelog.EndStopped,
			map[string]any{tracelog.AttrStep: "verify"}),
		a.cmd("implement goto", "b1", at(19.5), at(20)),
		a.span("t2", "Report builder", tracelog.TypeTask, "", at(20), at(30), tracelog.EndMoved, task2),
		a.span("a2", "analyze", tracelog.TypeStep, "t2", at(20), at(30), tracelog.EndInterrupted),
		a.open("h2", "review", tracelog.TypeSubAgent, "a2", at(22)),
		a.cmd("implement status", "a2", at(29.5), at(30)),
		a.span("f", "finished", tracelog.TypeStep, "", at(30), at(30), tracelog.EndFinished),
	)
}

func TestReportGoldenJSON(t *testing.T) {
	rep := reportOf(t, DefaultOptions, goldenRun())
	got, err := json.MarshalIndent(rep, "", "  ")
	require.NoError(t, err)
	got = append(got, '\n')

	path := filepath.Join("testdata", "report.golden.json")
	if *update {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "run go test -run TestReportGoldenJSON -update to write it")
	require.Equal(t, string(want), string(got))
}

// keys is m's keys, sorted.
func keys(t *testing.T, v any) []string {
	t.Helper()
	m, ok := v.(map[string]any)
	require.True(t, ok, "%v is not an object", v)
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sorted is ks, sorted.
func sorted(ks ...string) []string {
	sort.Strings(ks)
	return ks
}

// first is the first element of the array v.
func first(t *testing.T, v any) map[string]any {
	t.Helper()
	a, ok := v.([]any)
	require.True(t, ok, "%v is not an array", v)
	require.NotEmpty(t, a)
	m, ok := a[0].(map[string]any)
	require.True(t, ok)
	return m
}

// TestReportJSONFieldNamesAndUnits pins the report's JSON to the contract in
// the Data Structures section of plan 000075_run-stats-command (reproduced
// in Report's doc comment): the key sets below are written by hand from it.
func TestReportJSONFieldNamesAndUnits(t *testing.T) {
	data, err := json.Marshal(reportOf(t, DefaultOptions, goldenRun()))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(data, &doc))

	require.Equal(t, sorted("gap_threshold_ms", "run", "totals", "steps", "step_entries", "tasks", "helpers", "user_waits", "gaps"), keys(t, doc))
	require.Equal(t, sorted("run_id", "kind", "name", "agent", "log_file", "started_at", "ended_at", "wall_ms", "status"), keys(t, doc["run"]))
	require.Equal(t, sorted("wall_ms", "working_ms", "user_wait_ms", "helper_wait_ms", "command_ms", "commands", "helpers", "user_waits", "gap_ms", "gaps_over_threshold"), keys(t, doc["totals"]))
	require.Equal(t, sorted("step", "entries", "duration_ms", "commands", "command_ms", "helper_wait_ms", "user_wait_ms", "incomplete"), keys(t, first(t, doc["steps"])))
	require.Equal(t, sorted("step", "span_id", "task_id", "started_at", "ended_at", "duration_ms", "end_reason", "complete"), keys(t, first(t, doc["step_entries"])))
	require.Equal(t, sorted("task_id", "name", "span_id", "started_at", "ended_at", "duration_ms", "helper_wait_ms", "user_wait_ms", "end_reason", "complete"), keys(t, first(t, doc["tasks"])))
	waitKeys := sorted("span_id", "parent_span_id", "name", "step", "task_id", "started_at", "ended_at", "duration_ms", "end_reason", "status", "message", "complete")
	require.Equal(t, waitKeys, keys(t, first(t, doc["helpers"])))
	require.Equal(t, waitKeys, keys(t, first(t, doc["user_waits"])))
	gap := first(t, doc["gaps"])
	require.Equal(t, sorted("started_at", "ended_at", "duration_ms", "step", "after", "before"), keys(t, gap))
	require.Equal(t, sorted("event", "type", "name"), keys(t, gap["after"]))
	require.Equal(t, sorted("event", "type", "name"), keys(t, gap["before"]))

	// Units: the threshold and every duration are integer milliseconds,
	// every time RFC 3339 in UTC.
	require.Equal(t, float64(300000), doc["gap_threshold_ms"])
	run := doc["run"].(map[string]any)
	require.Equal(t, float64(1800000), run["wall_ms"])
	require.Equal(t, "2026-10-25T09:00:00Z", run["started_at"])
	require.Equal(t, "2026-10-25T09:30:00Z", run["ended_at"])
	entry := first(t, doc["step_entries"])
	require.Equal(t, float64(60000), entry["duration_ms"])
	require.Equal(t, "2026-10-25T09:01:00Z", entry["started_at"])
	for _, section := range []string{"step_entries", "tasks", "helpers", "user_waits", "gaps"} {
		for _, item := range doc[section].([]any) {
			m := item.(map[string]any)
			d, ok := m["duration_ms"].(float64)
			require.True(t, ok, "%s duration_ms is a number", section)
			require.Equal(t, float64(int64(d)), d, "%s duration_ms is whole", section)
			for _, k := range []string{"started_at", "ended_at"} {
				s, ok := m[k].(string)
				require.True(t, ok)
				require.True(t, strings.HasSuffix(s, "Z"), "%s %s is UTC: %s", section, k, s)
				_, err := time.Parse(time.RFC3339, s)
				require.NoError(t, err)
			}
		}
	}

	// Values that do not apply are null.
	require.Contains(t, entry, "task_id")
	require.Nil(t, entry["task_id"], "the setup step is outside any task")
	userWait := first(t, doc["user_waits"])
	require.Nil(t, userWait["parent_span_id"])
	require.Nil(t, userWait["message"])
	var openHelper map[string]any
	for _, h := range doc["helpers"].([]any) {
		if m := h.(map[string]any); m["span_id"] == "h2" {
			openHelper = m
		}
	}
	require.NotNil(t, openHelper)
	require.Equal(t, EndOpen, openHelper["end_reason"])
	require.Equal(t, false, openHelper["complete"])
	require.Nil(t, openHelper["status"])
	require.Nil(t, openHelper["message"])
	nullStep := false
	for _, g := range doc["gaps"].([]any) {
		m := g.(map[string]any)
		if v, ok := m["step"]; ok && v == nil {
			nullStep = true
		}
	}
	require.True(t, nullStep, "the gap before the first step falls in no step")
}
