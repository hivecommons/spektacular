package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// writeFixturePlan creates a minimal plan.md file inside the fake data dir so
// the implement workflow's plan-exists precondition passes.
func writeFixturePlan(t *testing.T, dataDir, name string) string {
	t.Helper()
	planDir := filepath.Join(dataDir, "plans", name)
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	planPath := filepath.Join(planDir, "plan.md")
	body := `# Plan: ` + name + `

## Overview

fixture

## Milestones & Phases

#### - [ ] Phase 1.1: First

#### - [ ] Phase 1.2: Second

#### - [x] Phase 1.3: Already done
`
	require.NoError(t, os.WriteFile(planPath, []byte(body), 0o644))
	return planPath
}

// writeInProgressState marshals a workflow.State to .spektacular/state.json so a
// `new` command's resume prologue (resumeOrClear) sees an in-progress workflow.
func writeInProgressState(t *testing.T, dataDir string, st workflow.State) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	b, err := json.MarshalIndent(st, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "state.json"), b, 0o644))
}

// setupImplementCmd resets rootCmd state for a clean test invocation.
func setupImplementCmd(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	resetRootCmd(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	return stdout, stderr
}

func TestImplementNew_RejectsMissingPlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"nosuch"}`})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan file not found")
}

func TestImplementNew_RejectsInvalidName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"Invalid Name"}`})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "name must match")
}

func TestImplementNew_SucceedsWithExistingPlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeFixturePlan(t, dataDir, "fixture")

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})

	require.NoError(t, rootCmd.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "read_plan", result["step"])
	require.Equal(t, "fixture", result["plan_name"])
	require.Equal(t, "plans/fixture/plan.md", result["plan_path"])
	require.Equal(t, "plan", result["plan_document"])
	require.NotEmpty(t, result["instruction"])
}

func TestImplementNew_StrictModeRejectsStalePlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "plan:\n  strict_spec_changes: true\n")
	writeArtifactStatusFile(t, filepath.Join(dataDir, "specs", "fixture.md"), "---\ncreated_date: 2026-02-01\ndocument_status: final\n---\n", time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC))
	writeArtifactStatusFile(t, filepath.Join(dataDir, "plans", "fixture", "plan.md"), "---\ncreated_date: 2026-02-01\ndocument_status: final\nclosed_date: 2026-02-02\nspec: fixture\n---\n", time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC))

	var er output.ErrorResponse
	stdout, stderr, code := runRootCmd(t, "implement", "new", "--data", `{"name":"fixture"}`)
	require.Equal(t, 1, code)
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.Equal(t, "plan_stale", er.Code)
	require.Contains(t, er.NextAction, "re-run the plan workflow")
}

func TestImplementGoto_RequiresActiveWorkflow(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--data", `{"step":"analyze"}`})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "no active implement workflow")
}

func TestImplementGoto_AdvancesThroughStep(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeFixturePlan(t, dataDir, "fixture")

	// Start the workflow — state file is written with name=fixture.
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})
	require.NoError(t, rootCmd.Execute())

	// Now goto analyze — should produce an analyze instruction.
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--data", `{"step":"analyze"}`})
	require.NoError(t, rootCmd.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "analyze", result["step"])
}

func TestImplementGoto_UpdateFeatureChangelogInstructsChangelogWrite(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeFixturePlan(t, dataDir, "fixture")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})
	require.NoError(t, rootCmd.Execute())

	steps := []string{
		"analyze",
		"implement",
		"test",
		"verify",
		"update_plan",
		"update_changelog",
		"test_plan",
		"update_feature_changelog",
	}
	var stdout *bytes.Buffer
	for _, step := range steps {
		stdout, _ = setupImplementCmd(t)
		rootCmd.SetArgs([]string{"implement", "goto", "--data", `{"step":"` + step + `"}`})
		require.NoError(t, rootCmd.Execute())
	}

	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	require.Equal(t, "update_feature_changelog", result["step"])
	instruction, ok := result["instruction"].(string)
	require.True(t, ok)
	require.Contains(t, instruction, "changelog file write")
	require.Contains(t, instruction, ".spektacular/tmp/changelog_project.md")
}

func TestImplementStatus_ReportsUncheckedPhases(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeFixturePlan(t, dataDir, "fixture")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})
	require.NoError(t, rootCmd.Execute())

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "status"})
	require.NoError(t, rootCmd.Execute())

	var status map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &status))
	// Fixture has 2 unchecked phases (1.1, 1.2) and 1 checked (1.3).
	require.EqualValues(t, 2, status["unchecked_phases"])
	require.Equal(t, "fixture", status["plan_name"])
	require.EqualValues(t, 12, status["total_steps"])
}

