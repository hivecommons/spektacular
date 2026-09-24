package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// repoWorkflowResult mirrors the JSON envelope both guided-add subcommands
// emit (internal/steps/repo.Result).
type repoWorkflowResult struct {
	Step        string `json:"step"`
	RepoPath    string `json:"repo_path"`
	RepoName    string `json:"repo_name"`
	Instruction string `json:"instruction"`
}

// repoGuidedSteps is the hand-maintained list of every step in the guided add,
// in the order a straight-through walk visits them. It is deliberately written
// out here rather than derived from reposteps.Steps() so the test carries its
// own independent oracle for the step table.
var repoGuidedSteps = []string{
	"new", "locate", "name", "description", "role", "tags",
	"placement", "confirm", "register", "finished",
}

// runRepoWorkflow invokes a repo subcommand through runRoot — the same wrapper
// Execute uses — after clearing the package-global cobra flags, and returns the
// captured stdout together with the process exit code.
func runRepoWorkflow(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	resetRootCmd(t)
	out, errBuf, code := runRootCmd(t, append([]string{"repo"}, args...)...)
	require.Empty(t, errBuf, "the repo workflow commands report on stdout only")
	return out, code
}

// repoWorkflowStep runs a repo subcommand, requires it to succeed, and decodes
// the step result it emitted.
func repoWorkflowStep(t *testing.T, args ...string) repoWorkflowResult {
	t.Helper()
	stdout, code := runRepoWorkflow(t, args...)
	require.Equal(t, 0, code, "expected the guided add command to succeed; got %s", stdout)
	var result repoWorkflowResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	return result
}

// repoWorkflowError runs a repo subcommand, requires it to fail, and decodes
// the shared error envelope it emitted.
func repoWorkflowError(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	stdout, code := runRepoWorkflow(t, args...)
	require.Equal(t, 1, code, "expected the guided add command to fail; got %s", stdout)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// repoGotoData builds a `repo goto` --data payload with the step plus any
// answers being supplied alongside it, marshaled so paths escape correctly.
func repoGotoData(t *testing.T, step string, answers map[string]any) string {
	t.Helper()
	fields := map[string]any{"step": step}
	for k, v := range answers {
		fields[k] = v
	}
	b, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(b)
}

// readRepoWorkflowState loads the guided add's own state file from a project
// root. It is a sibling of state.json, never the same file.
func readRepoWorkflowState(t *testing.T, project string) workflow.State {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(project, ".spektacular", "repo-state.json"))
	require.NoError(t, err)
	var st workflow.State
	require.NoError(t, json.Unmarshal(raw, &st))
	return st
}

// repoTargetDir creates a folder named `docs` inside a fresh temp dir and
// returns its absolute path, so the guided add has a stable, readable base
// name to seed its provisional repo name from.
func repoTargetDir(t *testing.T) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "docs")
	require.NoError(t, os.MkdirAll(target, 0o755))
	return target
}

// A guided add started with `repo new` opens on the `locate` question rather
// than on the internal `new` preamble, walks every remaining step through
// `repo goto`, and records its whole journey in its own repo-state.json —
// kind "repo", resting at finished, with all ten steps completed.
func TestRepoNew_WalksEveryStepToFinishedInItsOwnStateFile(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	first := repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	require.Equal(t, "locate", first.Step, "the internal `new` preamble must never be rendered")
	require.Equal(t, target, first.RepoPath)
	require.Equal(t, "docs", first.RepoName, "locate seeds a provisional name from the target folder")
	require.NotEmpty(t, first.Instruction)

	for _, step := range []string{"name", "description", "role", "tags", "placement", "confirm", "register", "finished"} {
		result := repoWorkflowStep(t, "goto", "--data", repoGotoData(t, step, nil))
		require.Equal(t, step, result.Step)
		require.Equal(t, target, result.RepoPath)
	}

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "repo", st.Kind)
	require.Equal(t, "finished", st.CurrentStep)
	require.Equal(t, repoGuidedSteps, st.CompletedSteps)

	require.NoFileExists(t, filepath.Join(project, ".spektacular", "state.json"),
		"the guided add must never touch the state file spec, plan and implement share")
}

