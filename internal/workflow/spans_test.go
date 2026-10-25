package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// fakeSink records the span events the workflow reports, one line each.
type fakeSink struct {
	events []string
	starts []SpanInfo
	runs   []RunRef
}

func (f *fakeSink) Started(run RunRef, s SpanInfo) {
	f.runs = append(f.runs, run)
	f.starts = append(f.starts, s)
	f.events = append(f.events, fmt.Sprintf("start %s %s", s.Type, s.Name))
}

func (f *fakeSink) Ended(run RunRef, s SpanInfo, at time.Time, reason EndReason) {
	f.events = append(f.events, fmt.Sprintf("end %s %s %s", s.Type, s.Name, reason))
}

func (f *fakeSink) Interrupted(run RunRef, s SpanInfo) {
	f.events = append(f.events, fmt.Sprintf("interrupted %s %s", s.Type, s.Name))
}

func (f *fakeSink) RunEnded(run RunRef, at time.Time, reason EndReason) {
	f.events = append(f.events, fmt.Sprintf("run-end %s", reason))
}

// spanSteps is a spec-like workflow a → b → c whose callbacks record the
// step span id each one is rendered with.
func spanSteps(seen map[string]string) []StepConfig {
	cb := func(name string) StepCallback {
		return func(_ Data, _ ResultWriter, _ store.Store, cfg Config) (string, error) {
			seen[name] = cfg.StepSpan
			return "", nil
		}
	}
	return []StepConfig{
		{Name: "a", Src: []string{"start"}, Dst: "a", Callback: cb("a")},
		{Name: "b", Src: []string{"a"}, Dst: "b", Callback: cb("b")},
		{Name: "c", Src: []string{"b"}, Dst: "c", Callback: cb("c")},
		{Name: "finished", Src: []string{"c"}, Dst: "finished", Callback: cb("finished")},
	}
}

func TestSpans_EveryMoveEndsTheOldStepAndStartsTheNew(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	seen := map[string]string{}
	wf := New(spanSteps(seen), sp, Config{Spans: sink}, nil, nil)
	wf.SetData("name", "000001_x")

	require.NoError(t, wf.Next())
	require.NoError(t, wf.Goto("b"))
	require.NoError(t, wf.Goto("c"))

	require.Equal(t, []string{
		"start step a",
		"end step a moved",
		"start step b",
		"end step b moved",
		"start step c",
	}, sink.events)

	saved, err := loadState(sp)
	require.NoError(t, err)
	require.NotNil(t, saved.StepSpan)
	require.Equal(t, "c", saved.StepSpan.Name)
	require.Regexp(t, `^[0-9a-f]{16}$`, saved.StepSpan.ID)
	require.Equal(t, sink.starts[2].ID, saved.StepSpan.ID)
	require.Equal(t, saved.RunID+"-"+saved.StepSpan.ID, seen["c"], "the callback sees the composite id of the span it renders")
	require.NotEqual(t, seen["a"], seen["b"])

	require.Equal(t, RunRef{RunID: saved.RunID, Kind: "", Name: "000001_x", StartedAt: saved.CreatedAt}, sink.runs[2])
}

func TestSpans_ChainedStepsReportEachStep(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	steps := []StepConfig{
		{Name: "new", Src: []string{"start"}, Dst: "new", Callback: func(Data, ResultWriter, store.Store, Config) (string, error) { return "read", nil }},
		{Name: "read", Src: []string{"new"}, Dst: "read", Callback: func(Data, ResultWriter, store.Store, Config) (string, error) { return "", nil }},
		{Name: "finished", Src: []string{"read"}, Dst: "finished"},
	}
	wf := New(steps, sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Next())

	require.Equal(t, []string{"start step new", "end step new moved", "start step read"}, sink.events)
}

func TestSpans_GotoCurrentStepInterruptsAndReopens(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	seen := map[string]string{}
	wf := New(spanSteps(seen), sp, Config{}, nil, nil)
	require.NoError(t, wf.Next())
	require.NoError(t, wf.Goto("b"))
	before, err := loadState(sp)
	require.NoError(t, err)

	// A new session resumes: a fresh workflow over the same state.
	sink := &fakeSink{}
	wf = New(spanSteps(seen), sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Goto("b"))

	require.Equal(t, []string{"interrupted step b", "start step b"}, sink.events)
	after, err := loadState(sp)
	require.NoError(t, err)
	require.Equal(t, before.CurrentStep, after.CurrentStep)
	require.Equal(t, before.CompletedSteps, after.CompletedSteps)
	require.NotEqual(t, before.StepSpan.ID, after.StepSpan.ID, "the resumed step has a span of its own")
	require.Equal(t, "b", after.StepSpan.Name)
	require.Equal(t, after.RunID+"-"+after.StepSpan.ID, seen["b"])
}

