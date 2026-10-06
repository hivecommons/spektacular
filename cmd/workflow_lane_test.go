package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests cover orchestrated plan and implement workflows running in
// lanes of their own (.spektacular/workflows/<kind>-<name>.json) beside the
// shared state.json, and goto's "name" routing key.

// laneProject lays out a throwaway project (auto_commit off, no git) and
// returns its .spektacular directory.
func laneProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "")
	return filepath.Join(dir, config.ProjectConfigDirName)
}

// laneState reads the kind lane for name, requiring it to exist.
func laneState(t *testing.T, dataDir, kind, name string) *workflow.State {
	t.Helper()
	s, err := workflow.ReadLane(dataDir, kind, name)
	require.NoError(t, err)
	require.NotNilf(t, s, "the %s lane for %q must exist", kind, name)
	return s
}

// sharedState reads state.json, requiring it to exist.
func sharedState(t *testing.T, dataDir string) *workflow.State {
	t.Helper()
	s, err := readState(stateFilePath(dataDir))
	require.NoError(t, err)
	require.NotNil(t, s, "state.json must exist")
	return s
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func dataKeys(d map[string]any) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// runOK runs a command through runRoot and requires it to succeed.
func runOK(t *testing.T, args ...string) string {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equalf(t, 0, code, "%v failed: %s", args, stdout)
	require.Empty(t, stderr)
	return stdout
}

// runRefused runs a command through runRoot, requires it to fail, and returns
// its error envelope.
func runRefused(t *testing.T, args ...string) (er struct {
	Code       string
	Message    string
	Resource   string
	NextAction string
	Current    string
}) {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equalf(t, 1, code, "%v must be refused: %s", args, stdout)
	require.Empty(t, stderr)
	env := errorEnvelope(t, stdout)
	er.Code, er.Message, er.Resource, er.NextAction = env.Code, env.Message, env.Resource, env.NextAction
	if env.State != nil {
		er.Current = env.State.Current
	}
	return er
}

// Criterion 1 (and 4, 6): two orchestrated plan workflows and one standalone
// plan workflow are all in progress in one project at once, and gotos
// interleaved between them each advance only the workflow they address.
func TestLanes_TwoOrchestratedAndOneStandaloneAdvanceIndependently(t *testing.T) {
	dataDir := laneProject(t)

	runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runOK(t, "plan", "new", "--data", `{"name":"beta","orchestrated":true}`)
	// A standalone start is not blocked by lanes for other specs.
	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)

	require.Equal(t, []string{"alpha", "beta"}, workflow.LaneNames(dataDir, "plan"))
	require.Equal(t, "overview", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, "overview", laneState(t, dataDir, "plan", "beta").CurrentStep)
	shared := sharedState(t, dataDir)
	require.Equal(t, "overview", shared.CurrentStep)
	require.Equal(t, "solo", shared.Data["name"])
	_, sharedOrchestrated := shared.Data["orchestrated"]
	require.False(t, sharedOrchestrated, "a standalone workflow is not marked orchestrated")
	require.Equal(t, true, laneState(t, dataDir, "plan", "alpha").Data["orchestrated"])

	// alpha advances; nothing else moves.
	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	require.Equal(t, "discovery", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, "overview", laneState(t, dataDir, "plan", "beta").CurrentStep)
	require.Equal(t, "overview", sharedState(t, dataDir).CurrentStep)

	// A goto with no name drives state.json exactly as before (criterion 6).
	runOK(t, "plan", "goto", "--data", `{"step":"discovery"}`)
	require.Equal(t, "discovery", sharedState(t, dataDir).CurrentStep)
	require.Equal(t, "discovery", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, "overview", laneState(t, dataDir, "plan", "beta").CurrentStep)

	// beta advances twice; alpha and solo stay put.
	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"beta"}`)
	runOK(t, "plan", "goto", "--data", `{"step":"architecture","name":"beta"}`)
	require.Equal(t, "architecture", laneState(t, dataDir, "plan", "beta").CurrentStep)
	require.Equal(t, "discovery", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, "discovery", sharedState(t, dataDir).CurrentStep)

	// Naming the spec state.json holds routes to the shared slot (criterion 4).
	runOK(t, "plan", "goto", "--data", `{"step":"architecture","name":"solo"}`)
	shared = sharedState(t, dataDir)
	require.Equal(t, "architecture", shared.CurrentStep)
	require.Equal(t, "solo", shared.Data["name"])
	require.Equal(t, "discovery", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, "architecture", laneState(t, dataDir, "plan", "beta").CurrentStep)

	// Each lane keeps its own spec name and step history.
	require.Equal(t, "alpha", laneState(t, dataDir, "plan", "alpha").Data["name"])
	require.Equal(t, "beta", laneState(t, dataDir, "plan", "beta").Data["name"])
	require.Equal(t, []string{"new", "overview", "discovery"}, laneState(t, dataDir, "plan", "beta").CompletedSteps)
}

// Criterion 4: "name" is a routing key, never workflow data. A lane advanced
// by a named goto ends up with exactly the data keys a shared workflow
// advanced by an unnamed goto has (plus the orchestrated marker new set), and
// its name is still the spec name new stored.
func TestLanes_GotoNameIsNotStoredAsWorkflowData(t *testing.T) {
	dataDir := laneProject(t)

	runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)

	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery"}`)

	lane := laneState(t, dataDir, "plan", "alpha")
	shared := sharedState(t, dataDir)
	require.Equal(t, "alpha", lane.Data["name"])

	laneKeys := dataKeys(lane.Data)
	withoutMarker := make([]string, 0, len(laneKeys))
	for _, k := range laneKeys {
		if k != "orchestrated" {
			withoutMarker = append(withoutMarker, k)
		}
	}
	require.Contains(t, laneKeys, "orchestrated")
	require.Equal(t, dataKeys(shared.Data), withoutMarker,
		"a named goto must add no data keys an unnamed goto would not")

	// A named goto against the shared slot leaves its data untouched too.
	before := dataKeys(sharedState(t, dataDir).Data)
	runOK(t, "plan", "goto", "--data", `{"step":"architecture","name":"solo"}`)
	shared = sharedState(t, dataDir)
	require.Equal(t, before, dataKeys(shared.Data))
	require.Equal(t, "solo", shared.Data["name"])
}

