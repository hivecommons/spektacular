package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// These tests cover how the command log follows orchestrated workflows: a
// command driving a lane is snapshotted from, and filed under, that lane's
// run rather than the shared state.json's.

// TestSnapshotStatePath covers which state file a command's record follows,
// across the argv shapes an agent sends.
func TestSnapshotStatePath(t *testing.T) {
	dataDir := laneProject(t)
	// alpha has a plan lane; beta has none.
	lanePath := workflow.LaneStatePath(dataDir, "plan", "alpha")
	require.NoError(t, os.MkdirAll(filepath.Dir(lanePath), 0o755))
	require.NoError(t, os.WriteFile(lanePath, []byte(`{"kind":"plan","current_step":"overview","data":{"name":"alpha","orchestrated":true}}`), 0o644))

	// gamma has both a plan and an implement lane.
	for _, k := range []string{"plan", "implement"} {
		require.NoError(t, os.WriteFile(workflow.LaneStatePath(dataDir, k, "gamma"), []byte(`{"kind":"`+k+`","current_step":"overview","data":{"name":"gamma","orchestrated":true}}`), 0o644))
	}

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
		{"plan file read naming a spec with a lane", []string{"plan", "file", "read", "alpha", "plan"}, alphaPlan},
		{"plan file write naming a spec with a lane", []string{"plan", "file", "write", "alpha", "plan", "--from", "x.md"}, alphaPlan},
		{"spec file read naming a spec with a lane", []string{"spec", "file", "read", "alpha"}, alphaPlan},
		{"status naming a spec with a lane", []string{"status", "alpha", "--format", "json"}, alphaPlan},
		{"status naming no lane, as for an epic", []string{"status", "todo-epic", "--format", "json"}, shared},
		{"argument naming a spec with no lane", []string{"plan", "file", "read", "beta", "plan"}, shared},
		{"argument that is not a spec name", []string{"plan", "file", "write", "alpha", "plan", "--from", ".spektacular/tmp/x.md"}, alphaPlan},
		{"plan command prefers the plan lane", []string{"plan", "file", "read", "gamma", "plan"}, workflow.LaneStatePath(dataDir, "plan", "gamma")},
		{"implement command prefers the implement lane", []string{"implement", "status", "gamma"}, workflow.LaneStatePath(dataDir, "implement", "gamma")},
		{"implement goal record naming a spec with a lane", []string{"implement", "goal", "record", "--data", `{"name":"gamma","item":"t-1"}`, "--from", "r.json"}, workflow.LaneStatePath(dataDir, "implement", "gamma")},
		{"implement goal decide naming a spec with no implement lane", []string{"implement", "goal", "decide", "--data", `{"name":"alpha","item":"t-1","decision":"skip"}`}, shared},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, snapshotStatePath(tc.argv))
		})
	}
}

// runDebugCmd runs args through runRoot with os.Args set to match, since the
// command log records (and routes by, and checks the logging exemption of)
// the real process argv, not the args handed to cobra.
func runDebugCmd(t *testing.T, args ...string) string {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"spektacular"}, args...)
	defer func() { os.Args = saved }()
	return runOK(t, args...)
}

// laneLogFile returns the sole log file of the plan run named name, whose
// file ends in "_<agent>_plan_<name>.jsonl".
func laneLogFile(t *testing.T, logs, agent, name string) string {
	t.Helper()
	suffix := "_" + agent + "_plan_" + name + ".jsonl"
	var path string
	for _, f := range logFiles(t, logs) {
		if strings.HasSuffix(f, suffix) {
			require.Emptyf(t, path, "expected one log file for plan %s", name)
			path = f
		}
	}
	require.NotEmptyf(t, path, "no log file for plan %s among %v", name, logFiles(t, logs))
	return path
}

// requireOneTrace requires every record to carry traceID.
func requireOneTrace(t *testing.T, recs []tracelog.Record, traceID string) {
	t.Helper()
	require.NotEmpty(t, recs)
	for i, r := range recs {
		require.Equalf(t, traceID, r.TraceID, "record %d (%s)", i, r.Name)
	}
}

