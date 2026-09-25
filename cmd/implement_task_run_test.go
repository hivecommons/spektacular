package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// twoTaskPlan is a plan of two agent tasks, the second depending on the
// first, each ticked or not as asked.
func twoTaskPlan(aDone, bDone bool) string {
	return taskPlanDoc(
		taskBlock("Task A", aDone, agentFields(idA), "- [ ] a works") +
			taskBlock("Task B", bDone, agentFields(idB, idA+" — Task A"), "- [ ] b works"),
	)
}

// instructionOf returns the instruction a goto or new printed.
func instructionOf(t *testing.T, stdout string) string {
	t.Helper()
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &res), stdout)
	s, _ := res["instruction"].(string)
	return s
}

// runTaskTo starts a single-task run and walks it to update_plan.
func runTaskTo(t *testing.T, task string) {
	t.Helper()
	stdout, code := implementNewTask(t, task)
	require.Equal(t, 0, code, stdout)
	walkSteps(t, "implement", "analyze", "implement", "test", "verify", "update_plan")
}

func gotoStep(t *testing.T, step string) (string, int) {
	t.Helper()
	stdout, _, code := runRootCmd(t, "implement", "goto", "--data", `{"step":"`+step+`"}`)
	return stdout, code
}

func TestTaskRun_WrapUpHappensOnlyOnTheLastTask(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan", twoTaskPlan(false, false))
	require.Equal(t, 0, code)
	dataDir := filepath.Join(dir, ".spektacular")
	testPlan := filepath.Join(dataDir, "plans", taskPlanName, "test-plan.md")
	changelog := filepath.Join(dataDir, "changelog", taskPlanName+".md")

	// First run: task A, with task B still open afterwards.
	runTaskTo(t, idA)
	_, code = writePlanDoc(t, taskPlanName, "plan", twoTaskPlan(true, false))
	require.Equal(t, 0, code)
	stdout, code := gotoStep(t, "update_changelog")
	require.Equal(t, 0, code, stdout)
	instr := instructionOf(t, stdout)
	require.Contains(t, instr, `{"step":"finished"}`)
	require.NotContains(t, instr, `{"step":"test_plan"}`)
	require.NotContains(t, instr, `{"step":"analyze"}`)

	stdout, code = gotoStep(t, "finished")
	require.Equal(t, 0, code, stdout)
	require.Contains(t, instructionOf(t, stdout), "1 task(s) in the plan remain open")
	require.NoFileExists(t, testPlan)
	require.NoFileExists(t, changelog)

	// Second run: task B completes the plan and does the wrap-up.
	runTaskTo(t, idB)
	_, code = writePlanDoc(t, taskPlanName, "plan", twoTaskPlan(true, true))
	require.Equal(t, 0, code)
	stdout, code = gotoStep(t, "update_changelog")
	require.Equal(t, 0, code, stdout)
	instr = instructionOf(t, stdout)
	require.Contains(t, instr, `{"step":"test_plan"}`)
	require.NotContains(t, instr, `{"step":"finished"}`)

	// Skipping the wrap-up on the last task is refused: the feature
	// changelog is required again.
	stdout, code = gotoStep(t, "finished")
	require.Equal(t, 1, code)
	require.Equal(t, "changelog_missing", decodeError(t, stdout).Code)

	walkSteps(t, "implement", "test_plan", "update_feature_changelog")
	src := filepath.Join(t.TempDir(), "changelog.md")
	require.NoError(t, os.WriteFile(src, []byte("# feature\n\nwhat was built\n"), 0o644))
	_, _, code = runRootCmd(t, "changelog", "file", "write", taskPlanName, "--from", src)
	require.Equal(t, 0, code)
	walkSteps(t, "implement", "reconcile_spec", "finished")
	require.FileExists(t, changelog)
}

