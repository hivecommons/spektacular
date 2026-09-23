package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/testutil/gittest"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// Phase 2.1: the uncommitted-changes gate the three `new` commands run before
// they write anything. These tests share gitProject and its helpers with
// autocommit_test.go, and drive the real CLI against a real git binary for
// the same reason: the thing under test is what git ends up holding.

// The files these tests use to stand in for the user's own work. notes.txt is
// committed into the fixture's baseline first, so writing to it afterwards is
// a modification rather than an untracked file; scratch.txt is never added at
// all, which is the other way a work tree goes dirty.
const (
	preExistingFile     = "notes.txt"
	preExistingOriginal = "committed before the workflow\n"
	preExistingEdit     = "the user's own uncommitted edit\n"
	untrackedFile       = "scratch.txt"
)

// workingContextSeed is what the working-context file holds going in. A
// started spec workflow empties it, so finding it intact afterwards is proof
// the workflow never started.
const workingContextSeed = "notes the previous session left behind\n"

// seedCommittedProjectFiles folds the two files the gate tests watch into the
// fixture's baseline commit: the file the user will later edit, and a
// working-context file with content in it.
func seedCommittedProjectFiles(t *testing.T, fx gitFixture) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, preExistingFile), []byte(preExistingOriginal), 0o644))
	wc := filepath.Join(fx.root, config.ProjectConfigDirName, "working-context.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(wc), 0o755))
	require.NoError(t, os.WriteFile(wc, []byte(workingContextSeed), 0o644))
	commitFixtures(t, fx)
}

// dirtyPreExistingFile makes the user's edit to the committed file.
func dirtyPreExistingFile(t *testing.T, fx gitFixture) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fx.root, preExistingFile), []byte(preExistingEdit), 0o644))
}

// specScaffoldPath is where a started spec workflow writes its scaffold, with
// the name the counter id method gives the fixture spec.
func specScaffoldPath(fx gitFixture) string {
	return filepath.Join(fx.root, config.ProjectConfigDirName, "specs", fixtureSpecName+".md")
}

// statePath is the workflow state file a started workflow writes.
func statePathOf(fx gitFixture) string {
	return filepath.Join(fx.root, config.ProjectConfigDirName, "state.json")
}

// requireNothingStarted is the "writes nothing" half of criterion 1: no
// workflow state was saved, and the working context the previous session left
// behind was not reset.
func requireNothingStarted(t *testing.T, fx gitFixture) {
	t.Helper()
	require.NoFileExists(t, statePathOf(fx))
	body, err := os.ReadFile(filepath.Join(fx.root, config.ProjectConfigDirName, "working-context.md"))
	require.NoError(t, err)
	require.Equal(t, workingContextSeed, string(body), "the working context must not be reset by a refused start")
}

// Phase 2.1 criterion 1: with automatic commits on and the work tree dirty,
// every one of the three `new` commands refuses to start, names the repos
// involved, hands back both re-run commands, and leaves the project exactly
// as it found it.
func TestStartGate_UncommittedChangesStopEveryWorkflowBeforeAnythingIsWritten(t *testing.T) {
	// Hand-written oracles. Only the project's own repo is dirty in these
	// runs, so the report names testproj alone, at the work tree git reports
	// for it. json.Marshal sorts object keys, which puts commit_existing
	// before name in both re-run commands.
	wantNextAction := func(kind string) string {
		return "ask the user whether to git commit these changes before the " + kind +
			" workflow starts; then run: spektacular " + kind +
			` new --data '{"commit_existing":true,"name":"billing"}' to commit them first,` +
			" or spektacular " + kind +
			` new --data '{"commit_existing":false,"name":"billing"}' to continue without committing`
	}

	triggers := []struct {
		name  string
		dirty func(t *testing.T, fx gitFixture)
	}{
		{"modified tracked file", dirtyPreExistingFile},
		{"untracked new file", func(t *testing.T, fx gitFixture) {
			t.Helper()
			require.NoError(t, os.WriteFile(filepath.Join(fx.root, untrackedFile), []byte("never added\n"), 0o644))
		}},
	}

	for _, kind := range []string{"spec", "plan", "implement"} {
		for _, trigger := range triggers {
			t.Run(kind+"/"+trigger.name, func(t *testing.T) {
				fx := gitProject(t, config.AutoCommitWorkflow)
				if kind == "implement" {
					// implement refuses a plan it cannot find, before the gate
					// ever runs.
					writeFixturePlan(t, filepath.Join(fx.root, config.ProjectConfigDirName), "billing")
				}
				seedCommittedProjectFiles(t, fx)
				trigger.dirty(t, fx)

				stdout, _, code := runRootCmd(t, kind, "new", "--data", `{"name":"billing"}`)
				require.Equal(t, 1, code)

				er := errorEnvelope(t, stdout)
				require.Equal(t, "uncommitted_changes", er.Code)
				require.Equal(t, "uncommitted changes in registered repos: testproj ("+fx.root+")", er.Message)
				require.Equal(t, gitFixtureProjectRepo, er.Resource)
				require.Equal(t, wantNextAction(kind), er.NextAction)

				requireNothingStarted(t, fx)
				require.Equal(t, "1", commitCount(t, fx.root), "a refused start must commit nothing")

				if kind == "spec" {
					require.NoFileExists(t, specScaffoldPath(fx), "no spec may be scaffolded by a refused start")
				}
			})
		}
	}
}

