package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

// These tests cover `trace start` and `trace stop`: the spans an agent marks
// for its own work (helper agents, waits on the user, other work), recorded
// as span_start and span_end records in the run's log beside the workflow's
// step and task spans. Expected values are hand-written; nothing compares a
// wall-clock value.

// traceStart runs `trace start args...` through runRoot, requires success
// and decodes its result.
func traceStart(t *testing.T, args ...string) TraceStartResult {
	t.Helper()
	out := runDebugCmd(t, append([]string{"trace", "start"}, args...)...)
	var res TraceStartResult
	require.NoError(t, json.Unmarshal([]byte(out), &res), "stdout: %s", out)
	return res
}

// traceStop runs `trace stop args...` through runRoot, requires success and
// decodes its result.
func traceStop(t *testing.T, args ...string) TraceStopResult {
	t.Helper()
	out := runDebugCmd(t, append([]string{"trace", "stop"}, args...)...)
	var res TraceStopResult
	require.NoError(t, json.Unmarshal([]byte(out), &res), "stdout: %s", out)
	return res
}

// spanPart is the span id half of a "<trace_id>-<span_id>" reference.
func spanPart(t *testing.T, ref string) string {
	t.Helper()
	r, err := tracelog.ParseSpanRef(ref)
	require.NoError(t, err)
	return r.SpanID
}

// spanRecordCount counts the span_start and span_end records of every log
// file in logs.
func spanRecordCount(t *testing.T, logs string) int {
	t.Helper()
	n := 0
	for _, f := range logFiles(t, logs) {
		for _, r := range readSpanLog(t, f) {
			if r.Record == tracelog.RecordSpanStart || r.Record == tracelog.RecordSpanEnd {
				n++
			}
		}
	}
	return n
}

// everySpanRecord is every span_start and span_end record of every log file
// in logs.
func everySpanRecord(t *testing.T, logs string) []tracelog.Record {
	t.Helper()
	var out []tracelog.Record
	for _, f := range logFiles(t, logs) {
		for _, r := range readSpanLog(t, f) {
			if r.Record == tracelog.RecordSpanStart || r.Record == tracelog.RecordSpanEnd {
				out = append(out, r)
			}
		}
	}
	return out
}

// loggedSpanByID pairs the span_start of span id in recs with its span_end,
// requiring exactly one start and at most one end.
func loggedSpanByID(t *testing.T, recs []tracelog.Record, id string) loggedSpan {
	t.Helper()
	var s loggedSpan
	starts := 0
	for i, r := range recs {
		if r.SpanID != id {
			continue
		}
		switch r.Record {
		case tracelog.RecordSpanStart:
			starts++
			s.start, s.at = r, i
		case tracelog.RecordSpanEnd:
			require.Nilf(t, s.end, "span %s ends twice", id)
			require.Equalf(t, 1, starts, "span %s ends before it starts", id)
			end := r
			s.end = &end
		}
	}
	require.Equalf(t, 1, starts, "span %s must start once", id)
	return s
}

// traceRefused runs a trace command that must be refused, requires it to
// record no span, and returns its error envelope.
func traceRefused(t *testing.T, logs string, args ...string) output.ErrorResponse {
	t.Helper()
	before := spanRecordCount(t, logs)
	stdout, code := runLogged(t, args...)
	require.Equalf(t, 1, code, "%v must be refused: %s", args, stdout)
	er := decodeError(t, stdout)
	require.NotEmpty(t, er.Message, "the refusal names the problem")
	require.NotEmpty(t, er.NextAction, "the refusal names the next step")
	require.Equalf(t, before, spanRecordCount(t, logs), "%v must record no span", args)
	return er
}

