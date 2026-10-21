package cmd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file tests the skill command family's serving of library skills
// (templates/skills/skill_*.md via `skill <name>` and `skill list`), focused
// on Phase 4.2's repo-management and cross-repo orchestration content. The
// wrapper/envelope mechanics themselves are covered in root_test.go.

// skillProject seeds a minimal project fixture in a fresh temp dir and chdirs
// into it, so skill invocations never read or touch this repository's own
// .spektacular state.
func skillProject(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
}

// fetchSkillInstructions runs `skill <name>` through the production wrapper
// and returns the served instructions from the decoded success envelope.
func fetchSkillInstructions(t *testing.T, name string) string {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, "skill", name)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &m))
	require.Equal(t, false, m["error"])
	require.Equal(t, name, m["name"], "the served skill must identify itself by name")

	instructions, ok := m["instructions"].(string)
	require.True(t, ok, "the envelope must carry string instructions")
	require.NotEmpty(t, instructions)
	return instructions
}

// The skill listing enumerates the library skills only. `spek-manage-repos` is
// installed into the agent's own skills directory by `init <agent>` (see
// internal/agent/skills.go's workflowSkills), so it is deliberately absent
// here: an installed skill served raw as well would be two copies of the same
// content, free to drift.
func TestSkillList_EnumeratesLibrarySkillsOnly(t *testing.T) {
	skillProject(t)
	stdout, stderr, code := runRootCmd(t, "skill", "list")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var listing struct {
		Skills []string `json:"skills"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listing))
	require.Contains(t, listing.Skills, "spawn-planning-agents",
		"the listing must still include the existing planning skill")
	require.Contains(t, listing.Skills, "spawn-implementation-agents",
		"the listing must still include the existing implementation skill")
	require.NotContains(t, listing.Skills, "spek-manage-repos",
		"spek-manage-repos is installed by init, not served as a library skill")
}

// An agent probing the library with a wrong name is steered to the skills the
// library actually serves.
func TestSkillUnknownName_ErrorListsAvailableSkills(t *testing.T) {
	skillProject(t)
	stdout, stderr, code := runRootCmd(t, "skill", "does-not-exist")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)
	require.Contains(t, stdout, "spawn-planning-agents",
		"the unknown-skill error must list the available skills")
}

// Criterion 3: the spawn-implementation-agents skill directs carrying each
// piece of attributed work out inside its repo's resolved root, keeping one
// agent's changes per repo so verification and changelog derivation stay
// per-repo.
func TestSkillSpawnImplementationAgents_DirectsWorkToAttributedRepo(t *testing.T) {
	skillProject(t)
	instructions := fetchSkillInstructions(t, "spawn-implementation-agents")

	require.Contains(t, instructions, "Work belongs in its attributed repo",
		"the skill must open with the attributed-repo rule")
	require.Contains(t, instructions, "carry that work out inside that repo's code",
		"attributed work must be carried out in the repo's code")
	require.Contains(t, instructions, "repo list",
		"where a repo's code lives must come from the `repo list` command")
	require.Contains(t, instructions, "changelog derivation stay per-repo",
		"changes must stay per-repo so derived changelog entries follow the attribution")
}

// Criteria 1 and 2: the spawn-planning-agents skill fans research out per
// registered repo, using each repo's metadata to scope which repos a question
// belongs to.
func TestSkillSpawnPlanningAgents_FansResearchOutPerRepo(t *testing.T) {
	skillProject(t)
	instructions := fetchSkillInstructions(t, "spawn-planning-agents")

	require.Contains(t, instructions, "Multi-repo projects",
		"the skill must carry the multi-repo fan-out paragraph")
	require.Contains(t, instructions, "fan the research out per repo",
		"research must fan out across registered repos")
	require.Contains(t, instructions, "repo list",
		"resolved local paths must come from the `repo list` command")
	require.Contains(t, instructions, "description, role, and tags",
		"repo metadata must scope which repos a research question is relevant to")
}

// The goal reviewer's brief is served as a library skill, with its shared
// partial rendered in, and listed with the other skills.
func TestSkillReviewTaskGoal_ServesTheReviewerBrief(t *testing.T) {
	skillProject(t)
	instructions := fetchSkillInstructions(t, "review-task-goal")
	require.Contains(t, instructions, "# Review Task Goal")
	require.Contains(t, instructions, "## You are read-only")
	require.Contains(t, instructions, `{"verdicts":[`)
	require.NotContains(t, instructions, "{{", "the partial is rendered, not served raw")

	stdout, _, code := runRootCmd(t, "skill", "list")
	require.Equal(t, 0, code)
	var listed struct {
		Skills []string `json:"skills"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &listed))
	require.Contains(t, listed.Skills, "review-task-goal")
}
