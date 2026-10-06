package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/sessionlog"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests cover how the session log follows orchestrated workflows: a
// command driving a lane is snapshotted from, and filed under, that lane
// rather than the shared state.json.

// TestSnapshotStatePath covers which state file a command's session record
// follows, across the argv shapes an agent sends.
func TestSnapshotStatePath(t *testing.T) {
	dataDir := laneProject(t)
	// alpha has a plan lane; beta has none.
	lanePath := workflow.LaneStatePath(dataDir, "plan", "alpha")
	require.NoError(t, os.MkdirAll(filepath.Dir(lanePath), 0o755))
	require.NoError(t, os.WriteFile(lanePath, []byte(`{"kind":"plan","current_step":"overview","data":{"name":"alpha","orchestrated":true}}`), 0o644))

	shared := stateFilePath(dataDir)
	alphaPlan := lanePath
	gotoAlpha := `{"step":"discovery","name":"alpha"}`

	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"orchestrated plan new", []string{"plan", "new", "--data", `{"name":"beta","orchestrated":true}`}, workflow.LaneStatePath(dataDir, "plan", "beta")},
		{"orchestrated implement new", []string{"implement", "new", "--data", `{"name":"beta","orchestrated":true}`}, workflow.LaneStatePath(dataDir, "implement", "beta")},
		{"standalone plan new", []string{"plan", "new", "--data", `{"name":"beta"}`}, shared},
		{"orchestrated false", []string{"plan", "new", "--data", `{"name":"beta","orchestrated":false}`}, shared},
		{"goto naming a spec with a lane", []string{"plan", "goto", "--data", gotoAlpha}, alphaPlan},
		{"goto naming a spec with no lane", []string{"plan", "goto", "--data", `{"step":"discovery","name":"beta"}`}, shared},
		{"goto naming a lane of another kind", []string{"implement", "goto", "--data", gotoAlpha}, shared},
		{"goto with no name", []string{"plan", "goto", "--data", `{"step":"discovery"}`}, shared},
		{"spec goto", []string{"spec", "goto", "--data", gotoAlpha}, shared},
		{"orchestrated spec new", []string{"spec", "new", "--data", `{"name":"alpha","orchestrated":true}`}, shared},
		{"other plan subcommand", []string{"plan", "steps", "--data", gotoAlpha}, shared},
		{"--fields value before the subcommand", []string{"--fields", "x", "plan", "goto", "--data", gotoAlpha}, alphaPlan},
		{"--file value before the subcommand", []string{"--file", "in.json", "plan", "goto", "--data", gotoAlpha}, alphaPlan},
		{"--data= form", []string{"plan", "goto", "--data=" + gotoAlpha}, alphaPlan},
		{"-d form", []string{"plan", "goto", "-d", gotoAlpha}, alphaPlan},
		{"--data before the subcommand", []string{"--data", gotoAlpha, "plan", "goto"}, alphaPlan},
		{"orchestrated new --data= form", []string{"plan", "new", `--data={"name":"beta","orchestrated":true}`}, workflow.LaneStatePath(dataDir, "plan", "beta")},
		{"invalid name", []string{"plan", "new", "--data", `{"name":"../alpha","orchestrated":true}`}, shared},
		{"upper-case name", []string{"plan", "goto", "--data", `{"step":"discovery","name":"Alpha"}`}, shared},
		{"malformed data", []string{"plan", "goto", "--data", `{"name":`}, shared},
		{"--data with no value", []string{"plan", "goto", "--data"}, shared},
		{"no arguments", nil, shared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, snapshotStatePath(tc.argv))
		})
	}
}

// runDebugCmd runs args through runRoot with os.Args set to match, since the
// session log records (and routes by) the real process argv, not the args
// handed to cobra.
func runDebugCmd(t *testing.T, args ...string) string {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"spektacular"}, args...)
	defer func() { os.Args = saved }()
	return runOK(t, args...)
}

// sessionEvents reads every event of the sole log file for sessionID.
func sessionEvents(t *testing.T, dir, sessionID string) []sessionlog.Event {
	t.Helper()
	suffix := "_" + strings.ReplaceAll(sessionID, ":", "_") + ".jsonl"
	var path string
	for _, f := range sessionLogFiles(t, dir) {
		if strings.HasSuffix(f, suffix) {
			require.Emptyf(t, path, "expected one log file for %s", sessionID)
			path = f
		}
	}
	require.NotEmptyf(t, path, "no log file for %s among %v", sessionID, sessionLogFiles(t, dir))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var events []sessionlog.Event
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var ev sessionlog.Event
		require.NoError(t, json.Unmarshal([]byte(line), &ev))
		events = append(events, ev)
	}
	return events
}

// A lane's commands are filed under the lane's own session, beside a
// standalone workflow in state.json, and the command that finishes the lane
// (removing its files) is still filed under it.
func TestSessionLog_LaneCommandsAreFiledUnderTheLane(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "debug:\n  enabled: true\n")
	resetRootCmd(t)
	dataDir := filepath.Join(dir, ".spektacular")

	runDebugCmd(t, "plan", "new", "--data", `{"name":"solo"}`)
	runDebugCmd(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)

	solo := sessionEvents(t, dir, "plan:solo")
	require.Len(t, solo, 1, "the lane's commands must not land in the standalone session")

	alpha := sessionEvents(t, dir, "plan:alpha")
	require.Len(t, alpha, 2)
	founding := alpha[0]
	require.Equal(t, "plan:alpha", founding.SessionID)
	require.Nil(t, founding.StateBefore)
	require.NotNil(t, founding.StateAfter)
	require.Equal(t, "overview", founding.StateAfter.CurrentStep)
	require.True(t, founding.Advanced)

	step := alpha[1]
	require.Equal(t, "plan:alpha", step.SessionID)
	require.Equal(t, "overview", step.StateBefore.CurrentStep)
	require.Equal(t, "discovery", step.StateAfter.CurrentStep)
	require.Equal(t, "alpha", step.StateAfter.Name)
	require.True(t, step.Advanced)

	// Walk the lane to its end; the finishing goto removes the lane's files.
	for _, s := range planStepsToWalkthrough[2:] {
		runDebugCmd(t, "plan", "goto", "--data", `{"step":"`+s+`","name":"alpha"}`)
	}
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"finished","name":"alpha"}`)
	require.NoFileExists(t, workflow.LaneStatePath(dataDir, "plan", "alpha"), "a finished lane's files are removed")

	alpha = sessionEvents(t, dir, "plan:alpha")
	require.Len(t, alpha, 2+len(planStepsToWalkthrough[2:])+1)
	last := alpha[len(alpha)-1]
	require.Equal(t, "plan:alpha", last.SessionID, "the finishing command is filed under the lane it drove")
	require.NotNil(t, last.StateBefore)
	require.Equal(t, "walkthrough", last.StateBefore.CurrentStep)
	require.Nil(t, last.StateAfter)

	require.Len(t, sessionEvents(t, dir, "plan:solo"), 1, "the standalone session is untouched")
	for _, f := range sessionLogFiles(t, dir) {
		require.NotContains(t, filepath.Base(f), "no-active-workflow")
	}
	require.Equal(t, "overview", sharedState(t, dataDir).CurrentStep)
}
