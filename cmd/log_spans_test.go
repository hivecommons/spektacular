package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// These tests cover the step and task spans the workflow records in a run's
// log beside its command records: span_start when a step or task begins,
// span_end when it ends, with the reason it ended. Expected values are
// hand-written; the only time assertions are equalities and orderings
// between recorded times.

// specRunSteps is every step a spec run moves through after `new`, in order,
// up to and including the terminal step.
var specRunSteps = []string{
	"interview", "overview", "requirements", "acceptance_criteria", "constraints",
	"technical_approach", "success_metrics", "non_goals", "verification", "split", "finished",
}

// specSpanName is the spec the counter id method names the fixture spec.
const specSpanName = "000001_billing"

// stateStepSpan is the id of the open step span in .spektacular/state.json
// under dir, or "" when the state holds none.
func stateStepSpan(t *testing.T, dir string) string {
	t.Helper()
	sp, ok := readStateMap(t, dir)["step_span"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := sp["id"].(string)
	return id
}

// stateRunID is the run id in .spektacular/state.json under dir.
func stateRunID(t *testing.T, dir string) string {
	t.Helper()
	id, _ := readStateMap(t, dir)["run_id"].(string)
	require.Regexp(t, traceIDPattern, id)
	return id
}

// runLogFile returns the sole log file in logs whose records carry runID.
func runLogFile(t *testing.T, logs, runID string) string {
	t.Helper()
	var path string
	for _, f := range logFiles(t, logs) {
		recs := readSpanLog(t, f)
		if len(recs) > 0 && recs[0].TraceID == runID {
			require.Emptyf(t, path, "expected one log file for run %s", runID)
			path = f
		}
	}
	require.NotEmptyf(t, path, "no log file for run %s among %v", runID, logFiles(t, logs))
	return path
}

// spanType is a record's spektacular.span.type.
func spanType(r tracelog.Record) string {
	s, _ := r.Attributes[tracelog.AttrSpanType].(string)
	return s
}

// endReason is a span_end record's spektacular.span.end_reason.
func endReason(r tracelog.Record) string {
	s, _ := r.Attributes[tracelog.AttrEndReason].(string)
	return s
}

// commandRecords are the command records among recs.
func commandRecords(recs []tracelog.Record) []tracelog.Record {
	var out []tracelog.Record
	for _, r := range recs {
		if r.Record == tracelog.RecordSpan {
			out = append(out, r)
		}
	}
	return out
}

// loggedSpan is one span of a log: its span_start, the index of that line,
// and its span_end, nil while it is open.
type loggedSpan struct {
	start tracelog.Record
	at    int
	end   *tracelog.Record
}

// spansOfType pairs every span_start of type typ in recs with its span_end,
// in start order, requiring no span to end twice or end before it starts.
func spansOfType(t *testing.T, recs []tracelog.Record, typ string) []loggedSpan {
	t.Helper()
	var out []loggedSpan
	for i, r := range recs {
		if r.Record == tracelog.RecordSpanStart && spanType(r) == typ {
			out = append(out, loggedSpan{start: r, at: i})
		}
	}
	for i, r := range recs {
		if r.Record != tracelog.RecordSpanEnd || spanType(r) != typ {
			continue
		}
		matched := false
		for j := range out {
			if out[j].start.SpanID == r.SpanID {
				require.Nilf(t, out[j].end, "span %s (%s) ends twice", r.SpanID, r.Name)
				require.Greaterf(t, i, out[j].at, "span %s (%s) ends before it starts", r.SpanID, r.Name)
				end := r
				out[j].end = &end
				matched = true
			}
		}
		require.Truef(t, matched, "span_end %s (%s) has no span_start", r.SpanID, r.Name)
	}
	return out
}

// spanNames are the names of spans, in order.
func spanNames(spans []loggedSpan) []string {
	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.start.Name
	}
	return names
}

// requireEnded requires s to have ended for reason, with the status that
// reason gets.
func requireEnded(t *testing.T, s loggedSpan, reason string) {
	t.Helper()
	require.NotNilf(t, s.end, "span %s must be closed", s.start.Name)
	require.Equalf(t, reason, endReason(*s.end), "end reason of %s", s.start.Name)
	require.Equalf(t, tracelog.ReasonStatus(reason), s.end.Status, "status of %s", s.start.Name)
	require.Equal(t, s.start.StartTime, s.end.StartTime, "a span_end repeats its start")
	require.Equal(t, s.start.ParentSpanID, s.end.ParentSpanID)
	require.Equal(t, s.start.Name, s.end.Name)
}

