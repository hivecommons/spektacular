package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// implementSharedSlotConfig turns implement.worktrees off, for a fixture that
// exercises an implement run in the main checkout and the shared state.json
// slot rather than a lane of its own.
const implementSharedSlotConfig = "implement:\n  worktrees: false\n"

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
	require.Contains(t, err.Error(), "has no plan")
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
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
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

// strictPlanConfig turns plan.strict_spec_changes on.
const strictPlanConfig = "plan:\n  strict_spec_changes: true\n"

// The two modification times the strict-mode amendment fixtures pin: the plan
// was approved on the earlier one and the spec last written on the later, so
// an mtime comparison alone would always call the plan stale.
var (
	strictPlanModTime = time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC)
	strictSpecModTime = time.Date(2026, time.February, 3, 0, 0, 0, 0, time.UTC)
)

// strictSpecFrontmatter is a stored, approved spec's frontmatter with no
// design references, for the strict-mode amendment fixtures.
const strictSpecFrontmatter = "---\ncreated_date: 2026-01-15\ndocument_status: final\n---\n\n"

// strictAmendProject lays out a project in a t.TempDir() and chdirs into it,
// with plan.strict_spec_changes on plus extraConfig, a stored final spec named
// name holding specAmendBody, and a final plan for it. The plan's mtime is
// pinned to strictPlanModTime and the spec's to strictSpecModTime, so the spec
// is strictly newer than the plan. It returns the .spektacular directory and
// the spec's path.
func strictAmendProject(t *testing.T, name, extraConfig string) (dataDir, specPath string) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir = filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, strictPlanConfig+extraConfig)
	specPath = writeSpecFixture(t, dir, name, strictSpecFrontmatter+specAmendBody)
	planPath := writeFixturePlan(t, dataDir, name)
	planBody := readBytes(t, planPath)
	planFM := "---\ncreated_date: 2026-01-20\ndocument_status: final\nclosed_date: 2026-01-21\nspec: " + name + "\n---\n\n"
	require.NoError(t, os.WriteFile(planPath, append([]byte(planFM), planBody...), 0o644))
	require.NoError(t, os.Chtimes(planPath, strictPlanModTime, strictPlanModTime))
	pinSpecNewer(t, specPath)
	return dataDir, specPath
}

// pinSpecNewer sets the spec's mtime to strictSpecModTime, after the plan's,
// so a staleness check that fell back to mtimes would fire.
func pinSpecNewer(t *testing.T, specPath string) {
	t.Helper()
	require.NoError(t, os.Chtimes(specPath, strictSpecModTime, strictSpecModTime))
}

// amendSuccessMetricViaCLI records a Success Metrics amendment on name through
// `spec amend`, staging the stored spec (frontmatter included) with the
// metric changed, then pins the spec newer than its plan again.
func amendSuccessMetricViaCLI(t *testing.T, name, specPath string) specAmendResult {
	t.Helper()
	stored := string(readBytes(t, specPath))
	amended := strings.Replace(stored, "99% of charges succeed first time", "97% of charges succeed first time", 1)
	require.NotEqual(t, stored, amended, "the fixture spec must carry the metric being amended")
	got := amendSpecOK(t, "--data", specAmendData(name, "99% is unreachable with the current gateway", "implement run, task 1.1", ""), "--from", stageSpecAmend(t, amended))
	require.Equal(t, []string{"Success Metrics"}, got.AmendedSections)
	pinSpecNewer(t, specPath)
	return got
}

// editSpecUnrecordedViaCLI changes the spec's preamble through
// `spec file write`, a change no amendment records, then pins the spec newer
// than its plan again.
func editSpecUnrecordedViaCLI(t *testing.T, name, specPath string) {
	t.Helper()
	stored := string(readBytes(t, specPath))
	edited := strings.Replace(stored, "Charge customers for their usage.", "Charge customers for their metered usage.", 1)
	require.NotEqual(t, stored, edited, "the fixture spec must carry the preamble being edited")
	resetRootCmd(t)
	runOK(t, "spec", "file", "write", name, "--from", stageSpecAmend(t, edited))
	pinSpecNewer(t, specPath)
}

// requirePlanStale runs args, requires the run to be refused, and asserts the
// refusal is plan_stale.
func requirePlanStale(t *testing.T, args ...string) {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equalf(t, 1, code, "%v must be refused: %s", args, stdout)
	require.Empty(t, stderr)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.Equal(t, "plan_stale", er.Code)
}

// Criterion: in strict mode, a spec newer than its final plan is refused until
// the change is recorded with `spec amend`; once it is, an interactive
// implement run starts and advances, although the spec is still newer than
// the plan.
func TestImplementNew_StrictModeAcceptsRecordedAmendment(t *testing.T) {
	_, specPath := strictAmendProject(t, "fixture", implementSharedSlotConfig)

	// The mtimes alone make the plan stale before anything is recorded.
	requirePlanStale(t, "implement", "new", "--data", `{"name":"fixture"}`)

	amendSuccessMetricViaCLI(t, "fixture", specPath)

	resetRootCmd(t)
	stdout := runOK(t, "implement", "new", "--data", `{"name":"fixture"}`)
	var started map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &started))
	require.Equal(t, "read_plan", started["step"])
	require.Equal(t, "fixture", started["plan_name"])

	resetRootCmd(t)
	stdout = runOK(t, "implement", "goto", "--data", `{"step":"analyze"}`)
	var advanced map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &advanced))
	require.Equal(t, "analyze", advanced["step"])
}

