package workflow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

var testSteps = []StepConfig{
	{Name: "one", Src: []string{"new"}, Dst: "one"},
	{Name: "two", Src: []string{"one"}, Dst: "two"},
	{Name: "three", Src: []string{"two"}, Dst: "three"},
}

func TestNew(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	require.Equal(t, "new", wf.Current())
	require.False(t, wf.IsComplete())
}

func TestNextAdvancesThroughAllSteps(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	err := wf.Next() // new → one
	require.NoError(t, err)
	require.Equal(t, "one", wf.Current())

	err = wf.Next() // one → two
	require.NoError(t, err)
	require.Equal(t, "two", wf.Current())

	err = wf.Next() // two → three
	require.NoError(t, err)
	require.Equal(t, "three", wf.Current())

	err = wf.Next() // three → done
	require.NoError(t, err)
	require.True(t, wf.IsComplete())
}

func TestNextOnCompleteErrors(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	for i := 0; i <= len(testSteps); i++ {
		err := wf.Next()
		require.NoError(t, err)
	}

	err := wf.Next()
	require.Error(t, err)
}

func TestGotoForward(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one

	err := wf.Goto("two")
	require.NoError(t, err)
	require.Equal(t, "two", wf.Current())
}

// captureWriter is an in-test ResultWriter that records each WriteResult call,
// letting the re-render test assert the step's instruction was re-emitted.
type captureWriter struct {
	calls int
	last  any
}

func (c *captureWriter) WriteResult(v any) error {
	c.calls++
	c.last = v
	return nil
}

func TestGotoSameStepReRenders(t *testing.T) {
	// invoked counts how many times the "one" callback fires; the closure also
	// writes a sentinel result so we can confirm the instruction is re-emitted.
	// Returning "" means the callback does NOT advance the workflow.
	invoked := 0
	steps := []StepConfig{
		{
			Name: "one",
			Src:  []string{"new"},
			Dst:  "one",
			Callback: func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error) {
				invoked++
				_ = out.WriteResult("one-instruction")
				return "", nil
			},
		},
		{Name: "two", Src: []string{"one"}, Dst: "two"},
		{Name: "three", Src: []string{"two"}, Dst: "three"},
	}

	sp := filepath.Join(t.TempDir(), "state.json")
	out := &captureWriter{}
	wf := New(steps, sp, Config{}, nil, out)

	require.NoError(t, wf.Next()) // new → one (fires "one" callback once)

	baselineInvoked := invoked
	baselineWrites := out.calls

	err := wf.Goto("one") // same as current step → re-render, no transition
	require.NoError(t, err)

	// The callback ran again, re-emitting the instruction, without advancing.
	require.Equal(t, baselineInvoked+1, invoked)
	require.Equal(t, baselineWrites+1, out.calls)
	require.Equal(t, "one-instruction", out.last)

	// Current step and completed-steps list are unchanged by the re-render.
	require.Equal(t, "one", wf.Current())
	require.Equal(t, []string{}, wf.State().CompletedSteps)
}

func TestGotoInvalidStepFails(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	// Fresh workflow: current step is "new" (testSteps[0].Src[0]), and the
	// only step reachable from "new" is "one" (testSteps[0].Src == ["new"]).
	err := wf.Goto("nonexistent")
	require.Error(t, err)

	var errResp *output.ErrorResponse
	require.True(t, errors.As(err, &errResp), "expected *output.ErrorResponse, got %T: %v", err, err)
	require.NotNil(t, errResp.State)
	require.Equal(t, "new", errResp.State.Current)
	require.Equal(t, []string{"one"}, errResp.State.ValidActions)
}

// TestGotoFromWalkthroughHintsAtFileWriteForRevisions asserts that a
// rejected transition away from a "walkthrough" step — whose only valid
// forward move is "finished" — gets an enriched next_action pointing the
// agent at `<kind> file write` for revising an earlier section, rather than
// just the bare "run: ... goto finished" suggestion. Without this hint an
// agent stuck here has no CLI-native way back to the correct move and may
// fall back to a full `new --force` restart just to fix one section.
func TestGotoFromWalkthroughHintsAtFileWriteForRevisions(t *testing.T) {
	steps := []StepConfig{
		{Name: "one", Src: []string{"new"}, Dst: "one"},
		{Name: "walkthrough", Src: []string{"one"}, Dst: "walkthrough"},
		{Name: "finished", Src: []string{"walkthrough"}, Dst: "finished"},
	}
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(steps, sp, Config{Command: "go run .", Kind: "plan"}, nil, nil)
	wf.SetData("name", "000045_config-file-migration")

	require.NoError(t, wf.Next()) // new -> one
	require.NoError(t, wf.Next()) // one -> walkthrough
	require.Equal(t, "walkthrough", wf.Current())

	err := wf.Goto("one") // trying to go backward to revise an earlier section
	require.Error(t, err)

	var errResp *output.ErrorResponse
	require.True(t, errors.As(err, &errResp), "expected *output.ErrorResponse, got %T: %v", err, err)
	require.Equal(t, "invalid_transition", errResp.Code)
	require.Contains(t, errResp.NextAction, "go run . plan file write 000045_config-file-migration <doc> --from <scratch>")
	require.Contains(t, errResp.NextAction, "do not try to goto backward")
	require.Contains(t, errResp.NextAction, `run: go run . plan goto --data '{"step":"finished"}'`)
}