// requireContiguous requires each step span to end exactly when the next one
// starts.
func requireContiguous(t *testing.T, spans []loggedSpan) {
	t.Helper()
	for i := 0; i+1 < len(spans); i++ {
		require.NotNilf(t, spans[i].end, "step %s must be closed", spans[i].start.Name)
		require.Truef(t, spans[i].end.EndTime.Equal(spans[i+1].start.StartTime),
			"step %s must end when %s starts", spans[i].start.Name, spans[i+1].start.Name)
	}
}

// startLoggedSpecRun founds the fixture spec run with logging at the
// configured level, in a fresh project under t's temp dir, and returns the
// project directory.
func startLoggedSpecRun(t *testing.T, extraConfig string) string {
	t.Helper()
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n"+extraConfig)
	resetRootCmd(t)
	runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	return dir
}

// specGoto moves the spec run to step, through runRoot with os.Args set.
func specGoto(t *testing.T, step string) string {
	t.Helper()
	return runDebugCmd(t, "spec", "goto", "--data", `{"step":"`+step+`"}`)
}

// Criterion 1: after new, interview and overview the log holds closed step
// spans for new and interview, each ending when the next began, and an open
// span for overview, the step state.json names.
func TestLogSpans_StepSpansFollowTheWorkflow(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	specGoto(t, "overview")

	runID := stateRunID(t, dir)
	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	steps := spansOfType(t, recs, tracelog.TypeStep)
	require.Equal(t, []string{"new", "interview", "overview"}, spanNames(steps))

	requireEnded(t, steps[0], tracelog.EndMoved)
	requireEnded(t, steps[1], tracelog.EndMoved)
	require.Nil(t, steps[2].end, "the current step's span is open")
	requireContiguous(t, steps)
	require.Equal(t, steps[2].start.SpanID, stateStepSpan(t, dir))

	seen := map[string]bool{}
	for _, s := range steps {
		require.False(t, seen[s.start.SpanID], "every step span has its own id")
		seen[s.start.SpanID] = true
		require.Equal(t, runID, s.start.TraceID)
		require.Empty(t, s.start.ParentSpanID, "a spec step belongs to the run")
		require.Equal(t, s.start.Name, s.start.Attributes[tracelog.AttrStep])
		require.Equal(t, "spec", s.start.Attributes[tracelog.AttrWorkflowKind])
		require.Equal(t, specSpanName, s.start.Attributes[tracelog.AttrWorkflowName])
	}
	require.Empty(t, spansOfType(t, recs, tracelog.TypeTask), "a spec run has no task spans")
}

// Criterion 2: every command recorded while the run is on a step, including
// rejected commands and commands that move nothing, names that step's span
// as its parent; the founding `spec new` names the span it opened.
func TestLogSpans_CommandsNameTheirStepSpanAsParent(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	runDebugCmd(t, "spec", "file", "read", specSpanName)
	_, code := runLogged(t, "spec", "goto", "--data", `{"step":"bogus"}`)
	require.Equal(t, 1, code)
	specGoto(t, "overview")
	specGoto(t, "requirements")
	runDebugCmd(t, "spec", "file", "read", specSpanName)

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), stateRunID(t, dir)))
	byStep := map[string]loggedSpan{}
	for _, s := range spansOfType(t, recs, tracelog.TypeStep) {
		byStep[s.start.Name] = s
	}
	cmds := commandRecords(recs)
	require.Equal(t, []string{
		"spec new", "spec goto", "spec file read", "spec goto", "spec goto", "spec goto", "spec file read",
	}, recordNames(cmds))
	wantStep := []string{"new", "new", "interview", "interview", "interview", "overview", "requirements"}

	for i, c := range cmds {
		parent, ok := byStep[wantStep[i]]
		require.Truef(t, ok, "no span for step %s", wantStep[i])
		require.Equalf(t, parent.start.SpanID, c.ParentSpanID, "command %d (%s) runs in step %s", i, c.Name, wantStep[i])
		if step, ok := c.Attributes[tracelog.AttrStep]; ok {
			require.Equal(t, wantStep[i], step)
		}
	}
	// The parent of every command is logged before the command is.
	for i, r := range recs {
		if r.Record != tracelog.RecordSpan {
			continue
		}
		found := false
		for _, s := range byStep {
			if s.start.SpanID == r.ParentSpanID {
				require.Lessf(t, s.at, i, "the span_start of %s precedes command %s", s.start.Name, r.Name)
				found = true
			}
		}
		require.True(t, found)
	}
}

