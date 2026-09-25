package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// resumeInstruction renders the resume-prompt template into the NextAction of
// an in-progress-workflow ErrorResponse. It tells the driving agent to ask the
// user resume-vs-new and embeds both follow-up commands: resume via
// `<command> <kind> goto` on the current step, or start fresh via
// `<command> <kind> new --force`.
//
// An implement workflow gets its own template (steps/resume_implement.md),
// which has the agent read the plan documents before anything else and find
// the current phase from the plan. Spec, plan and repo workflows share
// steps/resume.md.
//
// command is the CLI invocation prefix (workflow.Config.Command), rendered into
// the template via {{config.command}} to match the convention used by every
// other runtime-rendered step template, and as {{command}} for shared partials.
func resumeInstruction(command, kind, name, currentStep, task string) (string, error) {
	templatePath := "steps/resume.md"
	if kind == "implement" {
		templatePath = "steps/resume_implement.md"
	}
	return stepkit.RenderTemplate(templatePath, map[string]any{
		"config":       map[string]any{"command": command},
		"command":      command,
		"kind":         kind,
		"name":         name,
		"current_step": currentStep,
		"task":         task,
	})
}

// mismatchInstruction renders the cross-kind resume-prompt template. It is used
// when the in-progress workflow's kind differs from the kind the user is trying
// to run (e.g. running a spec command while a plan workflow is in progress). The
// instruction tells the agent a different-kind workflow is in progress and gives
// both follow-ups: continue it with its own skill (`<command> <kind> goto`), or
// overwrite it and start the requested kind (`<command> <requestedKind> new
// --force`).
func mismatchInstruction(command, kind, requestedKind, name, currentStep string) (string, error) {
	return stepkit.RenderTemplate("steps/resume_mismatch.md", map[string]any{
		"config":         map[string]any{"command": command},
		"command":        command,
		"kind":           kind,
		"requested_kind": requestedKind,
		"name":           name,
		"current_step":   currentStep,
	})
}

// emitResumeReport builds the shared ErrorResponse for an in-progress
// workflow. When the stored kind matches expectedKind it renders the normal
// same-kind resume prompt; when it differs it renders the cross-kind mismatch
// prompt. Either way the rendered instruction text — unchanged from before
// this phase — becomes the NextAction, and the workflow's identity and
// position are carried in Resource/State, so this flows through the same
// shared error shape and response envelope as any other failure instead of
// being a separately-shaped success response.
func emitResumeReport(command, expectedKind string, state *workflow.State) error {
	name, _ := state.Data["name"].(string)

	if state.Kind != expectedKind {
		instruction, err := mismatchInstruction(command, state.Kind, expectedKind, name, state.CurrentStep)
		if err != nil {
			return err
		}
		return output.NewError("cross_kind_workflow_in_progress",
			fmt.Sprintf("a %s workflow (%q) is in progress at step %q; cannot start a %s workflow while it is active", state.Kind, name, state.CurrentStep, expectedKind)).
			WithResource(name).
			WithState(state.CurrentStep, nil).
			WithNextAction(instruction)
	}

	task, _ := state.Data["task"].(string)
	instruction, err := resumeInstruction(command, state.Kind, name, state.CurrentStep, task)
	if err != nil {
		return err
	}
	what := fmt.Sprintf("a %s workflow (%q)", state.Kind, name)
	if task != "" {
		what = fmt.Sprintf("a %s workflow (%q, task %s)", state.Kind, name, task)
	}
	return output.NewError("workflow_in_progress",
		fmt.Sprintf("%s is already in progress at step %q", what, state.CurrentStep)).
		WithResource(name).
		WithState(state.CurrentStep, nil).
		WithNextAction(instruction)
}

// readState loads the persisted workflow state at statePath verbatim,
// regardless of whether it represents an in-progress or a finished
// workflow. It returns (nil, nil) when no state file exists, and an error
// only when the file exists but cannot be read or parsed. It never mutates
// anything on disk.
func readState(statePath string) (*workflow.State, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state workflow.State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}
	return &state, nil
}

// detectInProgress loads the persisted workflow state at statePath and returns
// it only when it represents an in-progress (resumable) workflow. It returns
// (nil, nil) when no state file exists or the state is not in progress (e.g.
// finished), and an error only when the file exists but cannot be read or
// parsed. It never mutates anything on disk.
func detectInProgress(statePath string) (*workflow.State, error) {
	state, err := readState(statePath)
	if err != nil || state == nil || !state.InProgress() {
		return nil, err
	}
	return state, nil
}

