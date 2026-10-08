package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests cover implement runs the user starts with implement.worktrees
// on (the default): each spec's run keeps its state and notes in a lane of
// its own (.spektacular/workflows/implement-<name>.json and .md), marked
// "lane" but never "orchestrated", so runs for different specs progress side
// by side and each stays interactive.

// implementLaneProject lays out a throwaway project (no git, worktrees on by
// default) with plans for the specs alpha and beta, and returns its
// .spektacular directory. Each spec already has a worktree record, as an
// earlier run would leave it, so starting a run makes no worktrees and runs
// no git.
func implementLaneProject(t *testing.T) string {
	t.Helper()
	dataDir := laneProject(t)
	writeFixturePlan(t, dataDir, "alpha")
	writeFixturePlan(t, dataDir, "beta")
	writeEmptyWorktreeRecord(t, dataDir, "alpha")
	writeEmptyWorktreeRecord(t, dataDir, "beta")
	return dataDir
}

// writeEmptyWorktreeRecord writes spec's worktree record by hand, mapping no
// repos, so implement new finds the spec's worktrees already made.
func writeEmptyWorktreeRecord(t *testing.T, dataDir, spec string) {
	t.Helper()
	recordDir := filepath.Join(dataDir, "worktrees", spec)
	require.NoError(t, os.MkdirAll(recordDir, 0o755))
	record := `{"spec":"` + spec + `","repos":{}}`
	require.NoError(t, os.WriteFile(filepath.Join(recordDir, "record.json"), []byte(record), 0o644))
}

// requireUserLane asserts the implement lane for name exists at step, keeps
// the spec's name, and is a user's lane run: marked lane, not orchestrated.
func requireUserLane(t *testing.T, dataDir, name, step string) {
	t.Helper()
	lane := laneState(t, dataDir, "implement", name)
	require.Equal(t, "implement", lane.Kind)
	require.Equal(t, step, lane.CurrentStep)
	require.Equal(t, name, lane.Data["name"])
	require.Equal(t, true, lane.Data["lane"])
	_, orchestrated := lane.Data["orchestrated"]
	require.False(t, orchestrated, "a run the user started is never marked orchestrated")
}

// instructionField decodes a step result and returns its instruction.
func instructionField(t *testing.T, stdout string) string {
	t.Helper()
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	instruction, _ := got["instruction"].(string)
	require.NotEmpty(t, instruction)
	return instruction
}

// Criterion 1: with worktrees on, runs for two different specs both start,
// each in its own lane, and a goto naming one advances only that one.
func TestImplementLane_TwoSpecsRunSideBySide(t *testing.T) {
	dataDir := implementLaneProject(t)

	runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	runOK(t, "implement", "new", "--data", `{"name":"beta"}`)

	require.Equal(t, []string{"alpha", "beta"}, workflow.LaneNames(dataDir, "implement"))
	require.FileExists(t, filepath.Join(dataDir, "workflows", "implement-alpha.json"))
	require.FileExists(t, filepath.Join(dataDir, "workflows", "implement-beta.json"))
	requireUserLane(t, dataDir, "alpha", "read_plan")
	requireUserLane(t, dataDir, "beta", "read_plan")
	require.NoFileExists(t, stateFilePath(dataDir), "a lane start never writes state.json")

	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)
	requireUserLane(t, dataDir, "alpha", "analyze")
	requireUserLane(t, dataDir, "beta", "read_plan")
	require.NoFileExists(t, stateFilePath(dataDir))

	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"beta"}`)
	runOK(t, "implement", "goto", "--data", `{"step":"implement","name":"beta"}`)
	requireUserLane(t, dataDir, "alpha", "analyze")
	requireUserLane(t, dataDir, "beta", "implement")
}

// Criterion 2: starting a run for a spec whose own run is in progress reports
// that run for resume, naming its goto and its lane notes, and writes nothing.
func TestImplementLane_RepeatedStartOffersResume(t *testing.T) {
	dataDir := implementLaneProject(t)
	runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)
	lanePath := workflow.LaneStatePath(dataDir, "implement", "alpha")
	before := readBytes(t, lanePath)

	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "analyze", er.Current)
	require.Contains(t, er.NextAction, `spektacular implement goto --data '{"step":"analyze","name":"alpha"}'`)
	require.Contains(t, er.NextAction, "`.spektacular/workflows/implement-alpha.md`")
	require.Contains(t, er.NextAction, `spektacular implement new --force --data '{"name":"alpha"}'`)
	require.NotContains(t, er.NextAction, `"orchestrated":true`)
	require.Contains(t, er.NextAction, "Ask the user whether to **resume**")

	require.Equal(t, before, readBytes(t, lanePath), "a resume report must not touch the lane")
	require.NoFileExists(t, stateFilePath(dataDir))
}

// Criterion 2: a start with no name finds nothing in the shared slot, so it
// asks for a name and lists the runs in progress in their own lanes.
func TestImplementLane_NamelessStartListsInProgressLanes(t *testing.T) {
	dataDir := implementLaneProject(t)
	runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	runOK(t, "implement", "new", "--data", `{"name":"beta"}`)
	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"beta"}`)

	er := runRefused(t, "implement", "new")
	require.Equal(t, "name_required", er.Code)
	require.Contains(t, er.NextAction,
		`"alpha" at step "read_plan" (resume with: spektacular implement goto --data '{"step":"read_plan","name":"alpha"}')`)
	require.Contains(t, er.NextAction,
		`"beta" at step "analyze" (resume with: spektacular implement goto --data '{"step":"analyze","name":"beta"}')`)
	require.NoFileExists(t, stateFilePath(dataDir))
}

