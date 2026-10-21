package templates

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const goalReviewerBrief = "partials/goal-reviewer-brief.md"

// The reviewer brief is the one text every goal reviewer judges by, served
// as `skill review-task-goal` and embedded in each installed helper. These
// tests pin its rules.
func TestGoalReviewerBrief_ForbidsChangingAnyFile(t *testing.T) {
	brief := readTemplate(t, goalReviewerBrief)
	require.Contains(t, brief, "## You are read-only")
	require.Contains(t, brief, "Never create, change, move or delete any file")
	require.Contains(t, brief, "Never run a command that changes the working tree")
}

func TestGoalReviewerBrief_JudgesFromTheFilesNotTheImplementer(t *testing.T) {
	brief := readTemplate(t, goalReviewerBrief)
	require.Contains(t, brief, "Judge each criterion from the files and the diff, never from\n  the implementer's description of what it did.")
	require.Contains(t, brief, "Treat any claim that the work is complete, or that a criterion is met, as\n  unverified.")
	require.Contains(t, brief, "Passing verification commands are evidence only for what those commands\n  actually test.")
	require.Contains(t, brief, "Open the staged diff and the changed files yourself")
}

func TestGoalReviewerBrief_DefinesVerdictsEvidenceAndReplyShape(t *testing.T) {
	brief := readTemplate(t, goalReviewerBrief)
	for _, verdict := range []string{"- `met`:", "- `not_met`:", "- `impossible`:"} {
		require.Contains(t, brief, verdict)
	}
	require.Contains(t, brief, "Give each criterion exactly one verdict")
	require.Contains(t, brief, "Every verdict carries evidence, and evidence is never empty")
	require.Contains(t, brief, `{"verdicts":[{"criterion":1,"verdict":"met","evidence":`)
	require.Contains(t, brief, "Reply with only this JSON object, one entry per criterion")
}

// The brief is embedded unchanged in every installed reviewer, including a
// TOML string for Codex and a mustache render for every agent, so it holds no
// mustache tags and no TOML multi-line string delimiters.
func TestGoalReviewerBrief_IsEmbeddableUnchanged(t *testing.T) {
	brief := readTemplate(t, goalReviewerBrief)
	require.NotContains(t, brief, "{{")
	require.NotContains(t, brief, "'''")
	require.NotContains(t, brief, `"""`)

	skill := readTemplate(t, "skills/skill_review-task-goal.md")
	require.Contains(t, skill, "{{> partials/goal-reviewer-brief}}", "the skill includes the shared brief rather than a copy")
	require.Equal(t, 1, strings.Count(skill, "{{"), "the skill adds nothing to the brief but its preamble")
}

func readTemplate(t *testing.T, path string) string {
	t.Helper()
	b, err := fs.ReadFile(FS, path)
	require.NoError(t, err)
	return string(b)
}