// A `repo goto` naming a step that is not reachable from the current one is
// refused as invalid_transition, naming both steps, and carries a next action
// spelling out the goto that would actually succeed. The refusal leaves the
// add resting where it was.
func TestRepoGoto_OutOfOrderStepIsRefusedWithNextAction(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	require.Equal(t, "locate", repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target})).Step)

	er := repoWorkflowError(t, "goto", "--data", repoGotoData(t, "confirm", nil))
	require.Equal(t, "invalid_transition", er.Code)
	require.Contains(t, er.Message, "confirm", "the message must name the step that was asked for")
	require.Contains(t, er.Message, "locate", "the message must name the step the add is resting at")
	require.NotEmpty(t, er.NextAction)
	require.Contains(t, er.NextAction, `spektacular repo goto --data '{"step":"name"}'`,
		"the next action must spell out the transition that would succeed")

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "locate", st.CurrentStep, "a refused goto must not advance the add")
	require.Equal(t, []string{"new"}, st.CompletedSteps)
}

// Starting a second guided add while one is unfinished returns the shared
// resume report — coded workflow_in_progress, naming the add's provisional
// name and the step it stopped at — instead of silently restarting.
func TestRepoNew_SecondAddWhileOneIsInProgressReportsResume(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	require.Equal(t, "locate", repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target})).Step)
	require.Equal(t, "name", repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", nil)).Step)

	er := repoWorkflowError(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "docs", er.Resource)
	require.NotNil(t, er.State)
	require.Equal(t, "name", er.State.Current)
	require.Contains(t, er.Message, "repo", "the message must name the in-progress kind")
	require.Contains(t, er.Message, "name", "the message must name the in-progress step")
	require.Contains(t, er.NextAction, `spektacular repo goto --data '{"step":"name"}'`)
	require.Contains(t, er.NextAction, "spektacular repo new --force")

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "name", st.CurrentStep, "a refused new must not disturb the in-progress add")
}

// `repo new --force` discards an unfinished guided add and starts a fresh one
// back at the locate question, with the earlier add's progress gone.
func TestRepoNew_ForceDiscardsInProgressAddAndRestartsAtLocate(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", map[string]any{"name": "docs-site"}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "description", nil))

	restarted := repoWorkflowStep(t, "new", "--force", "--data", repoAddJSON(t, map[string]any{"location": target}))
	require.Equal(t, "locate", restarted.Step)
	require.Equal(t, "docs", restarted.RepoName, "the discarded add's agreed name must not survive --force")

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "repo", st.Kind)
	require.Equal(t, "locate", st.CurrentStep)
	require.Equal(t, []string{"new"}, st.CompletedSteps)
}

// Answers handed to one `repo goto` are still there several commands later:
// each invocation rebuilds the workflow from repo-state.json, so the values
// gathered so far survive in the persisted data.
func TestRepoGoto_AnswersPersistAcrossRebuildsOfTheWorkflow(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", map[string]any{"name": "docs-site"}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "description", map[string]any{"description": "the documentation repo"}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "role", map[string]any{"role": "documentation"}))
	final := repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "tags", map[string]any{"tags": []string{"docs", "markdown"}}))

	require.Equal(t, "docs-site", final.RepoName, "the agreed name supersedes the provisional one")

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "tags", st.CurrentStep)
	require.Equal(t, target, st.Data["location"])
	require.Equal(t, "docs-site", st.Data["name"])
	require.Equal(t, "the documentation repo", st.Data["description"])
	require.Equal(t, "documentation", st.Data["role"])
	require.Equal(t, []any{"docs", "markdown"}, st.Data["tags"])
}

// The delegation shortcut: a user who answers the first proposal by handing
// over the whole set advances from name straight to placement in a single
// goto. The skipped questions are never marked completed, and every value
// supplied in that one call is persisted.
func TestRepoGoto_DelegationShortcutJumpsFromNameToPlacement(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", nil))

	result := repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "placement", map[string]any{
		"name":        "docs-site",
		"description": "the documentation repo",
		"role":        "documentation",
		"tags":        []string{"docs", "markdown"},
	}))
	require.Equal(t, "placement", result.Step)
	require.Equal(t, "docs-site", result.RepoName)

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "placement", st.CurrentStep)
	require.Equal(t, []string{"new", "locate", "name"}, st.CompletedSteps)
	require.NotContains(t, st.CompletedSteps, "description")
	require.NotContains(t, st.CompletedSteps, "role")
	require.NotContains(t, st.CompletedSteps, "tags")

	require.Equal(t, "docs-site", st.Data["name"])
	require.Equal(t, "the documentation repo", st.Data["description"])
	require.Equal(t, "documentation", st.Data["role"])
	require.Equal(t, []any{"docs", "markdown"}, st.Data["tags"])
}

