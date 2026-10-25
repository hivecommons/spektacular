package implement

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// spanSink records the task and step spans an implement run reports.
type spanSink struct {
	starts []workflow.SpanInfo
	ends   []string
}

func (s *spanSink) Started(_ workflow.RunRef, i workflow.SpanInfo) { s.starts = append(s.starts, i) }
func (s *spanSink) Ended(_ workflow.RunRef, i workflow.SpanInfo, _ time.Time, r workflow.EndReason) {
	s.ends = append(s.ends, i.Type+" "+i.Name+" "+string(r))
}
func (s *spanSink) Interrupted(_ workflow.RunRef, i workflow.SpanInfo) {
	s.ends = append(s.ends, i.Type+" "+i.Name+" interrupted")
}
func (s *spanSink) RunEnded(workflow.RunRef, time.Time, workflow.EndReason) {}

// stepStart returns the step span started for step the n-th time (from 0)
// that step was entered.
func (s *spanSink) stepStart(t *testing.T, step string, n int) workflow.SpanInfo {
	t.Helper()
	for _, i := range s.starts {
		if i.Type == workflow.SpanTypeStep && i.Name == step {
			if n == 0 {
				return i
			}
			n--
		}
	}
	t.Fatalf("no step span %d for %s", n, step)
	return workflow.SpanInfo{}
}

func (s *spanSink) tasks() []workflow.SpanInfo {
	var out []workflow.SpanInfo
	for _, i := range s.starts {
		if i.Type == workflow.SpanTypeTask {
			out = append(out, i)
		}
	}
	return out
}

// tickFirstTask ticks the plan's first task, as update_plan has the agent do.
func tickFirstTask(t *testing.T, f goalFixture) {
	t.Helper()
	raw, err := f.st.Read("plans/test/plan.md")
	require.NoError(t, err)
	require.NoError(t, f.st.Write("plans/test/plan.md", []byte(strings.Replace(string(raw), "#### - [ ] Task: Count items", "#### - [x] Task: Count items", 1))))
}

func newTaskSpanRun(t *testing.T, goalCheck bool) (*workflow.Workflow, *spanSink, goalFixture, string) {
	t.Helper()
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	f.cfg.GoalCheck = goalCheck
	sink := &spanSink{}
	cfg := f.cfg
	cfg.Kind = "implement"
	cfg.Spans = sink
	statePath := filepath.Join(t.TempDir(), "state.json")
	wf := workflow.New(Steps(), statePath, cfg, f.st, &captureWriter{})
	wf.SetData("name", "test")
	return wf, sink, f, statePath
}

func TestTaskSpans_TwoTasksEachParentTheirSteps(t *testing.T) {
	wf, sink, f, _ := newTaskSpanRun(t, false)

	work := []string{"implement", "test", "verify", "update_plan", "update_changelog"}
	for _, s := range append([]string{"read_plan", "analyze"}, work...) {
		require.NoError(t, advance(wf, s))
	}
	tickFirstTask(t, f)
	for _, s := range append([]string{"analyze"}, work...) {
		require.NoError(t, advance(wf, s))
	}
	require.NoError(t, wf.Goto("test_plan"))

	tasks := sink.tasks()
	require.Len(t, tasks, 2)
	require.Equal(t, "Count items", tasks[0].Name)
	require.Equal(t, "t-1", tasks[0].TaskID)
	require.Equal(t, "Another task", tasks[1].Name)
	require.Equal(t, "t-2", tasks[1].TaskID)
	for _, task := range tasks {
		require.Empty(t, task.Parent, "a task span belongs to the run")
		require.Equal(t, "analyze", task.Step)
	}

	for n, task := range tasks {
		for _, s := range append([]string{"analyze"}, work...) {
			require.Equal(t, task.ID, sink.stepStart(t, s, n).Parent, "%s of task %d nests under its task", s, n+1)
		}
	}
	require.Empty(t, sink.stepStart(t, "read_plan", 0).Parent)
	require.Empty(t, sink.stepStart(t, "test_plan", 0).Parent)

	require.Contains(t, sink.ends, "task Count items moved", "looping back ends the first task")
	require.Contains(t, sink.ends, "task Another task moved", "moving on to the test plan ends the second")
}

func TestTaskSpans_GoalCheckRetryStaysInTheTask(t *testing.T) {
	wf, sink, f, _ := newTaskSpanRun(t, true)
	for _, s := range []string{"read_plan", "analyze", "implement", "test", "verify", "check_goal"} {
		require.NoError(t, advance(wf, s))
	}
	f.record(t, "t-1", "met", "not_met", "met")
	require.NoError(t, wf.Goto("implement"))

	require.Len(t, sink.tasks(), 1)
	task := sink.tasks()[0]
	require.Equal(t, task.ID, sink.stepStart(t, "implement", 1).Parent)
	for _, e := range sink.ends {
		require.NotContains(t, e, "task ", "the retry ends no task span")
	}
}

func TestTaskSpans_ResumeKeepsTheTaskSpan(t *testing.T) {
	wf, sink, _, statePath := newTaskSpanRun(t, false)
	for _, s := range []string{"read_plan", "analyze", "implement", "test"} {
		require.NoError(t, advance(wf, s))
	}
	task := sink.tasks()[0]

	// A new session resumes at test.
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	cfg := f.cfg
	cfg.Kind = "implement"
	resumed := &spanSink{}
	cfg.Spans = resumed
	wf = workflow.New(Steps(), statePath, cfg, f.st, &captureWriter{})
	require.NoError(t, wf.Goto("test"))

	require.Equal(t, []string{"step test interrupted"}, resumed.ends)
	require.Empty(t, resumed.tasks(), "no new task span")
	require.Equal(t, task.ID, resumed.stepStart(t, "test", 0).Parent)
	require.Equal(t, task.ID, wf.State().TaskSpan.ID)
}

func TestTaskSpans_SingleTaskRunNamesItsTask(t *testing.T) {
	wf, sink, _, _ := newTaskSpanRun(t, false)
	wf.SetData("task", "t-2")
	require.NoError(t, advance(wf, "read_plan"))
	require.NoError(t, advance(wf, "analyze"))

	require.Len(t, sink.tasks(), 1)
	require.Equal(t, "Another task", sink.tasks()[0].Name)
	require.Equal(t, "t-2", sink.tasks()[0].TaskID)
}

func TestTaskSpans_LegacyPhaseNamesItsPhase(t *testing.T) {
	f := newGoalFixture(t, goalLegacyPlan, "")
	sink := &spanSink{}
	cfg := f.cfg
	cfg.GoalCheck = false
	cfg.Spans = sink
	wf := workflow.New(Steps(), filepath.Join(t.TempDir(), "state.json"), cfg, f.st, &captureWriter{})
	wf.SetData("name", "test")
	require.NoError(t, advance(wf, "read_plan"))
	require.NoError(t, advance(wf, "analyze"))

	require.Len(t, sink.tasks(), 1)
	require.Equal(t, "Old style", sink.tasks()[0].Name)
	require.Equal(t, "Phase 1.1", sink.tasks()[0].TaskID)
}
