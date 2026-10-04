package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// orchestratedSectionHeading opens the section a per-spec workflow skill
// carries for the case where an epic orchestrator, not the user, started it.
// Hand-maintained here rather than read from the skills.
const orchestratedSectionHeading = "# When an orchestrator starts this skill"

// orchestratedSkillCase pins one per-spec skill's orchestrated section. The
// expected phrases are hand-maintained copies of the skill text, so a reword
// that drops a load-bearing instruction fails here instead of silently
// changing what a child agent is told.
type orchestratedSkillCase struct {
	path    string
	phrases []string
}

var orchestratedSkillCases = []orchestratedSkillCase{
	{
		path: "skills/workflows/spek-plan/SKILL.md",
		phrases: []string{
			"The `spek-plan-epic` skill plans a whole epic by starting one agent per spec, each running this skill.",
			"`{{command}} plan new --data '{\"name\":\"<spec_name>\",\"orchestrated\":true}'`",
			"skips the uncommitted-changes question",
			"Running the same command again resumes the lane.",
			"Every `goto` carries `\"name\":\"<spec_name>\"`",
			"Never ask the user anything yourself.",
			"a final message whose first line is `QUESTION: <spec_name>`",
			"There is no sign-off walkthrough: the walkthrough step has you prepare a summary instead.",
			"End the run with `DONE: <spec_name>` and that summary, or with `FAILED: <spec_name>` and the reason.",
		},
	},
	{
		path: "skills/workflows/spek-implement/SKILL.md",
		phrases: []string{
			"The `spek-implement-epic` skill implements a whole epic by starting one agent per spec, each running this skill in the spec's own worktree.",
			"`{{command}} implement new --data '{\"name\":\"<spec_name>\",\"orchestrated\":true}'`, from the worktree you were given.",
			"skips the uncommitted-changes question",
			"Running the same command again resumes the lane.",
			"Every `goto` carries `\"name\":\"<spec_name>\"`",
			"never ask whether to continue between tasks: tasks run one after another.",
			"a final message whose first line is `QUESTION: <spec_name>`",
			"End the run with `DONE: <spec_name>` and the completion summary, or with `FAILED: <spec_name>` and the reason.",
		},
	},
}

// splitOrchestratedSection returns the skill body before the orchestrated
// section and the section itself, requiring the heading exactly once.
func splitOrchestratedSection(t *testing.T, path string) (before, section string) {
	t.Helper()
	content, err := FS.ReadFile(path)
	require.NoErrorf(t, err, "reading %s", path)
	body := string(content)
	require.Equalf(t, 1, strings.Count(body, orchestratedSectionHeading+"\n"),
		"%s must carry the %q section exactly once", path, orchestratedSectionHeading)
	i := strings.Index(body, orchestratedSectionHeading+"\n")
	return body[:i], body[i:]
}

// Each per-spec skill explains what changes when an orchestrator starts it.
func TestPerSpecSkillsExplainOrchestratedStart(t *testing.T) {
	for _, c := range orchestratedSkillCases {
		_, section := splitOrchestratedSection(t, c.path)
		for _, phrase := range c.phrases {
			require.Containsf(t, section, phrase,
				"%s's orchestrated section lost a load-bearing phrase", c.path)
		}
	}
}

// The orchestrated section is the skill's final section and the only place the
// orchestrated run is described, so a user-driven run reads the rest of the
// skill exactly as before.
func TestOrchestratedSectionIsSelfContainedAndLast(t *testing.T) {
	for _, c := range orchestratedSkillCases {
		before, section := splitOrchestratedSection(t, c.path)

		// No further top-level heading follows the section.
		for i, line := range strings.Split(section, "\n")[1:] {
			require.Falsef(t, strings.HasPrefix(line, "# "),
				"%s: the orchestrated section must be the skill's last section, but heading %q follows it (line %d of the section)",
				c.path, line, i+2)
		}

		// The orchestrated-only instructions live in the section alone.
		for _, needle := range []string{`"orchestrated":true`, "QUESTION:", "DONE:", "FAILED:"} {
			require.NotContainsf(t, before, needle,
				"%s: %q belongs only in the orchestrated section, not in the user-driven flow", c.path, needle)
		}
	}
}