func TestSpans_TerminalStepIsInstantAndEndsTheRun(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	seen := map[string]string{}
	wf := New(spanSteps(seen), sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Next())
	require.NoError(t, wf.Goto("b"))
	require.NoError(t, wf.Goto("c"))
	sink.events = nil
	require.NoError(t, wf.Goto("finished"))

	require.Equal(t, []string{
		"end step c moved",
		"start step finished",
		"end step finished finished",
		"run-end finished",
	}, sink.events)
	require.Equal(t, "", seen["finished"], "the terminal step's instruction names no span")

	saved, err := loadState(sp)
	require.NoError(t, err)
	require.Nil(t, saved.StepSpan, "no step span is left open")
}

func TestSpans_NoSinkStillKeepsIDs(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	seen := map[string]string{}
	wf := New(spanSteps(seen), sp, Config{}, nil, nil)
	require.NoError(t, wf.Next())

	saved, err := loadState(sp)
	require.NoError(t, err)
	require.NotNil(t, saved.StepSpan)
	require.Equal(t, saved.RunID+"-"+saved.StepSpan.ID, seen["a"])
}

func TestSpans_DryRunReportsAndSavesNothing(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	wf := New(spanSteps(map[string]string{}), sp, Config{Spans: sink, DryRun: true}, nil, nil)
	require.NoError(t, wf.Next())
	require.NoError(t, wf.Goto("a"))

	require.Empty(t, sink.events)
	require.NoFileExists(t, sp)
}

func TestAbandonRun(t *testing.T) {
	t.Run("in progress reports its open spans abandoned", func(t *testing.T) {
		sp := filepath.Join(t.TempDir(), "state.json")
		wf := New(spanSteps(map[string]string{}), sp, Config{}, nil, nil)
		require.NoError(t, wf.Next())
		require.NoError(t, wf.Goto("b"))

		sink := &fakeSink{}
		AbandonRun(sp, sink)
		require.Equal(t, []string{"end step b abandoned", "run-end abandoned"}, sink.events)
	})
	t.Run("an open task span is abandoned too", func(t *testing.T) {
		sp := filepath.Join(t.TempDir(), "state.json")
		state := &State{Kind: "implement", RunID: NewRunID(), CurrentStep: "test", CompletedSteps: []string{},
			StepSpan: &OpenSpan{ID: NewSpanID(), Name: "test"},
			TaskSpan: &OpenSpan{ID: NewSpanID(), Name: "Task one", Task: "t-1", Step: "analyze"},
			Data:     map[string]any{"name": "x"}}
		require.NoError(t, saveState(sp, state))
		sink := &fakeSink{}
		AbandonRun(sp, sink)
		require.Equal(t, []string{"end step test abandoned", "end task Task one abandoned", "run-end abandoned"}, sink.events)
	})
	t.Run("finished reports nothing", func(t *testing.T) {
		sp := filepath.Join(t.TempDir(), "state.json")
		wf := New(spanSteps(map[string]string{}), sp, Config{}, nil, nil)
		require.NoError(t, wf.Next())
		require.NoError(t, wf.Goto("b"))
		require.NoError(t, wf.Goto("c"))
		require.NoError(t, wf.Goto("finished"))

		sink := &fakeSink{}
		AbandonRun(sp, sink)
		require.Empty(t, sink.events)
	})
	t.Run("missing reports nothing", func(t *testing.T) {
		sink := &fakeSink{}
		AbandonRun(filepath.Join(t.TempDir(), "state.json"), sink)
		require.Empty(t, sink.events)
	})
	t.Run("nil sink is safe", func(t *testing.T) {
		sp := filepath.Join(t.TempDir(), "state.json")
		require.NoError(t, os.WriteFile(sp, []byte(`{"current_step":"a","run_id":"x"}`), 0o644))
		require.NotPanics(t, func() { AbandonRun(sp, nil) })
	})
}