// Criterion 1: starting a sub-agent span named test-writer prints its id,
// under the run's trace, and stopping it records a closed sub-agent span of
// that name with a start, an end and a success status.
func TestTrace_StartAndStopRecordASubAgentSpan(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	runID := stateRunID(t, dir)
	step := stateStepSpan(t, dir)

	started := traceStart(t, "test-writer", "--type", "sub-agent")
	require.Regexp(t, `^`+runID+`-[0-9a-f]{16}$`, started.SpanID)
	require.Equal(t, "test-writer", started.Name)
	require.Equal(t, tracelog.TypeSubAgent, started.Type)
	require.True(t, started.Recorded)

	stopped := traceStop(t, started.SpanID)
	require.Equal(t, TraceStopResult{SpanID: started.SpanID, Status: tracelog.StatusOK, Recorded: true}, stopped)

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	s := loggedSpanByID(t, recs, spanPart(t, started.SpanID))
	require.Equal(t, "test-writer", s.start.Name)
	require.Equal(t, tracelog.TypeSubAgent, spanType(s.start))
	require.Equal(t, runID, s.start.TraceID)
	require.Equal(t, step, s.start.ParentSpanID)
	require.Equal(t, "interview", s.start.Attributes[tracelog.AttrStep])
	require.Equal(t, "spec", s.start.Attributes[tracelog.AttrWorkflowKind])
	require.Equal(t, specSpanName, s.start.Attributes[tracelog.AttrWorkflowName])
	require.False(t, s.start.StartTime.IsZero())

	require.NotNil(t, s.end, "the stopped span is closed")
	require.Equal(t, tracelog.TypeSubAgent, spanType(*s.end))
	require.Equal(t, "test-writer", s.end.Name)
	require.False(t, s.end.EndTime.IsZero())
	require.Equal(t, tracelog.Status{Code: tracelog.StatusOK}, s.end.Status)
	require.Equal(t, tracelog.EndStopped, endReason(*s.end))
	require.Equal(t, s.start.StartTime, s.end.StartTime)
	require.Equal(t, s.start.ParentSpanID, s.end.ParentSpanID)

	// Both commands are recorded in the run's log, under the step.
	var names []string
	for _, c := range commandRecords(recs) {
		if strings.HasPrefix(c.Name, "trace ") {
			names = append(names, c.Name)
			require.Equal(t, step, c.ParentSpanID)
		}
	}
	require.Equal(t, []string{"trace start", "trace stop"}, names)
}

// Criterion 2: stopping a span with --status error records it with an error
// status carrying the message; a message given with an ok status is not
// recorded.
func TestTrace_StopWithErrorStatusRecordsTheMessage(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	runID := stateRunID(t, dir)

	failed := traceStart(t, "test-writer", "--type", "sub-agent")
	res := traceStop(t, failed.SpanID, "--status", "error", "--message", "tests did not compile")
	require.Equal(t, TraceStopResult{SpanID: failed.SpanID, Status: tracelog.StatusError, Recorded: true}, res)

	fine := traceStart(t, "reviewer", "--type", "sub-agent")
	traceStop(t, fine.SpanID, "--status", "ok", "--message", "ignored")

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	f := loggedSpanByID(t, recs, spanPart(t, failed.SpanID))
	require.NotNil(t, f.end)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusError, Message: "tests did not compile"}, f.end.Status)
	require.Equal(t, tracelog.EndStopped, endReason(*f.end))

	ok := loggedSpanByID(t, recs, spanPart(t, fine.SpanID))
	require.NotNil(t, ok.end)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusOK}, ok.end.Status)
}

// Criterion 3: a span started without --parent nests under the current
// step's span, whichever step that is.
func TestTrace_SpanWithoutParentNestsUnderTheStep(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	runID := stateRunID(t, dir)
	newStep := stateStepSpan(t, dir)
	inNew := traceStart(t, "early work")
	specGoto(t, "interview")
	interview := stateStepSpan(t, dir)
	require.NotEqual(t, newStep, interview)
	inInterview := traceStart(t, "ask the user", "--type", "user-wait")

	require.Equal(t, tracelog.TypeWork, inNew.Type, "work is the default type")
	require.Equal(t, runID+"-"+newStep, inNew.Parent)
	require.Equal(t, runID+"-"+interview, inInterview.Parent)

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	a := loggedSpanByID(t, recs, spanPart(t, inNew.SpanID))
	require.Equal(t, newStep, a.start.ParentSpanID)
	require.Equal(t, tracelog.TypeWork, spanType(a.start))
	require.Equal(t, "new", a.start.Attributes[tracelog.AttrStep])
	b := loggedSpanByID(t, recs, spanPart(t, inInterview.SpanID))
	require.Equal(t, interview, b.start.ParentSpanID)
	require.Equal(t, "interview", b.start.Attributes[tracelog.AttrStep])
	steps := spansOfType(t, recs, tracelog.TypeStep)
	require.Less(t, steps[1].at, b.at, "the step's span_start precedes the span nested under it")
}