// Criterion 2: a start with no name and no lanes in progress asks for a name
// without listing anything.
func TestImplementLane_NamelessStartWithNoLanesListsNone(t *testing.T) {
	implementLaneProject(t)

	er := runRefused(t, "implement", "new")
	require.Equal(t, "name_required", er.Code)
	require.NotContains(t, er.NextAction, "in progress in their own lanes")
}

// Criterion 2: a start with no name still offers to resume a run in the
// shared slot.
func TestImplementLane_NamelessStartResumesSharedSlot(t *testing.T) {
	dataDir := implementLaneProject(t)
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "implement",
		CurrentStep:    "verify",
		CompletedSteps: []string{"new", "read_plan", "analyze", "implement", "test"},
		Data:           map[string]any{"name": "alpha"},
	})
	before := readBytes(t, stateFilePath(dataDir))

	er := runRefused(t, "implement", "new")
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "verify", er.Current)
	require.Contains(t, er.NextAction, "`.spektacular/working-context.md`")
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
	require.Empty(t, workflow.LaneNames(dataDir, "implement"))
}

// Criterion 2: with worktrees on, a named start for a spec whose run is still
// in progress in the shared slot resumes that run rather than orphaning it.
func TestImplementLane_NamedStartResumesSameSpecInSharedSlot(t *testing.T) {
	dataDir := implementLaneProject(t)
	writeInProgressState(t, dataDir, workflow.State{
		Kind:           "implement",
		CurrentStep:    "verify",
		CompletedSteps: []string{"new", "read_plan", "analyze", "implement", "test"},
		Data:           map[string]any{"name": "alpha"},
	})
	before := readBytes(t, stateFilePath(dataDir))

	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "verify", er.Current)
	require.Contains(t, er.NextAction, "`.spektacular/working-context.md`")
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
	require.Empty(t, workflow.LaneNames(dataDir, "implement"), "resuming the shared run must not found a lane")

	// A different spec is not blocked by it, and starts in its own lane.
	runOK(t, "implement", "new", "--data", `{"name":"beta"}`)
	requireUserLane(t, dataDir, "beta", "read_plan")
	require.Equal(t, before, readBytes(t, stateFilePath(dataDir)))
}