// TestGotoInvalidStepFromNonWalkthroughStepOmitsRevisionHint asserts the
// walkthrough-specific hint is scoped to walkthrough only — a rejected
// transition from any other step keeps the plain, unembellished next_action
// (the concrete valid goto command(s)), since only walkthrough's
// forward-only-to-finished shape creates the "no way back" trap.
func TestGotoInvalidStepFromNonWalkthroughStepOmitsRevisionHint(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{Command: "go run .", Kind: "plan"}, nil, nil)

	err := wf.Goto("nonexistent")
	require.Error(t, err)

	var errResp *output.ErrorResponse
	require.True(t, errors.As(err, &errResp), "expected *output.ErrorResponse, got %T: %v", err, err)
	require.NotContains(t, errResp.NextAction, "file write")
	require.Equal(t, `run: go run . plan goto --data '{"step":"one"}'`, errResp.NextAction)
}

func TestAutoSaveOnTransition(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one
	wf.Next() // → two

	// Rebuild from persisted state (auto-saved by enter_state).
	loaded := New(testSteps, sp, Config{}, nil, nil)
	require.Equal(t, "two", loaded.Current())
	require.Equal(t, []string{"one"}, loaded.State().CompletedSteps)
}

func TestStepStatus(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one
	wf.Next() // → two

	infos := wf.StepStatus()
	require.Len(t, infos, 3)
	require.Equal(t, "completed", infos[0].Status)
	require.Equal(t, "current", infos[1].Status)
	require.Equal(t, "pending", infos[2].Status)
}

func TestGotoBackwardFails(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one
	wf.Next() // → two
	wf.Next() // → three

	// Current step is "three". The event "one" exists (Src == ["new"]) but
	// "three" isn't a valid source for it, so this is a rejected transition,
	// not an unknown event. The only step reachable from "three" is the
	// implicit "done" transition (Src == [testSteps[2].Dst] == ["three"]).
	err := wf.Goto("one")
	require.Error(t, err)

	var errResp *output.ErrorResponse
	require.True(t, errors.As(err, &errResp), "expected *output.ErrorResponse, got %T: %v", err, err)
	require.NotNil(t, errResp.State)
	require.Equal(t, "three", errResp.State.Current)
	require.Equal(t, []string{"done"}, errResp.State.ValidActions)
}

// TestNextCallbackErrorNotTranslated proves that an error returned by a
// step's own callback (surfaced via the "before_" callback's e.Cancel(err))
// is not a rejected-transition error and must pass through
// translateTransitionError unchanged: not rewritten into an
// *output.ErrorResponse. looplab/fsm wraps a before_ callback's Cancel(err)
// as fsm.CanceledError{Err: err} (unlike after_, which returns err
// unwrapped), so the original error is checked via errors.Is (which follows
// CanceledError's Unwrap) rather than exact string equality.
func TestNextCallbackErrorNotTranslated(t *testing.T) {
	callbackErr := errors.New("callback exploded: disk full")
	steps := []StepConfig{
		{
			Name: "one",
			Src:  []string{"new"},
			Dst:  "one",
			Callback: func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error) {
				return "", callbackErr
			},
		},
		{Name: "two", Src: []string{"one"}, Dst: "two"},
	}
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(steps, sp, Config{}, nil, nil)

	err := wf.Next() // new → one; "one" callback fails

	require.Error(t, err)

	var errResp *output.ErrorResponse
	require.False(t, errors.As(err, &errResp), "callback error must not be translated into an *output.ErrorResponse, got: %v", err)
	require.ErrorIs(t, err, callbackErr, "the original callback error must survive, however the FSM library wraps it")
	require.NotContains(t, err.Error(), "invalid_transition")
	require.NotContains(t, err.Error(), "cannot run step")
}