// walkLoggedSpecToFinished founds the fixture spec run and walks it to
// finished, returning the project directory and the run id.
func walkLoggedSpecToFinished(t *testing.T) (dir, runID string) {
	t.Helper()
	dir = startLoggedSpecRun(t, "")
	runID = stateRunID(t, dir)
	for _, s := range specRunSteps {
		specGoto(t, s)
	}
	return dir, runID
}

// Criterion 3: once the workflow reaches finished no step span of the run is
// left open; finished itself is an instant span ended as finished.
func TestLogSpans_FinishedRunLeavesNoStepSpanOpen(t *testing.T) {
	dir, runID := walkLoggedSpecToFinished(t)
	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))

	steps := spansOfType(t, recs, tracelog.TypeStep)
	require.Equal(t, append([]string{"new"}, specRunSteps...), spanNames(steps))
	for _, s := range steps[:len(steps)-1] {
		requireEnded(t, s, tracelog.EndMoved)
	}
	fin := steps[len(steps)-1]
	requireEnded(t, fin, tracelog.EndFinished)
	require.True(t, fin.start.StartTime.Equal(fin.end.EndTime), "the terminal step's span is instant")
	require.Empty(t, tracelog.NewIndex(recs).OpenAll(), "no span of the run is left open")

	if _, err := os.Stat(filepath.Join(dir, ".spektacular", "state.json")); err == nil {
		require.Empty(t, stateStepSpan(t, dir), "a finished run keeps no open step span")
	}
}

// Success metric: the step spans of a whole spec run are contiguous, from
// the first step to finished: each step's span ends exactly when the next
// step's begins.
func TestLogSpans_SpecRunStepSpansAreContiguous(t *testing.T) {
	dir, runID := walkLoggedSpecToFinished(t)
	steps := spansOfType(t, readSpanLog(t, runLogFile(t, logsDir(dir), runID)), tracelog.TypeStep)
	require.Len(t, steps, 1+len(specRunSteps))
	require.Equal(t, "new", steps[0].start.Name)
	require.Equal(t, "finished", steps[len(steps)-1].start.Name)
	requireContiguous(t, steps)
	for i := 0; i+1 < len(steps); i++ {
		require.False(t, steps[i].start.StartTime.After(steps[i].end.EndTime))
	}
}

// Criterion 4: `spec new --force` while the run is mid-step closes that
// run's open step span in the old run's log, marked abandoned, and leaves
// nothing of the old run open; the new run opens its own span in its own
// file.
func TestLogSpans_ForceRestartAbandonsTheOpenStep(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	oldRun := stateRunID(t, dir)
	openSpan := stateStepSpan(t, dir)
	require.Regexp(t, spanIDPattern, openSpan)

	runDebugCmd(t, "spec", "new", "--force", "--data", `{"name":"billing"}`)
	newRun := stateRunID(t, dir)
	require.NotEqual(t, oldRun, newRun)

	old := readSpanLog(t, runLogFile(t, logsDir(dir), oldRun))
	steps := spansOfType(t, old, tracelog.TypeStep)
	require.Equal(t, []string{"new", "interview"}, spanNames(steps))
	require.Equal(t, openSpan, steps[1].start.SpanID)
	requireEnded(t, steps[0], tracelog.EndMoved)
	requireEnded(t, steps[1], tracelog.EndAbandoned)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusError, Message: "abandoned"}, steps[1].end.Status)
	require.Equal(t, oldRun, steps[1].end.TraceID)
	require.Empty(t, tracelog.NewIndex(old).OpenAll(), "nothing of the abandoned run is left open")

	fresh := readSpanLog(t, runLogFile(t, logsDir(dir), newRun))
	newSteps := spansOfType(t, fresh, tracelog.TypeStep)
	require.Equal(t, []string{"new"}, spanNames(newSteps))
	require.Nil(t, newSteps[0].end)
	require.Equal(t, newSteps[0].start.SpanID, stateStepSpan(t, dir))
	require.NotEqual(t, openSpan, newSteps[0].start.SpanID)
}

