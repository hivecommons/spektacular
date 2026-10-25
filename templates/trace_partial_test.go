package templates_test

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// Every workflow skill carries the shared trace block straight after its
// version check, and the helper-launch library skills carry its helper rule,
// so agents are told what to mark in one identical wording everywhere.

const (
	traceMarkingInclude = "{{> partials/trace-marking}}"
	traceHelpersInclude = "{{> partials/trace-helpers}}"
	versionCheckInclude = "{{> partials/version-check}}"
)

func workflowSkills(t *testing.T) []string {
	t.Helper()
	matches, err := fs.Glob(templates.FS, "skills/workflows/*/SKILL.md")
	require.NoError(t, err)
	require.Len(t, matches, 8, "spec, plan, implement, plan-epic, implement-epic, design, knowledge and manage-repos")
	return matches
}

func TestEveryWorkflowSkillCarriesTheTraceBlockAfterItsVersionCheck(t *testing.T) {
	for _, p := range workflowSkills(t) {
		raw := readTemplate(t, p)
		require.Equalf(t, 1, strings.Count(raw, traceMarkingInclude), "%s includes the trace block once", p)
		require.Containsf(t, raw, versionCheckInclude+"\n\n"+traceMarkingInclude+"\n", "%s: the trace block follows the version check", p)

		out, err := mustache.RenderPartials(raw, stepkit.FSPartials{FS: templates.FS}, map[string]string{"command": "spektacular"})
		require.NoError(t, err)
		require.Containsf(t, out, "`spektacular trace start <helper> --type sub-agent` and put the id it prints", "%s", p)
		require.Containsf(t, out, "`spektacular trace start <question> --type user-wait`", "%s", p)
		require.NotContainsf(t, out, "--parent  ", "%s", p)
		require.NotContainsf(t, out, "--type sub-agent --parent", "%s: a skill names no fixed parent", path.Dir(p))
	}
}

func TestEpicChildPromptsPassTheChildItsParentSpan(t *testing.T) {
	for _, p := range []string{"skills/workflows/spek-plan-epic/SKILL.md", "skills/workflows/spek-implement-epic/SKILL.md"} {
		out, err := mustache.RenderPartials(readTemplate(t, p), stepkit.FSPartials{FS: templates.FS}, map[string]string{"command": "spektacular"})
		require.NoError(t, err)
		require.Containsf(t, out, "- its parent span: the id `spektacular trace start` printed when you marked this child's launch", "%s", p)
	}
}

func TestSpawnLibrarySkillsCarryTheHelperRule(t *testing.T) {
	for _, p := range []string{"skills/skill_spawn-planning-agents.md", "skills/skill_spawn-implementation-agents.md"} {
		raw := readTemplate(t, p)
		require.Equalf(t, 1, strings.Count(raw, traceHelpersInclude), "%s includes the helper rule once", p)
		require.NotContainsf(t, raw, traceMarkingInclude, "%s: library skills carry the helper rule only", p)
		out, err := stepkit.RenderTemplate(p, map[string]any{"command": "spekx"})
		require.NoError(t, err)
		require.Containsf(t, out, "`spekx trace start <helper> --type sub-agent`", "%s renders the command name", p)
		require.Containsf(t, out, "`spekx trace stop <id>`", "%s", p)
	}
}

func TestTraceBlockStaysShort(t *testing.T) {
	out, err := stepkit.RenderTemplate("partials/trace-marking.md", map[string]any{
		"command": "spektacular", "trace_parent": "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
	})
	require.NoError(t, err)
	require.LessOrEqual(t, len(strings.Fields(out)), 120, "the trace block is one short paragraph pair (spec cost steer)")
}
