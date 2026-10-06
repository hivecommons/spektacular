package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests cover an orchestrated run handing its stops back to the
// orchestrator that started it rather than asking the user: the hand-back
// section every orchestrated step carries, the lane's own notes file named by
// the footer, and an orchestrated plan finishing without a sign-off.

// orchestratedSection is the heading of templates/partials/orchestrated-stop.md,
// hand-copied.
const orchestratedSection = "## Running under an orchestrator"

// contractLaneFooter is contractWorkingContextFooter as an orchestrated step of
// kind renders it: the same paragraph naming the lane's own notes file.
func contractLaneFooter(kind string) string {
	lane := map[string]string{
		"plan":      ".spektacular/workflows/plan-demo-feature.md",
		"implement": ".spektacular/workflows/implement-demo-feature.md",
	}[kind]
	return "**Before you advance:** refresh `" + lane + "` with your cross-cutting working context only — the key decisions and substitutions made, the answers the user gave to your questions, and learnings worth carrying forward. Keep it to learnings and decisions, not a transcript and not a copy of content already captured elsewhere (such as a section's own working file). Use your own file tools. This file is git-tracked, and a resumed session reads it back to pick up where you left off, so keep it current every time before running the `goto` command above.\n"
}

// Every continuing orchestrated plan and implement step ends with the footer
// naming its lane's own notes file, and no orchestrated step names the shared
// working context.
func TestOrchestratedStepsNameTheLaneNotesFile(t *testing.T) {
	checked := map[string]int{}
	for _, ri := range renderAllStepInstructionsWith(t, "spektacular", "", true) {
		if ri.workflow != "plan" && ri.workflow != "implement" {
			continue
		}
		if ri.nextStep == "" {
			require.NotContainsf(t, ri.body, "**Before you advance:**",
				"%s is terminal and must carry no working-context footer", ri.templatePath)
			continue
		}
		footer := contractLaneFooter(ri.workflow)
		require.Equalf(t, 1, strings.Count(ri.body, footer),
			"orchestrated %s must carry exactly one lane footer", ri.templatePath)
		require.Truef(t, strings.HasSuffix(ri.body, "\n\n---\n\n"+footer),
			"orchestrated %s must end with a rule followed by the lane footer", ri.templatePath)
		checked[ri.workflow]++
	}
	require.NotZero(t, checked["plan"])
	require.NotZero(t, checked["implement"])
}

// No orchestrated plan or implement step sends the agent to the shared working
// context anywhere in its body: a lane's notes are its own.
func TestOrchestratedStepsNeverNameTheSharedWorkingContext(t *testing.T) {
	var offenders []string
	for _, ri := range renderAllStepInstructionsWith(t, "spektacular", "", true) {
		if ri.workflow != "plan" && ri.workflow != "implement" {
			continue
		}
		if strings.Contains(ri.body, ".spektacular/working-context.md") {
			offenders = append(offenders, ri.templatePath)
		}
	}
	require.Empty(t, offenders, "these orchestrated steps name the shared working context")
}

// Every orchestrated plan and implement step carries the hand-back section
// once, naming the spec on its QUESTION and FAILED lines, ahead of the footer.
func TestEveryOrchestratedStepCarriesTheHandBackSection(t *testing.T) {
	for _, ri := range renderAllStepInstructionsWith(t, "spektacular", "", true) {
		if ri.workflow != "plan" && ri.workflow != "implement" {
			continue
		}
		require.Equalf(t, 1, strings.Count(ri.body, orchestratedSection),
			"orchestrated %s must carry the hand-back section exactly once", ri.templatePath)
		require.Containsf(t, ri.body, "first line is exactly `QUESTION: demo-feature`", ri.templatePath)
		require.Containsf(t, ri.body, "first line is exactly `FAILED: demo-feature`", ri.templatePath)
		if ri.nextStep != "" {
			require.Lessf(t, strings.Index(ri.body, orchestratedSection), strings.Index(ri.body, "**Before you advance:**"),
				"orchestrated %s must put the hand-back section before the footer", ri.templatePath)
		}
	}
}

// A standalone run is untouched by orchestration: no step of any workflow, in
// any auto_commit mode, carries the hand-back section or its markers.
func TestStandaloneStepsCarryNoOrchestratorHandBack(t *testing.T) {
	for _, mode := range []string{"", "workflow", "full"} {
		for _, ri := range renderAllStepInstructionsMode(t, "spektacular", mode) {
			for _, s := range []string{orchestratedSection, "orchestrator", "orchestrated", "QUESTION:", "FAILED:", "DONE:", ".spektacular/workflows/"} {
				require.NotContainsf(t, ri.body, s,
					"standalone %s (auto_commit=%q) must not mention %q", ri.templatePath, mode, s)
			}
		}
	}
}