// taskSteps is an implement-like table: prep, then a task loop start →
// work → close, then wrap and finished, with start naming the task from
// the titles queue.
func taskSteps(titles *[]string) []StepConfig {
	return []StepConfig{
		{Name: "prep", Src: []string{"begin"}, Dst: "prep"},
		{Name: "start", Src: []string{"prep", "close"}, Dst: "start", Task: TaskStart, Callback: func(d Data, _ ResultWriter, _ store.Store, _ Config) (string, error) {
			title := (*titles)[0]
			*titles = (*titles)[1:]
			d.Set(TaskDataKey, map[string]any{"id": "id-" + title, "title": title})
			return "", nil
		}},
		{Name: "work", Src: []string{"start", "close"}, Dst: "work", Task: TaskWithin},
		{Name: "close", Src: []string{"work"}, Dst: "close", Task: TaskWithin},
		{Name: "wrap", Src: []string{"close"}, Dst: "wrap"},
		{Name: "finished", Src: []string{"wrap", "close"}, Dst: "finished"},
	}
}

func TestSpans_TaskRolesNestStepsUnderTaskSpans(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	titles := []string{"First", "Second"}
	wf := New(taskSteps(&titles), sp, Config{Spans: sink}, nil, nil)

	require.NoError(t, wf.Next()) // prep
	for _, s := range []string{"start", "work", "close", "work", "close", "start", "work", "close", "wrap"} {
		require.NoError(t, wf.Goto(s))
	}

	require.Equal(t, []string{
		"start step prep",
		"end step prep moved",
		"start task First",
		"start step start",
		"end step start moved",
		"start step work",
		"end step work moved",
		"start step close",
		"end step close moved",
		"start step work", // a retry back into the task stays in it
		"end step work moved",
		"start step close",
		"end step close moved",
		"end task First moved",
		"start task Second",
		"start step start",
		"end step start moved",
		"start step work",
		"end step work moved",
		"start step close",
		"end step close moved",
		"end task Second moved",
		"start step wrap",
	}, sink.events)

	// Parents: every step inside a task nests under that task's span.
	var first, second string
	for _, s := range sink.starts {
		switch {
		case s.Type == SpanTypeTask && s.Name == "First":
			first = s.ID
			require.Equal(t, "id-First", s.TaskID)
			require.Equal(t, "start", s.Step)
			require.Empty(t, s.Parent, "a task span belongs to the run")
		case s.Type == SpanTypeTask && s.Name == "Second":
			second = s.ID
		}
	}
	task := ""
	for _, s := range sink.starts {
		if s.Type == SpanTypeTask {
			if s.Name == "First" {
				task = first
			} else {
				task = second
			}
			continue
		}
		switch s.Name {
		case "prep", "wrap":
			require.Empty(t, s.Parent, "%s is outside any task", s.Name)
		default:
			require.Equal(t, task, s.Parent, "%s nests under its task", s.Name)
		}
	}

	saved, err := loadState(sp)
	require.NoError(t, err)
	require.Nil(t, saved.TaskSpan)
}

func TestSpans_FinishingInsideATaskEndsIt(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	sink := &fakeSink{}
	titles := []string{"Only"}
	wf := New(taskSteps(&titles), sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Next())
	for _, s := range []string{"start", "work", "close"} {
		require.NoError(t, wf.Goto(s))
	}
	sink.events = nil
	require.NoError(t, wf.Goto("finished"))
	require.Equal(t, []string{
		"end step close moved",
		"end task Only finished",
		"start step finished",
		"end step finished finished",
		"run-end finished",
	}, sink.events)
}

func TestSpans_ResumeInsideATaskKeepsTheTaskSpan(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	titles := []string{"Only"}
	wf := New(taskSteps(&titles), sp, Config{}, nil, nil)
	require.NoError(t, wf.Next())
	require.NoError(t, wf.Goto("start"))
	require.NoError(t, wf.Goto("work"))
	before, err := loadState(sp)
	require.NoError(t, err)

	sink := &fakeSink{}
	wf = New(taskSteps(&titles), sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Goto("work"))
	require.Equal(t, []string{"interrupted step work", "start step work"}, sink.events)

	after, err := loadState(sp)
	require.NoError(t, err)
	require.Equal(t, before.TaskSpan, after.TaskSpan)
	require.Equal(t, before.TaskSpan.ID, after.StepSpan.Parent)
}

func TestSpans_WithinStepOpensATaskForARunWithoutOne(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	// A run from before spans existed, resumed mid-task: no task span, but
	// analyze named the task in the data.
	state := &State{RunID: NewRunID(), CurrentStep: "start", CompletedSteps: []string{"prep"},
		Data: map[string]any{TaskDataKey: map[string]any{"id": "t-9", "title": "Legacy"}}}
	require.NoError(t, saveState(sp, state))
	sink := &fakeSink{}
	wf := New(taskSteps(&[]string{}), sp, Config{Spans: sink}, nil, nil)
	require.NoError(t, wf.Goto("work"))
	require.Equal(t, []string{"start task Legacy", "start step work"}, sink.events)
}