// `repo new --schema` declares the one optional input the guided add accepts
// and the step result both subcommands return.
func TestRepoNew_SchemaDocumentsLocationInputAndStepOutput(t *testing.T) {
	repoProject(t)

	stdout, code := runRepoWorkflow(t, "new", "--schema")
	require.Equal(t, 0, code)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Input)
	require.Contains(t, schema.Input.Properties, "location")
	require.Empty(t, schema.Input.Required, "an add can be started cold, so nothing is required")
	require.NotNil(t, schema.Output)
	for _, field := range []string{"step", "repo_path", "repo_name", "instruction"} {
		require.Contains(t, schema.Output.Properties, field)
	}
}

// `repo goto --schema` requires a step and advertises the full step
// vocabulary, so a driving agent can see every move the guided add supports.
func TestRepoGoto_SchemaEnumeratesEveryStep(t *testing.T) {
	repoProject(t)

	stdout, code := runRepoWorkflow(t, "goto", "--schema")
	require.Equal(t, 0, code)

	var schema commandSchema
	require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
	require.NotNil(t, schema.Input)
	require.Equal(t, []string{"step"}, schema.Input.Required)
	require.Contains(t, schema.Input.Properties, "step")
	require.ElementsMatch(t, repoGuidedSteps, schema.Input.Properties["step"].Enum)
	require.NotNil(t, schema.Output)
	for _, field := range []string{"step", "repo_path", "repo_name", "instruction"} {
		require.Contains(t, schema.Output.Properties, field)
	}
}

// The metadata an agent and user agree on during the guided adds below. Every
// value is hand-written here and asserted verbatim where it lands, so nothing
// a test expects is derived from what the workflow produced.
const (
	agreedRepoName        = "docs-site"
	agreedRepoDescription = "the documentation repo"
	agreedRepoRole        = "documentation"
)

var agreedRepoTags = []string{"docs", "markdown"}

// walkRepoAddToConfirm drives a guided add from `repo new` through every
// question to the confirm step, answering each one with the agreed metadata
// above. A non-empty placement is recorded at the placement question;
// otherwise the step is passed with no answer, leaving the default in force.
func walkRepoAddToConfirm(t *testing.T, target, placement string) {
	t.Helper()

	repoWorkflowStep(t, "new", "--data", repoAddJSON(t, map[string]any{"location": target}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "name", map[string]any{"name": agreedRepoName}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "description", map[string]any{"description": agreedRepoDescription}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "role", map[string]any{"role": agreedRepoRole}))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "tags", map[string]any{"tags": agreedRepoTags}))

	answers := map[string]any{}
	if placement != "" {
		answers["placement"] = placement
	}
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "placement", answers))
	repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "confirm", nil))
}

// repoRegistrationRecord reads back what the register step cached in the
// guided add's own state file: the outcome the closing instructions report.
func repoRegistrationRecord(t *testing.T, project string) map[string]any {
	t.Helper()
	st := readRepoWorkflowState(t, project)
	raw, ok := st.Data["registration"]
	require.True(t, ok, "the register step must record its outcome in workflow data")
	record, ok := raw.(map[string]any)
	require.True(t, ok, "the recorded outcome must survive the state file's JSON round trip as an object")
	return record
}

// readOnlyRepoDir returns a directory named `docs` that nothing can be
// created inside, so registration's attempt to scaffold a footprint there
// genuinely fails. Root bypasses the permission bits, so the test that uses
// it is skipped there; the mode is restored on cleanup so t.TempDir's own
// removal can still descend into the folder.
func readOnlyRepoDir(t *testing.T) string {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission bits, so a read-only directory cannot be simulated")
	}

	dir := filepath.Join(t.TempDir(), "docs")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	return dir
}