// Criterion 5: resuming a run whose session stopped mid-step, with a helper
// span still open, closes the interrupted step at the end of its last real
// command (not the resume probe), marked interrupted, closes the helper the
// same way, and opens a new span for the resumed step; the run's progress is
// untouched.
func TestLogSpans_ResumeClosesTheInterruptedStep(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	specGoto(t, "overview")

	state := readStateMap(t, dir)
	runID := stateRunID(t, dir)
	stepSpan := stateStepSpan(t, dir)
	require.Regexp(t, spanIDPattern, stepSpan)
	createdAt, err := time.Parse(time.RFC3339Nano, state["created_at"].(string))
	require.NoError(t, err)
	completedBefore := state["completed_steps"]

	// A helper agent's span, started by the stopped session and never
	// stopped.
	logger, err := tracelog.New("file", tracelog.FileOptions{Directory: logsDir(dir)})
	require.NoError(t, err)
	helper := tracelog.Spans{
		Log: logger,
		Run: tracelog.Run{TraceID: runID, Kind: "spec", Name: specSpanName, StartedAt: createdAt, Agent: "unknown"},
		Base: map[string]any{
			tracelog.AttrAgent:        "unknown",
			tracelog.AttrWorkflowKind: "spec",
			tracelog.AttrWorkflowName: specSpanName,
		},
		Resource: tracelog.Resource(version),
	}.Start("research helper", tracelog.TypeSubAgent, stepSpan, time.Now(), nil)

	// The stopped session's last real command, then the resume: the probe
	// and the re-render of the current step.
	runDebugCmd(t, "spec", "file", "read", specSpanName)
	probe, _ := runLogged(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, "workflow_in_progress", errorEnvelope(t, probe).Code)
	specGoto(t, "overview")

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	cmds := commandRecords(recs)
	require.Equal(t, []string{"spec new", "spec goto", "spec goto", "spec file read", "spec new", "spec goto"}, recordNames(cmds))
	lastWork, probeRec := cmds[3], cmds[4]
	require.Equal(t, stepSpan, lastWork.ParentSpanID)
	require.Equal(t, stepSpan, probeRec.ParentSpanID)
	require.True(t, probeRec.EndTime.After(lastWork.EndTime), "the probe ran after the last real command")

	steps := spansOfType(t, recs, tracelog.TypeStep)
	require.Equal(t, []string{"new", "interview", "overview", "overview"}, spanNames(steps))
	interrupted, resumed := steps[2], steps[3]
	require.Equal(t, stepSpan, interrupted.start.SpanID)
	requireEnded(t, interrupted, tracelog.EndInterrupted)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusError, Message: "interrupted"}, interrupted.end.Status)
	require.True(t, interrupted.end.EndTime.Equal(lastWork.EndTime), "the interrupted step ends with its last real command")
	require.False(t, interrupted.end.EndTime.Equal(probeRec.EndTime))

	helpers := spansOfType(t, recs, tracelog.TypeSubAgent)
	require.Len(t, helpers, 1)
	require.Equal(t, helper.SpanID, helpers[0].start.SpanID)
	require.Equal(t, stepSpan, helpers[0].start.ParentSpanID)
	requireEnded(t, helpers[0], tracelog.EndInterrupted)
	require.True(t, helpers[0].end.EndTime.Equal(lastWork.EndTime), "the helper ends with the step")

	require.NotEqual(t, stepSpan, resumed.start.SpanID)
	require.Nil(t, resumed.end, "the resumed step's span is open")
	require.Greater(t, resumed.at, interrupted.at)
	require.Empty(t, resumed.start.ParentSpanID)
	require.Equal(t, resumed.start.SpanID, stateStepSpan(t, dir))

	after := readStateMap(t, dir)
	require.Equal(t, "overview", after["current_step"])
	require.Equal(t, completedBefore, after["completed_steps"])
	require.Equal(t, runID, after["run_id"])
}

// implementTaskConfig is an implement run in the shared slot with the goal
// check off, so verify goes straight to update_plan.
const implementTaskConfig = "implement:\n  worktrees: false\n  goal_check:\n    enabled: false\n"

// writePlanDocLogged writes body as the plan document of taskPlanName
// through `plan file write`, with os.Args set as an agent's would be.
func writePlanDocLogged(t *testing.T, body string) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(src, []byte(body), 0o644))
	runDebugCmd(t, "plan", "file", "write", taskPlanName, "plan", "--from", src)
}

