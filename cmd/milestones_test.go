package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests drive the real CLI against a real git binary, the same way
// cmd/autocommit_test.go does, and reuse its fixture helpers: gitProject,
// commitFixtures, commitCount, stageCommitMessage, currentStep, walkSteps,
// errorEnvelope and the staged-message path.

// milestoneSpecName is the name the implement workflow carries for the
// fixture below, and so the name every milestone commit message must name.
const milestoneSpecName = "billing"

// writeTwoMilestonePlan lays out a plan whose Milestones & Phases section
// holds two milestones of one phase each, both open. Ticking one phase
// therefore completes exactly one milestone.
func writeTwoMilestonePlan(t *testing.T, dataDir, name string) string {
	t.Helper()
	planDir := filepath.Join(dataDir, "plans", name)
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	planPath := filepath.Join(planDir, "plan.md")
	body := `# Plan: ` + name + `

## Overview

fixture

## Milestones & Phases

### Milestone 1: first

#### - [ ] Phase 1.1: a

### Milestone 2: second

#### - [ ] Phase 2.1: b
`
	require.NoError(t, os.WriteFile(planPath, []byte(body), 0o644))
	return planPath
}

// writeTwoMilestoneTaskPlan is writeTwoMilestonePlan in the task format:
// two milestones of one open task each.
func writeTwoMilestoneTaskPlan(t *testing.T, dataDir, name string) string {
	t.Helper()
	planDir := filepath.Join(dataDir, "plans", name)
	require.NoError(t, os.MkdirAll(planDir, 0o755))
	planPath := filepath.Join(planDir, "plan.md")
	body := `# Plan: ` + name + `

## Overview

fixture

## Milestones & Tasks

### Milestone 1: first

#### - [ ] Task: a
**Id:** 11111111-1111-4111-8111-111111111111
**Repo:** root
**Depends on:** none
**Execution:** agent

### Milestone 2: second

#### - [ ] Task: b
**Id:** 22222222-2222-4222-8222-222222222222
**Repo:** root
**Depends on:**
- 11111111-1111-4111-8111-111111111111 — a
**Execution:** agent
`
	require.NoError(t, os.WriteFile(planPath, []byte(body), 0o644))
	return planPath
}

// tickPhase ticks one phase's checkbox in the plan on disk, standing in for
// what the agent does during the update_plan step.
func tickPhase(t *testing.T, planPath, phase string) {
	t.Helper()
	body, err := os.ReadFile(planPath)
	require.NoError(t, err)
	open := "#### - [ ] " + phase
	require.Containsf(t, string(body), open, "the plan has no open %q to tick", phase)
	require.NoError(t, os.WriteFile(planPath,
		[]byte(strings.Replace(string(body), open, "#### - [x] "+phase, 1)), 0o644))
}

// milestoneProject builds a project whose plan has two milestones, founds an
// implement workflow over it, and returns the fixture and the plan's path.
// The project-level changelog record the terminal step insists on is seeded
// up front, the way update_feature_changelog would have written it.
func milestoneProject(t *testing.T, mode string) (gitFixture, string) {
	t.Helper()
	return milestoneProjectWith(t, mode, writeTwoMilestonePlan)
}