// Criterion 4: two sub-agent spans open at the same time under different
// parents keep their own parents, and the spans each helper starts with its
// own id as --parent nest under that helper.
func TestTrace_ConcurrentHelpersKeepTheirOwnParents(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	runID := stateRunID(t, dir)
	step := stateStepSpan(t, dir)

	coordinator := traceStart(t, "coordinator")
	helperA := traceStart(t, "helper-a", "--type", "sub-agent", "--parent", runID+"-"+step)
	helperB := traceStart(t, "helper-b", "--type", "sub-agent", "--parent", coordinator.SpanID)
	require.Equal(t, runID+"-"+step, helperA.Parent)
	require.Equal(t, coordinator.SpanID, helperB.Parent)
	childA := traceStart(t, "a-tests", "--parent", helperA.SpanID)
	childB := traceStart(t, "b-tests", "--type", "sub-agent", "--parent", helperB.SpanID)
	childA2 := traceStart(t, "a-ask", "--type", "user-wait", "--parent", helperA.SpanID)
	require.Equal(t, helperA.SpanID, childA.Parent)
	require.Equal(t, helperB.SpanID, childB.Parent)
	traceStop(t, childB.SpanID)
	traceStop(t, childA.SpanID)
	traceStop(t, helperA.SpanID)
	traceStop(t, childA2.SpanID)
	traceStop(t, helperB.SpanID)

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	want := map[string]string{
		coordinator.SpanID: step,
		helperA.SpanID:     step,
		helperB.SpanID:     spanPart(t, coordinator.SpanID),
		childA.SpanID:      spanPart(t, helperA.SpanID),
		childA2.SpanID:     spanPart(t, helperA.SpanID),
		childB.SpanID:      spanPart(t, helperB.SpanID),
	}
	ids := map[string]bool{}
	for ref, parent := range want {
		s := loggedSpanByID(t, recs, spanPart(t, ref))
		require.Equalf(t, parent, s.start.ParentSpanID, "parent of %s", s.start.Name)
		require.Equal(t, runID, s.start.TraceID)
		require.False(t, ids[s.start.SpanID], "every span has its own id")
		ids[s.start.SpanID] = true
		if ref == coordinator.SpanID {
			require.Nil(t, s.end, "the coordinator was never stopped")
			continue
		}
		require.NotNilf(t, s.end, "%s was stopped", s.start.Name)
		require.Equal(t, parent, s.end.ParentSpanID)
		require.Equal(t, tracelog.EndStopped, endReason(*s.end))
	}
}

// Criterion 5: a span started with --type user-wait is recorded as
// user-wait, and across a whole spec run with spans of the other types no
// other span (step, command, sub-agent or work) ever is.
func TestTrace_OnlyUserWaitSpansAreUserWait(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	runID := stateRunID(t, dir)
	waits := map[string]bool{}
	typed := map[string]string{}
	for _, s := range specRunSteps {
		switch s {
		case "interview", "constraints":
			w := traceStart(t, "ask the user", "--type", "user-wait")
			require.Equal(t, tracelog.TypeUserWait, w.Type)
			waits[spanPart(t, w.SpanID)] = true
			typed[spanPart(t, w.SpanID)] = tracelog.TypeUserWait
			if s == "interview" {
				traceStop(t, w.SpanID)
			}
		case "overview":
			h := traceStart(t, "researcher", "--type", "sub-agent")
			typed[spanPart(t, h.SpanID)] = tracelog.TypeSubAgent
			traceStop(t, h.SpanID)
		case "requirements":
			w := traceStart(t, "draft")
			typed[spanPart(t, w.SpanID)] = tracelog.TypeWork
		}
		specGoto(t, s)
	}
	require.Len(t, waits, 2)

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	seenWait := map[string]bool{}
	for _, r := range recs {
		if want, ok := typed[r.SpanID]; ok {
			require.Equalf(t, want, spanType(r), "type of %s (%s)", r.Name, r.Record)
		}
		if spanType(r) == tracelog.TypeUserWait {
			require.Truef(t, waits[r.SpanID], "%s %s (%s) must not be user-wait", r.Record, r.Name, r.SpanID)
			seenWait[r.SpanID] = true
		}
	}
	require.Equal(t, waits, seenWait, "every user-wait span is recorded as user-wait")
	for _, r := range allRecords(t, logsDir(dir)) {
		require.Equal(t, tracelog.TypeCommand, r.Attributes[tracelog.AttrSpanType])
	}
}