// A guided add walked all the way to the confirm question has written
// nothing at all: the target repo has no .spektacular folder, and the
// project's config.yaml is byte for byte what it was before the add started.
// Every question up to that point gathers; only the user's confirmation
// causes anything to be written.
func TestRepoGuidedAdd_NothingIsWrittenBeforeTheUserConfirms(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	configPath := filepath.Join(project, ".spektacular", "config.yaml")
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)

	walkRepoAddToConfirm(t, target, "")

	require.NoDirExists(t, filepath.Join(target, ".spektacular"),
		"the target repo must be untouched until the add is confirmed")

	after, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after),
		"the project registry must be byte-identical until the add is confirmed")

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "confirm", st.CurrentStep)
}

// Confirming the add registers the repo with exactly the metadata the
// conversation gathered: the project registry gains an entry under the agreed
// name pointing at the footprint scaffolded inside the target, that
// footprint's repo.yaml carries the agreed description, role and tags, and
// the outcome carries no metadata note because the repo is fully described.
func TestRepoGuidedAdd_ConfirmingRegistersWithTheGatheredMetadata(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	walkRepoAddToConfirm(t, target, "")
	require.Equal(t, "register", repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "register", nil)).Step)

	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2, "the project entry plus the repo the add registered")
	entry := cfg.Repos[1]
	require.Equal(t, agreedRepoName, entry.Name)
	require.Equal(t, filepath.Join(target, ".spektacular"), entry.ResolvedLocation(project),
		"the registered location must resolve to the footprint inside the target repo")

	require.ElementsMatch(t, minimalFootprint, listPaths(t, target),
		"confirming scaffolds the minimal footprint inside the target and nothing more")

	targetCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(target, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, agreedRepoDescription, targetCfg.Description)
	require.Equal(t, agreedRepoRole, targetCfg.Role)
	require.Equal(t, agreedRepoTags, targetCfg.Tags)

	record := repoRegistrationRecord(t, project)
	require.Equal(t, true, record["registered"])
	require.Equal(t, "created", record["footprint"])
	require.Equal(t, "", record["metadata_note"],
		"a repo registered with a description, a role and tags needs no metadata warning")
}

// Recording the project placement puts the project's files for the repo under
// the project instead of inside the repo: the target repo is left holding
// nothing but its own code, the footprint is scaffolded under the project,
// and that footprint's repo.yaml names the target as the source its code
// lives at.
func TestRepoGuidedAdd_ProjectPlacementLeavesTheTargetHoldingOnlyItsCode(t *testing.T) {
	project := repoProject(t)
	target := repoTargetDir(t)

	walkRepoAddToConfirm(t, target, "project")
	require.Equal(t, "register", repoWorkflowStep(t, "goto", "--data", repoGotoData(t, "register", nil)).Step)

	require.NoDirExists(t, filepath.Join(target, ".spektacular"),
		"project placement must leave the target repo holding nothing but its code")

	footprintHome := filepath.Join(project, "repos", agreedRepoName)
	require.ElementsMatch(t, minimalFootprint, listPaths(t, footprintHome),
		"project placement scaffolds the minimal footprint under the project instead")

	cfg, err := config.FromYAMLFile(filepath.Join(project, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 2, "the project entry plus the repo the add registered")
	require.Equal(t, filepath.Join(footprintHome, ".spektacular"), cfg.Repos[1].ResolvedLocation(project))

	repoCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(footprintHome, ".spektacular", config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, "file", repoCfg.Source.Provider)
	require.Equal(t, target, repoCfg.Source.Config.Location,
		"a footprint kept under the project must name the folder the code actually lives in")
	require.Equal(t, agreedRepoDescription, repoCfg.Description)
	require.Equal(t, agreedRepoRole, repoCfg.Role)
	require.Equal(t, agreedRepoTags, repoCfg.Tags)
}

// Registration runs before the transition it belongs to is committed, so a
// register that genuinely fails — here because the target repo cannot be
// written to at all — vetoes the move and leaves the add still standing on
// confirm, where the user can change an answer and try again.
func TestRepoGuidedAdd_FailingRegisterLeavesTheAddStandingOnConfirm(t *testing.T) {
	project := repoProject(t)
	target := readOnlyRepoDir(t)

	walkRepoAddToConfirm(t, target, "")

	er := repoWorkflowError(t, "goto", "--data", repoGotoData(t, "register", nil))
	require.NotEmpty(t, er.Message)

	st := readRepoWorkflowState(t, project)
	require.Equal(t, "confirm", st.CurrentStep, "a failed register must not advance the add")
	require.NotContains(t, st.CompletedSteps, "register")
}
