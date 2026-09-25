package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

const idD = "44444444-4444-4444-8444-444444444444"

func namedPlanStatus(t *testing.T) map[string]any {
	t.Helper()
	stdout, _, code := runRootCmd(t, "plan", "status", taskPlanName)
	require.Equal(t, 0, code, stdout)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// previousStatusKeys is every key `plan status <name>` reported before
// per-task progress, written out by hand.
var previousStatusKeys = []string{
	"error", "kind", "name", "artifact_id", "document_status", "current_step",
	"completed_steps", "created_at", "modified_at", "closed_at", "spec", "plan",
}

func TestPlanStatus_ReportsProgressPerTask(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan.md", taskPlanDoc(
		taskBlock("One", true, agentFields(idA), "- [x] a", "- [x] b", "- [ ] c")+
			taskBlock("Two", true, agentFields(idB), "- [x] a", "- [ ] b"),
		taskBlock("Three", false, agentFields(idC), "- [ ] a")+
			taskBlock("Four", false, agentFields(idD)),
	))
	require.Equal(t, 0, code)

	got := namedPlanStatus(t)
	require.Equal(t, map[string]any{"tasks_completed": float64(2), "tasks_total": float64(4)}, got["progress"])
	require.Equal(t, []any{
		map[string]any{"id": idA, "title": "One", "milestone": float64(1), "completed": true,
			"acceptance_criteria": map[string]any{"met": float64(2), "total": float64(3)}},
		map[string]any{"id": idB, "title": "Two", "milestone": float64(1), "completed": true,
			"acceptance_criteria": map[string]any{"met": float64(1), "total": float64(2)}},
		map[string]any{"id": idC, "title": "Three", "milestone": float64(2), "completed": false,
			"acceptance_criteria": map[string]any{"met": float64(0), "total": float64(1)}},
		map[string]any{"id": idD, "title": "Four", "milestone": float64(2), "completed": false,
			"acceptance_criteria": map[string]any{"met": float64(0), "total": float64(0)}},
	}, got["tasks"], "completion and criteria are separate: task Two is complete with a criterion unmet")

	for _, k := range previousStatusKeys {
		require.Contains(t, got, k)
	}
}

func TestPlanStatus_LegacyPlanKeepsItsPreviousShape(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	_, code := writePlanDoc(t, taskPlanName, "plan.md", "# Plan\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n#### - [x] Phase 1.1: Work\n")
	require.Equal(t, 0, code)

	got := namedPlanStatus(t)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, previousStatusKeys, keys)
}

func TestPlanStatus_SchemaAddsProgressOnlyForPlans(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	schemaOf := func(kind string) commandSchema {
		stdout, _, code := runRootCmd(t, kind, "status", "x", "--schema")
		require.Equal(t, 0, code, stdout)
		var s commandSchema
		require.NoError(t, json.Unmarshal([]byte(stdout), &s))
		return s
	}
	plan := schemaOf("plan")
	require.Contains(t, plan.Output.Properties, "progress")
	require.Contains(t, plan.Output.Properties, "tasks")
	require.Contains(t, plan.Output.Properties, "document_status")

	spec := schemaOf("spec")
	require.NotContains(t, spec.Output.Properties, "progress")
	require.NotContains(t, spec.Output.Properties, "tasks")
}
