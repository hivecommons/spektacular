package implement

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// orchestratedImplementConfig is the config an orchestrated implement lane
// renders with.
var orchestratedImplementConfig = workflow.Config{Command: "spektacular", Kind: "implement"}

// renderOrchestratedImplement drives an implement step callback as an
// orchestrated whole-plan lane for 000007_billing.
func renderOrchestratedImplement(t *testing.T, cb workflow.StepCallback, st store.Store) string {
	t.Helper()
	data := &testData{values: map[string]any{"name": "000007_billing", "orchestrated": true}}
	writer := &captureWriter{}
	_, err := cb(data, writer, st, orchestratedImplementConfig)
	require.NoError(t, err)
	return writer.result.Instruction
}

// An orchestrated whole-plan run loops on to its next task without asking,
// and still advances with the spec named.
func TestOrchestratedUpdateChangelogLoopsWithoutAsking(t *testing.T) {
	out := renderOrchestratedImplement(t, updateChangelog(), store.NewFileStore(t.TempDir(), "project"))

	require.Contains(t, out, "This run is orchestrated: do not ask whether to continue. Loop on to the next task automatically.")
	require.NotContains(t, out, "ask the user whether to continue")
	require.NotContains(t, out, "run without asking")
	require.Contains(t, out, `spektacular implement goto --data '{"step":"analyze","name":"000007_billing"}'`)
	require.Contains(t, out, `spektacular implement goto --data '{"step":"test_plan","name":"000007_billing"}'`)
	require.Contains(t, out, "## Running under an orchestrator")
	require.Contains(t, out, "`QUESTION: 000007_billing`")
	require.Contains(t, out, "refresh `.spektacular/workflows/implement-000007_billing.md`")
	require.NotContains(t, out, "refresh `.spektacular/working-context.md`")
}

// The standalone run keeps its between-task question and carries none of the
// orchestrated wording.
func TestStandaloneUpdateChangelogStillAsksBetweenTasks(t *testing.T) {
	out := renderStep(t, updateChangelog())
	require.Contains(t, out, "By default, ask the user whether to continue with the next task or pause here.")
	require.Contains(t, out, `If the user has previously said "run without asking"`)
	require.NotContains(t, out, "orchestrat")
	require.NotContains(t, out, "QUESTION:")
	require.Contains(t, out, "refresh `.spektacular/working-context.md`")
}

// An orchestrated whole-plan finish hands back DONE with the spec named; the
// standalone finish does not.
func TestOrchestratedFinishedHandsBackDone(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	seed, err := metadata.Render(metadata.Metadata{
		CreatedDate:    time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		DocumentStatus: metadata.StatusDraft,
	}, []byte("# body\n"))
	require.NoError(t, err)
	require.NoError(t, st.Write(ChangelogFilePath(orchestratedImplementConfig.ChangelogDir, "000007_billing"), seed))

	out := renderOrchestratedImplement(t, finished(), st)
	require.Contains(t, out, "first line is exactly `DONE: 000007_billing`")
	require.Contains(t, out, "## Running under an orchestrator")

	standalone := renderFinishedStep(t)
	require.NotContains(t, standalone, "DONE:")
	require.NotContains(t, standalone, "orchestrat")
}