// Criterion 6: a span left open is closed, marked abandoned, when the
// workflow moves to the next step (with the spans nested under it), and
// when the workflow reaches finished.
func TestTrace_OpenSpansAreAbandonedWhenTheWorkflowMoves(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	runID := stateRunID(t, dir)

	left := traceStart(t, "forgotten helper", "--type", "sub-agent")
	nested := traceStart(t, "its own helper", "--type", "sub-agent", "--parent", left.SpanID)
	stopped := traceStart(t, "stopped helper", "--type", "sub-agent")
	traceStop(t, stopped.SpanID)
	specGoto(t, "overview")

	for _, s := range specRunSteps[2:] {
		if s == "finished" {
			break
		}
		specGoto(t, s)
	}
	require.Equal(t, "split", readStateMap(t, dir)["current_step"])
	atSplit := traceStart(t, "last helper", "--type", "sub-agent")
	specGoto(t, "finished")

	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	steps := spansOfType(t, recs, tracelog.TypeStep)
	interview, split := steps[1], steps[len(steps)-2]
	require.Equal(t, "interview", interview.start.Name)
	require.Equal(t, "split", split.start.Name)

	for _, ref := range []string{left.SpanID, nested.SpanID} {
		s := loggedSpanByID(t, recs, spanPart(t, ref))
		requireEnded(t, s, tracelog.EndAbandoned)
		require.Equal(t, tracelog.Status{Code: tracelog.StatusError, Message: "abandoned"}, s.end.Status)
		require.True(t, s.end.EndTime.Equal(interview.end.EndTime), "%s ends with its step", s.start.Name)
	}
	nestedID, leftID := spanPart(t, nested.SpanID), spanPart(t, left.SpanID)
	require.Equal(t, leftID, loggedSpanByID(t, recs, nestedID).start.ParentSpanID)
	require.Less(t, endIndex(recs, nestedID), endIndex(recs, leftID), "the nested span closes first")
	require.Less(t, endIndex(recs, leftID), endIndex(recs, interview.start.SpanID), "agent spans close before their step")

	requireEnded(t, loggedSpanByID(t, recs, spanPart(t, stopped.SpanID)), tracelog.EndStopped)

	last := loggedSpanByID(t, recs, spanPart(t, atSplit.SpanID))
	requireEnded(t, last, tracelog.EndAbandoned)
	require.Equal(t, split.start.SpanID, last.start.ParentSpanID)
	require.Empty(t, tracelog.NewIndex(recs).OpenAll(), "a finished run leaves no span open")
}

// endIndex is the line index of span id's span_end in recs, or -1.
func endIndex(recs []tracelog.Record, id string) int {
	for i, r := range recs {
		if r.Record == tracelog.RecordSpanEnd && r.SpanID == id {
			return i
		}
	}
	return -1
}