// TestFailedStepDoesNotAdvancePersistedState is the Phase 3.1 regression
// test: it proves that when a step's own callback fails, the FSM's before_
// callback Cancel(err) genuinely vetoes the transition, so neither the
// in-memory Workflow nor the persisted state.json file on disk advances past
// the source step. Before the before_/after_ registration fix, enter_state
// (which persists state.json) ran ahead of the step's own after_ callback,
// so a failing step's Cancel(err) came too late — the transition had already
// committed and been written to disk.
func TestFailedStepDoesNotAdvancePersistedState(t *testing.T) {
	callbackErr := errors.New("callback exploded: disk full")
	steps := []StepConfig{
		// "one" has no callback and succeeds trivially, so there is a
		// persisted state.json on disk (CurrentStep "one") before the
		// failing step ever runs.
		{Name: "one", Src: []string{"new"}, Dst: "one"},
		{
			Name: "two",
			Src:  []string{"one"},
			Dst:  "two",
			Callback: func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error) {
				return "", callbackErr
			},
		},
		{Name: "three", Src: []string{"two"}, Dst: "three"},
	}
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(steps, sp, Config{}, nil, nil)

	require.NoError(t, wf.Next()) // new → one (succeeds, persists state.json)
	require.Equal(t, "one", wf.Current())

	err := wf.Next() // one → two; "two" callback fails

	// The error surfaces to the caller and is the original callback error,
	// not rewritten into an *output.ErrorResponse (same guarantee as
	// TestNextCallbackErrorNotTranslated).
	require.Error(t, err)
	require.ErrorIs(t, err, callbackErr)
	var errResp *output.ErrorResponse
	require.False(t, errors.As(err, &errResp), "callback error must not be translated into an *output.ErrorResponse, got: %v", err)

	// Criterion 1: the in-memory Workflow still shows the source step "one",
	// not the would-be destination "two", and "one" is not recorded as
	// completed.
	require.Equal(t, "one", wf.Current())
	require.Equal(t, "one", wf.State().CurrentStep)
	require.NotContains(t, wf.State().CompletedSteps, "one")

	// Criterion 1 (independent check): read state.json directly off disk
	// rather than trusting the in-memory struct, since the whole point of
	// this test is proving the ON-DISK file was not advanced either.
	raw, readErr := os.ReadFile(sp)
	require.NoError(t, readErr)
	var persisted State
	require.NoError(t, json.Unmarshal(raw, &persisted))
	require.Equal(t, "one", persisted.CurrentStep)
	require.NotContains(t, persisted.CompletedSteps, "one")
}

func TestNextStepName(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one
	require.Equal(t, "two", wf.NextStepName())

	wf.Next() // → two
	require.Equal(t, "three", wf.NextStepName())

	wf.Next() // → three
	require.Equal(t, "", wf.NextStepName())
}

func TestCallbackGotoAdvances(t *testing.T) {
	steps := []StepConfig{
		{
			Name: "init",
			Src:  []string{"new"},
			Dst:  "init",
			Callback: func(data Data, out ResultWriter, st store.Store, cfg Config) (string, error) {
				return "real", nil
			},
		},
		{Name: "real", Src: []string{"init"}, Dst: "real"},
		{Name: "final", Src: []string{"real"}, Dst: "final"},
	}
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(steps, sp, Config{}, nil, nil)

	err := wf.Next() // fires "init", callback returns "real", so advances to "real"
	require.NoError(t, err)
	require.Equal(t, "real", wf.Current())
}

// TestMultiSourceTransition verifies that workflow.StepConfig.Src accepts a
// multi-element source slice and that the FSM can fire the transition from any
// of the listed sources. This is a prerequisite for the implement workflow's
// analyze step, which must be reachable from both read_plan and update_changelog.
func TestMultiSourceTransition(t *testing.T) {
	steps := []StepConfig{
		{Name: "one", Src: []string{"start"}, Dst: "one"},
		{Name: "two", Src: []string{"one", "three"}, Dst: "two"},
		{Name: "three", Src: []string{"two"}, Dst: "three"},
	}

	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(steps, sp, Config{}, nil, nil)

	require.NoError(t, wf.Goto("one"))
	require.Equal(t, "one", wf.Current())

	require.NoError(t, wf.Goto("two"))
	require.Equal(t, "two", wf.Current())

	require.NoError(t, wf.Goto("three"))
	require.Equal(t, "three", wf.Current())

	// Loop back: three → two via the multi-source edge.
	require.NoError(t, wf.Goto("two"))
	require.Equal(t, "two", wf.Current())
}

func TestCompletedStepsTracked(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{}, nil, nil)

	wf.Next() // → one
	wf.Next() // → two
	require.Equal(t, []string{"one"}, wf.State().CompletedSteps)

	wf.Next() // → three (terminal)
	// The terminal step is marked completed by commitTerminal after the
	// event fires, since no further transition will mark it later.
	require.Equal(t, []string{"one", "two", "three"}, wf.State().CompletedSteps)
}

func TestKindPersistedOnNew(t *testing.T) {
	sp := filepath.Join(t.TempDir(), "state.json")
	wf := New(testSteps, sp, Config{Kind: "spec"}, nil, nil)

	// Trigger a save so the state file is written to disk.
	require.NoError(t, wf.Next()) // new → one

	// Read the persisted state back from disk and assert the recorded kind.
	loaded, err := loadState(sp)
	require.NoError(t, err)
	require.Equal(t, "spec", loaded.Kind)
}

func TestInProgress(t *testing.T) {
	tests := []struct {
		name        string
		currentStep string
		expected    bool
	}{
		{name: "empty step is not in progress", currentStep: "", expected: false},
		{name: "finished step is not in progress", currentStep: "finished", expected: false},
		{name: "unfinished step is in progress", currentStep: "one", expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &State{CurrentStep: tt.currentStep}
			require.Equal(t, tt.expected, s.InProgress())
		})
	}
}
