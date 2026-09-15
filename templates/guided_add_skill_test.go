package templates

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// manageReposSkill is the playbook the agent loads when a user asks to add a
// repo. It is the driver of a CLI-owned workflow, not a description of one,
// which is what the tests below hold it to.
const manageReposSkill = "skills/workflows/spek-manage-repos/SKILL.md"

// loopDisciplineMarkers are hand-written sentences from the block that keeps
// the agent driving the workflow. Each covers a different way the agent can
// stop too early: treating any single call as done, treating the first step
// as the whole job, and treating a clean `repo new` response as the finish
// line. `repo new` is named explicitly because it is the call the agent is
// most likely to mistake for completion, being the only one it runs cold.
var loopDisciplineMarkers = []string{
	"A single successful CLI call, including the very first `repo new`, is **NOT** task completion.",
	"**This is a loop. Do not stop after the first step.**",
	"**Concretely: do not stop after `repo new`.**",
}

// TestManageReposSkillKeepsDrivingAfterTheFirstCall verifies that the skill
// tells the agent to keep looping instead of reporting success once a command
// has returned cleanly. A skill that only described the commands would leave
// the agent free to stop after the first one, which is the failure this block
// exists to prevent.
func TestManageReposSkillKeepsDrivingAfterTheFirstCall(t *testing.T) {
	body := mustReadTemplate(t, manageReposSkill)

	for _, marker := range loopDisciplineMarkers {
		require.Containsf(t, body, marker,
			"the skill must carry the loop-discipline sentence %q", marker)
	}

	// The loop is only closed if the skill also says what ends it, otherwise
	// "keep going" has no terminating condition.
	require.Contains(t, body, "until a returned instruction tells you the workflow is *finished*",
		"the skill must name the condition that ends the loop")
}

// TestManageReposSkillDescribesResumingAndDiscarding verifies that the skill
// covers the in-progress case in full: recognising the resume report, the one
// branch the repo skill cannot handle itself, and both of the user's two
// options. Describing only one option would leave the agent guessing at the
// other.
func TestManageReposSkillDescribesResumingAndDiscarding(t *testing.T) {
	body := mustReadTemplate(t, manageReposSkill)

	require.Contains(t, body, `"resumable": true`,
		"the skill must describe recognising a resume report by its resumable flag")

	// A resume report can belong to a different workflow, which this skill
	// must hand off rather than try to drive.
	require.Contains(t, body, "If it is not `repo`",
		"the skill must branch on a resume report belonging to another workflow's kind")

	require.Contains(t, body, `repo goto --data '{"step":"<current_step>"}'`,
		"the skill must give the resume command, carrying the report's current_step")

	require.Contains(t, body, "repo new --force",
		"the skill must give the command that discards the in-progress add")
}

// removedAddHeadings are the section headings of the hand-written eight-step
// add flow the skill used to carry. They now live in templates/steps/repo/,
// where the CLI hands them to the agent one at a time. A copy left behind in
// the skill would be a second, silently diverging account of the same
// conversation, and the agent would read it before ever calling the CLI.
var removedAddHeadings = []string{
	"## 1. See what is already registered",
	"## 2. Ask which repo",
	"## 4. Ask for the rest one at a time",
	"## 5. Where Spektacular's files go",
	"## 6. Confirm, then write",
	"## 7. Run the command",
	"## 8. Read the result, then verify",
}

// preservedSkillHeadings are the sections that describe the parts of the
// `repo` surface that are not workflows, plus the shared vocabulary. None of
// them belong to the add conversation, so the rewrite had to leave them
// standing.
var preservedSkillHeadings = []string{
	"# Concepts",
	"# Rules that apply to every add",
	"# Inspecting the registry",
	"# Materialization and staleness",
	"# Footprint repair",
	"# Removal",
}

// numberedStepHeading matches any numbered second-level heading, which is the
// shape every rung of the old playbook had. Asserting on the shape as well as
// the exact headings catches a renumbered or reworded flow creeping back in.
var numberedStepHeading = regexp.MustCompile(`(?m)^## \d+\. `)

// TestManageReposSkillNoLongerNarratesTheAddConversation verifies that the
// skill has given up its own step-by-step account of the add. The step
// instructions are the feature: whatever the skill says about the wording of
// each question is a second source of truth the CLI does not control.
func TestManageReposSkillNoLongerNarratesTheAddConversation(t *testing.T) {
	body := mustReadTemplate(t, manageReposSkill)

	for _, heading := range removedAddHeadings {
		require.NotContainsf(t, body, heading,
			"the skill must not narrate the add conversation itself (found %q)", heading)
	}
	require.Empty(t, numberedStepHeading.FindAllString(body, -1),
		"the skill must carry no numbered step headings; the CLI issues the steps")

	// The vocabulary rule is per-instruction now, asserted for every step by
	// TestVocabularyContainmentPresent. Restating it here would give the
	// agent a standing rule that the instructions could later contradict.
	require.NotContains(t, body, vocabularyContainmentMarker,
		"the vocabulary rule belongs to the step instructions, not to the skill")

	for _, heading := range preservedSkillHeadings {
		require.Containsf(t, body, heading,
			"the rewrite must preserve the non-workflow section %q", heading)
	}
}

// TestManageReposSkillResumesWithoutWorkingFiles verifies that the skill's
// resume path reads only what an add actually leaves behind. A spec or plan
// scatters per-section working files, but every answer an add has agreed
// travels inside the workflow, so pointing the agent at .spektacular/work/
// would send it to a directory that does not exist for this workflow.
func TestManageReposSkillResumesWithoutWorkingFiles(t *testing.T) {
	body := mustReadTemplate(t, manageReposSkill)

	require.NotContains(t, body, ".spektacular/work/",
		"an add has no per-section working files, so the skill must not point at that directory")
	require.Contains(t, body, ".spektacular/working-context.md",
		"the skill must still tell the agent to read the working-context file on resume")
}

// TestResumeStepExemptsRepoAddFromWorkingFiles verifies the same exemption on
// the shared resume instruction, which is rendered for every workflow kind
// including a repo add. Without the clause, the instruction would tell a
// resuming add to read files it never wrote.
func TestResumeStepExemptsRepoAddFromWorkingFiles(t *testing.T) {
	body := mustReadTemplate(t, "steps/resume.md")

	require.Contains(t, body, "neither does a **repo** add",
		"the shared resume instruction must exempt a repo add from the per-section working files")
	require.Contains(t, body, "travels inside the workflow itself",
		"the resume instruction must say where a repo add's agreed answers actually live")
}

// TestManageReposSkillUsesNoEmDashes verifies the project's prose convention
// across the rewritten skill. Unlike the step instructions, the skill carries
// no quoted standing footer, so the whole file is authored prose and none of
// it is exempt.
func TestManageReposSkillUsesNoEmDashes(t *testing.T) {
	body := mustReadTemplate(t, manageReposSkill)

	require.Equal(t, 0, strings.Count(body, "—"),
		"the manage-repos skill must use no em dashes in its prose")
}