// Criterion 3: a lane run's instructions never hand back to an orchestrator,
// and its notes footer names the lane's notes file.
func TestImplementLane_InstructionsStayInteractiveWithLaneNotes(t *testing.T) {
	implementLaneProject(t)

	outputs := []string{runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)}
	for _, s := range []string{"analyze", "implement", "test", "verify", "update_plan", "update_changelog"} {
		outputs = append(outputs, runOK(t, "implement", "goto", "--data", `{"step":"`+s+`","name":"alpha"}`))
	}
	for i, out := range outputs {
		instruction := instructionField(t, out)
		require.NotContainsf(t, instruction, "QUESTION:", "output %d must not hand back to an orchestrator", i)
		require.NotContainsf(t, instruction, "DONE:", "output %d must not hand back to an orchestrator", i)
		require.NotContainsf(t, instruction, "This run is orchestrated", "output %d", i)
		require.Containsf(t, instruction, "refresh `.spektacular/workflows/implement-alpha.md`", "output %d", i)
		require.NotContainsf(t, instruction, ".spektacular/working-context.md", "output %d", i)
	}
}

// Criterion 3: walking a lane run to finished removes its lane files, state
// and notes both, and leaves another spec's lane alone.
func TestImplementLane_FinishRemovesTheLane(t *testing.T) {
	dataDir := implementLaneProject(t)
	changelog := filepath.Join(dataDir, config.DefaultChangelogDir, "alpha.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(changelog), 0o755))
	require.NoError(t, os.WriteFile(changelog, []byte("# alpha\n\nwhat was built\n"), 0o644))

	runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	runOK(t, "implement", "new", "--data", `{"name":"beta"}`)
	for _, s := range []string{
		"analyze", "implement", "test", "verify", "update_plan", "update_changelog",
		"test_plan", "update_feature_changelog", "reconcile_spec",
	} {
		runOK(t, "implement", "goto", "--data", `{"step":"`+s+`","name":"alpha"}`)
	}
	notes := filepath.Join(dataDir, "workflows", "implement-alpha.md")
	require.NoError(t, os.WriteFile(notes, []byte("notes\n"), 0o644))

	fin := instructionField(t, runOK(t, "implement", "goto", "--data", `{"step":"finished","name":"alpha"}`))
	require.NotContains(t, fin, "DONE:", "a lane run reports to the user, not an orchestrator")

	require.NoFileExists(t, filepath.Join(dataDir, "workflows", "implement-alpha.json"))
	require.NoFileExists(t, notes)
	requireUserLane(t, dataDir, "beta", "read_plan")
	require.NoFileExists(t, stateFilePath(dataDir))
}

// A user's start for a spec whose lane holds an in-progress orchestrated run
// is refused, naming the lane, and writes nothing.
func TestImplementLane_StartRefusedWhileOrchestratedLaneInProgress(t *testing.T) {
	dataDir := implementLaneProject(t)
	runOK(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)
	lanePath := workflow.LaneStatePath(dataDir, "implement", "alpha")
	before := readBytes(t, lanePath)

	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha"}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "analyze", er.Current)
	require.Contains(t, er.Message, ".spektacular/workflows/implement-alpha.json")
	require.Contains(t, er.NextAction, `implement goto --data '{"step":"analyze","name":"alpha"}'`)

	require.Equal(t, before, readBytes(t, lanePath))
	require.NoFileExists(t, stateFilePath(dataDir))
}

// Criterion 4: with worktrees off, a start uses the shared state.json exactly
// as before: no lane, no "lane" mark, and a goto with no name drives it.
func TestImplementLane_WorktreesOffUsesSharedSlot(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
	dataDir := filepath.Join(dir, config.ProjectConfigDirName)
	writeFixturePlan(t, dataDir, "alpha")
	writeFixturePlan(t, dataDir, "beta")

	out := runOK(t, "implement", "new", "--data", `{"name":"alpha"}`)
	require.Contains(t, instructionField(t, out), "refresh `.spektacular/working-context.md`")
	shared := sharedState(t, dataDir)
	require.Equal(t, "read_plan", shared.CurrentStep)
	require.Equal(t, "alpha", shared.Data["name"])
	require.Equal(t, []string{"name"}, dataKeys(shared.Data))
	require.Empty(t, workflow.LaneNames(dataDir, "implement"))

	runOK(t, "implement", "goto", "--data", `{"step":"analyze"}`)
	require.Equal(t, "analyze", sharedState(t, dataDir).CurrentStep)

	// A second spec cannot start beside it: the shared slot offers a resume.
	er := runRefused(t, "implement", "new", "--data", `{"name":"beta"}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.NextAction, "spektacular implement new --force\n")
	require.Empty(t, workflow.LaneNames(dataDir, "implement"))
}

// A session record follows an implement start into its lane when worktrees
// are on, and into the shared slot for a dry run or with worktrees off.
func TestImplementLane_SnapshotStatePathFollowsTheLane(t *testing.T) {
	t.Run("worktrees on", func(t *testing.T) {
		dataDir := laneProject(t)
		lane := workflow.LaneStatePath(dataDir, "implement", "alpha")
		shared := stateFilePath(dataDir)
		require.Equal(t, lane, snapshotStatePath([]string{"implement", "new", "--data", `{"name":"alpha"}`}))
		require.Equal(t, shared, snapshotStatePath([]string{"implement", "new", "--dry-run", "--data", `{"name":"alpha"}`}))
		require.Equal(t, shared, snapshotStatePath([]string{"implement", "new", "-n", "--data", `{"name":"alpha"}`}))
		require.Equal(t, shared, snapshotStatePath([]string{"plan", "new", "--data", `{"name":"alpha"}`}))
	})
	t.Run("worktrees off", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, implementSharedSlotConfig)
		dataDir := filepath.Join(dir, config.ProjectConfigDirName)
		require.Equal(t, stateFilePath(dataDir), snapshotStatePath([]string{"implement", "new", "--data", `{"name":"alpha"}`}))
	})
}

// Criterion: in strict mode, an orchestrated run interrupted by a recorded
// spec amendment carries on: repeating its start reports the lane for resume
// rather than plan_stale, and a goto naming the spec advances it, although
// the spec is now newer than the plan. A later unrecorded edit stops it.
func TestImplementLane_OrchestratedResumeAfterRecordedAmendment(t *testing.T) {
	dataDir, specPath := strictAmendProject(t, "alpha", "")
	writeEmptyWorktreeRecord(t, dataDir, "alpha")

	// Start before the spec is touched again: the plan is newer than the
	// spec for this start only.
	planPath := filepath.Join(dataDir, "plans", "alpha", "plan.md")
	require.NoError(t, os.Chtimes(specPath, strictPlanModTime.Add(-time.Hour), strictPlanModTime.Add(-time.Hour)))
	resetRootCmd(t)
	runOK(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	resetRootCmd(t)
	runOK(t, "implement", "goto", "--data", `{"step":"analyze","name":"alpha"}`)

	amendSuccessMetricViaCLI(t, "alpha", specPath)
	planInfo, err := os.Stat(planPath)
	require.NoError(t, err)
	specInfo, err := os.Stat(specPath)
	require.NoError(t, err)
	require.True(t, specInfo.ModTime().After(planInfo.ModTime()), "the amended spec must be newer than its plan")

	resetRootCmd(t)
	er := runRefused(t, "implement", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	require.Equal(t, "workflow_in_progress", er.Code)
	require.Equal(t, "alpha", er.Resource)
	require.Equal(t, "analyze", er.Current)

	resetRootCmd(t)
	runOK(t, "implement", "goto", "--data", `{"step":"implement","name":"alpha"}`)
	lane := laneState(t, dataDir, "implement", "alpha")
	require.Equal(t, "implement", lane.CurrentStep)
	require.Equal(t, true, lane.Data["orchestrated"])
	require.NoFileExists(t, stateFilePath(dataDir))

	editSpecUnrecordedViaCLI(t, "alpha", specPath)
	requirePlanStale(t, "implement", "goto", "--data", `{"step":"test","name":"alpha"}`)
	require.Equal(t, "implement", laneState(t, dataDir, "implement", "alpha").CurrentStep, "a refused goto leaves the lane where it was")
}
