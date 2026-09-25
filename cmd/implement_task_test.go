package cmd

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/stretchr/testify/require"
)

// taskRunPlan has one completed task, one open task depending on it, one
// open task depending on an open task, and one human task.
func taskRunPlan() string {
	return taskPlanDoc(
		taskBlock("Done already", true, agentFields(idA))+
			taskBlock("Ready to start", false, agentFields(idB, idA+" — Done already")),
		taskBlock("Blocked", false, agentFields(idC, idB+" — Ready to start"))+
			taskBlock("Needs a person", false, []string{
				"**Id:** " + idD, "**Repo:** testproj", "**Depends on:** none",
				"**Execution:** human — needs the production signing key",
			}),
	)
}

func taskRunProject(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	stdout, code := writePlanDoc(t, taskPlanName, "plan", body)
	require.Equal(t, 0, code, stdout)
	return filepath.Join(dir, ".spektacular", "state.json")
}

func implementNewTask(t *testing.T, task string) (string, int) {
	t.Helper()
	stdout, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`","task":"`+task+`"}`)
	return stdout, code
}

func TestImplementNewTask_RefusesTasksThatCannotStart(t *testing.T) {
	cases := []struct {
		name     string
		task     string
		code     string
		contains []string
	}{
		{"unknown task", "99999999-9999-4999-8999-999999999999", "task_not_found", []string{"99999999-9999-4999-8999-999999999999"}},
		{"completed task", idA, "task_completed", []string{idA, "Done already"}},
		{"incomplete dependency", idC, "task_dependencies_incomplete", []string{idB, "Ready to start"}},
		{"human task", idD, "task_requires_human", []string{"needs the production signing key"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			statePath := taskRunProject(t, taskRunPlan())

			stdout, code := implementNewTask(t, tc.task)
			require.Equal(t, 1, code)
			er := decodeError(t, stdout)
			require.Equal(t, tc.code, er.Code)
			for _, want := range tc.contains {
				require.Contains(t, er.Message, want)
			}
			require.NotEmpty(t, er.NextAction)
			require.NoFileExists(t, statePath, "a refused task run starts no workflow")
		})
	}
}

func TestImplementNewTask_DependencyRefusalNamesTheNextRun(t *testing.T) {
	taskRunProject(t, taskRunPlan())
	stdout, _ := implementNewTask(t, idC)
	er := decodeError(t, stdout)
	require.Contains(t, er.NextAction, `"task":"`+idB+`"`)
}

func TestImplementNewTask_PlanWithoutTasksIsRefused(t *testing.T) {
	statePath := taskRunProject(t, "# Plan\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n#### - [ ] Phase 1.1: Work\n")

	stdout, code := implementNewTask(t, idA)
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, plantask.CodeStructureInvalid, er.Code)
	require.Contains(t, er.Message, "no task ids")
	require.NoFileExists(t, statePath)
}

func TestImplementNewTask_StartsAndReportsTheTask(t *testing.T) {
	statePath := taskRunProject(t, taskRunPlan())

	stdout, code := implementNewTask(t, idB)
	require.Equal(t, 0, code, stdout)
	var res map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	require.Equal(t, "read_plan", res["step"])
	require.FileExists(t, statePath)

	stdout, _, code = runRootCmd(t, "implement", "status")
	require.Equal(t, 0, code, stdout)
	var status map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &status))
	require.Equal(t, idB, status["task"])

	// A second start finds the run in progress; its resume report carries
	// the task.
	stdout, code = implementNewTask(t, idB)
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Contains(t, er.Message, idB)
	require.Contains(t, er.NextAction, "only task `"+idB+"`")
}

func TestImplementNew_WithoutATaskReportsNoTask(t *testing.T) {
	taskRunProject(t, taskRunPlan())

	_, _, code := runRootCmd(t, "implement", "new", "--data", `{"name":"`+taskPlanName+`"}`)
	require.Equal(t, 0, code)
	stdout, _, code := runRootCmd(t, "implement", "status")
	require.Equal(t, 0, code)
	var status map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &status))
	require.NotContains(t, status, "task")
}