// The resume report for an orchestrated lane tells the agent to resume rather
// than ask the user; a standalone one still asks.
func TestEmitResumeReport_OrchestratedDoesNotAskTheUser(t *testing.T) {
	for _, kind := range []string{"plan", "implement"} {
		t.Run(kind, func(t *testing.T) {
			state := &workflow.State{
				Kind:        kind,
				CurrentStep: "discovery",
				Data:        map[string]any{"name": "000024_resume", "orchestrated": true},
			}
			if kind == "implement" {
				state.CurrentStep = "analyze"
			}
			er, ok := emitResumeReport("spektacular", kind, state).(*output.ErrorResponse)
			require.True(t, ok)
			require.NotContains(t, er.NextAction, "Ask the user whether to **resume**")
			require.Contains(t, er.NextAction, "do not ask the user. Resume it")

			state.Data = map[string]any{"name": "000024_resume"}
			er, ok = emitResumeReport("spektacular", kind, state).(*output.ErrorResponse)
			require.True(t, ok)
			require.Contains(t, er.NextAction, "Ask the user whether to **resume**")
			require.NotContains(t, er.NextAction, "do not ask the user")
		})
	}
}

// An orchestrated plan lane driven end to end through the CLI reaches its
// walkthrough with no sign-off to ask for, and its finished step closes all
// three documents final, hands back DONE with the spec named, and removes the
// lane's state and notes files (auto_commit is off here).
func TestOrchestratedPlanFinishesWithoutSignOff(t *testing.T) {
	dataDir := laneProject(t)
	runOK(t, "plan", "new", "--data", `{"name":"20260709000000-alpha","orchestrated":true}`)

	steps := []string{
		"overview", "discovery", "architecture", "components", "data_structures",
		"implementation_detail", "dependencies", "testing_approach", "milestones",
		"tasks", "open_questions", "out_of_scope", "assemble", "verification",
		"write_plan", "write_context", "write_research",
	}
	for _, s := range steps {
		runOK(t, "plan", "goto", "--data", `{"step":"`+s+`","name":"20260709000000-alpha"}`)
	}

	for _, doc := range []string{"plan", "context", "research"} {
		src := filepath.Join(t.TempDir(), doc+".md")
		require.NoError(t, os.WriteFile(src, []byte("# "+doc+"\n\n## Real content\n"), 0o644))
		runOK(t, "plan", "file", "write", "20260709000000-alpha", doc, "--from", src)
	}

	walk := instructionOf(t, runOK(t, "plan", "goto", "--data", `{"step":"walkthrough","name":"20260709000000-alpha"}`))
	require.Equal(t, "walkthrough", laneState(t, dataDir, "plan", "20260709000000-alpha").CurrentStep)
	require.Contains(t, walk, "there is **no sign-off here**")
	require.NotContains(t, walk, "explicit affirmative")
	require.NotContains(t, walk, "walk the user through")
	require.Contains(t, walk, `spektacular plan goto --data '{"step":"finished","name":"20260709000000-alpha"}'`)
	require.Contains(t, walk, "`.spektacular/workflows/plan-20260709000000-alpha.md`")

	// The agent keeps the lane's notes beside its state file.
	notes := filepath.Join(dataDir, "workflows", "plan-20260709000000-alpha.md")
	require.NoError(t, os.WriteFile(notes, []byte("# notes\n"), 0o644))

	fin := instructionOf(t, runOK(t, "plan", "goto", "--data", `{"step":"finished","name":"20260709000000-alpha"}`))
	require.Contains(t, fin, "first line is exactly `DONE: 20260709000000-alpha`")
	require.NotContains(t, fin, "The user signed off")

	// A finished lane is removed, state and notes both: the documents now say
	// the plan is done.
	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "plan-20260709000000-alpha.json"))
	require.NoFileExists(t, notes)
	lane, err := workflow.ReadLane(dataDir, "plan", "20260709000000-alpha")
	require.NoError(t, err)
	require.Nil(t, lane)

	for _, doc := range []string{"plan", "context", "research"} {
		raw, err := os.ReadFile(filepath.Join(dataDir, "plans", "20260709000000-alpha", doc+".md"))
		require.NoError(t, err)
		meta, _, err := metadata.Split(raw)
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.Equalf(t, metadata.StatusFinal, meta.DocumentStatus, "%s must be closed final", doc)
	}
}