// Criterion 6: implementing a plan of two tasks leaves one task span per
// task, named after the task and carrying its id, holding the step spans of
// that task's work; the steps outside the tasks belong to the run, and no
// trace command was run to get there.
func TestLogSpans_TaskSpansHoldEachTasksSteps(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, implementTaskConfig)
	resetRootCmd(t)
	writePlanDocLogged(t, twoTaskPlan(false, false))

	implGoto := func(step string) {
		t.Helper()
		runDebugCmd(t, "implement", "goto", "--data", `{"step":"`+step+`"}`)
	}
	// implement new enters new and moves straight on to read_plan.
	runDebugCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	runID := stateRunID(t, dir)
	require.Equal(t, "read_plan", readStateMap(t, dir)["current_step"])
	work := []string{"analyze", "implement", "test", "verify", "update_plan"}
	for _, s := range work {
		implGoto(s)
	}
	writePlanDocLogged(t, twoTaskPlan(true, false))
	implGoto("update_changelog")
	for _, s := range work {
		implGoto(s)
	}
	writePlanDocLogged(t, twoTaskPlan(true, true))
	implGoto("update_changelog")
	implGoto("test_plan")

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	tasks := spansOfType(t, recs, tracelog.TypeTask)
	require.Equal(t, []string{"Task A", "Task B"}, spanNames(tasks))
	require.Equal(t, idA, tasks[0].start.Attributes[tracelog.AttrTaskID])
	require.Equal(t, idB, tasks[1].start.Attributes[tracelog.AttrTaskID])
	for _, task := range tasks {
		require.Empty(t, task.start.ParentSpanID, "a task span belongs to the run")
		require.Equal(t, "analyze", task.start.Attributes[tracelog.AttrStep], "analyze starts the task")
		requireEnded(t, task, tracelog.EndMoved)
	}

	taskSteps := append(append([]string{}, work...), "update_changelog")
	steps := spansOfType(t, recs, tracelog.TypeStep)
	wantNames := []string{"new", "read_plan"}
	wantNames = append(wantNames, taskSteps...)
	wantNames = append(wantNames, taskSteps...)
	wantNames = append(wantNames, "test_plan")
	require.Equal(t, wantNames, spanNames(steps))
	requireContiguous(t, steps)

	n := len(taskSteps)
	for i, s := range steps {
		want := ""
		switch {
		case i >= 2 && i < 2+n:
			want = tasks[0].start.SpanID
		case i >= 2+n && i < 2+2*n:
			want = tasks[1].start.SpanID
		}
		require.Equalf(t, want, s.start.ParentSpanID, "parent of step %d (%s)", i, s.start.Name)
	}
	// Each task spans exactly its own steps.
	first, last := steps[2], steps[1+n]
	require.True(t, tasks[0].start.StartTime.Equal(first.start.StartTime))
	require.True(t, tasks[0].end.EndTime.Equal(last.end.EndTime))
	first, last = steps[2+n], steps[1+2*n]
	require.True(t, tasks[1].start.StartTime.Equal(first.start.StartTime))
	require.True(t, tasks[1].end.EndTime.Equal(last.end.EndTime))
	require.Equal(t, steps[2+2*n].start.SpanID, stateStepSpan(t, dir))

	for _, c := range commandRecords(recs) {
		require.False(t, c.Name == "trace" || strings.HasPrefix(c.Name, "trace "), "no trace command was run: %s", c.Name)
	}
	for _, c := range allRecords(t, logsDir(dir)) {
		require.False(t, c.Name == "trace" || strings.HasPrefix(c.Name, "trace "), "no trace command was run: %s", c.Name)
	}
}