// Phase 2.1 criterion 2: answering "commit first" saves the user's own work
// in a commit of its own, named for the spec and saying plainly whose changes
// it holds, and that commit lands before anything the workflow goes on to
// commit.
func TestStartGate_CommitExistingTrueCommitsTheUsersWorkFirst(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	seedCommittedProjectFiles(t, fx)
	dirtyPreExistingFile(t, fx)

	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"commit_existing":true,"name":"billing"}`)
	require.Equal(t, 0, code, "answering the question must let the workflow start")

	require.Equal(t, "2", commitCount(t, fx.root), "exactly one commit holds the pre-existing changes")
	require.Equal(t, "1", commitCount(t, fx.other), "a clean repo gains no pre-workflow commit")

	require.Equal(t, "Save uncommitted changes before spec workflow for "+fixtureSpecName,
		gittest.RunGit(t, fx.root, "log", "-1", "--format=%s"))
	require.Equal(t,
		"These are the user's changes from before the spec workflow for "+fixtureSpecName+
			" started. Spektacular committed them separately at the user's request so they"+
			" are not mixed with the agent's work.",
		gittest.RunGit(t, fx.root, "log", "-1", "--format=%b"))

	// The commit holds the user's edit, not the file as it was.
	require.Equal(t, "the user's own uncommitted edit",
		gittest.RunGit(t, fx.root, "show", "HEAD:"+preExistingFile))

	// Run the workflow out to its own completion commit and read the history
	// back: the user's commit sits below the workflow's, above the baseline.
	walkSpecToVerification(t)
	stageCommitMessage(t, fx, "Specify "+fixtureSpecName+"\n\nThe billing spec is written and verified.\n")
	_, _, code = runRootCmd(t, "spec", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	require.Equal(t,
		"Specify "+fixtureSpecName+"\n"+
			"Save uncommitted changes before spec workflow for "+fixtureSpecName+"\n"+
			"initial",
		gittest.RunGit(t, fx.root, "log", "--format=%s"))
}

// Phase 2.1 criterion 3: answering "continue" starts the workflow with no
// commit of its own, and the user's pre-existing change is swept into the
// workflow's first automatic commit.
func TestStartGate_CommitExistingFalseFoldsTheChangeIntoTheWorkflowsCommit(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	seedCommittedProjectFiles(t, fx)
	dirtyPreExistingFile(t, fx)

	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"commit_existing":false,"name":"billing"}`)
	require.Equal(t, 0, code)
	require.FileExists(t, statePathOf(fx), "the workflow must have started")
	require.Equal(t, "1", commitCount(t, fx.root), "continuing must not commit anything at the start")

	walkSpecToVerification(t)
	stageCommitMessage(t, fx, "Specify "+fixtureSpecName+"\n\nThe billing spec is written and verified.\n")
	_, _, code = runRootCmd(t, "spec", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	require.Equal(t, "2", commitCount(t, fx.root))
	require.Contains(t, gittest.RunGit(t, fx.root, "show", "--name-only", "--format=", "HEAD"), preExistingFile,
		"the pre-existing change belongs to the workflow's first automatic commit")
}

// Phase 2.1 criterion 4: the question is only ever asked when there is
// something to ask about — a dirty work tree, automatic commits on, and a
// repo git actually tracks.
func TestStartGate_StaysSilentWhenThereIsNothingToAsk(t *testing.T) {
	t.Run("clean work tree", func(t *testing.T) {
		fx := gitProject(t, config.AutoCommitWorkflow)
		seedCommittedProjectFiles(t, fx)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)
		require.FileExists(t, statePathOf(fx))
	})

	t.Run("auto_commit off", func(t *testing.T) {
		fx := gitProject(t, config.AutoCommitOff)
		seedCommittedProjectFiles(t, fx)
		dirtyPreExistingFile(t, fx)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code)
		require.FileExists(t, statePathOf(fx))
		require.Equal(t, "1", commitCount(t, fx.root), "auto_commit off commits nothing, gate or otherwise")
	})

	t.Run("registered repo is not a git work tree", func(t *testing.T) {
		fx := gitProject(t, config.AutoCommitWorkflow)
		seedCommittedProjectFiles(t, fx)
		// Take git away from the second repo and leave a file lying in it:
		// were it still tracked, this would be the dirty repo the gate
		// reports.
		require.NoError(t, os.RemoveAll(filepath.Join(fx.other, ".git")))
		dirtyOtherRepo(t, fx)

		_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		require.Equal(t, 0, code, "a repo outside git is skipped, not reported and not an error")
		require.FileExists(t, statePathOf(fx))
	})
}

