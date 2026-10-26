package workflow

import "time"

// RunRef names the run a span belongs to: its run id (the trace id), kind,
// spec name and the moment its state was created.
type RunRef struct {
	RunID     string
	Kind      string
	Name      string
	StartedAt time.Time
}

// SpanInfo describes a step or task span the workflow opened.
type SpanInfo struct {
	ID     string
	Parent string
	Name   string
	// Type is "step" or "task".
	Type string
	// Step is the step the span belongs to: the step itself for a step
	// span, the step that opened it for a task span.
	Step string
	// TaskID is a task span's task id.
	TaskID string
	Start  time.Time
}

// EndReason is why the workflow ended a span.
type EndReason string

const (
	// EndMoved: the workflow left the step or task normally.
	EndMoved EndReason = "moved"
	// EndFinished: the terminal step's instant span, and the step or task
	// closed on reaching it.
	EndFinished EndReason = "finished"
	// EndInterrupted: a step left open by a stopped session, closed when the
	// run resumes.
	EndInterrupted EndReason = "interrupted"
	// EndAbandoned: a span still open when its run finished or restarted.
	EndAbandoned EndReason = "abandoned"
)

// TaskRole is a step's part in the work on one plan task.
type TaskRole int

const (
	// TaskNone: the step is outside any task; its span belongs to the run.
	TaskNone TaskRole = iota
	// TaskStart: entering the step starts the work on a task. Its callback
	// names the task under TaskDataKey; the open task span, if any, ends and
	// one opens for the named task.
	TaskStart
	// TaskWithin: the step is part of the current task's work; its span
	// nests under the task span.
	TaskWithin
)

// TaskDataKey is the workflow data key under which a TaskStart step's
// callback names the task it starts, as a map with "id" and "title".
const TaskDataKey = "trace_task"

// Span types the workflow records.
const (
	SpanTypeStep = "step"
	SpanTypeTask = "task"
)

// SpanSink receives the workflow's span events, so they can be written to a
// log. The workflow itself knows nothing of logs: with no sink it only keeps
// its span ids in state.
type SpanSink interface {
	// Started reports a span the workflow opened.
	Started(run RunRef, s SpanInfo)
	// Ended reports a span the workflow closed at at for reason. Ending a
	// span also ends every span still open under it, marked abandoned.
	Ended(run RunRef, s SpanInfo, at time.Time, reason EndReason)
	// Interrupted reports a step span left open by a stopped session. It
	// ends, with the spans open under it, at the last activity recorded
	// under it, or at its own start when there was none.
	Interrupted(run RunRef, s SpanInfo)
	// RunEnded reports the end of the run, for reason (finished or
	// abandoned): every span still open in it ends at at, marked abandoned.
	RunEnded(run RunRef, at time.Time, reason EndReason)
}

// stepInfo is the SpanInfo of an open step span.
func stepInfo(s *OpenSpan) SpanInfo {
	return SpanInfo{ID: s.ID, Parent: s.Parent, Name: s.Name, Type: SpanTypeStep, Step: s.Name, Start: s.Start}
}

// AbandonRun reports the run whose state is at statePath as abandoned to
// sink: its open step span, its open task span and every span still open in
// it. A restart calls it before discarding the state. A missing, unreadable
// or finished state, or one with no run id, reports nothing.
func AbandonRun(statePath string, sink SpanSink) {
	if sink == nil {
		return
	}
	s, err := loadState(statePath)
	if err != nil || !s.InProgress() || s.RunID == "" {
		return
	}
	ref := s.RunRef()
	now := time.Now().UTC()
	if s.StepSpan != nil {
		sink.Ended(ref, stepInfo(s.StepSpan), now, EndAbandoned)
	}
	if s.TaskSpan != nil {
		sink.Ended(ref, taskInfo(s.TaskSpan), now, EndAbandoned)
	}
	sink.RunEnded(ref, now, EndAbandoned)
}

// taskInfo is the SpanInfo of an open task span.
func taskInfo(s *OpenSpan) SpanInfo {
	return SpanInfo{ID: s.ID, Name: s.Name, Type: SpanTypeTask, Step: s.Step, TaskID: s.Task, Start: s.Start}
}