// Criterion: in strict mode, a spec edit made with `spec file write` after a
// recorded amendment is not covered by it, so implement is refused as stale.
func TestImplementNew_StrictModeStillRejectsUnrecordedEdit(t *testing.T) {
	dataDir, specPath := strictAmendProject(t, "fixture", implementSharedSlotConfig)
	amendSuccessMetricViaCLI(t, "fixture", specPath)
	editSpecUnrecordedViaCLI(t, "fixture", specPath)

	requirePlanStale(t, "implement", "new", "--data", `{"name":"fixture"}`)
	require.NoFileExists(t, stateFilePath(dataDir), "a refused start writes no workflow state")
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
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
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
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
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
	require.Contains(t, instruction, ".spektacular/tmp/fixture/changelog_project.md")
	require.NotContains(t, instruction, ".spektacular/tmp/changelog_project.md",
		"the project record must be staged in the spec's own scratch folder")
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
	t.Chdir(t.TempDir())
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"name"`)
	require.Contains(t, stdout.String(), `"plan_path"`)
}

func TestImplementGoto_SchemaOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--schema"})
	require.NoError(t, rootCmd.Execute())
	require.Contains(t, stdout.String(), `"step"`)
	// The enum should list every step name and none of the removed ones.
	require.Contains(t, stdout.String(), "read_plan")
	require.Contains(t, stdout.String(), "test_plan")
	require.NotContains(t, stdout.String(), "update_repo_changelog")
}

func TestImplementSteps_SchemaOutput(t *testing.T) {
	t.Chdir(t.TempDir())
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
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig+
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

// A spec built in its own worktrees has a worktree record in the project.
// implement new and implement goto read it as a plain file: they still run no
// git and persist no repo roster, while the rendered instructions name each
// recorded repo's worktree code root as where its code lives.
func TestImplementNewAndGoto_WorktreeRecordNamesCodeRootsWithoutGit(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)
	dataDir := filepath.Join(dir, ".spektacular")
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig+
		"repos:\n"+
		"  - name: core\n"+
		"    location: .\n"+
		"  - name: api\n"+
		"    location: ../repos/api/.spektacular\n")
	writeFixturePlan(t, dataDir, "fixture")

	coreCfg := config.NewDefaultRepoConfig()
	coreCfg.Source = config.DefaultRepoSource
	require.NoError(t, coreCfg.ToYAMLFile(
		filepath.Join(dataDir, config.RepoConfigFileName)))

	apiLocation := filepath.Join(dir, "repos", "api")
	require.NoError(t, os.MkdirAll(filepath.Join(apiLocation, ".spektacular"), 0o755))
	apiCfg := config.NewDefaultRepoConfig()
	apiCfg.Source = config.GitSource("https://example.com/api.git")
	require.NoError(t, apiCfg.ToYAMLFile(
		filepath.Join(apiLocation, ".spektacular", config.RepoConfigFileName)))

	// The spec's worktree record, written by hand as the worktree manager
	// would leave it: each registered repo mapped to an absolute code root.
	wtBase := t.TempDir()
	coreRoot := filepath.Join(wtBase, "core-fixture")
	apiRoot := filepath.Join(wtBase, "api-fixture")
	recordDir := filepath.Join(dataDir, "worktrees", "fixture")
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `{"spec":"fixture","repos":{"core":"` + coreRoot + `","api":"` + apiRoot + `"}}`
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "record.json"), []byte(record), 0o644))

	git := &stubGit{}
	swapRepoGit(t, git)

	assertNoGitNoRoster := func(after string) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(dataDir, "state.json"))
		require.NoError(t, err, "state.json must exist after %s", after)
		var st workflow.State
		require.NoError(t, json.Unmarshal(raw, &st))
		_, ok := st.Data["repos"]
		require.False(t, ok, "state.json must carry no repo roster after %s", after)
		_, ok = st.Data["worktree_roots"]
		require.False(t, ok, "state.json must not persist the worktree code roots after %s", after)
		require.Empty(t, git.clones, "%s must not clone", after)
		require.Zero(t, git.calls, "%s must not run git at all", after)
	}

	assertNamesRoots := func(out, after string) {
		t.Helper()
		require.Contains(t, out, "built in its own worktrees",
			"%s must say the spec is built in its own worktrees", after)
		require.Contains(t, out, "- `core`: `"+coreRoot+"`",
			"%s must name core's worktree code root", after)
		require.Contains(t, out, "- `api`: `"+apiRoot+"`",
			"%s must name api's worktree code root", after)
	}

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "new", "--data", `{"name":"fixture"}`})
	require.NoError(t, rootCmd.Execute())
	assertNoGitNoRoster("implement new")
	assertNamesRoots(stdout.String(), "implement new")
	require.NotContains(t, stdout.String(), "Run `spektacular repo list` now if you have not already",
		"implement new must not send a worktree spec to repo list for its code roots")

	stdout, _ = setupImplementCmd(t)
	rootCmd.SetArgs([]string{"implement", "goto", "--data", `{"step":"read_plan"}`})
	require.NoError(t, rootCmd.Execute())
	assertNoGitNoRoster("implement goto read_plan")
	assertNamesRoots(stdout.String(), "implement goto read_plan")
}