// milestoneProjectWith is milestoneProject over the plan writePlan lays out.
func milestoneProjectWith(t *testing.T, mode string, writePlan func(*testing.T, string, string) string) (gitFixture, string) {
	t.Helper()
	fx := gitProject(t, mode)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)
	planPath := writePlan(t, dataDir, milestoneSpecName)

	changelog := filepath.Join(dataDir, config.DefaultChangelogDir, milestoneSpecName+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(changelog), 0o755))
	require.NoError(t, os.WriteFile(changelog, []byte("# "+milestoneSpecName+"\n\nwhat was built\n"), 0o644))
	commitFixtures(t, fx)

	_, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"`+milestoneSpecName+`"}`)
	require.Equal(t, 0, code)
	return fx, planPath
}

// walkPhase drives one implementation phase from the step the workflow is on
// after `new`, or after looping back to analyze, up to update_changelog —
// the step a milestone commit is made on the way out of.
func walkPhase(t *testing.T, from string) {
	t.Helper()
	steps := []string{"analyze", "implement", "test", "verify", "update_plan", "update_changelog"}
	if from == "analyze" {
		steps = steps[1:]
	}
	walkSteps(t, "implement", steps...)
}

// dirtyOtherRepoFile writes a distinctly named file into the second repo's
// work tree, standing in for work the agent did there during one phase. A
// fresh name each time is what makes the repo dirty again after the previous
// milestone's commit swept the last one up.
func dirtyOtherRepoFile(t *testing.T, fx gitFixture, name string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fx.other, name), []byte("agent work in "+name+"\n"), 0o644))
}

// gotoWithStagedMessage is the --data payload that runs a transition
// carrying the message staged at the advertised path.
func gotoWithStagedMessage(step string) string {
	return `{"step":"` + step + `","commit_message_from":"` + stagedCommitMessagePath + `"}`
}

// stateData reads the workflow data map out of the state file, where the
// record of which milestones have been committed lives.
func stateData(t *testing.T, fx gitFixture) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fx.root, config.ProjectConfigDirName, "state.json"))
	require.NoError(t, err)
	var st workflow.State
	require.NoError(t, json.Unmarshal(b, &st))
	return st.Data
}

// requireMilestoneCommitted asserts each work tree is on the given number of
// commits and that its newest one carries the milestone's message.
func requireMilestoneCommitted(t *testing.T, fx gitFixture, count, message string) {
	t.Helper()
	for _, dir := range []string{fx.root, fx.other} {
		require.Equal(t, count, commitCount(t, dir), "finishing a milestone must add exactly one commit")
		require.Contains(t, gittest.RunGit(t, dir, "log", "-1", "--format=%B"), message,
			"the commit message must name the spec and the milestone it records")
	}
}

// Phase 3.1 criterion 4: a phase wrap-up in the middle of a milestone is an
// ordinary transition. With no phase ticked, moving on asks for no commit
// message and commits nothing — and a message the agent staged anyway is
// left exactly where it was rather than consumed.
func TestMilestoneCommit_MidMilestoneTransitionAsksForNothing(t *testing.T) {
	t.Run("no message is required", func(t *testing.T) {
		fx, _ := milestoneProject(t, config.AutoCommitFull)
		walkPhase(t, "")
		dirtyOtherRepoFile(t, fx, "phase-1.txt")

		_, _, code := runRootCmd(t, "implement", "goto", "--data", `{"step":"analyze"}`)
		require.Equal(t, 0, code, "an unfinished milestone must not ask for a commit message")

		require.Equal(t, "analyze", currentStep(t, fx))
		require.Equal(t, "1", commitCount(t, fx.root), "no commit may be made inside a milestone")
		require.Equal(t, "1", commitCount(t, fx.other), "no commit may be made inside a milestone")
	})

	t.Run("a staged message is left untouched", func(t *testing.T) {
		fx, _ := milestoneProject(t, config.AutoCommitFull)
		walkPhase(t, "")
		dirtyOtherRepoFile(t, fx, "phase-1.txt")
		stageCommitMessage(t, fx, "Implement billing Milestone 1\n\nStaged too early.\n")

		_, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
		require.Equal(t, 0, code)

		require.FileExists(t, filepath.Join(fx.root, filepath.FromSlash(stagedCommitMessagePath)),
			"a message staged for a milestone that is not finished must not be consumed")
		require.Equal(t, "1", commitCount(t, fx.root))
		require.Equal(t, "1", commitCount(t, fx.other))
	})
}

// Phase 3.1 criterion 1: in full mode each finished milestone adds exactly
// one commit to every repo that changed, naming the spec and that milestone
// — and the second milestone is committed separately from the first.
func TestMilestoneCommit_CommitsEachChangedRepoPerMilestone(t *testing.T) {
	fx, planPath := milestoneProject(t, config.AutoCommitFull)

	walkPhase(t, "")
	tickPhase(t, planPath, "Phase 1.1:")
	dirtyOtherRepoFile(t, fx, "phase-1.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 1\n\nThe first milestone's phases are all built.\n")

	_, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
	require.Equal(t, 0, code)
	require.Equal(t, "analyze", currentStep(t, fx))
	requireMilestoneCommitted(t, fx, "2", "Implement billing Milestone 1")

	walkPhase(t, "analyze")
	tickPhase(t, planPath, "Phase 2.1:")
	dirtyOtherRepoFile(t, fx, "phase-2.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 2\n\nThe second milestone's phases are all built.\n")

	_, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("test_plan"))
	require.Equal(t, 0, code)
	require.Equal(t, "test_plan", currentStep(t, fx))
	requireMilestoneCommitted(t, fx, "3", "Implement billing Milestone 2")

	// Each milestone is recorded as it is committed, so neither is ever
	// committed a second time.
	require.Equal(t, []any{float64(1), float64(2)}, stateData(t, fx)["committed_milestones"])
}

// A task-format plan makes milestone commits exactly as a phase plan does:
// ticking the only task of a milestone completes it.
func TestMilestoneCommit_TaskPlanCommitsEachMilestone(t *testing.T) {
	fx, planPath := milestoneProjectWith(t, config.AutoCommitFull, writeTwoMilestoneTaskPlan)

	walkPhase(t, "")
	tickPhase(t, planPath, "Task: a")
	dirtyOtherRepoFile(t, fx, "task-a.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 1\n\nThe first milestone's tasks are all built.\n")

	_, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
	require.Equal(t, 0, code)
	requireMilestoneCommitted(t, fx, "2", "Implement billing Milestone 1")

	walkPhase(t, "analyze")
	tickPhase(t, planPath, "Task: b")
	dirtyOtherRepoFile(t, fx, "task-b.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 2\n\nThe second milestone's tasks are all built.\n")

	_, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("test_plan"))
	require.Equal(t, 0, code)
	requireMilestoneCommitted(t, fx, "3", "Implement billing Milestone 2")
	require.Equal(t, []any{float64(1), float64(2)}, stateData(t, fx)["committed_milestones"])
}

// Phase 3.1 criterion 2: the completion commit after the milestone commits
// picks up only what changed since the last one. The project's own repo has
// moved on — the workflow keeps writing its state file — so it gains a
// commit; the second repo has been untouched since its milestone commit, so
// it is skipped silently rather than failing on an empty commit.
func TestMilestoneCommit_CompletionCommitsOnlyWhatChangedSince(t *testing.T) {
	fx, planPath := milestoneProject(t, config.AutoCommitFull)

	walkPhase(t, "")
	tickPhase(t, planPath, "Phase 1.1:")
	dirtyOtherRepoFile(t, fx, "phase-1.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 1\n\nThe first milestone's phases are all built.\n")
	_, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
	require.Equal(t, 0, code)

	walkPhase(t, "analyze")
	tickPhase(t, planPath, "Phase 2.1:")
	dirtyOtherRepoFile(t, fx, "phase-2.txt")
	stageCommitMessage(t, fx, "Implement billing Milestone 2\n\nThe second milestone's phases are all built.\n")
	_, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("test_plan"))
	require.Equal(t, 0, code)

	// Nothing new is written into the second repo from here on, so it is
	// clean by the time the workflow completes.
	walkSteps(t, "implement", "update_feature_changelog", "reconcile_spec")
	require.Empty(t, gittest.RunGit(t, fx.other, "status", "--porcelain"),
		"the second repo must have nothing left to commit at completion")

	stageCommitMessage(t, fx, "Implement billing\n\nEvery milestone of the billing plan is built and verified.\n")
	_, _, code = runRootCmd(t, "implement", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code, "a repo with nothing left to commit must not fail the completion")

	require.Equal(t, "finished", currentStep(t, fx))
	require.Equal(t, "4", commitCount(t, fx.root), "the project repo still had changes, so completion commits it")
	require.Equal(t, "3", commitCount(t, fx.other), "a repo with nothing left must gain no commit")
}

// Phase 3.1 criterion 3: once a milestone is finished the transition out of
// update_changelog insists on a message that names both the spec and that
// milestone. Either refusal names the file to write and the exact command to
// re-run, commits nothing, and leaves the workflow where it was.
func TestMilestoneCommit_RefusesAMessageThatDoesNotNameTheMilestone(t *testing.T) {
	const wantNextAction = `write the git commit message to .spektacular/tmp/git-commit-message.md, ` +
		`then run: spektacular implement goto --data '{"step":"analyze","commit_message_from":".spektacular/tmp/git-commit-message.md"}'`

	requireRefused := func(t *testing.T, fx gitFixture, stdout string, code int, wantCode, wantMessage string) {
		t.Helper()
		require.Equal(t, 1, code)

		er := errorEnvelope(t, stdout)
		require.Equal(t, wantCode, er.Code)
		require.Equal(t, wantMessage, er.Message)
		require.Equal(t, "analyze", er.Resource)
		require.Equal(t, wantNextAction, er.NextAction)

		require.Equal(t, "update_changelog", currentStep(t, fx), "the workflow must stay where it was")
		require.Equal(t, "1", commitCount(t, fx.root), "a refused milestone commit must commit nothing")
		require.Equal(t, "1", commitCount(t, fx.other), "a refused milestone commit must commit nothing")
	}

	t.Run("no message supplied at all", func(t *testing.T) {
		fx, planPath := milestoneProject(t, config.AutoCommitFull)
		walkPhase(t, "")
		tickPhase(t, planPath, "Phase 1.1:")
		dirtyOtherRepoFile(t, fx, "phase-1.txt")

		stdout, _, code := runRootCmd(t, "implement", "goto", "--data", `{"step":"analyze"}`)

		requireRefused(t, fx, stdout, code, "commit_message_required",
			"advancing to this step makes a git commit, and no commit message was supplied")
	})

	t.Run("message names the spec but not the milestone", func(t *testing.T) {
		fx, planPath := milestoneProject(t, config.AutoCommitFull)
		walkPhase(t, "")
		tickPhase(t, planPath, "Phase 1.1:")
		dirtyOtherRepoFile(t, fx, "phase-1.txt")
		stageCommitMessage(t, fx, "Implement billing\n\nSome phases got done.\n")

		stdout, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))

		requireRefused(t, fx, stdout, code, "commit_message_invalid",
			`the git commit message does not name "Milestone 1", whose tasks are now all complete`)
	})
}

// Phase 3.1 criterion 5: workflow mode commits only when a workflow
// finishes. Both milestones complete without a message being asked for or a
// commit being made, and the single commit each repo gains is the completion
// one at the end.
func TestMilestoneCommit_WorkflowModeMakesNoMilestoneCommits(t *testing.T) {
	fx, planPath := milestoneProject(t, config.AutoCommitWorkflow)

	walkPhase(t, "")
	tickPhase(t, planPath, "Phase 1.1:")
	dirtyOtherRepoFile(t, fx, "phase-1.txt")
	_, _, code := runRootCmd(t, "implement", "goto", "--data", `{"step":"analyze"}`)
	require.Equal(t, 0, code, "workflow mode must not ask for a milestone commit message")
	require.Equal(t, "1", commitCount(t, fx.root), "workflow mode must make no milestone commit")
	require.Equal(t, "1", commitCount(t, fx.other), "workflow mode must make no milestone commit")

	walkPhase(t, "analyze")
	tickPhase(t, planPath, "Phase 2.1:")
	dirtyOtherRepoFile(t, fx, "phase-2.txt")
	_, _, code = runRootCmd(t, "implement", "goto", "--data", `{"step":"test_plan"}`)
	require.Equal(t, 0, code, "workflow mode must not ask for a milestone commit message")
	require.Equal(t, "1", commitCount(t, fx.root), "workflow mode must make no milestone commit")
	require.Equal(t, "1", commitCount(t, fx.other), "workflow mode must make no milestone commit")

	require.NotContains(t, stateData(t, fx), "committed_milestones",
		"workflow mode records no committed milestones")

	walkSteps(t, "implement", "update_feature_changelog", "reconcile_spec")
	stageCommitMessage(t, fx, "Implement billing\n\nEvery milestone of the billing plan is built and verified.\n")
	_, _, code = runRootCmd(t, "implement", "goto", "--data", finishWithMessage)
	require.Equal(t, 0, code)

	require.Equal(t, "2", commitCount(t, fx.root), "completion is workflow mode's only commit")
	require.Equal(t, "2", commitCount(t, fx.other), "completion is workflow mode's only commit")
}

// Phase 3.1 criterion 6: a milestone commit git refuses leaves the workflow
// on the phase wrap-up step and forgets the milestone, so the identical
// retry asks for the same milestone again and commits it once the cause is
// fixed.
func TestMilestoneCommit_HookRejectionRollsBackTheRecordedMilestone(t *testing.T) {
	fx, planPath := milestoneProject(t, config.AutoCommitFull)

	hook := filepath.Join(fx.root, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho 'lint-failed-in-hook' >&2\nexit 1\n"), 0o755))

	walkPhase(t, "")
	tickPhase(t, planPath, "Phase 1.1:")
	dirtyOtherRepoFile(t, fx, "phase-1.txt")
	const message = "Implement billing Milestone 1\n\nThe first milestone's phases are all built.\n"
	stageCommitMessage(t, fx, message)

	stdout, _, code := runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
	require.Equal(t, 1, code)

	er := errorEnvelope(t, stdout)
	require.Equal(t, "auto_commit_failed", er.Code)
	require.Equal(t, gitFixtureProjectRepo, er.Resource)
	require.Contains(t, er.Message, "lint-failed-in-hook")

	require.Equal(t, "update_changelog", currentStep(t, fx), "the workflow must be put back on the phase wrap-up step")
	require.Equal(t, "1", commitCount(t, fx.root), "a refused milestone commit must commit nothing")
	require.NotContains(t, stateData(t, fx), "committed_milestones",
		"a milestone whose commit was refused must not be recorded as committed")

	// Fix the cause, re-stage the message as the remediation says, and re-run
	// the identical command.
	require.NoError(t, os.Remove(hook))
	stageCommitMessage(t, fx, message)

	_, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("analyze"))
	require.Equal(t, 0, code)

	require.Equal(t, "analyze", currentStep(t, fx))
	require.Equal(t, "2", commitCount(t, fx.root))
	require.Equal(t, []any{float64(1)}, stateData(t, fx)["committed_milestones"],
		"the retry must record the milestone it committed")
}
