package templates_test

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// Seeding a spec from existing material is carried by three templates: the
// spek-new skill (recognising a source, fetching it, an epic for a source with
// child items, the epic question), the interview step (the seeded and epic
// branches, which a resumed session re-renders from workflow data), and the
// seven section steps (a pre-filled draft is confirmed, never asked from
// scratch). These tests pin each behaviour where it lives.
//
// This is an external test package so it can render the skill exactly as the
// installer does (internal/agent installWorkflowSkills): mustache with
// stepkit.FSPartials over the embedded templates FS and {{command}} filled in.
// stepkit imports templates, so the in-package tests cannot do this.

const (
	spekNewSkill     = "skills/workflows/spek-new/SKILL.md"
	interviewStep    = "steps/spec/00b-interview.md"
	splitFlowPart    = "partials/split-flow.md"
	installedCommand = "spektacular"
)

// readTemplate returns an embedded template's raw text.
func readTemplate(t *testing.T, p string) string {
	t.Helper()
	b, err := fs.ReadFile(templates.FS, p)
	require.NoErrorf(t, err, "reading embedded template %s", p)
	return string(b)
}

// flat collapses whitespace so a phrase soft-wrapped across lines matches.
func flat(s string) string { return strings.Join(strings.Fields(s), " ") }

// installedSkill renders the spek-new skill the way the installer does, so
// every partial must resolve and every tag must render.
func installedSkill(t *testing.T) string {
	t.Helper()
	out, err := mustache.RenderPartials(readTemplate(t, spekNewSkill), stepkit.FSPartials{FS: templates.FS},
		map[string]string{"command": installedCommand})
	require.NoError(t, err, "the skill must render with every partial resolved")
	require.NotContains(t, out, "{{", "no mustache tag may survive installation")
	return out
}

// section returns the part of body from the heading up to the next heading of
// the same or a higher level, so an assertion can be held to one section.
func section(t *testing.T, body, heading string) string {
	t.Helper()
	start := strings.Index(body, heading+"\n")
	require.GreaterOrEqualf(t, start, 0, "missing section %q", heading)
	level := strings.IndexFunc(heading, func(r rune) bool { return r != '#' })
	rest := body[start+len(heading):]
	for _, line := range strings.Split(rest, "\n") {
		if strings.HasPrefix(line, "#") {
			l := strings.IndexFunc(line, func(r rune) bool { return r != '#' })
			if l > 0 && l <= level && strings.HasPrefix(line[l:], " ") {
				return body[start : start+len(heading)+strings.Index(rest, "\n"+line+"\n")]
			}
		}
	}
	return body[start:]
}

func requirePhrases(t *testing.T, label, body string, phrases ...string) {
	t.Helper()
	for _, p := range phrases {
		require.Containsf(t, body, p, "%s must carry %q", label, p)
	}
}

// Criterion: the skill recognises a source given as a number, a link or
// "spec from …", including in its trigger description, so being understood
// does not depend on careful wording.
func TestSpekNewRecognisesASourceHoweverItIsPhrased(t *testing.T) {
	skill := installedSkill(t)

	desc := regexp.MustCompile(`(?m)^description: (.*)$`).FindStringSubmatch(skill)
	require.Len(t, desc, 2, "the skill must carry a description line")
	requirePhrases(t, "the skill's trigger description", desc[1],
		"existing material", "an issue", "a tracker epic", "a design document", "a file", "a web page", "pasted text",
		`"spec from #45"`, "a pasted link",
	)

	material := flat(section(t, skill, "## Starting from existing material"))
	requirePhrases(t, "Starting from existing material", material,
		"Recognise a source however the request is phrased",
		`"spec from issue 45"`, `"#45"`, `"start from LIN-123"`,
		"a pasted link to any tracker", "a file path", "pasted text",
		"Being understood must not depend on careful wording.",
		"If it is ambiguous, such as a bare issue number with no tracker or repository, ask the user rather than guess.",
	)

	start := flat(section(t, skill, "## Starting a new spec"))
	requirePhrases(t, "Starting a new spec", start,
		"**Does the request point at existing material?**",
		"however they word it, follow \"Starting from existing material\" below before choosing a name",
	)
}

// toolInstruction matches an instruction to use one particular fetching tool
// or tracker client: a gh/glab/jira/linear command, a named fetch tool, an
// MCP tool id, or "use the <tracker> API".
var toolInstruction = regexp.MustCompile(`(?i)` +
	"`gh |\\bgh (issue|api|pr)\\b|\\bglab\\b|\\bjira\\b|\\blinear (api|cli|issue)\\b|" +
	`\bWebFetch\b|\bcurl\b|mcp__|\buse the \w+ (api|cli)\b`)