// Criterion 7: every bad input to the trace commands is refused with an
// error naming the problem and the next step, and records no span.
func TestTrace_RefusalsNameTheProblemAndRecordNoSpan(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	runID := stateRunID(t, dir)
	newStep := stateStepSpan(t, dir)
	specGoto(t, "interview")
	step := stateStepSpan(t, dir)
	logs := logsDir(dir)

	done := traceStart(t, "done helper", "--type", "sub-agent")
	traceStop(t, done.SpanID)
	never := runID + "-0123456789abcdef"

	cases := []struct {
		name     string
		args     []string
		code     string
		resource string
		inMsg    string
		inNext   string
	}{
		{"unknown parent", []string{"trace", "start", "x", "--parent", never}, "parent_not_open", never, never, "trace start"},
		{"stopped parent", []string{"trace", "start", "x", "--parent", done.SpanID}, "parent_not_open", done.SpanID, done.SpanID, "--parent"},
		{"ended step as parent", []string{"trace", "start", "x", "--parent", runID + "-" + newStep}, "parent_not_open", runID + "-" + newStep, newStep, "--parent"},
		{"parent of another trace", []string{"trace", "start", "x", "--parent", "0123456789abcdef0123456789abcdef-" + step}, "parent_not_open", "0123456789abcdef0123456789abcdef-" + step, step, "--parent"},
		{"malformed parent", []string{"trace", "start", "x", "--parent", "abc"}, "invalid_span_id", "abc", "abc", "--parent"},
		{"stop never started", []string{"trace", "stop", never}, "span_not_open", never, never, "trace start"},
		{"stop already stopped", []string{"trace", "stop", done.SpanID}, "span_already_stopped", done.SpanID, done.SpanID, "next command"},
		{"stop the current step", []string{"trace", "stop", runID + "-" + step}, "span_owned_by_workflow", runID + "-" + step, "step", "goto"},
		{"stop an ended step", []string{"trace", "stop", runID + "-" + newStep}, "span_owned_by_workflow", runID + "-" + newStep, "step", "goto"},
		{"stop a malformed id", []string{"trace", "stop", "not-an-id"}, "invalid_span_id", "not-an-id", "not-an-id", "trace stop"},
		{"stop an uppercase id", []string{"trace", "stop", strings.ToUpper(done.SpanID)}, "invalid_span_id", strings.ToUpper(done.SpanID), "lowercase", "trace stop"},
		{"stop with no id", []string{"trace", "stop"}, "invalid_span_id", "", "span id", "trace stop"},
		{"bad status", []string{"trace", "stop", done.SpanID, "--status", "maybe"}, "invalid_span_status", "maybe", "maybe", "--status"},
		{"type step", []string{"trace", "start", "x", "--type", "step"}, "invalid_span_type", "step", "workflow", "--type"},
		{"type task", []string{"trace", "start", "x", "--type", "task"}, "invalid_span_type", "task", "workflow", "--type"},
		{"type command", []string{"trace", "start", "x", "--type", "command"}, "invalid_span_type", "command", "command", "sub-agent, user-wait, work"},
		{"type bogus", []string{"trace", "start", "x", "--type", "bogus"}, "invalid_span_type", "bogus", "bogus", "sub-agent, user-wait, work"},
		{"no name", []string{"trace", "start"}, "invalid_span_name", "", "name", "trace start"},
		{"blank name", []string{"trace", "start", "  "}, "invalid_span_name", "", "name", "trace start"},
		{"multi-line name", []string{"trace", "start", "a\nb"}, "invalid_span_name", "", "one line", "trace start"},
		{"long name", []string{"trace", "start", strings.Repeat("n", maxSpanName+1)}, "invalid_span_name", "", "200 characters", "trace start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			er := traceRefused(t, logs, c.args...)
			require.Equal(t, c.code, er.Code)
			require.Equal(t, c.resource, er.Resource)
			require.Contains(t, er.Message, c.inMsg)
			require.Contains(t, er.NextAction, c.inNext)
		})
	}
	// A name of exactly the limit is accepted.
	require.True(t, traceStart(t, strings.Repeat("n", maxSpanName)).Recorded)
	require.Equal(t, "interview", readStateMap(t, dir)["current_step"])
}

// Criterion 7: stopping the task span of an implement run is refused as the
// workflow's own, and records no span.
func TestTrace_StoppingATaskSpanIsRefused(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, implementTaskConfig)
	resetRootCmd(t)
	writePlanDocLogged(t, twoTaskPlan(false, false))
	runDebugCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	runDebugCmd(t, "implement", "goto", "--data", `{"step":"analyze"}`)
	runID := stateRunID(t, dir)
	task, ok := readStateMap(t, dir)["task_span"].(map[string]any)
	require.True(t, ok, "analyze opens the task's span")
	taskID, _ := task["id"].(string)
	require.Regexp(t, spanIDPattern, taskID)

	ref := runID + "-" + taskID
	er := traceRefused(t, logsDir(dir), "trace", "stop", ref)
	require.Equal(t, "span_owned_by_workflow", er.Code)
	require.Equal(t, ref, er.Resource)
	require.Contains(t, er.Message, "task")
	require.Contains(t, er.NextAction, "goto")

	// The task span is still a parent an agent may nest under.
	child := traceStart(t, "task helper", "--type", "sub-agent", "--parent", ref)
	recs := readSpanLog(t, runLogFile(t, logsDir(dir), runID))
	require.Equal(t, taskID, loggedSpanByID(t, recs, spanPart(t, child.SpanID)).start.ParentSpanID)
	tasks := spansOfType(t, recs, tracelog.TypeTask)
	require.Len(t, tasks, 1)
	require.Nil(t, tasks[0].end, "the refused stop left the task span open")
}