// Two orchestrated plan lanes and one standalone plan workflow run beside
// each other with interleaved gotos: each lane's records are filed under its
// own run, in its own <ts>_<agent>_plan_<name>.jsonl, with the lane's
// constant run id as trace id, distinct from the other lane's and from the
// standalone run's; the command that finishes a lane (removing its files) is
// still filed under it.
func TestLog_LaneCommandsAreFiledUnderTheLane(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: verbose\n")
	resetRootCmd(t)
	dataDir := filepath.Join(dir, ".spektacular")
	logs := logsDir(dir)

	runDebugCmd(t, "plan", "new", "--data", `{"name":"solo"}`)
	runDebugCmd(t, "plan", "new", "--data", `{"name":"alpha","orchestrated":true}`)
	runDebugCmd(t, "plan", "new", "--data", `{"name":"beta","orchestrated":true}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"discovery","name":"alpha"}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"discovery","name":"beta"}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"architecture","name":"alpha"}`)
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"architecture","name":"beta"}`)

	soloRun := sharedState(t, dataDir).RunID
	alphaRun := laneState(t, dataDir, "plan", "alpha").RunID
	betaRun := laneState(t, dataDir, "plan", "beta").RunID
	require.Regexp(t, traceIDPattern, alphaRun)
	require.Regexp(t, traceIDPattern, betaRun)
	require.NotEqual(t, alphaRun, betaRun)
	require.NotEqual(t, soloRun, alphaRun)
	require.NotEqual(t, soloRun, betaRun)

	solo := readRecords(t, laneLogFile(t, logs, "unknown", "solo"))
	require.Len(t, solo, 1, "the lanes' commands must not land in the standalone run")
	requireOneTrace(t, solo, soloRun)

	alpha := readRecords(t, laneLogFile(t, logs, "unknown", "alpha"))
	require.Len(t, alpha, 3)
	requireOneTrace(t, alpha, alphaRun)
	beta := readRecords(t, laneLogFile(t, logs, "unknown", "beta"))
	require.Len(t, beta, 3)
	requireOneTrace(t, beta, betaRun)

	founding := alpha[0]
	require.Equal(t, "plan new", founding.Name)
	require.Nil(t, founding.Attributes[tracelog.AttrStateBefore])
	require.Equal(t, "overview", stateAttr(t, founding, tracelog.AttrStateAfter)["current_step"])
	require.Equal(t, "overview", founding.Attributes[tracelog.AttrStepTo])
	require.NotContains(t, founding.Attributes, tracelog.AttrStep)

	step := alpha[1]
	require.Equal(t, "plan goto", step.Name)
	require.Equal(t, "plan", step.Attributes[tracelog.AttrWorkflowKind])
	require.Equal(t, "alpha", step.Attributes[tracelog.AttrWorkflowName])
	require.Equal(t, "overview", step.Attributes[tracelog.AttrStep])
	require.Equal(t, "discovery", step.Attributes[tracelog.AttrStepTo])
	require.Equal(t, "alpha", stateAttr(t, step, tracelog.AttrStateAfter)["name"])
	require.Equal(t, alphaRun, stateAttr(t, step, tracelog.AttrStateAfter)["run_id"])
	require.Equal(t, "beta", beta[1].Attributes[tracelog.AttrWorkflowName])

	// Walk alpha to its end; the finishing goto removes the lane's files.
	for _, s := range planStepsToWalkthrough[3:] {
		runDebugCmd(t, "plan", "goto", "--data", `{"step":"`+s+`","name":"alpha"}`)
	}
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"finished","name":"alpha"}`)
	require.NoFileExists(t, workflow.LaneStatePath(dataDir, "plan", "alpha"), "a finished lane's files are removed")

	alpha = readRecords(t, laneLogFile(t, logs, "unknown", "alpha"))
	require.Len(t, alpha, 3+len(planStepsToWalkthrough[3:])+1)
	requireOneTrace(t, alpha, alphaRun)
	last := alpha[len(alpha)-1]
	require.Equal(t, "walkthrough", last.Attributes[tracelog.AttrStep], "the finishing command is filed under the lane it drove")
	require.NotContains(t, last.Attributes, tracelog.AttrStepTo)
	require.Equal(t, "walkthrough", stateAttr(t, last, tracelog.AttrStateBefore)["current_step"])
	require.Nil(t, last.Attributes[tracelog.AttrStateAfter])

	// beta keeps its own run after alpha finished.
	runDebugCmd(t, "plan", "goto", "--data", `{"step":"components","name":"beta"}`)
	beta = readRecords(t, laneLogFile(t, logs, "unknown", "beta"))
	require.Len(t, beta, 4)
	requireOneTrace(t, beta, betaRun)

	require.Len(t, readRecords(t, laneLogFile(t, logs, "unknown", "solo")), 1, "the standalone run is untouched")
	for _, f := range logFiles(t, logs) {
		require.NotContains(t, filepath.Base(f), "standalone_")
	}
	require.Len(t, logFiles(t, logs), 3)
	require.Equal(t, "overview", sharedState(t, dataDir).CurrentStep)
}