// Criterion: the skill names no specific fetching tool or tracker; the agent
// fetches with whatever it has.
func TestSpekNewNamesNoFetchingToolOrTracker(t *testing.T) {
	skill := installedSkill(t)
	interview := readTemplate(t, interviewStep)

	for label, body := range map[string]string{spekNewSkill: skill, interviewStep: interview} {
		require.Emptyf(t, toolInstruction.FindAllString(body, -1), "%s must not instruct the agent to use a particular tool or tracker", label)
	}

	material := flat(section(t, skill, "## Starting from existing material"))
	requirePhrases(t, "Starting from existing material", material,
		"**Fetch it with your own tools.** Spektacular prescribes no tool and no tracker",
		"a CLI, an MCP server, a web fetch or a file read are all fine",
		"collect the title and body, the discussion",
		"its child items",
		"a stable link to record as provenance",
	)
	require.Contains(t, flat(interview), "Fetch each source again with your own tools** (Spektacular prescribes none)")
}

// Criterion: an unreachable source leads the agent to say so and ask for the
// content, not to start a blank interview; in the skill and again in the
// interview step a resumed session renders.
func TestSpekNewUnreachableSourceAsksForTheContent(t *testing.T) {
	material := flat(section(t, installedSkill(t), "## Starting from existing material"))
	requirePhrases(t, "Starting from existing material", material,
		"**If you cannot reach it,** because no tool you have can read it or fetching fails, say so plainly and ask the user to paste the content.",
		"Never fall back to a blank interview without saying so.",
	)
	require.Less(t, strings.Index(material, "**If you cannot reach it,**"), strings.Index(material, "**Start the workflow with provenance:**"),
		"an unreachable source is handled before the workflow starts")

	requirePhrases(t, interviewStep, flat(readTemplate(t, interviewStep)),
		"If a source cannot be reached, say so and ask the user to paste its content; never fall back to a blank interview without saying so.",
	)
}

// Criterion: a source with child items leads to an offer to create an epic
// and specify each child in it; a child item added later leads to an offer of
// a spec for it in that epic.
func TestSpekNewSourceWithChildItemsOffersAnEpic(t *testing.T) {
	skill := installedSkill(t)

	material := flat(section(t, skill, "## Starting from existing material"))
	requirePhrases(t, "Starting from existing material", material,
		"**Check for child items.** A source that already has child items is a set of pieces of work, not one spec.",
		"Offer to set up an epic for it instead, with one spec per child",
		"On decline, continue with a single spec.",
		"**Propose a name** from the source's title",
		`spec new`+"` with `"+`"sources": [{"uri": "<stable link>"}]`,
	)

	children := flat(section(t, skill, "### A source with child items"))
	requirePhrases(t, "A source with child items", children,
		"When the user accepts an epic",
		"**Create the epic first,** with the parent as its source and no specs yet.",
		installedCommand+` epic write <short-name> --from .spektacular/tmp/epic_body.md --data '{"specs": [], "sources": [{"uri": "<parent link>"}]}'`,
		"rm .spektacular/tmp/epic_body.md",
		"**Specify the first child** as its own spec in that epic",
		installedCommand+` spec new --data '{"name": "<child name>", "sources": [{"uri": "<child link>"}], "epic": "<epic name>"}'`,
		"the workflow offers the next child that has no spec yet",
		"**A child item that appears later.**",
		"offer to start a spec for it in that epic",
	)
	require.Less(t, strings.Index(children, "**Create the epic first,**"), strings.Index(children, "**Specify the first child**"),
		"the epic exists before its first child is specified")
}

// Criterion: in a project with epics, starting a spec asks whether it belongs
// to one; in a project without epics it does not. The question is asked
// before the name, and a completed epic is never joined without asking.
func TestSpekNewAsksAboutAnEpicOnlyWhenTheProjectHasOne(t *testing.T) {
	start := flat(section(t, installedSkill(t), "## Starting a new spec"))
	requirePhrases(t, "Starting a new spec", start,
		"**Does the spec belong to an epic?** Run `"+installedCommand+" epic list`.",
		"If it lists any epics, ask the user whether this spec belongs to one of them, unless the request already says.",
		`pass that epic as `+"`"+`"epic"`+"`"+` on `+"`spec new`",
		"the interview reads the epic and its specs first",
		"If the list is empty, the project has no epics: do not ask.",
		"**If the epic is complete** (`code: epic_complete`)",
		"ask whether to add the spec anyway",
		`"confirm_completed_epic": true`,
		"Never add it without asking.",
	)
	require.Less(t, strings.Index(start, "**Does the spec belong to an epic?**"), strings.Index(start, "Ask the user for a spec name now."),
		"the epic question comes before the name")
}

