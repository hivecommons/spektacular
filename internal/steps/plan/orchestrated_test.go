package plan

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// renderOrchestrated drives a plan step callback as an orchestrated lane for
// 000007_billing, against st, and returns its instruction.
func renderOrchestrated(t *testing.T, cb workflow.StepCallback, st store.Store) string {
	t.Helper()
	data := &testData{values: map[string]any{"name": "000007_billing", "orchestrated": true}}
	writer := &captureWriter{}
	cfg := workflow.Config{Command: "spektacular", Kind: "plan", PlanDir: "plans", SpecDir: "specs"}
	_, err := cb(data, writer, st, cfg)
	require.NoError(t, err)
	return writer.result.Instruction
}

// An orchestrated walkthrough asks for no sign-off: it reads the committed
// documents back, prepares the hand-back summary, and advances to finished
// with the spec named.
func TestOrchestratedWalkthroughHasNoSignOff(t *testing.T) {
	out := renderOrchestrated(t, walkthrough(), store.NewFileStore(t.TempDir(), "project"))

	require.Contains(t, out, "there is **no sign-off here**")
	require.Contains(t, out, "Do not ask the user to review this plan, and do not wait for an answer.")
	require.NotContains(t, out, "explicit affirmative")
	require.NotContains(t, out, "walk the user through")
	require.NotContains(t, out, "mandatory")
	require.NotContains(t, out, "Once the user has explicitly signed off")

	for _, doc := range []string{"plan", "context", "research"} {
		require.Contains(t, out, "spektacular plan file read 000007_billing "+doc)
	}
	require.Contains(t, out, "## Drafting assumptions")
	require.Contains(t, out, "5. The project-wide rules this plan relies on or decides:")
	require.Contains(t, out, "one line each with what this plan does, or say there are none.")
	require.Contains(t, out, "Your orchestrator compares these across the epic's plans to find where they disagree.")
	require.Contains(t, out, `spektacular plan goto --data '{"step":"finished","name":"000007_billing"}'`)
	require.Contains(t, out, "## Running under an orchestrator")
	require.Contains(t, out, "`QUESTION: 000007_billing`")
	require.Contains(t, out, "refresh `.spektacular/workflows/plan-000007_billing.md`")
}

// An orchestrated drafting step carries the contradiction stop and hands a
// STOP back to the orchestrator as a QUESTION naming the spec.
func TestOrchestratedGatheringStepHandsBackContradictionStop(t *testing.T) {
	for name, cb := range map[string]workflow.StepCallback{
		"discovery":    discovery(),
		"architecture": architecture(),
		"tasks":        tasks(),
	} {
		t.Run(name, func(t *testing.T) {
			out := renderOrchestrated(t, cb, store.NewFileStore(t.TempDir(), "project"))
			require.Contains(t, out, "contradict a decision the user recorded")
			require.Contains(t, out, "STOP and ask the user before going further")
			require.Contains(t, out, "## Running under an orchestrator")
			require.Contains(t, out, "**Wherever this step says to STOP, to ask the user, or to report to the user**, hand it back instead.")
			require.Contains(t, out, "`QUESTION: 000007_billing`")
		})
	}
}

// The standalone walkthrough keeps its mandatory sign-off and carries none of
// the orchestrated branch or hand-back section.
func TestStandaloneWalkthroughKeepsSignOff(t *testing.T) {
	out := renderStep(t, walkthrough())
	require.Contains(t, out, "explicit affirmative")
	require.Contains(t, out, "Once the user has explicitly signed off")
	require.NotContains(t, out, "no sign-off here")
	require.NotContains(t, out, "orchestrat")
	require.NotContains(t, out, "QUESTION:")
	require.NotContains(t, out, "project-wide rules")
}

// An orchestrated finished step closes all three documents final and hands
// back DONE with the spec named, instead of reporting a sign-off to the user.
func TestOrchestratedFinishedClosesDocsAndHandsBackDone(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	seedFilledPlanDocs(t, st, "plans", "000007_billing", created)

	out := renderOrchestrated(t, finished(), st)

	for _, doc := range planDocs {
		p := doc.path("plans", "000007_billing")
		raw, err := st.Read(p)
		require.NoError(t, err)
		meta, _, err := metadata.Split(raw)
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.Equal(t, metadata.StatusFinal, meta.DocumentStatus, "%s must be final", p)
	}

	require.Contains(t, out, "first line is exactly `DONE: 000007_billing`")
	require.Contains(t, out, "do not report to the user")
	require.Contains(t, out, "(approach, milestones and tasks with any `human` tasks, out of scope, drafting assumptions, project-wide rules)")
	require.NotContains(t, out, "The user signed off")
	require.NotContains(t, out, "approved and ready for implementation")
	require.Contains(t, out, "## Running under an orchestrator")
	require.NotContains(t, out, "**Before you advance:**", "finished is terminal and carries no footer")
}

// The standalone finished step still reports the sign-off and hands nothing
// back.
func TestStandaloneFinishedHasNoDoneHandBack(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seedFilledPlanDocs(t, st, "plans", "fixture", time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))
	writer := &captureWriter{}
	_, err := finished()(&testData{values: map[string]any{"name": "fixture"}}, writer, st,
		workflow.Config{Command: "spektacular", Kind: "plan", PlanDir: "plans", SpecDir: "specs"})
	require.NoError(t, err)

	out := writer.result.Instruction
	require.Contains(t, out, "The user signed off on the plan during the walkthrough")
	require.NotContains(t, out, "DONE:")
	require.NotContains(t, out, "orchestrat")
}