// Criterion 7: an orchestrated lane's step spans land in the lane's own run
// log, under the lane's run id, and none in the shared run's log.
func TestLogSpans_LaneStepSpansLandInTheLaneLog(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	resetRootCmd(t)
	dataDir := filepath.Join(dir, ".spektacular")
	logs := logsDir(dir)

	runDebugCmd(t, "plan", "new", "--data", `{"name":"solo"}`)
	runDebugCmd(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"architecture","name":"alpha"}`)

	soloRun := sharedState(t, dataDir).RunID
	lane := laneState(t, dataDir, "plan", "alpha")
	require.NotEqual(t, soloRun, lane.RunID)

	alpha := readSpanLog(t, laneLogFile(t, logs, "unknown", "alpha"))
	steps := spansOfType(t, alpha, tracelog.TypeStep)
	// plan new enters new and moves straight on to overview.
	require.Equal(t, []string{"new", "overview", "discovery", "architecture"}, spanNames(steps))
	requireEnded(t, steps[0], tracelog.EndMoved)
	requireEnded(t, steps[1], tracelog.EndMoved)
	requireEnded(t, steps[2], tracelog.EndMoved)
	require.Nil(t, steps[3].end)
	requireContiguous(t, steps)
	for _, r := range alpha {
		require.Equal(t, lane.RunID, r.TraceID)
	}
	for _, s := range steps {
		require.Equal(t, "alpha", s.start.Attributes[tracelog.AttrWorkflowName])
	}
	require.NotNil(t, lane.StepSpan)
	require.Equal(t, steps[3].start.SpanID, lane.StepSpan.ID)
	cmds := commandRecords(alpha)
	require.Equal(t, []string{steps[1].start.SpanID, steps[1].start.SpanID, steps[2].start.SpanID},
		[]string{cmds[0].ParentSpanID, cmds[1].ParentSpanID, cmds[2].ParentSpanID})

	solo := readSpanLog(t, laneLogFile(t, logs, "unknown", "solo"))
	soloSteps := spansOfType(t, solo, tracelog.TypeStep)
	require.Equal(t, []string{"new", "overview"}, spanNames(soloSteps), "the lane's spans must not land in the shared run")
	require.Nil(t, soloSteps[1].end, "the shared run's step is untouched by the lane")
	for _, r := range solo {
		require.Equal(t, soloRun, r.TraceID)
		require.NotEqual(t, "alpha", r.Attributes[tracelog.AttrWorkflowName])
	}
	require.Equal(t, soloSteps[1].start.SpanID, sharedState(t, dataDir).StepSpan.ID)
}

// idPattern matches the 32- and 16-hex ids (run and span ids) an output may
// carry, which differ between two otherwise identical runs.
var idPattern = regexp.MustCompile(`[0-9a-f]{32}|[0-9a-f]{16}`)

// runSpecSet runs the fixture spec run from new to finished, with a rejected
// goto and a file read along the way, in a fresh project logging at level,
// and returns each command's output and exit code with paths, clock times
// and ids normalised, and the normalised final state.
func runSpecSet(t *testing.T, level string) ([]parityRun, string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  level: "+level+"\n")
	resetRootCmd(t)

	cmds := [][]string{
		{"spec", "new", "--data", `{"name":"billing"}`},
		{"spec", "goto", "--data", `{"step":"bogus"}`},
		{"spec", "file", "read", specSpanName},
	}
	for _, s := range specRunSteps {
		cmds = append(cmds, []string{"spec", "goto", "--data", `{"step":"` + s + `"}`})
		if s == "interview" {
			cmds = append(cmds, []string{"spec", "goto", "--data", `{"step":"interview"}`})
		}
	}
	normalise := func(s string) string {
		s = strings.ReplaceAll(s, dir, "TESTDIR")
		s = timestampPattern.ReplaceAllString(s, "TIMESTAMP")
		return idPattern.ReplaceAllString(s, "ID")
	}
	var runs []parityRun
	for _, args := range cmds {
		stdout, code := runLogged(t, args...)
		runs = append(runs, parityRun{Args: args, Stdout: normalise(stdout), Code: code})
	}
	require.Equal(t, 1, runs[1].Code, "the set includes a rejection")

	state := ""
	if b, err := os.ReadFile(filepath.Join(dir, ".spektacular", "state.json")); err == nil {
		state = normalise(string(b))
	}
	if level == config.LogLevelOff {
		require.NoDirExists(t, logsDir(dir), "with logging off nothing is written")
	} else {
		recs := allRecords(t, logsDir(dir))
		require.NotEmpty(t, recs)
	}
	return runs, state
}

// Criterion 8: with logging off a spec run behaves exactly as with logging
// on (the same outputs, exit codes and final state once paths, times and ids
// are normalised) and nothing is written.
func TestLogSpans_OffBehavesAsBeforeAndWritesNothing(t *testing.T) {
	useConfiguredLogLevel(t)
	wantRuns, wantState := runSpecSet(t, config.LogLevelOff)
	gotRuns, gotState := runSpecSet(t, config.LogLevelStandard)
	require.Equal(t, wantRuns, gotRuns)
	require.Equal(t, wantState, gotState)
}
