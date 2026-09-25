package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// exportPlan is a three-task plan across two milestones, one task complete
// and one needing a person.
func exportPlan(readerDone bool) string {
	return taskPlanDoc(
		taskBlock("Add a plan task reader", readerDone, agentFields(idA))+
			taskBlock("Add the plan export command", false, agentFields(idB, idA+" — Add a plan task reader")),
		taskBlock("Publish the release signing key", false, []string{
			"**Id:** " + idC, "**Repo:** testproj",
			dependsLine(idB + " — Add the plan export command"),
			"**Execution:** human — needs access to the production key vault",
		}),
	)
}

func exportProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	stdout, code := writePlanDoc(t, taskPlanName, "plan.md", exportPlan(true))
	require.Equal(t, 0, code, stdout)
	return dir
}

func exportJSON(t *testing.T) map[string]any {
	t.Helper()
	stdout, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", "json")
	require.Equal(t, 0, code, stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

func TestPlanExport_JSONCarriesTheDesignFields(t *testing.T) {
	exportProject(t)

	stdout, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", "json")
	require.Equal(t, 0, code, stdout)

	var got struct {
		Kind           string          `json:"kind"`
		Name           string          `json:"name"`
		DocumentStatus string          `json:"document_status"`
		Tasks          []map[string]any `json:"tasks"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "plan", got.Kind)
	require.Equal(t, taskPlanName, got.Name)
	require.Equal(t, "draft", got.DocumentStatus)
	require.Len(t, got.Tasks, 3)

	fields := []string{"id", "title", "milestone", "repo", "depends_on", "execution", "completed"}
	for _, task := range got.Tasks {
		keys := make([]string, 0, len(task))
		for k := range task {
			keys = append(keys, k)
		}
		require.ElementsMatch(t, fields, keys)
	}

	require.Equal(t, map[string]any{
		"id":         idA,
		"title":      "Add a plan task reader",
		"milestone":  float64(1),
		"repo":       map[string]any{"name": "testproj", "location": ""},
		"depends_on": []any{},
		"execution":  map[string]any{"type": "agent", "reason": ""},
		"completed":  true,
	}, got.Tasks[0], "a task declaring none has an empty dependency list, not null")
	require.Equal(t, idB, got.Tasks[1]["id"])
	require.Equal(t, []any{idA}, got.Tasks[1]["depends_on"])
	require.Equal(t, false, got.Tasks[1]["completed"])
	require.Equal(t, map[string]any{
		"id":         idC,
		"title":      "Publish the release signing key",
		"milestone":  float64(2),
		"repo":       map[string]any{"name": "testproj", "location": ""},
		"depends_on": []any{idB},
		"execution":  map[string]any{"type": "human", "reason": "needs access to the production key vault"},
		"completed":  false,
	}, got.Tasks[2])
}

func TestPlanExport_PrettyIsTheDefault(t *testing.T) {
	exportProject(t)

	def, _, code := runRootCmd(t, "plan", "export", taskPlanName)
	require.Equal(t, 0, code, def)
	pretty, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", "pretty")
	require.Equal(t, 0, code, pretty)
	require.Equal(t, def, pretty)

	for _, want := range []string{
		taskPlanName + "  (draft)  1/3 tasks complete",
		"Milestone 1", "Milestone 2",
		"[x] Add a plan task reader",
		"[ ] Add the plan export command",
		"human: needs access to the production key vault",
		idC,
		"depends on: Add the plan export command",
	} {
		require.Contains(t, pretty, want)
	}
}

func TestPlanExport_UnsupportedFormatIsRefused(t *testing.T) {
	exportProject(t)

	stdout, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", "yaml")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "export_format_unsupported", er.Code)
	require.Contains(t, er.Message, "pretty")
	require.Contains(t, er.Message, "json")
	require.NotContains(t, stdout, "Add a plan task reader", "no tasks are printed")
}

func TestPlanExport_LocationComesOnlyFromADeclaredGitSource(t *testing.T) {
	t.Run("git source", func(t *testing.T) {
		dir := exportProject(t)
		rc := config.NewDefaultRepoConfig()
		rc.Source = config.GitSource("https://github.com/example/widgets")
		require.NoError(t, rc.ToYAMLFile(filepath.Join(dir, ".spektacular", config.RepoConfigFileName)))

		for _, task := range exportJSON(t)["tasks"].([]any) {
			require.Equal(t, "https://github.com/example/widgets", task.(map[string]any)["repo"].(map[string]any)["location"])
		}
	})

	t.Run("file source with a git remote", func(t *testing.T) {
		dir := exportProject(t)
		gittest.RunGit(t, dir, "init", "-q")
		gittest.RunGit(t, dir, "remote", "add", "origin", "https://github.com/example/should-not-appear")

		for _, task := range exportJSON(t)["tasks"].([]any) {
			require.Equal(t, "", task.(map[string]any)["repo"].(map[string]any)["location"])
		}
	})
}

func TestPlanExport_ReflectsTheCurrentPlan(t *testing.T) {
	exportProject(t)
	require.Equal(t, false, exportJSON(t)["tasks"].([]any)[1].(map[string]any)["completed"])

	ticked := strings.Replace(exportPlan(true), "#### - [ ] Task: Add the plan export command", "#### - [x] Task: Add the plan export command", 1)
	_, code := writePlanDoc(t, taskPlanName, "plan.md", ticked)
	require.Equal(t, 0, code)
	require.Equal(t, true, exportJSON(t)["tasks"].([]any)[1].(map[string]any)["completed"])
}

func TestPlanExport_DocumentStatusMatchesPlanStatus(t *testing.T) {
	exportProject(t)

	statusOf := func() string {
		stdout, _, code := runRootCmd(t, "plan", "status", taskPlanName)
		require.Equal(t, 0, code, stdout)
		var s map[string]any
		require.NoError(t, json.Unmarshal([]byte(stdout), &s))
		return s["document_status"].(string)
	}

	require.Equal(t, "draft", statusOf())
	require.Equal(t, statusOf(), exportJSON(t)["document_status"])

	_, _, code := runRootCmd(t, "plan", "file", "set-document-status", taskPlanName+"/plan.md", "--document-status", "final")
	require.Equal(t, 0, code)
	require.Equal(t, "final", statusOf())
	require.Equal(t, statusOf(), exportJSON(t)["document_status"])
}

func TestPlanExport_MissingPlanIsAStructuredError(t *testing.T) {
	exportProject(t)
	stdout, _, code := runRootCmd(t, "plan", "export", "20260709000000-nosuch")
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Equal(t, "artifact_not_found", er.Code)
	require.NotEmpty(t, er.NextAction)
}

func TestPlanExport_PlanWithoutTasksSaysWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan.md", "# Plan\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n#### - [ ] Phase 1.1: Work\n")
	require.Equal(t, 0, code)

	for _, format := range []string{"pretty", "json"} {
		stdout, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", format)
		require.Equal(t, 1, code)
		er := decodeError(t, stdout)
		require.Equal(t, plantask.CodeStructureInvalid, er.Code)
		require.Contains(t, er.Message, "task ids")
		for _, word := range []string{"version", "old", "legacy", "age"} {
			require.NotContains(t, strings.ToLower(er.Message), word)
		}
	}
}

// Success metric: a plan accepted by `plan file write` always exports.
func TestPlanExport_EverySavedPlanExports(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	for _, body := range []string{exportPlan(true), exportPlan(false), validTaskPlan()} {
		_, code := writePlanDoc(t, taskPlanName, "plan.md", body)
		require.Equal(t, 0, code)
		for _, format := range []string{"pretty", "json"} {
			stdout, _, code := runRootCmd(t, "plan", "export", taskPlanName, "--format", format)
			require.Equal(t, 0, code, stdout)
		}
	}
}
