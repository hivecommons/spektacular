package templates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWorkflowSkillsDirectAgentToCLIList verifies that each workflow skill
// tells its driving agent to enumerate the plan/spec store via the CLI's
// `file list` command.
//
// The matching prohibition — never poke a store directory with `ls`/`find`/
// `Read`, never write one with `Write`/`Edit` — is deliberately NOT asserted
// here any more. It used to be restated in all three skills, which meant an
// agent was told only while one of them was loaded: never in an ad-hoc
// session, and never in a sub-agent, which inherits AGENTS.md but not the
// skill that spawned it. It now lives once, in the managed AGENTS.md section,
// and is pinned by TestRenderedStoreAccessSection* in internal/agent. A skill
// must still name its own list command, which is what this test guards.
func TestWorkflowSkillsDirectAgentToCLIList(t *testing.T) {
	cases := []struct {
		skill   string
		listCmd string
	}{
		{"skills/workflows/spek-new/SKILL.md", "spec file list"},
		{"skills/workflows/spek-plan/SKILL.md", "spec file list"},
		{"skills/workflows/spek-implement/SKILL.md", "plan file list"},
	}
	for _, c := range cases {
		body := mustReadTemplate(t, c.skill)
		require.Containsf(t, body, c.listCmd,
			"%s must document the %q command so the agent enumerates via the CLI, not the filesystem", c.skill, c.listCmd)
	}
}

// TestWorkflowSkillsDocumentDesignCommands verifies that each workflow skill
// names the design-document commands its half of the flow needs: spek-new
// introduces the capture side (discover the declared sources, store a
// document the user handed over, author one Spektacular wrote with them,
// record the reference), and spek-plan the consumption side (list a
// spec's references, read each document). A skill that introduces design
// documents without naming the commands leaves the agent to guess at them, so
// the command names are the property worth pinning.
//
// This is a separate table from TestWorkflowSkillsDirectAgentToCLIList rather
// than extra rows in it: that table's third field is a store directory the
// agent must not poke with ls/find/Read, and design documents have no such
// directory — they live in whatever locations the project declares as design
// sources.
func TestWorkflowSkillsDocumentDesignCommands(t *testing.T) {
	cases := []struct {
		skill    string
		commands []string
	}{
		{"skills/workflows/spek-new/SKILL.md", []string{
			"design sources",
			"design list",
			"design write",
			"design author",
			"design ref add",
		}},
		{"skills/workflows/spek-plan/SKILL.md", []string{
			"design ref list",
			"design read",
		}},
		{"skills/workflows/spek-design/SKILL.md", []string{
			"design author",
			"design write",
			"design ref add",
			"design sources",
		}},
	}
	for _, c := range cases {
		body := mustReadTemplate(t, c.skill)
		for _, cmd := range c.commands {
			require.Containsf(t, body, cmd,
				"%s must document the %q command so the agent reaches design documents through the CLI", c.skill, cmd)
		}
	}
}

// TestSpekDesignSkillCarriesItsContract pins the load-bearing prose of the
// spek-design skill. Every expected phrase below is hand-maintained here as a
// literal: none is derived from the template, because an assertion built from
// the file under test would pass whatever that file said.
//
// The skill is a static playbook rather than a CLI-driven workflow, so nothing
// at runtime checks that it still says what it must. These anchors are the
// only guard: the four intents it routes between, the interview's goal and
// stopping condition, and the propose-then-confirm contract that keeps a
// decline from writing anything.
//
// Prose is matched against the template with its line wrapping collapsed, so
// reflowing a paragraph does not fail the test while changing its wording
// still does. Headings are matched against the raw body, where they are
// genuinely at the start of a line.
func TestSpekDesignSkillCarriesItsContract(t *testing.T) {
	body := mustReadTemplate(t, "skills/workflows/spek-design/SKILL.md")
	flat := strings.Join(strings.Fields(body), " ")

	// The skill routes between four intents, each of which must have its own
	// section for the agent to land in.
	for _, heading := range []string{
		"\n# Intent: author\n",
		"\n# Intent: bring in\n",
		"\n# Intent: revise\n",
		"\n# Intent: reference only\n",
	} {
		require.Containsf(t, body, heading,
			"the spek-design skill must carry the %q section, or that intent is unreachable", strings.TrimSpace(heading))
	}

	contract := []struct {
		needle string
		why    string
	}{
		{
			needle: "it does not drive an interactive CLI state machine — it is a static playbook",
			why:    "an agent that expects a state machine would wait for steps that never come; design conversation happens inside a spec workflow, where a second state machine cannot run",
		},
		{
			needle: `arXiv:2302.11382`,
			why:    "the interview follows the Flipped Interaction pattern, and the citation is what lets a reader check it",
		},
		{
			needle: "**Stated goal:** understand the design well enough to write a document someone could build from.",
			why:    "an interview with no stated goal has nothing to steer its questions toward",
		},
		{
			needle: "**Stop once a further answer would not change the document.** That is the stopping condition",
			why:    "without an explicit stopping condition the interview runs past the point where answers stop changing the document",
		},
		{
			needle: "Nothing is written without the user's explicit agreement.",
			why:    "every branch that writes is gated on explicit agreement, and the gate exists only in this prose",
		},
		{
			needle: "stop and leave the source untouched",
			why:    "a decline must leave the declared source exactly as it was",
		},
		{
			needle: "Remove any staged scratch file at `.spektacular/tmp/<slug>.md` either way; a half-finished proposal should not linger on disk.",
			why:    "a decline must leave no file behind, staged draft included",
		},
		{
			needle: "It also means the detail does not get smuggled into the spec body instead",
			why:    "a declined design must not be written into the spec as a consolation prize",
		},
	}
	for _, c := range contract {
		require.Containsf(t, flat, c.needle,
			"the spek-design skill lost a load-bearing phrase (%s): %q", c.why, c.needle)
	}

	// Negative: the skill must never tell an agent to fetch another skill
	// through the CLI. `skill <name>` serves library skills only — the
	// workflow skills are installed on disk and are not reachable that way, so
	// such an instruction would fail at the exact moment it was followed.
	require.NotContains(t, flat, "skill spek-",
		"the spek-design skill must not instruct fetching a workflow skill through the CLI")
}
