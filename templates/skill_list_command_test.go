package templates

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWorkflowSkillsDirectAgentToCLIList verifies that each workflow skill
// tells its driving agent to enumerate the plan/spec store via the CLI's
// `file list` command and explicitly forbids poking `.spektacular/plans/`
// or `.spektacular/specs/` with `ls`/`find`/`Read`. Both signals must be
// present so a future edit that quietly drops one fails this test rather
// than silently regressing the guardrail.
func TestWorkflowSkillsDirectAgentToCLIList(t *testing.T) {
	cases := []struct {
		skill    string
		listCmd  string
		storeDir string
	}{
		{"skills/workflows/spek-new/SKILL.md", "spec file list", ".spektacular/specs/"},
		{"skills/workflows/spek-plan/SKILL.md", "spec file list", ".spektacular/specs/"},
		{"skills/workflows/spek-implement/SKILL.md", "plan file list", ".spektacular/plans/"},
	}
	for _, c := range cases {
		body := mustReadTemplate(t, c.skill)
		require.Containsf(t, body, c.listCmd,
			"%s must document the %q command so the agent enumerates via the CLI, not the filesystem", c.skill, c.listCmd)
		require.Containsf(t, body, "Do not",
			"%s must explicitly forbid ls/find/Read against the store directory", c.skill)
		require.Containsf(t, body, c.storeDir,
			"%s must name %s as the store directory the agent must not poke directly", c.skill, c.storeDir)
	}
}

// TestWorkflowSkillsDocumentDesignCommands verifies that each workflow skill
// names the design-document commands its half of the flow needs: spek-new
// introduces the capture side (discover the declared sources, write the
// document, record the reference), and spek-plan the consumption side (list a
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
			"design ref add",
		}},
		{"skills/workflows/spek-plan/SKILL.md", []string{
			"design ref list",
			"design read",
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
