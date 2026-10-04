package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// TestSpecNew_CrossKindReturnsMismatchError asserts that running `spec new`
// while a *plan* workflow is in progress does not resume the plan as a spec:
// it fails with the shared cross_kind_workflow_in_progress error (naming both
// the in-progress kind and the requested kind) and leaves the plan's state
// untouched.
func TestSpecNew_CrossKindReturnsMismatchError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "plan",
		CurrentStep:    "discovery",
		CompletedSteps: []string{"new", "overview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"whatever"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "cross_kind_workflow_in_progress", er.Code, "must be reported as a cross-kind mismatch, not a same-kind resume")
	require.Equal(t, "000024_resume", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "discovery", er.State.Current)
	require.Contains(t, er.Message, "plan", "message must name the in-progress kind")
	require.Contains(t, er.Message, "spec", "message must name the requested kind")
	require.Contains(t, er.NextAction, "plan", "instruction must name the in-progress kind")
	require.Contains(t, er.NextAction, "spec new --force", "instruction must offer overwriting with the requested kind")

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a cross-kind new must not mutate the in-progress state")
}

// TestSpecGoto_CrossKindRefusesAndPreservesState asserts the CLI back door is
// closed: `spec goto` against an in-progress plan refuses (mismatch report)
// instead of applying spec steps to the plan's state, and does not advance it.
func TestSpecGoto_CrossKindRefusesAndPreservesState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "plan",
		CurrentStep:    "discovery",
		CompletedSteps: []string{"new", "overview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "goto", "--data", `{"step":"overview"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "cross_kind_workflow_in_progress", er.Code)
	require.Equal(t, "000024_resume", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "discovery", er.State.Current)
	require.Contains(t, er.Message, "plan")
	require.Contains(t, er.Message, "spec")

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a refused cross-kind goto must not advance the plan's state")
}