func TestImplementStatus_CountsUncheckedTasksInTaskPlan(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan", taskPlanDoc(
		taskBlock("Done", true, agentFields(idA)),
		taskBlock("Open one", false, agentFields(idB))+taskBlock("Open two", false, agentFields(idC)),
	))
	require.Equal(t, 0, code)

	_, _, code = runRootCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	require.Equal(t, 0, code)
	stdout, _, code := runRootCmd(t, "implement", "status")
	require.Equal(t, 0, code)

	var status map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &status))
	require.EqualValues(t, 2, status["unchecked_phases"], "the existing field counts open tasks")
}

func TestImplementSteps_ListsAllSteps(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular"), 0o755))

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "steps"})
	require.NoError(t, rootCmd.Execute())

	var result map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	steps := result["steps"].([]any)
	require.Len(t, steps, 12)
	expected := []string{
		"new",
		"read_plan",
		"analyze",
		"implement",
		"test",
		"verify",
		"update_plan",
		"update_changelog",
		"test_plan",
		"update_feature_changelog",
		"reconcile_spec",
		"finished",
	}
	for i, want := range expected {
		require.Equal(t, want, steps[i])
	}
}

// TestImplementNew_InProgressReturnsWorkflowInProgressError proves `implement
// new` runs the shared resume prologue and fails with the shared
// workflow_in_progress error for an in-progress implement workflow instead of
// starting fresh.
func TestImplementNew_InProgressReturnsWorkflowInProgressError(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, "")
	writeFixturePlan(t, dataDir, "fixture")

	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "implement",
		CurrentStep:    "analyze",
		CompletedSteps: []string{"new", "read_plan"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "fixture"},
	})

	stdout, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"fixture"}`)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "fixture", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "analyze", er.State.Current)
}

func TestImplementNew_SchemaOutput(t *testing.T) {
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"name"`)
	require.Contains(t, stdout.String(), `"plan_path"`)
}

func TestImplementGoto_SchemaOutput(t *testing.T) {
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"step"`)
	// The enum should list every step name and none of the removed ones.
	require.Contains(t, stdout.String(), "read_plan")
	require.Contains(t, stdout.String(), "test_plan")
	require.NotContains(t, stdout.String(), "update_repo_changelog")
}

func TestImplementStatus_SchemaOutput(t *testing.T) {
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "status", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"unchecked_phases"`)
}

func TestImplementSteps_SchemaOutput(t *testing.T) {
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "steps", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"steps"`)
}

// `implement new` and `implement goto` carry no repo roster in state.json:
// where each repo's code lives is reported live by `repo list`, so neither
// command resolves a source, and neither clones or runs any git at all —
// including for a member repo whose git source has never been cloned.
func TestImplementNewAndGoto_PersistNoRosterAndRunNoGit(t *testing.T) {
	resetRootCmd(t) // the schema/data flags persist on rootCmd across tests
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir,
		"repos:\n"+
			"  - name: core\n"+
			"    location: .\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")
	writeFixturePlan(t, dataDir, "fixture")

	// The colocated repo's footprint sits in .spektacular, so its code is
	// that folder's parent: the project directory itself.
	coreCfg := config.NewDefaultRepoConfig()
	coreCfg.Source = config.DefaultRepoSource
	require.NoError(t, coreCfg.ToYAMLFile(
		filepath.Join(dataDir, config.RepoConfigFileName)))

	// The member's footprint at its registered location declares a git
	// source that has never been cloned.
	apiLocation := filepath.Join(dir, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(apiLocation, ".spektacular"), 0o755))
	apiCfg := config.NewDefaultRepoConfig()
	apiCfg.Source = config.GitSource("https://example.com/api.git")
	require.NoError(t, apiCfg.ToYAMLFile(
		filepath.Join(apiLocation, ".spektacular", config.RepoConfigFileName)))

	git := &stubGit{}
	swapRepoGit(t, git)

	assertNoRoster := func(after string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
		require.NoError(t, err, "state.json must exist after %s", after)
		var st workflow.State
		require.NoError(t, json.Unmarshal(raw, &st))

		_, ok := st.Data["repos"]
		require.False(t, ok, "state.json must carry no repo roster after %s", after)

		require.Empty(t, git.clones, "%s must not clone", after)
		require.Zero(t, git.calls, "%s must not run git at all", after)
		require.NoDirExists(t, filepath.Join(dataDir, "repos", "api"), "%s must leave no clone on disk", after)
	}

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})
	require.NoError(t, rootCmd.Execute())
	assertNoRoster("implement new")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--data", `{"step":"read_plan"}`})
	require.NoError(t, rootCmd.Execute())
	assertNoRoster("implement goto")
}
