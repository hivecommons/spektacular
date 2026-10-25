package cmd

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

// These tests pin the trace block every step instruction carries: one short
// paragraph, appended by stepkit.WriteStepResult straight after the step's
// own text, telling the agent to mark helper launches and questions to the
// user as spans under the step's own span.

const (
	traceLeadIn       = "**Mark spans.**"
	contractStepSpan  = "4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7"
	traceSubAgentRule = "spektacular trace start <helper> --type sub-agent"
	traceUserWaitRule = "spektacular trace start <question> --type user-wait"
)

func TestEveryStepCarriesTheTraceBlockOnceAfterItsOwnText(t *testing.T) {
	for _, orchestrated := range []bool{false, true} {
		for _, mode := range []string{config.AutoCommitOff, config.AutoCommitWorkflow, config.AutoCommitFull} {
			for _, ri := range renderAllStepInstructionsSpan(t, "spektacular", mode, orchestrated, contractStepSpan) {
				name := ri.templatePath
				require.Equalf(t, 1, strings.Count(ri.body, traceLeadIn), "%s must carry the trace block exactly once", name)
				at := strings.Index(ri.body, traceLeadIn)
				require.Truef(t, strings.HasPrefix(ri.body[at-len("\n\n---\n\n"):], "\n\n---\n\n"), "%s: the trace block follows a rule", name)
				for _, later := range []string{orchestratedSection, "## Automatic git commit", "**Before you advance:**"} {
					if i := strings.Index(ri.body, later); i >= 0 {
						require.Lessf(t, at, i, "%s: the trace block comes before %q", name, later)
					}
				}
				require.Containsf(t, ri.body, traceSubAgentRule, "%s carries the helper rule", name)
				if orchestrated {
					require.NotContainsf(t, ri.body, "--type user-wait", "%s: an orchestrated run never asks the user", name)
				} else {
					require.Containsf(t, ri.body, traceUserWaitRule, "%s carries the user-wait rule", name)
				}
				if ri.nextStep == "" {
					require.NotContainsf(t, ri.body, "--parent "+contractStepSpan, "%s: the terminal step names no span", name)
					require.Containsf(t, ri.body, "--type sub-agent` and put", "%s: with no span the rule has no --parent", name)
				} else {
					want := 2
					if orchestrated {
						want = 1
					}
					require.Equalf(t, want, strings.Count(ri.body, "--parent "+contractStepSpan+"`"), "%s names its step span as the parent of each rule", name)
				}
			}
		}
	}
}

func TestTheTraceBlockNamesTheConfiguredCommand(t *testing.T) {
	for _, ri := range renderAllStepInstructions(t, "spekx") {
		at := strings.Index(ri.body, traceLeadIn)
		require.GreaterOrEqual(t, at, 0)
		block := ri.body[at:]
		require.Containsf(t, block, "`spekx trace start <helper>", "%s", ri.templatePath)
		require.Containsf(t, block, "`spekx trace stop <id>`", "%s", ri.templatePath)
	}
}

func TestNoStepTemplateCopiesTheTraceBlock(t *testing.T) {
	err := fs.WalkDir(templates.FS, "steps", func(p string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		body, err := fs.ReadFile(templates.FS, p)
		require.NoError(t, err)
		for _, s := range []string{traceLeadIn, "trace start", "partials/trace-marking", "partials/trace-helpers"} {
			require.NotContainsf(t, string(body), s, "%s must not carry the trace block; it is appended at render time", p)
		}
		return nil
	})
	require.NoError(t, err)
}

func TestSpawnLibrarySkillsCarryTheHelperRule(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	resetRootCmd(t)
	for _, name := range []string{"spawn-planning-agents", "spawn-implementation-agents"} {
		var res SkillResult
		require.NoError(t, json.Unmarshal([]byte(runOK(t, "skill", name)), &res))
		require.Contains(t, res.Instructions, "`spektacular trace start <helper> --type sub-agent` and put the id it prints in the helper's brief", name)
		require.Contains(t, res.Instructions, "`spektacular trace stop <id>`", name)
		require.NotContains(t, res.Instructions, "{{", name)
	}
}
