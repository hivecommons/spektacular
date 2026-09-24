package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests drive the real CLI, end to end, against a real git binary: the
// automatic commit is the whole point of the feature, so faking git here
// would leave the thing under test unexercised. Only the commit-rejection
// test needs anything special, and it gets it from a real pre-commit hook
// rather than a stub. The autoCommitGit swap var is therefore never used.

// stagedCommitMessagePath is the project-relative path the git-commit
// instruction tells the agent to stage its message at, hand-copied here
// rather than read from the production constant.
const stagedCommitMessagePath = ".spektacular/tmp/git-commit-message.md"

// finishWithMessage is the --data payload that advances a workflow to its
// terminal step carrying the staged commit message.
const finishWithMessage = `{"step":"finished","commit_message_from":".spektacular/tmp/git-commit-message.md"}`

// gitFixtureProjectRepo and gitFixtureOtherRepo are the two repositories
// every fixture project registers, in registry order.
const (
	gitFixtureProjectRepo = "testproj"
	gitFixtureOtherRepo   = "other"
)

// gitFixture is a temp Spektacular project that is itself a git work tree,
// alongside a second registered repository in a work tree of its own.
type gitFixture struct {
	root  string // project root; the work tree of the "testproj" repo
	other string // the work tree of the "other" repo
}

// pinGitIdentity puts the test's git identity into the environment of the
// process under test. The production commit path builds its child
// environment from os.Environ(), so this is the identity it commits as —
// and with git's global and system configuration masked out, git has no
// other identity to fall back on.
func pinGitIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "spektacular-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@spektacular.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "spektacular-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@spektacular.invalid")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// tempWorkTree returns a fresh temp directory with symlinks resolved, so it
// compares equal to the work-tree root git reports for it.
func tempWorkTree(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return dir
}

// gitProject lays out a project registering two repositories — the project's
// own footprint and a second repo in a separate work tree — chdirs into the
// project root, and makes an initial commit in each work tree so a later
// commit is observable as a count that grew. An empty autoCommit leaves the
// key out of config.yaml entirely.
func gitProject(t *testing.T, autoCommit string) gitFixture {
	t.Helper()
	gittest.RequireGit(t)
	pinGitIdentity(t)

	root := tempWorkTree(t)
	other := tempWorkTree(t)

	// The second repo keeps its footprint in a .spektacular folder inside its
	// own code — the shape `repo add` produces — so its registered location
	// is that folder and its code is the folder's parent.
	footprint := filepath.Join(other, config.ProjectConfigDirName)
	require.NoError(t, os.MkdirAll(footprint, 0o755))
	rc := config.NewDefaultRepoConfig()
	rc.Source = config.DefaultRepoSource
	require.NoError(t, rc.ToYAMLFile(filepath.Join(footprint, config.RepoConfigFileName)))

	body := ""
	if autoCommit != "" {
		body += "auto_commit: " + autoCommit + "\n"
	}
	// counter ids make the resolved spec name deterministic, so the commit
	// message's oracle below can be written by hand.
	body += "spec:\n  id_method: counter\n"
	body += "repos:\n" +
		"  - name: " + gitFixtureProjectRepo + "\n    location: \".\"\n" +
		"  - name: " + gitFixtureOtherRepo + "\n    location: \"" + footprint + "\"\n"

	t.Chdir(root)
	writeSpecCommandConfig(t, root, body)

	for _, dir := range []string{root, other} {
		gittest.RunGit(t, dir, "init")
		gittest.RunGit(t, dir, "add", "-A")
		gittest.RunGit(t, dir, "commit", "-m", "initial")
	}
	return gitFixture{root: root, other: other}
}

// commitFixtures folds whatever a test seeded into the project after
// gitProject built it — plan documents, changelog records and the like —
// into the baseline commit. Those files are scaffolding for the scenario,
// not work the agent did, so they belong in the committed starting state.
// Without this the start gate added in Phase 2.1 correctly reports them as
// the user's uncommitted changes and refuses to start the workflow.
//
// It amends rather than adding a commit, so every work tree still starts on
// exactly one commit and the hand-written counts below stay true.
func commitFixtures(t *testing.T, fx gitFixture) {
	t.Helper()
	gittest.RunGit(t, fx.root, "add", "-A")
	gittest.RunGit(t, fx.root, "commit", "--amend", "--no-edit")
}

