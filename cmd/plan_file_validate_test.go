package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/stretchr/testify/require"
)

const (
	idA = "11111111-1111-4111-8111-111111111111"
	idB = "22222222-2222-4222-8222-222222222222"
	idC = "33333333-3333-4333-8333-333333333333"
)

// validTaskPlan is a two-task plan every refusal test starts from.
func validTaskPlan() string {
	return taskPlanDoc(
		taskBlock("Build the reader", false, agentFields(idA), "- [ ] reads"),
		taskBlock("Build the export", false, agentFields(idB, idA+" — Build the reader"), "- [ ] exports"),
	)
}

func planOnDisk(t *testing.T, dir string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".spektacular", "plans", taskPlanName, "plan.md"))
	require.NoError(t, err)
	return b
}

func TestPlanFileWrite_RefusesInvalidTaskStructure(t *testing.T) {
	reader := taskBlock("Build the reader", false, agentFields(idA))
	cases := []struct {
		name   string
		broken string // the task block that breaks a rule, titled "Broken"
	}{
		{"missing id", taskBlock("Broken", false, []string{"**Repo:** testproj", "**Depends on:** none", "**Execution:** agent"})},
		{"missing repo", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Depends on:** none", "**Execution:** agent"})},
		{"two repos", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** testproj, other", "**Depends on:** none", "**Execution:** agent"})},
		{"unregistered repo", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** nowhere", "**Depends on:** none", "**Execution:** agent"})},
		{"missing dependency declaration", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** testproj", "**Execution:** agent"})},
		{"unknown dependency", taskBlock("Broken", false, agentFields(idB, idC+" — Not in this plan"))},
		{"missing execution", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** testproj", "**Depends on:** none"})},
		{"unknown execution", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** testproj", "**Depends on:** none", "**Execution:** robot"})},
		{"human without reason", taskBlock("Broken", false, []string{"**Id:** " + idB, "**Repo:** testproj", "**Depends on:** none", "**Execution:** human"})},
		{"duplicate id", taskBlock("Broken", false, agentFields(idA))},
		{"dependency cycle", taskBlock("Broken", false, agentFields(idB, idB+" — Broken"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "")

			_, code := writePlanDoc(t, taskPlanName, "plan.md", validTaskPlan())
			require.Equal(t, 0, code)
			before := planOnDisk(t, dir)

			stdout, code := writePlanDoc(t, taskPlanName, "plan.md", taskPlanDoc(reader+tc.broken))
			require.Equal(t, 1, code)
			er := decodeError(t, stdout)
			require.Equal(t, plantask.CodeTaskInvalid, er.Code)
			require.Contains(t, er.Message, `"Broken"`)
			require.NotEmpty(t, er.NextAction)

			require.Equal(t, string(before), string(planOnDisk(t, dir)), "a refused write leaves the stored plan unchanged")
		})
	}
}

func TestPlanFileWrite_UnregisteredRepoListsRegisteredRepos(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "repos:\n  - name: alpha\n    location: .\n  - name: beta\n    location: .\n")

	stdout, code := writePlanDoc(t, taskPlanName, "plan.md", taskPlanDoc(
		taskBlock("Wrong repo", false, []string{"**Id:** " + idA, "**Repo:** gamma", "**Depends on:** none", "**Execution:** agent"}),
	))
	require.Equal(t, 1, code)
	er := decodeError(t, stdout)
	require.Contains(t, er.Message, `"gamma"`)
	require.Contains(t, er.NextAction, "alpha")
	require.Contains(t, er.NextAction, "beta")
}

func TestPlanFileWrite_AcceptsValidAndNonTaskDocuments(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	human := taskBlock("Publish the key", false, []string{"**Id:** " + idC, "**Repo:** testproj", dependsLine(idB + " — Build the export"), "**Execution:** human — needs the key vault"})
	valid := taskPlanDoc(
		taskBlock("Build the reader", true, agentFields(idA)),
		taskBlock("Build the export", false, agentFields(idB, idA+" — Build the reader"))+human,
	)
	legacy := "# Plan\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n#### - [ ] Phase 1.1: Old work\n**Repo:** anything, at all\n"
	// Documents other than plan.md are never checked, even when they quote
	// task headings that would be refused in plan.md.
	stray := "## Milestones & Tasks\n\n#### - [ ] Task: Quoted in prose\n"

	for _, doc := range []struct{ path, body string }{
		{"plan.md", valid},
		{"plan.md", legacy},
		{"context.md", stray},
		{"research.md", stray},
		{"test-plan.md", stray},
	} {
		stdout, code := writePlanDoc(t, taskPlanName, doc.path, doc.body)
		require.Equal(t, 0, code, "%s: %s", doc.path, stdout)
	}
}

func TestPlanFileWrite_IdsSurviveEdits(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")

	_, code := writePlanDoc(t, taskPlanName, "plan.md", validTaskPlan())
	require.Equal(t, 0, code)

	// Reorder the two tasks, retitle one and add a third.
	edited := taskPlanDoc(
		taskBlock("Build the export command", false, agentFields(idB, idA+" — Build the reader")) +
			taskBlock("Build the reader", false, agentFields(idA)) +
			taskBlock("Document it", false, agentFields(idC, idB+" — Build the export command")),
	)
	stdout, code := writePlanDoc(t, taskPlanName, "plan.md", edited)
	require.Equal(t, 0, code, stdout)

	p := plantask.Parse(planOnDisk(t, dir))
	got := map[string]string{}
	for _, task := range p.Tasks {
		got[task.Title] = task.ID
	}
	require.Equal(t, map[string]string{
		"Build the export command": idB,
		"Build the reader":         idA,
		"Document it":              idC,
	}, got)
}