// Criterion 2: repeating an orchestrated start for a spec whose lane is in
// progress reports that lane for resume and does not start it over.
func TestLanes_RepeatedOrchestratedStartReportsResume(t *testing.T) {
	dataDir := laneProject(t)

	runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	lanePath := workflow.LaneStatePath(dataDir, "plan", "alpha")
	before := readBytes(t, lanePath)

	er := runRefused(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "discovery", er.Current)
	require.NotEmpty(t, er.NextAction)

	require.Equal(t, before, readBytes(t, lanePath), "a repeated orchestrated start must not touch the lane")
	require.NoFileExists(t, stateFilePath(dataDir), "an orchestrated start never writes state.json")
}

// An orchestrated start is not blocked by a standalone workflow in state.json,
// and does not touch it.
func TestLanes_OrchestratedStartIgnoresStandaloneWorkflow(t *testing.T) {
	dataDir := laneProject(t)

	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery"}`)
	before := readBytes(t, stateFilePath(dataDir))

	runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	require.Equal(t, "overview", laneState(t, dataDir, "plan", "alpha").CurrentStep)
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
}

// --force on an orchestrated start clears and restarts only that lane.
func TestLanes_ForcedOrchestratedStartClearsOnlyItsLane(t *testing.T) {
	dataDir := laneProject(t)

	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery"}`)
	runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	runOK(t, "plan", "new", "--data", `{"name":"beta","orchestrated":true}`)
	runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"beta"}`)
	sharedBefore := readBytes(t, stateFilePath(dataDir))
	betaBefore := readBytes(t, workflow.LaneStatePath(dataDir, "plan", "beta"))

	runOK(t, "plan", "new", "--force", "--data", `{"name":"alpha","orchestrated":true}`)

	alpha := laneState(t, dataDir, "plan", "alpha")
	require.Equal(t, "overview", alpha.CurrentStep)
	require.Equal(t, []string{"new"}, alpha.CompletedSteps)
	require.Equal(t, sharedBefore, readBytes(t, stateFilePath(dataDir)))
	require.Equal(t, betaBefore, readBytes(t, workflow.LaneStatePath(dataDir, "plan", "beta")))
}

// Criterion 3: an ordinary start for a spec whose lane is in progress is
// refused, naming the lane, and writes nothing — even with --force.
func TestLanes_StandaloneStartRefusedWhileLaneInProgress(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "plain"
		if force {
			name = "force"
		}
		t.Run(name, func(t *testing.T) {
			dataDir := laneProject(t)
			runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
			runOK(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
			laneBefore := readBytes(t, workflow.LaneStatePath(dataDir, "plan", "alpha"))

			args := []string{"plan", "new"}
			if force {
				args = append(args, "--force")
			}
			args = append(args, "--data", `{"name":"alpha"}`)
			er := runRefused(t, args...)

			require.Equal(t, "workflow_in_progress", er.Code)
			require.Equal(t, "alpha", er.Resource)
			require.Equal(t, "discovery", er.Current)
			require.Contains(t, er.Message, ".spektacular/workflows/plan-alpha.json", "the refusal must name the lane")
			require.Contains(t, er.NextAction, `plan goto --data '{"step":"discovery","name":"alpha"}'`)
			require.Contains(t, er.NextAction, `plan new --force --data '{"name":"alpha","orchestrated":true}'`)

			require.NoFileExists(t, stateFilePath(dataDir), "a refused standalone start must write nothing")
			require.Equal(t, laneBefore, readBytes(t, workflow.LaneStatePath(dataDir, "plan", "alpha")))
		})
	}
}

// A finished lane no longer blocks a standalone start for its spec.
func TestLanes_FinishedLaneDoesNotBlockStandaloneStart(t *testing.T) {
	dataDir := laneProject(t)
	lanePath := workflow.LaneStatePath(dataDir, "plan", "alpha")
	require.NoError(t, os.MkdirAll(filepath.Dir(lanePath), 0o755))
	b, err := json.Marshal(workflow.State{Kind: "plan", CurrentStep: "finished", Data: map[string]any{"name": "alpha"}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lanePath, b, 0o644))

	runOK(t, "plan", "new", "--data", `{"name":"alpha"}`)
	require.Equal(t, "alpha", sharedState(t, dataDir).Data["name"])
}

// Criterion 5: a goto naming a spec with no workflow — nothing at all, or
// something other than what the shared record holds — is refused with
// workflow_not_found, an actionable next step, and no state change.
func TestLanes_GotoNamingUnknownSpecIsRefused(t *testing.T) {
	t.Run("empty project", func(t *testing.T) {
		dataDir := laneProject(t)
		er := runRefused(t, "plan", "goto", "--data", `{"step":"discovery","name":"ghost"}`)
		require.Equal(t, "workflow_not_found", er.Code)
		require.Equal(t, "ghost", er.Resource)
		require.NotEmpty(t, er.NextAction)
		require.Contains(t, er.NextAction, `plan new --data '{"name":"ghost"}'`)
		require.NoFileExists(t, stateFilePath(dataDir))
		require.Empty(t, workflow.LaneNames(dataDir, "plan"))
	})

	t.Run("does not match shared record or any lane", func(t *testing.T) {
		dataDir := laneProject(t)
		runOK(t, "plan", "new", "--data", `{"name":"solo"}`)
		runOK(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
		sharedBefore := readBytes(t, stateFilePath(dataDir))
		laneBefore := readBytes(t, workflow.LaneStatePath(dataDir, "plan", "alpha"))

		er := runRefused(t, "plan", "goto", "--data", `{"step":"discovery","name":"ghost"}`)
		require.Equal(t, "workflow_not_found", er.Code)
		require.Equal(t, "ghost", er.Resource)
		require.NotEmpty(t, er.NextAction)
		// The message lists the workflows that are in progress.
		require.Contains(t, er.Message, `"solo"`)
		require.Contains(t, er.Message, `"alpha"`)

		require.Equal(t, sharedBefore, readBytes(t, stateFilePath(dataDir)))
		require.Equal(t, laneBefore, readBytes(t, workflow.LaneStatePath(dataDir, "plan", "alpha")))
		require.NoFileExists(t, workflow.LaneStatePath(dataDir, "plan", "ghost"))
	})

	t.Run("implement", func(t *testing.T) {
		laneProject(t)
		er := runRefused(t, "implement", "goto", "--data", `{"step":"analyze","name":"ghost"}`)
		require.Equal(t, "workflow_not_found", er.Code)
		require.NotEmpty(t, er.NextAction)
	})
}

// A goto with an invalid name is refused before anything is read or written.
func TestLanes_GotoWithInvalidNameIsRefused(t *testing.T) {
	dataDir := laneProject(t)
	runOK(t, "plan", "new", "--data", `{"name":"solo"}`)
	before := readBytes(t, stateFilePath(dataDir))

	_, _, code := runRootCmd(t, "plan", "goto", "--data", `{"step":"discovery","name":"Not Valid"}`)
	require.Equal(t, 1, code)
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
}

// A named goto while state.json holds another kind's workflow still gets the
// cross-kind report it always did, rather than workflow_not_found.
func TestLanes_NamedGotoAgainstOtherKindKeepsCrossKindReport(t *testing.T) {
	dataDir := laneProject(t)
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "spec",
		CurrentStep:    "overview",
		CompletedSteps: []string{"new"},
		CreatedAt:      fixedResumeTime,
		UpdatedAt:      fixedResumeTime,
		Data:           map[string]any{"name": "000024_resume"},
	})
	before := readBytes(t, stateFilePath(dataDir))

	er := runRefused(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	require.Equal(t, "cross_kind_workflow_in_progress", er.Code)
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
}

// An orchestrated start skips the uncommitted-changes gate; a standalone start
// in the same dirty project is still stopped by it.
func TestLanes_OrchestratedStartSkipsUncommittedChangesGate(t *testing.T) {
	fx := gitProject(t, config.AutoCommitWorkflow)
	writeFixturePlan(t, filepath.Join(fx.root, config.ProjectConfigDirName), "billing")
	seedCommittedProjectFiles(t, fx)
	dirtyPreExistingFile(t, fx)
	dataDir := filepath.Join(fx.root, config.ProjectConfigDirName)

	for _, kind := range []string{"plan", "implement"} {
		er := runRefused(t, kind, "new", "--data", `{"name":"billing"}`)
		require.Equalf(t, "uncommitted_changes", er.Code, "standalone %s new must still hit the gate", kind)
	}
	require.NoFileExists(t, statePathOf(fx))

	runOK(t, "plan", "new", "--data", `{"name":"billing","orchestrated":true}`)
	runOK(t, "implement", "new", "--data", `{"name":"billing","orchestrated":true}`)

	require.Equal(t, "overview", laneState(t, dataDir, "plan", "billing").CurrentStep)
	require.Equal(t, "read_plan", laneState(t, dataDir, "implement", "billing").CurrentStep)
	require.NoFileExists(t, statePathOf(fx), "orchestrated starts never write state.json")
	require.Equal(t, "1", commitCount(t, fx.root), "skipping the gate must not commit the user's work")
}

// An orchestrated implement start writes an implement lane, marked
// orchestrated, and a named implement goto advances it.
func TestLanes_OrchestratedImplementRunsInItsLane(t *testing.T) {
	dataDir := laneProject(t)
	writeFixturePlan(t, dataDir, "alpha")

	runOK(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	require.NoFileExists(t, stateFilePath(dataDir))
	lane := laneState(t, dataDir, "implement", "alpha")
	require.Equal(t, "implement", lane.Kind)
	require.Equal(t, "read_plan", lane.CurrentStep)
	require.Equal(t, "alpha", lane.Data["name"])
	require.Equal(t, true, lane.Data["orchestrated"])
	require.Empty(t, workflow.LaneNames(dataDir, "plan"), "an implement lane is not a plan lane")

	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)
	lane = laneState(t, dataDir, "implement", "alpha")
	require.Equal(t, "analyze", lane.CurrentStep)
	require.Equal(t, "alpha", lane.Data["name"])
	require.NoFileExists(t, stateFilePath(dataDir))

	// A standalone implement start for the same spec is refused, naming the lane.
	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Contains(t, er.Message, ".spektacular/workflows/implement-alpha.json")

	// Repeating the orchestrated start reports the lane for resume.
	er = runRefused(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "analyze", er.Current)
	require.Equal(t, "analyze", laneState(t, dataDir, "implement", "alpha").CurrentStep)
}

// An orchestrated start without a valid name is refused and writes nothing.
func TestLanes_OrchestratedStartNeedsAName(t *testing.T) {
	dataDir := laneProject(t)
	for _, data := range []string{`{"orchestrated":true}`, `{"name":"Bad Name","orchestrated":true}`} {
		_, _, code := runRootCmd(t, "plan", "new", "--data", data)
		require.Equalf(t, 1, code, "data %s", data)
	}
	require.Empty(t, workflow.LaneNames(dataDir, "plan"))
	require.NoFileExists(t, stateFilePath(dataDir))
}