// commitCount is the number of commits reachable from HEAD, as a string so
// failures print git's own answer.
func commitCount(t *testing.T, dir string) string {
	t.Helper()
	return gittest.RunGit(t, dir, "rev-list", "--count", "HEAD")
}

// dirtyOtherRepo writes a file into the second repo's work tree, standing in
// for work an agent did in a repo other than the project's own.
func dirtyOtherRepo(t *testing.T, fx gitFixture) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fx.other, "work.txt"), []byte("agent work\n"), 0o644))
}

// stageCommitMessage writes body where the git-commit instruction tells the
// agent to stage it.
func stageCommitMessage(t *testing.T, fx gitFixture, body string) {
	t.Helper()
	path := filepath.Join(fx.root, filepath.FromSlash(stagedCommitMessagePath))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// currentStep reads the step the workflow state file says the workflow is on.
func currentStep(t *testing.T, fx gitFixture) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fx.root, config.ProjectConfigDirName, "state.json"))
	require.NoError(t, err)
	var st workflow.State
	require.NoError(t, json.Unmarshal(b, &st))
	return st.CurrentStep
}

// jsonDocuments counts the top-level JSON documents in s, so a failure that
// both buffered step output and an error envelope would print twice can be
// told from one that prints once.
func jsonDocuments(t *testing.T, s string) int {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	n := 0
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if err == io.EOF {
			return n
		}
		require.NoError(t, err)
		n++
	}
}

// errorEnvelope unmarshals a failed command's stdout into the error envelope
// a real caller would parse.
func errorEnvelope(t *testing.T, stdout string) output.ErrorResponse {
	t.Helper()
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// walkSteps drives a workflow through the named steps in order, requiring
// each to succeed.
func walkSteps(t *testing.T, kind string, steps ...string) {
	t.Helper()
	for _, step := range steps {
		_, _, code := runRootCmd(t, kind, "goto", "--data", `{"step":"`+step+`"}`)
		require.Equalf(t, 0, code, "%s goto %s", kind, step)
	}
}

// fixtureSpecName is the name the counter id method gives the fixture spec,
// and so the name every spec-workflow commit message must carry.
const fixtureSpecName = "000001_billing"

// startSpecWorkflow founds a spec workflow and walks it to verification, the
// last step before the completion commit.
func startSpecWorkflow(t *testing.T) {
	t.Helper()
	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)
	walkSpecToVerification(t)
}

// walkSpecToVerification drives a spec workflow that `new` has already
// founded through every step up to verification, the last one before the
// completion commit.
func walkSpecToVerification(t *testing.T) {
	t.Helper()
	walkSteps(t, "spec",
		"interview", "overview", "requirements", "acceptance_criteria", "constraints",
		"technical_approach", "success_metrics", "non_goals", "verification")
}

// requireNothingCommittedYet is the "no commits appear while the workflow is
// still running" half of criterion 1: every work tree is still on the single
// commit the fixture made, although both have had changes written into them.
func requireNothingCommittedYet(t *testing.T, fx gitFixture) {
	t.Helper()
	require.Equal(t, "1", commitCount(t, fx.root), "no commit may be made before the workflow completes")
	require.Equal(t, "1", commitCount(t, fx.other), "no commit may be made before the workflow completes")
}

// requireCompletionCommitted is the other half of criterion 1 plus the whole
// of criterion 2: each changed work tree gained exactly one commit, carrying
// the message the agent staged, and nothing the workflow itself wrote — its
// own state file included — is left behind uncommitted.
func requireCompletionCommitted(t *testing.T, fx gitFixture, name string) {
	t.Helper()
	for _, dir := range []string{fx.root, fx.other} {
		require.Equal(t, "2", commitCount(t, dir), "completion must add exactly one commit")
		require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain"),
			"the work tree must be clean straight after completion")
		require.Contains(t, gittest.RunGit(t, dir, "log", "-1", "--format=%B"), name,
			"the commit message must name the work it records")
	}
}

