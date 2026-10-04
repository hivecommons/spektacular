package templates_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// The spek-plan-epic skill drives the plan workflow for every outstanding spec
// of an epic through child agents. These tests pin its contract on the
// template itself, rendered exactly as the installer renders a workflow skill
// (mustache, partials from templates.FS, only {{command}} as data), so they
// hold before and after the skill is registered for installation.

const (
	planEpicSkill   = "skills/workflows/spek-plan-epic/SKILL.md"
	versionCheckRaw = "partials/version-check.md"
)

// renderPlanEpicSkill renders the skill with the given command name and
// requires every partial and tag to resolve.
func renderPlanEpicSkill(t *testing.T, command string) string {
	t.Helper()
	out, err := mustache.RenderPartials(readTemplate(t, planEpicSkill), stepkit.FSPartials{FS: templates.FS},
		map[string]string{"command": command})
	require.NoError(t, err, "the skill must render with every partial resolved")
	require.NotContains(t, out, "{{", "no mustache tag may survive installation")
	return out
}

func installedPlanEpicSkill(t *testing.T) string {
	t.Helper()
	return renderPlanEpicSkill(t, installedCommand)
}

// planEpicBody is the rendered skill after its frontmatter.
func planEpicBody(t *testing.T, skill string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(skill, "---\n"), "the skill must open with frontmatter")
	end := strings.Index(skill[4:], "\n---\n")
	require.GreaterOrEqual(t, end, 0, "the frontmatter must be closed")
	return skill[4+end+len("\n---\n"):]
}

// Criterion 1: the skill is triggered by plain wording.
func TestPlanEpicSkillIsTriggeredByPlainWording(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	require.Regexp(t, `(?m)^name: spek-plan-epic$`, skill)
	desc := regexp.MustCompile(`(?m)^description: (.*)$`).FindStringSubmatch(skill)
	require.Len(t, desc, 2, "the skill must carry a description line")
	requirePhrases(t, "the skill's trigger description", desc[1],
		`"plan this epic"`, `"plan the epic"`, `"plan <epic name>"`, `"plan all the specs in this epic"`,
	)
}

// The installed skill opens on the version check, renders {{command}}
// everywhere, and leaves no tag behind.
func TestPlanEpicSkillRendersLikeAnInstalledSkill(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	partial, err := mustache.Render(readTemplate(t, versionCheckRaw), map[string]string{"command": installedCommand})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(strings.TrimLeft(planEpicBody(t, skill), "\n"), strings.TrimRight(partial, "\n")),
		"the skill body must open with the version-check partial")

	// Every {{command}} in the skill and the partial renders to the command
	// passed, and no CLI mention is left hard-coded to "spektacular".
	raw := readTemplate(t, planEpicSkill)
	want := strings.Count(raw, "{{command}}") + strings.Count(readTemplate(t, versionCheckRaw), "{{command}}")
	custom := renderPlanEpicSkill(t, "spekx")
	require.Equal(t, want, strings.Count(custom, "spekx"), "every {{command}} must render to the installed command")
	require.NotRegexp(t, "`spektacular |\\bspektacular (status|plan|spec|epic|repo|version|migrate)\\b", custom,
		"no CLI mention may bypass {{command}}")
	requirePhrases(t, "the skill rendered with a custom command", flat(custom),
		"`spekx status <epic> --format json`",
		"`spekx plan new --data '{\"name\":\"<spec>\",\"orchestrated\":true}'`",
		"`spekx plan goto --data '{\"step\":\"<current_step>\",\"name\":\"<spec>\"}'`",
		"`spekx plan file read <spec> <doc>`",
		"`spekx plan file write <spec> <doc> --from <path>`",
		"`spekx epic list`",
		"`spekx repo list`",
	)
}