func TestWholePlanRun_UpdateChangelogExitsAreUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan", twoTaskPlan(false, false))
	require.Equal(t, 0, code)

	_, _, code = runRootCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	require.Equal(t, 0, code)
	walkSteps(t, "implement", "analyze", "implement", "test", "verify", "update_plan")
	stdout, code := gotoStep(t, "update_changelog")
	require.Equal(t, 0, code)
	instr := instructionOf(t, stdout)
	require.Contains(t, instr, `{"step":"analyze"}`)
	require.Contains(t, instr, `{"step":"test_plan"}`)
	require.NotContains(t, instr, `{"step":"finished"}`)

	// A whole-plan run cannot skip the wrap-up by jumping to finished.
	stdout, code = gotoStep(t, "finished")
	require.Equal(t, 1, code)
	require.Equal(t, "changelog_missing", decodeError(t, stdout).Code)
}

// writeTaskMilestonePlan writes, directly to disk, a plan of two milestones
// with one task each, so finishing task a finishes Milestone 1.
func writeTaskMilestonePlan(t *testing.T, root, name string, aDone bool) string {
	t.Helper()
	box := " "
	if aDone {
		box = "x"
	}
	path := filepath.Join(root, config.ProjectConfigDirName, "plans", name, "plan.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	body := "# Plan: " + name + "\n\n## Milestones & Tasks\n\n### Milestone 1: first\n\n" +
		"#### - [" + box + "] Task: a\n**Id:** " + idA + "\n**Repo:** testproj\n**Depends on:** none\n**Execution:** agent\n\n" +
		"### Milestone 2: second\n\n" +
		"#### - [ ] Task: b\n**Id:** " + idB + "\n**Repo:** testproj\n**Depends on:**\n- " + idA + " — a\n**Execution:** agent\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestTaskRun_EarlyFinishIsCommitted(t *testing.T) {
	for _, mode := range []string{config.AutoCommitWorkflow, config.AutoCommitFull} {
		t.Run(mode, func(t *testing.T) {
			fx := gitProject(t, mode)
			writeTaskMilestonePlan(t, fx.root, milestoneSpecName, false)
			commitFixtures(t, fx)

			_, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"`+milestoneSpecName+`","task":"`+idA+`"}`)
			require.Equal(t, 0, code)
			walkSteps(t, "implement", "analyze", "implement", "test", "verify", "update_plan")
			writeTaskMilestonePlan(t, fx.root, milestoneSpecName, true)

			stdout, code := gotoStep(t, "update_changelog")
			require.Equal(t, 0, code)
			instr := instructionOf(t, stdout)
			require.Contains(t, instr, "Automatic git commit")
			require.Contains(t, instr, `"step":"finished","commit_message_from"`)
			dirtyOtherRepo(t, fx)

			if mode == config.AutoCommitFull {
				// Task a closed Milestone 1, so the commit must name it.
				require.Contains(t, instr, "last open task of its milestone")
				stageCommitMessage(t, fx, "Implement billing task a\n\nThe reader.\n")
				stdout, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("finished"))
				require.Equal(t, 1, code)
				require.Equal(t, "commit_message_invalid", decodeError(t, stdout).Code)
				stageCommitMessage(t, fx, "Implement billing Milestone 1: task a\n\nThe reader.\n")
			} else {
				require.NotContains(t, instr, "last open task of its milestone")
				stageCommitMessage(t, fx, "Implement billing task a\n\nThe reader.\n")
			}

			stdout, _, code = runRootCmd(t, "implement", "goto", "--data", gotoWithStagedMessage("finished"))
			require.Equal(t, 0, code, stdout)
			require.Equal(t, "finished", currentStep(t, fx))
			require.Equal(t, "2", commitCount(t, fx.other), "the task run's work is committed")
			require.Contains(t, gittest.RunGit(t, fx.other, "log", "-1", "--format=%B"), "billing")

			if mode == config.AutoCommitFull {
				require.Equal(t, []any{float64(1)}, stateData(t, fx)["committed_milestones"],
					"the milestone the task closed is recorded, so no later run asks for it again")
			}
		})
	}
}