// Phase 1.3 criteria 1, 2 and 5: finishing a spec workflow in `workflow`
// mode commits both repositories that changed, exactly once each and only at
// completion, with the workflow's own bookkeeping inside the commit — and
// the caller is handed an ordinary step result, never a question to put to
// the user.
func TestAutoCommit_SpecCompletionCommitsEachChangedRepoOnce(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)

	startSpecWorkflow(t)
	dirtyOtherRepo(t, fx)
	requireNothingCommittedYet(t, fx)

	stageCommitMessage(t, fx, "Specify "+fixtureSpecName+"\n\nThe billing spec is written and verified.\n")
	stdout, _, code := runRootCmd(t, "spec", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	requireCompletionCommitted(t, fx, fixtureSpecName)
	require.Equal(t, "finished", currentStep(t, fx))

	// Criterion 5: the response is the step's own result, with no field
	// asking the user to approve anything. There is no confirmation channel
	// in the envelope for one to arrive through.
	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.NotContains(t, result, "question")
	require.Equal(t, false, result["error"])
}

// Phase 1.3 criterion 1, plan workflow.
func TestAutoCommit_PlanCompletionCommitsEachChangedRepoOnce(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)

	_, _, code := runRootCmd(t, "plan", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)
	walkSteps(t, "plan",
		"discovery", "architecture", "components", "data_structures", "implementation_detail",
		"dependencies", "testing_approach", "milestones", "phases", "open_questions",
		"out_of_scope", "assemble", "verification", "write_plan", "write_context",
		"write_research", "walkthrough")

	dirtyOtherRepo(t, fx)
	requireNothingCommittedYet(t, fx)

	stageCommitMessage(t, fx, "Plan billing\n\nThe implementation plan for billing is complete.\n")
	_, _, code = runRootCmd(t, "plan", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	requireCompletionCommitted(t, fx, "billing")
	require.Equal(t, "finished", currentStep(t, fx))
}

// Phase 1.3 criterion 1, implement workflow. The implement workflow's
// terminal step refuses to finish without a project-level changelog record,
// so one is seeded the way update_feature_changelog would have written it.
func TestAutoCommit_ImplementCompletionCommitsEachChangedRepoOnce(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)
	writeFixturePlan(t, dataDir, "billing")

	changelog := filepath.Join(dataDir, config.DefaultChangelogDir, "billing.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(changelog), 0o755))
	require.NoError(t, os.WriteFile(changelog, []byte("# billing\n\nwhat was built\n"), 0o644))
	commitFixtures(t, fx)

	_, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)
	walkSteps(t, "implement",
		"analyze", "implement", "test", "verify", "update_plan", "update_changelog",
		"test_plan", "update_feature_changelog", "reconcile_spec")

	dirtyOtherRepo(t, fx)
	requireNothingCommittedYet(t, fx)

	stageCommitMessage(t, fx, "Implement billing\n\nEvery phase of the billing plan is built and verified.\n")
	_, _, code = runRootCmd(t, "implement", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	requireCompletionCommitted(t, fx, "billing")
	require.Equal(t, "finished", currentStep(t, fx))
}

// Phase 1.3 criterion 3: a completion the agent tries to make without a
// usable commit message is refused, with instructions naming the file to
// write and the exact command to re-run — and nothing moves: no commit, and
// the workflow stays on the step it was on.
func TestAutoCommit_RefusesCompletionWithoutAUsableMessage(t *testing.T) {
	// The remediation is identical for both refusals: stage the message at
	// the advertised path, then re-run the same goto carrying it.
	const wantNextAction = `write the git commit message to .spektacular/tmp/git-commit-message.md, ` +
		`then run: spektacular spec goto --data '{"step":"finished","commit_message_from":".spektacular/tmp/git-commit-message.md"}'`

	requireRefused := func(t *testing.T, fx gitFixture, stdout string, code int, wantCode, wantMessage string) {
		t.Helper()
		require.Equal(t, 1, code)

		er := errorEnvelope(t, stdout)
		require.Equal(t, wantCode, er.Code)
		require.Equal(t, wantMessage, er.Message)
		require.Equal(t, "finished", er.Resource)
		require.Equal(t, wantNextAction, er.NextAction)

		require.Equal(t, "verification", currentStep(t, fx), "the workflow must stay where it was")
		require.Equal(t, "1", commitCount(t, fx.root), "a refused completion must commit nothing")
	}

	t.Run("no message supplied at all", func(t *testing.T) {
		fx := gitProject(t, config.AutoCommitWorkflow)
		startSpecWorkflow(t)

		stdout, _, code := runRootCmd(t, "spec", "goto", "--data", `{"step":"finished"}`)

		requireRefused(t, fx, stdout, code, "commit_message_required",
			"advancing to this step makes a git commit, and no commit message was supplied")
	})

	t.Run("message does not name the spec", func(t *testing.T) {
		fx := gitProject(t, config.AutoCommitWorkflow)
		startSpecWorkflow(t)
		stageCommitMessage(t, fx, "Write up some thoughts\n\nNo identifier anywhere in here.\n")

		stdout, _, code := runRootCmd(t, "spec", "goto", "--data", finishWithMessage)

		requireRefused(t, fx, stdout, code, "commit_message_invalid",
			`the git commit message does not name the spec "`+fixtureSpecName+`"`)
	})
}

