package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// workflowSkills are the driving-agent playbooks whose "How to start"
// sections must react to a resume report rather than inspecting state.json.
// The guided add joined them once it became a CLI-owned workflow of its own.
var workflowSkills = []string{
	"skills/workflows/spek-new/SKILL.md",
	"skills/workflows/spek-plan/SKILL.md",
	"skills/workflows/spek-implement/SKILL.md",
	"skills/workflows/spek-manage-repos/SKILL.md",
}

// TestWorkflowSkillsAreResumeAware verifies the acceptance criteria for Phase
// 3.2: each playbook tells the agent to react to a resume report (prompt the
// user, read the working-context file, and force a fresh start), and no
// playbook instructs the agent to read .spektacular/state.json directly.
func TestWorkflowSkillsAreResumeAware(t *testing.T) {
	for _, skill := range workflowSkills {
		content, err := FS.ReadFile(skill)
		require.NoErrorf(t, err, "reading %s", skill)
		body := string(content)

		// AC #4: no playbook references the state file directly.
		require.NotContainsf(t, body, "state.json",
			"%s must not instruct the agent to read .spektacular/state.json directly", skill)

		// AC #1-3: the resume-report flow is present — recognise the report,
		// read the working-context file, and offer a forced fresh start.
		require.Containsf(t, body, "resumable",
			"%s must describe recognising a resume report (\"resumable\": true)", skill)
		require.Containsf(t, body, ".spektacular/working-context.md",
			"%s must tell the agent to read the working-context file on resume", skill)
		require.Containsf(t, body, "--force",
			"%s must give the start-fresh (new --force) command", skill)
		require.Truef(t, strings.Contains(body, "resume") && strings.Contains(body, "start a new"),
			"%s must prompt the user to resume vs start a new workflow", skill)

		// Only the spec and plan workflows gather content into per-section
		// working files, so only their playbooks describe them. The implement
		// playbook has no assembled document, and a guided add carries every
		// answer inside the workflow itself, so neither may reference that
		// directory. Asserting the absence keeps the spec/plan-only scope
		// honest rather than letting the phrase drift into playbooks it does
		// not apply to.
		switch {
		case strings.HasSuffix(skill, "spek-implement/SKILL.md"),
			strings.HasSuffix(skill, "spek-manage-repos/SKILL.md"):
			require.NotContainsf(t, body, ".spektacular/work/",
				"%s must not reference the spec/plan per-section working-file directory", skill)
		default:
			require.Containsf(t, body, ".spektacular/work/",
				"%s must describe the per-section working files under .spektacular/work/", skill)
		}
	}
}

// TestImplementSkillResumeReadsPlanFirst verifies the implement playbook's
// resume path points at the plan documents and the first unchecked phase,
// while the plan-documents partial is included only once in the skill (in its
// "The plan documents" section) rather than repeated in the resume steps.
func TestImplementSkillResumeReadsPlanFirst(t *testing.T) {
	content, err := FS.ReadFile("skills/workflows/spek-implement/SKILL.md")
	require.NoError(t, err)
	body := string(content)

	require.Contains(t, body, "The plan documents")
	require.Contains(t, body, "first unchecked")
	require.Equal(t, 1, strings.Count(body, "{{> partials/implement-plan-documents}}"),
		"the plan-documents partial must be included exactly once")
}

// TestImplementSkillDocumentsSingleTaskRuns verifies the implement playbook
// tells the agent how to start a run for one task, and how to find a task's
// id when the user names it by title.
func TestImplementSkillDocumentsSingleTaskRuns(t *testing.T) {
	content, err := FS.ReadFile("skills/workflows/spek-implement/SKILL.md")
	require.NoError(t, err)
	body := string(content)

	require.Contains(t, body, `implement new --data '{"name": "<plan_name>", "task": "<task_id>"}'`)
	require.Contains(t, body, "plan export <plan_name> --format json")
	for _, code := range []string{"task_not_found", "task_completed", "task_dependencies_incomplete", "task_requires_human"} {
		require.Contains(t, body, code)
	}
}