// Phase 2.1 criterion 5: an interrupted workflow is offered for resume before
// the uncommitted-changes question is ever reached, so a dirty work tree
// cannot hide the fact that there is something to resume.
func TestStartGate_ResumeIsOfferedBeforeTheUncommittedChangesQuestion(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	seedCommittedProjectFiles(t, fx)

	_, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 0, code)

	dirtyPreExistingFile(t, fx)

	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 1, code)

	er := errorEnvelope(t, stdout)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, `a spec workflow ("`+fixtureSpecName+`") is already in progress at step "new"`, er.Message)
	require.Equal(t, fixtureSpecName, er.Resource)
}

// Phase 2.1 criterion 6: a "commit first" that git refuses stops the workflow
// starting, names the repo that failed and quotes git's reason, and offers the
// same command to re-run once the cause is fixed.
func TestStartGate_FailedCommitExistingStopsTheWorkflowStarting(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	seedCommittedProjectFiles(t, fx)
	dirtyPreExistingFile(t, fx)

	hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint-failed-in-hook' >&2\nexit 1\n"), 0o755))

	stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"commit_existing":true,"name":"billing"}`)
	require.Equal(t, 1, code)

	er := errorEnvelope(t, stdout)
	require.Equal(t, "auto_commit_failed", er.Code)
	require.Equal(t, gitFixtureProjectRepo, er.Resource)
	require.Contains(t, er.Message, "lint-failed-in-hook")
	require.Equal(t,
		"fix the cause git reported above (a failing hook, for example), then re-run: "+
			`spektacular spec new --data '{"commit_existing":true,"name":"billing"}'`,
		er.NextAction)

	requireNothingStarted(t, fx)
	require.NoFileExists(t, specScaffoldPath(fx))
	require.Equal(t, "1", commitCount(t, fx.root))
}

// Phase 2.1: the split that makes criterion 1's "writes nothing" true.
// probeResume reports on a stale state file without removing it, so the gate
// can refuse afterwards with the project still untouched; clearState is the
// half that does the removing, and the `new` commands run it only once the
// gate has let them through.
func TestProbeResume_LeavesTheStaleStateFileForClearStateToRemove(t *testing.T) {
	dir := t.TempDir()
	writeInProgressState(t, dir, workflow.State{
		Kind:        "spec",
		CurrentStep: "finished",
		Data:        map[string]any{"name": "billing"},
	})
	statePath := filepath.Join(dir, "state.json")

	handled, err := probeResume(statePath, "spektacular", "spec", false)
	require.NoError(t, err)
	require.False(t, handled, "a finished workflow is not offered for resume")
	require.FileExists(t, statePath, "probeResume must not touch disk")

	clearState(statePath)
	require.NoFileExists(t, statePath)
}

// Phase 2.1: the re-run commands the uncommitted-changes report hands back.
func TestNewCommandWith(t *testing.T) {
	require.Equal(t,
		`spektacular spec new --data '{"commit_existing":true,"name":"billing"}'`,
		newCommandWith("spektacular", "spec", `{"name":"billing"}`, true))

	require.Equal(t,
		`spektacular implement new --data '{"commit_existing":false,"name":"billing"}'`,
		newCommandWith("spektacular", "implement", `{"name":"billing"}`, false))

	// Nothing to echo back: the answer is still carried.
	require.Equal(t,
		`spektacular plan new --data '{"commit_existing":true}'`,
		newCommandWith("spektacular", "plan", "", true))

	// Every other argument the caller passed is preserved, and an answer
	// already present is replaced rather than duplicated.
	require.Equal(t,
		`spektacular spec new --data '{"commit_existing":true,"id":"0042","name":"billing"}'`,
		newCommandWith("spektacular", "spec", `{"name":"billing","id":"0042","commit_existing":false}`, true))
}

// Phase 2.1: the answer is read out of the raw --data, and "not asked yet"
// has to be distinguishable from an explicit no — the first starts nothing,
// the second starts the workflow.
func TestCommitExistingAnswer(t *testing.T) {
	cases := []struct {
		name         string
		data         string
		wantChoice   bool
		wantAnswered bool
	}{
		{"no data at all", "", false, false},
		{"key absent", `{"name":"billing"}`, false, false},
		{"explicit no", `{"commit_existing":false,"name":"billing"}`, false, true},
		{"explicit yes", `{"commit_existing":true,"name":"billing"}`, true, true},
		{"unparsable data", `{"name":`, false, false},
		{"not a boolean", `{"commit_existing":"yes"}`, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			choice, answered := commitExistingAnswer(tc.data)
			require.Equal(t, tc.wantChoice, choice)
			require.Equal(t, tc.wantAnswered, answered)
		})
	}
}