// Phase 1.3 criterion 4: a commit git refuses — here because a pre-commit
// hook rejects it — leaves the workflow on the step it was on, reports only
// the failure, naming the repository and quoting git's reason, and is
// retried simply by re-running the same command once the cause is fixed.
func TestAutoCommit_HookRejectionLeavesWorkflowOnPreviousStep(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)

	hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint-failed-in-hook' >&2\nexit 1\n"), 0o755))

	startSpecWorkflow(t)
	stageCommitMessage(t, fx, "Specify "+fixtureSpecName+"\n\nThe billing spec is written and verified.\n")

	stdout, _, code := runRootCmd(t, "spec", "goto", "--data", finishWithMessage)
	require.Equal(t, 1, code)

	er := errorEnvelope(t, stdout)
	require.Equal(t, "auto_commit_failed", er.Code)
	require.Equal(t, gitFixtureProjectRepo, er.Resource)
	require.Contains(t, er.Message, "lint-failed-in-hook")
	require.Equal(t,
		`fix the cause git reported above (a failing hook, for example), `+
			`re-stage the message at .spektacular/tmp/git-commit-message.md, `+
			`then re-run: spektacular spec goto --data '{"step":"finished","commit_message_from":".spektacular/tmp/git-commit-message.md"}'`,
		er.NextAction)

	// Only the failure is reported: the step's own output is dropped rather
	// than printed alongside it.
	require.Equal(t, 1, jsonDocuments(t, stdout))

	require.Equal(t, "verification", currentStep(t, fx), "the workflow must be put back on its previous step")
	require.Equal(t, "1", commitCount(t, fx.root))

	// Fix the cause, re-stage the message as the remediation says, and re-run
	// the identical command.
	require.NoError(t, os.Remove(hook))
	stageCommitMessage(t, fx, "Specify "+fixtureSpecName+"\n\nThe billing spec is written and verified.\n")

	_, _, code = runRootCmd(t, "spec", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	require.Equal(t, "2", commitCount(t, fx.root))
	require.Equal(t, "finished", currentStep(t, fx))
}

// Phase 1.3 criterion 6: with auto_commit off — set explicitly or left out
// of config.yaml altogether — a workflow runs to completion with no commit
// message anywhere in sight, every git log is exactly where it was, and the
// agent's work is still sitting in the working tree for the user to commit.
func TestAutoCommit_OffCompletesWorkflowWithoutCommitting(t *testing.T) {
	for _, mode := range []string{config.AutoCommitOff, ""} {
		name := mode
		if name == "" {
			name = "key absent"
		}
		t.Run(name, func(t *testing.T) {
			fx := gitProject(t, mode)

			startSpecWorkflow(t)
			dirtyOtherRepo(t, fx)

			_, _, code := runRootCmd(t, "spec", "goto", "--data", `{"step":"finished"}`)
			require.Equal(t, 0, code, "finishing must not ask for a commit message")

			require.Equal(t, "finished", currentStep(t, fx))
			for _, dir := range []string{fx.root, fx.other} {
				require.Equal(t, "1", commitCount(t, dir), "no commit may be made with auto_commit off")
				require.NotEmpty(t, gittest.RunGit(t, dir, "status", "--porcelain"),
					"the agent's work must be left uncommitted in the working tree")
			}
		})
	}
}