// TestImplementGoto_AfterOtherKindFinishedReportsNoActiveWorkflow is the
// `goto` counterpart: the same stale, finished cross-kind state must not be
// silently loaded into an implement FSM to attempt a transition against.
func TestImplementGoto_AfterOtherKindFinishedReportsNoActiveWorkflow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "plan",
		CurrentStep:    "finished",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000045_config-file-migration"},
	})

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "implement", "goto", "--data", `{"step":"analyze"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.Equal(t, "no_active_workflow", er.Code)
	require.Contains(t, er.NextAction, "implement new")
}

// TestMismatchInstruction_RendersBothPathsAcrossKinds asserts the cross-kind
// resume template names the in-progress kind, the requested kind, and both
// follow-up commands (continue the other kind, or overwrite with --force).
func TestMismatchInstruction_RendersBothPathsAcrossKinds(t *testing.T) {
	out, err := mismatchInstruction("spektacular", "plan", "spec", "000024_resume", "discovery")
	require.NoError(t, err)

	require.NotContains(t, out, "{{", "template must be fully rendered")
	require.Contains(t, out, "plan")
	require.Contains(t, out, "spec")
	require.Contains(t, out, "000024_resume")
	require.Contains(t, out, "discovery")
	// Continue the in-progress (plan) workflow with its own skill.
	require.Contains(t, out, `spektacular plan goto --data '{"step":"discovery","name":"000024_resume"}'`)
	// Or overwrite and start the requested (spec) workflow.
	require.Contains(t, out, "spektacular spec new --force")
	// Must not steer the agent to resume the plan as a spec.
	require.NotContains(t, out, "spec goto", "must not suggest resuming the other kind as a spec")
	require.True(t, strings.Contains(out, "in progress"), "must state a workflow is in progress")
}

// TestMismatchInstruction_SpecInProgressCarriesNoName asserts the cross-kind
// prompt resumes an in-progress spec without a name: only plan and implement
// gotos route by the spec name.
func TestMismatchInstruction_SpecInProgressCarriesNoName(t *testing.T) {
	out, err := mismatchInstruction("spektacular", "spec", "plan", "000024_resume", "overview")
	require.NoError(t, err)

	require.Contains(t, out, `spektacular spec goto --data '{"step":"overview"}'`)
	require.NotContains(t, out, `"name":`)
	require.Contains(t, out, "spektacular plan new --force")
}

// TestRepoGuidedAdd_RunsToCompletionBesideAnInProgressSpec is the isolation
// guarantee: the guided add keeps its own repo-state.json, so a complete add
// runs start to finish while a spec workflow is unfinished — never reporting a
// conflict of any kind — and leaves the spec's state.json byte-identical.
func TestRepoGuidedAdd_RunsToCompletionBesideAnInProgressSpec(t *testing.T) {
	project := repoProject(t)
	dataDir := filepath.Join(project, ".spektacular")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "requirements",
		CompletedSteps: []string{"new", "interview", "overview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	before, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)

	target := repoTargetDir(t)
	first := repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	require.Equal(t, "locate", first.Step, "a guided add must start normally beside an in-progress spec")

	for _, step := range []string{"name", "description", "role", "tags", "placement", "confirm", "register", "finished"} {
		require.Equal(t, step, repoWorkflowStep(t, "goto", "--data", repoGotoData(t, step, nil)).Step)
	}

	addState := readRepoWorkflowState(t, project)
	require.Equal(t, "repo", addState.Kind)
	require.Equal(t, "finished", addState.CurrentStep)
	require.Equal(t, repoGuidedSteps, addState.CompletedSteps)

	after, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
	require.NoError(t, err)
	require.Equal(t, before, after, "a guided add must not write a single byte of the shared state file")
}

// TestSpecStatus_ResumesAtItsOwnStepAfterAGuidedAdd asserts the spec is still
// exactly where it was left once a whole guided add has run alongside it: its
// status reports the same instance, step and completed steps as before.
func TestSpecStatus_ResumesAtItsOwnStepAfterAGuidedAdd(t *testing.T) {
	project := repoProject(t)
	dataDir := filepath.Join(project, ".spektacular")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "requirements",
		CompletedSteps: []string{"new", "interview", "overview"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})

	target := repoTargetDir(t)
	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	for _, step := range []string{"name", "description", "role", "tags", "placement", "confirm", "register", "finished"} {
		repoWorkflowStep(t, "goto", "--data", repoGotoData(t, step, nil))
	}

	stdout, _, code := runRootCmd(t, "status", "--format", "json")
	require.Equal(t, 0, code, stdout)

	var st struct {
		Workflow *struct {
			Kind           string   `json:"kind"`
			Name           string   `json:"name"`
			CurrentStep    string   `json:"current_step"`
			CompletedSteps []string `json:"completed_steps"`
		} `json:"workflow"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &st))
	require.NotNil(t, st.Workflow)
	require.Equal(t, "spec", st.Workflow.Kind)
	require.Equal(t, "000024_resume", st.Workflow.Name)
	require.Equal(t, "requirements", st.Workflow.CurrentStep, "the spec resumes at the step it was on")
	require.Equal(t, []string{"new", "interview", "overview"}, st.Workflow.CompletedSteps)
}

// TestSpecNew_NotRefusedWhileAGuidedAddIsInProgress asserts the isolation runs
// the other way too: an unfinished guided add is not a workflow that contends
// for the shared slot, so `spec new` starts normally rather than reporting a
// conflict — and the add is left resting where it was.
func TestSpecNew_NotRefusedWhileAGuidedAddIsInProgress(t *testing.T) {
	project := repoProject(t)

	target := repoTargetDir(t)
	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	require.Equal(t, "name", repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", nil)).Step)

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing-export"}`)
	require.Equal(t, 0, code, "an in-progress guided add must not block a new spec; got %s", stdout)

	var result specCommandResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, "new", result.Step)
	require.Equal(t, "specs/"+result.SpecName+".md", result.SpecPath)
	require.FileExists(t, filepath.Join(project, ".spektacular", result.SpecPath))

	addState := readRepoWorkflowState(t, project)
	require.Equal(t, "repo", addState.Kind)
	require.Equal(t, "name", addState.CurrentStep, "starting a spec must not disturb the guided add")
}