// Criterion 8: starting and stopping spans leaves the workflow state,
// including its current and completed steps, byte for byte unchanged.
func TestTrace_CommandsLeaveTheWorkflowStateUnchanged(t *testing.T) {
	dir := startLoggedSpecRun(t, "")
	specGoto(t, "interview")
	specGoto(t, "overview")
	statePath := filepath.Join(dir, ".spektacular", "state.json")
	before := readBytes(t, statePath)
	m := readStateMap(t, dir)
	require.Equal(t, "overview", m["current_step"])
	completed := m["completed_steps"]

	h := traceStart(t, "helper", "--type", "sub-agent")
	require.Equal(t, string(before), string(readBytes(t, statePath)), "trace start writes no state")
	w := traceStart(t, "ask", "--type", "user-wait", "--parent", h.SpanID)
	traceStop(t, w.SpanID)
	traceStop(t, h.SpanID, "--status", "error", "--message", "gave up")
	_, code := runLogged(t, "trace", "stop", h.SpanID)
	require.Equal(t, 1, code)

	require.Equal(t, string(before), string(readBytes(t, statePath)), "trace commands write no state")
	after := readStateMap(t, dir)
	require.Equal(t, "overview", after["current_step"])
	require.Equal(t, completed, after["completed_steps"])
}

// Criterion 9: with logging off both commands succeed, report that nothing
// was recorded, and write nothing.
func TestTrace_LoggingOffSucceedsAndWritesNothing(t *testing.T) {
	dir := startLoggedSpecRun(t, "log:\n  level: off\n")
	statePath := filepath.Join(dir, ".spektacular", "state.json")
	runID := stateRunID(t, dir)
	before := readBytes(t, statePath)

	started := traceStart(t, "test-writer", "--type", "sub-agent")
	require.Regexp(t, `^`+runID+`-[0-9a-f]{16}$`, started.SpanID)
	require.Equal(t, "test-writer", started.Name)
	require.Equal(t, tracelog.TypeSubAgent, started.Type)
	require.False(t, started.Recorded)
	stopped := traceStop(t, started.SpanID, "--status", "error", "--message", "boom")
	require.Equal(t, TraceStopResult{SpanID: started.SpanID, Status: tracelog.StatusError, Recorded: false}, stopped)

	parented := traceStart(t, "child", "--parent", started.SpanID)
	require.Equal(t, started.SpanID, parented.Parent)
	require.False(t, parented.Recorded)

	require.NoDirExists(t, logsDir(dir), "with logging off nothing is written")
	require.Equal(t, string(before), string(readBytes(t, statePath)))
}

// Criterion 10: a trace start and stop for a span of an orchestrated lane
// land in that lane's run log, the commands and the span both, and none of
// it in the shared run's log.
func TestTrace_LaneSpanIsRecordedInTheLaneLog(t *testing.T) {
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
	lane := laneState(t, dataDir, "plan", "alpha")
	require.NotNil(t, lane.StepSpan)
	soloRun := sharedState(t, dataDir).RunID
	require.NotEqual(t, soloRun, lane.RunID)
	soloBefore := readBytes(t, laneLogFile(t, logs, "unknown", "solo"))

	parent := lane.RunID + "-" + lane.StepSpan.ID
	started := traceStart(t, "lane helper", "--type", "sub-agent", "--parent", parent)
	require.Regexp(t, `^`+lane.RunID+`-[0-9a-f]{16}$`, started.SpanID)
	require.Equal(t, parent, started.Parent)
	traceStop(t, started.SpanID)

	alpha := readSpanLog(t, laneLogFile(t, logs, "unknown", "alpha"))
	s := loggedSpanByID(t, alpha, spanPart(t, started.SpanID))
	require.Equal(t, lane.RunID, s.start.TraceID)
	require.Equal(t, lane.StepSpan.ID, s.start.ParentSpanID)
	require.Equal(t, "alpha", s.start.Attributes[tracelog.AttrWorkflowName])
	require.Equal(t, "discovery", s.start.Attributes[tracelog.AttrStep])
	requireEnded(t, s, tracelog.EndStopped)

	var traceCmds []tracelog.Record
	for _, c := range commandRecords(alpha) {
		if strings.HasPrefix(c.Name, "trace ") {
			traceCmds = append(traceCmds, c)
		}
	}
	require.Equal(t, []string{"trace start", "trace stop"}, recordNames(traceCmds))
	for _, c := range traceCmds {
		require.Equal(t, lane.RunID, c.TraceID)
		require.Equal(t, "alpha", c.Attributes[tracelog.AttrWorkflowName])
	}

	require.Equal(t, string(soloBefore), string(readBytes(t, laneLogFile(t, logs, "unknown", "solo"))),
		"nothing of the lane's span lands in the shared run's log")
	for _, f := range logFiles(t, logs) {
		require.NotContains(t, filepath.Base(f), "standalone_")
	}
	require.Equal(t, lane.StepSpan.ID, laneState(t, dataDir, "plan", "alpha").StepSpan.ID)
}