// Criterion: a spec joining an epic has the agent read the epic and its specs
// before the interview questions, and offer to record dependencies on them.
// The branch lives in the interview step, gated on the epic.
func TestInterviewReadsTheEpicFirstAndOffersDependencies(t *testing.T) {
	raw := readTemplate(t, interviewStep)
	open, closeAt := strings.Index(raw, "{{#epic}}"), strings.Index(raw, "{{/epic}}")
	require.True(t, open >= 0 && open < closeAt, "the interview must carry an {{#epic}} branch")
	branch := flat(raw[open:closeAt])
	requirePhrases(t, "the interview's epic branch", branch,
		"Read it first.",
		"Before asking anything, run `{{command}} epic read {{epic}}`",
		"then read each spec it lists with `{{command}} spec file read <name>`",
		"do not ask again about scope those specs settle",
		"offer to record that dependency in the epic",
		"`{{command}} epic write {{epic}}` with this spec's `depends_on` updated",
		"record it only if the user agrees",
	)
	require.Less(t, closeAt, strings.Index(raw, "**Ask adaptive, open questions"), "the epic is read before the questions")
}

// Criterion: a seeded interview lists the uncovered sections and asks only
// about those, and each later step presents its pre-filled draft to confirm.
// Seeding survives a resumed session because it lives in the interview step,
// which the resume re-renders from workflow data, not only in the skill.
func TestSeededInterviewAsksOnlyAboutGapsAndSectionsConfirmDrafts(t *testing.T) {
	raw := readTemplate(t, interviewStep)
	open, closeAt := strings.Index(raw, "{{#seeded}}"), strings.Index(raw, "{{/seeded}}")
	require.True(t, open >= 0 && open < closeAt, "the seeded branch must live in the interview step")
	inner := raw[open:closeAt]
	require.Contains(t, inner, "{{#sources}}", "the sources are listed inside the seeded branch, which is gated once")
	require.NotContains(t, raw, "{{#sources}}\n**This spec was started", "the branch must not be gated on the list itself")

	branch := flat(inner)
	requirePhrases(t, "the interview's seeded branch", branch,
		"Seed it before you ask anything.",
		"**Seed the section working files** under `.spektacular/work/{{spec_name}}/`",
		"title and body → `overview.md` and `requirements.md`",
		"checklist or task-list items → `acceptance_criteria.md`",
		`"must" / "must not" remarks → `+"`constraints.md`",
		`"out of scope" remarks → `+"`non_goals.md`",
		"Leave a section's file absent when the material says nothing about it.",
		"**List the gaps to the user:** every section the material does not cover",
		"**Ask only about the gaps.** Do not re-ask what the material already answers",
		"each later section step presents its seeded draft for the user to confirm",
		"In `interview.md`, record what came from the source and what came from the user",
		"a resumed session can tell them apart",
	)
	require.Less(t, closeAt, strings.Index(raw, "**Ask adaptive, open questions"), "seeding happens before the questions")

	for file, work := range map[string]string{
		"steps/spec/01-overview.md":            "overview.md",
		"steps/spec/02-requirements.md":        "requirements.md",
		"steps/spec/03-acceptance_criteria.md": "acceptance_criteria.md",
		"steps/spec/04-constraints.md":         "constraints.md",
		"steps/spec/05-technical_approach.md":  "technical_approach.md",
		"steps/spec/06-success_metrics.md":     "success_metrics.md",
		"steps/spec/07-non_goals.md":           "non_goals.md",
	} {
		requirePhrases(t, file, flat(readTemplate(t, file)),
			"**A pre-filled draft is confirmed, never asked from scratch.** If `.spektacular/work/{{spec_name}}/"+work+"` already holds content",
			"that content is your draft. Present it to the user to confirm or refine",
			"do not ask for this section as if it were blank",
		)
	}
}

// Criterion: the skill includes both split partials, in the section for
// splitting a spec already written, and both resolve on installation; a
// completed epic is never extended by a split without asking.
func TestSpekNewIncludesBothSplitPartials(t *testing.T) {
	raw := readTemplate(t, spekNewSkill)
	split := section(t, raw, "## Splitting a spec that is already written")
	check, flow := strings.Index(split, "{{> partials/split-check}}"), strings.Index(split, "{{> partials/split-flow}}")
	require.GreaterOrEqual(t, check, 0, "the split section must include the split-check partial")
	require.Greater(t, flow, check, "the split-flow partial must follow the check")

	installed := installedSkill(t)
	rendered := section(t, installed, "## Splitting a spec that is already written")
	requirePhrases(t, "the installed split section", rendered, "### The split check", "### The split flow",
		installedCommand+" epic split --from .spektacular/tmp/epic_split.json")

	requirePhrases(t, splitFlowPart, flat(readTemplate(t, splitFlowPart)),
		"If that epic is complete (`code: epic_complete`",
		"tell the user and ask whether to add to it anyway",
		"only if they agree, add `\"confirm_completed_epic\": true` to the staged description",
	)
}