// Criterion 2: only specs without a plan are planned, in dependency order,
// with independent specs started together.
func TestPlanEpicSkillPlansOutstandingSpecsInDependencyOrder(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	loop := flat(section(t, skill, "# Step 2: The loop"))
	requirePhrases(t, "the loop", loop,
		"A child that handed back a `QUESTION:` and is waiting for its answer still counts as running.",
		"Never start a second child for a spec that already has one, waiting or not.")
	requirePhrases(t, "The loop", loop,
		"Repeat until nothing is left to start and no child is running",
		"Run `"+installedCommand+" status <epic> --format json`. Read `epic.run` and, for each spec, `run.plan`.",
		"`run.plan.state` is `in_progress` and has no child running, start a child to **resume** it, at the `current_step` reported.",
		"`run.plan.state` is `ready`, start a child to **start** it.",
		"Specs that are `done` are skipped; specs that are `blocked` wait for the specs in their `waiting_on`.",
		"independent specs are planned in overlapping time",
		"Start every ready spec; there is no limit on how many run at once.",
		"Problems never block planning",
	)

	child := flat(section(t, skill, "# Step 3: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"to **start**: run `"+installedCommand+` plan new --data '{"name":"<spec>","orchestrated":true}'`+"`",
		"to **resume**: run the same `plan new` command. It returns a resume report for the spec's lane instead of starting over",
		"then run the `goto` it gives, `"+installedCommand+` plan goto --data '{"step":"<current_step>","name":"<spec>"}'`+"`",
		"Follow the `spek-plan` skill.",
	)

	what := flat(section(t, skill, "# What this skill does"))
	requirePhrases(t, "What this skill does", what,
		"Specs that do not depend on each other are planned side by side",
		"a spec whose dependencies are not yet planned waits for them",
		"an epic is never planned or implemented",
	)
}

// askMention finds every instruction to ask, or not to ask, something.
var askMention = regexp.MustCompile(`(?i)\bask\w*`)

// Criterion 3: the only user stops are genuine open questions relayed from a
// child and the end-of-planning review; the only other question is which
// epic is meant, when that is ambiguous.
func TestPlanEpicSkillStopsOnlyForQuestionsAndTheReview(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	relay := flat(section(t, skill, "# Step 4: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", relay,
		"**`QUESTION:`** — put the question to the user.",
		"Present it as ordinary text first, naming the spec, the options and the recommended default, then ask.",
		"the other children keep going",
		"Send the user's answer back to the same child",
	)

	review := flat(section(t, skill, "# Step 6: The end-of-planning review"))
	requirePhrases(t, "The end-of-planning review", review,
		"one summary entry for **every plan produced in this run**",
		"before anything is implemented",
		"Invite changes.",
		"Close the review on a direct confirmation question.",
		"Skip the review if no plan was produced in this run.",
	)

	questions := flat(section(t, skill, "## What counts as a genuine open question"))
	requirePhrases(t, "What counts as a genuine open question", questions,
		"A child never asks for approval of a section",
		"never asks the user to sign off its plan",
	)

	// Every mention of asking, outside the version-check partial, is one of
	// the sanctioned ones.
	partial, err := mustache.Render(readTemplate(t, versionCheckRaw), map[string]string{"command": installedCommand})
	require.NoError(t, err)
	body := flat(strings.Replace(planEpicBody(t, skill), partial, "", 1))
	allowed := []string{
		"ask the user which epic they mean, but only when it is still ambiguous",
		"you never ask the user anything yourself",
		"A child never asks for approval of a section, and never asks the user to sign off its plan.",
		"naming the spec, the options and the recommended default, then ask.",
		"so a question the user already answered is not asked again",
		"the walkthrough prepares a summary instead of asking for sign-off",
	}
	for _, loc := range askMention.FindAllStringIndex(body, -1) {
		lo, hi := max(0, loc[0]-80), min(len(body), loc[1]+80)
		window := body[lo:hi]
		ok := false
		for _, a := range allowed {
			if i := strings.Index(body, a); i >= 0 && i <= loc[0] && loc[1] <= i+len(a) {
				ok = true
				break
			}
		}
		require.Truef(t, ok, "unsanctioned stop for the user: %q", window)
	}
	require.NotContains(t, strings.ToLower(body), "approve each", "no per-section approval")
}

// Criterion 4: a genuine open question, the hand-back contract and stopping
// on failure are all defined.
func TestPlanEpicSkillDefinesHandBackAndStopping(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	contract := flat(section(t, skill, "## The hand-back contract"))
	requirePhrases(t, "The hand-back contract", contract,
		"final message whose **first line** is exactly one of",
		"`DONE: <spec>`", "`QUESTION: <spec>`", "`FAILED: <spec>`",
		"followed by the step it reached and the reason it cannot go on",
		"the question, the options, and the default it recommends",
	)

	questions := flat(section(t, skill, "## What counts as a genuine open question"))
	requirePhrases(t, "What counts as a genuine open question", questions,
		"Only a stop the plan workflow itself defines earns a question",
		"a design reference that does not resolve",
		"a disagreement with a referenced design",
		"a drafting decision with no reasonable default that only the user can settle",
		"Everything else the child decides itself, records as a drafting assumption",
	)

	child := flat(section(t, skill, "# Step 3: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the hand-back contract and the definition of a genuine open question, below.",
	)

	handling := flat(section(t, skill, "# Step 4: Handling a hand-back"))
	requirePhrases(t, "Handling a hand-back", handling,
		"**`FAILED:`** — enter stopping mode.",
		"**The user declines to answer now**",
		"**Stopping mode:** start no new child, let the children already running finish, then go to the final report.",
		"Name the spec that stopped the run and why.",
	)
}

// Criterion 5: repeating the request resumes interrupted plans and skips
// finished ones.
func TestPlanEpicSkillResumesOnARepeatedRequest(t *testing.T) {
	what := flat(section(t, installedPlanEpicSkill(t), "# What this skill does"))
	requirePhrases(t, "What this skill does", what,
		"repeating the request after an interruption picks up exactly where the epic stands",
		"finished plans are skipped, and an interrupted plan resumes at the step it reached",
	)
}

// The orchestrator keeps its notes in working-context.md, restates the
// store-access rule to itself and to every child, reports progress and ends
// on a completed/skipped/outstanding report.
func TestPlanEpicSkillCarriesNotesStoreAccessProgressAndReport(t *testing.T) {
	skill := installedPlanEpicSkill(t)

	require.Contains(t, flat(section(t, skill, "# What this skill does")), "`.spektacular/working-context.md`")

	store := flat(section(t, skill, "# Spektacular's files are reached through Spektacular"))
	requirePhrases(t, "the store-access rule", store,
		"Never use your own file tools on a store directory, and never build a store path by hand.",
		"`"+installedCommand+" plan file read`",
		"`"+installedCommand+" plan file write --from <path>`",
		"Every child prompt must restate this rule",
	)

	child := flat(section(t, skill, "# Step 3: The child prompt"))
	requirePhrases(t, "The child prompt", child,
		"the spec name, and the project root to work in;",
		"the store-access rule above, word for word;",
	)
	require.Contains(t, flat(section(t, skill, "# Step 1: Find the epic")),
		"Run `"+installedCommand+" repo list` and note the `root` of the project's own repo. Every child runs there.")

	progress := flat(section(t, skill, "# Step 5: Progress"))
	requirePhrases(t, "Progress", progress,
		"After every child you start and every hand-back, tell the user in one line which specs are being planned",
		"how many remain",
	)

	report := flat(section(t, skill, "# Step 7: The final report"))
	requirePhrases(t, "The final report", report,
		"End every run, finished or stopped",
		"**Completed in this run:**", "**Skipped:**", "**Still outstanding:**",
	)
}