// resumeOrClear is the shared prologue for the three `new` commands. It decides
// what to do with any existing workflow state before a fresh workflow starts:
//
//   - force=true        → remove the state file and proceed fresh (handled=false).
//   - no in-progress    → remove any stale state file and proceed fresh (handled=false).
//   - in-progress, same kind → return a same-kind "workflow_in_progress" error
//     and handled=true; the caller returns immediately, leaving state untouched.
//   - in-progress, different kind → return a "cross_kind_workflow_in_progress"
//     error (continue that kind's workflow with its skill, or overwrite with
//     the requested kind via --force) and handled=true. The in-progress
//     workflow is never silently resumed or clobbered.
//   - in-progress, no kind → return an error; a kind-less (pre-feature) state
//     is never clobbered or guessed at.
//
// expectedKind is the kind of the `new` command being run ("spec"/"plan"/
// "implement"); command is the CLI invocation prefix rendered into the
// instruction.
func resumeOrClear(statePath, command, expectedKind string, force bool) (handled bool, err error) {
	handled, err = probeResume(statePath, command, expectedKind, force)
	if err != nil || handled {
		return handled, err
	}
	clearState(statePath)
	return false, nil
}

// probeResume is resumeOrClear's read-only half: it reports an in-progress
// workflow without touching disk. It is split out because the `new` commands
// grew a second prologue check — the uncommitted-changes gate — that must run
// before anything is written, including before the stale state file is
// deleted. A caller that has no such gate uses resumeOrClear above.
func probeResume(statePath, command, expectedKind string, force bool) (handled bool, err error) {
	if force {
		return false, nil
	}

	state, err := detectInProgress(statePath)
	if err != nil {
		return false, err
	}
	if state == nil {
		return false, nil
	}
	if state.Kind == "" {
		return false, fmt.Errorf("existing workflow state at %s has no kind marker; run with --force to start fresh, or finish the existing workflow with `goto`", statePath)
	}

	return true, emitResumeReport(command, expectedKind, state)
}

// clearState removes a state file that probeResume found nothing to resume
// in — a finished workflow's leftovers, or any state at all under --force.
// It is the first thing a `new` run writes, so every check that must see the
// project untouched has to run before it.
func clearState(statePath string) {
	_ = os.Remove(statePath)
}

// guardKind is the shared prologue for the `goto` and `status` commands. Those
// commands operate on the persisted state directly, so they must refuse to act
// on a workflow whose kind differs from the command's own kind — otherwise a
// `spec goto` would apply spec steps to a plan's state. It returns handled=true
// (with an error) whenever a different-kind workflow is recorded, whether
// still in progress or already finished: a finished other-kind workflow is
// not "no workflow" and must never be silently reinterpreted as this kind's
// workflow — the FSM engine loads state.json's CurrentStep/CompletedSteps
// verbatim with no kind check of its own, so letting it through here
// previously produced a nonsensical mashup (a plan's 20 completed step names
// reported under an implement command whose own step list only has 13,
// yielding e.g. "progress": "20/13"). A same-kind workflow (in progress or
// finished — the normal target either way), a kind-less legacy state, or no
// state at all all still return handled=false so the caller proceeds as
// before.
func guardKind(statePath, command, expectedKind string) (handled bool, err error) {
	state, err := readState(statePath)
	if err != nil {
		return false, err
	}
	if state == nil || state.Kind == "" || state.Kind == expectedKind {
		return false, nil
	}
	if state.InProgress() {
		return true, emitResumeReport(command, expectedKind, state)
	}
	return true, noActiveWorkflowAfterOtherKindFinished(command, expectedKind, state)
}

// noActiveWorkflowAfterOtherKindFinished builds guardKind's error for a
// different-kind workflow that has already finished: unlike the in-progress
// case, there is nothing to resume and nothing at risk of being clobbered —
// the correct next step for the requested kind is simply to start it.
func noActiveWorkflowAfterOtherKindFinished(command, expectedKind string, state *workflow.State) error {
	name, _ := state.Data["name"].(string)
	return output.NewError("no_active_workflow",
		fmt.Sprintf("no active %s workflow — the last workflow recorded here was a finished %s workflow (%q)", expectedKind, state.Kind, name)).
		WithNextAction(fmt.Sprintf("run `%s %s new` to start one", command, expectedKind))
}