// With no workflow in progress a span is standalone: its start and stop
// share one trace, with no parent, in the day's standalone file.
func TestTrace_StandaloneSpanSharesOneTraceInTheDayFile(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	resetRootCmd(t)

	started := traceStart(t, "side quest")
	require.Regexp(t, `^[0-9a-f]{32}-[0-9a-f]{16}$`, started.SpanID)
	require.Empty(t, started.Parent)
	require.True(t, started.Recorded)
	traceStop(t, started.SpanID)
	ref, err := tracelog.ParseSpanRef(started.SpanID)
	require.NoError(t, err)

	spans := everySpanRecord(t, logsDir(dir))
	require.Len(t, spans, 2)
	s := loggedSpanByID(t, spans, ref.SpanID)
	require.NotNil(t, s.end)
	if s.start.StartTime.UTC().Format("20060102") != s.end.EndTime.UTC().Format("20060102") {
		t.Skip("the span straddled midnight UTC")
	}
	for _, r := range spans {
		require.Equal(t, ref.TraceID, r.TraceID, "start and stop share one trace")
		require.Empty(t, r.ParentSpanID)
		require.Equal(t, tracelog.TypeWork, spanType(r))
		require.NotContains(t, r.Attributes, tracelog.AttrWorkflowKind)
		require.NotContains(t, r.Attributes, tracelog.AttrStep)
	}
	requireEnded(t, s, tracelog.EndStopped)

	var file string
	for _, f := range logFiles(t, logsDir(dir)) {
		for _, r := range readSpanLog(t, f) {
			if r.SpanID == ref.SpanID {
				require.True(t, file == "" || file == f, "start and stop share one file")
				file = f
			}
		}
	}
	require.Equal(t, "standalone_"+s.start.StartTime.UTC().Format("20060102")+".jsonl", filepath.Base(file))
	_, err = os.Stat(filepath.Join(dir, ".spektacular", "state.json"))
	require.True(t, os.IsNotExist(err), "a standalone span opens no workflow")
}

// Both commands publish a schema naming their input, output and flags.
func TestTrace_SchemaDescribesBothCommands(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: "+config.LogLevelOff+"\n")
	resetRootCmd(t)

	var start commandSchema
	require.NoError(t, json.Unmarshal([]byte(runOK(t, "trace", "start", "--schema")), &start))
	require.Equal(t, []string{"name"}, start.Input.Required)
	require.Contains(t, start.Output.Properties, "span_id")
	require.Contains(t, start.Output.Properties, "recorded")
	require.Equal(t, tracelog.AgentTypes, start.Flags["type"].Enum)
	require.Contains(t, start.Flags, "parent")

	var stop commandSchema
	require.NoError(t, json.Unmarshal([]byte(runOK(t, "trace", "stop", "--schema")), &stop))
	require.Equal(t, []string{"id"}, stop.Input.Required)
	require.Equal(t, []string{tracelog.StatusOK, tracelog.StatusError}, stop.Flags["status"].Enum)
	require.Contains(t, stop.Flags, "message")
	require.Contains(t, stop.Output.Properties, "status")
}
