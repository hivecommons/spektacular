package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// These tests cover runRoot's command log: one OpenTelemetry-model span
// record per command run in a project, written through the configured logger
// at the configured level. Expected values are hand-written; nothing compares
// a wall-clock value.

const (
	traceIDPattern = `^[0-9a-f]{32}$`
	spanIDPattern  = `^[0-9a-f]{16}$`
)

// useConfiguredLogLevel lifts the suite's -log-level override for t, so a
// test asserting logging output sees the level its project configures even
// when the suite runs with logging forced off.
func useConfiguredLogLevel(t *testing.T) {
	t.Helper()
	saved := forcedLogLevel
	forcedLogLevel = ""
	t.Cleanup(func() { forcedLogLevel = saved })
}

// isProjectLogPath reports whether path lies in a project's default log
// directory (.spektacular/logs). The shared snapshot helpers skip it: every
// logged command appends to it, so a test proving a command wrote nothing
// compares everything else.
func isProjectLogPath(path string) bool {
	return strings.Contains("/"+filepath.ToSlash(path)+"/", "/.spektacular/logs/")
}

// logsDir is the default log directory of the project rooted at dir.
func logsDir(dir string) string {
	return filepath.Join(dir, ".spektacular", "logs")
}

// logFiles returns every log file in the log directory dir, sorted; none
// when the directory does not exist.
func logFiles(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	sort.Strings(matches)
	return matches
}

// readRecords reads the command records of the log file at path, in order,
// and requires each to be well formed. The workflow's span_start and
// span_end records beside them are left out (readSpanLog returns those).
func readRecords(t *testing.T, path string) []tracelog.Record {
	t.Helper()
	var recs []tracelog.Record
	for _, rec := range readSpanLog(t, path) {
		if rec.Record == tracelog.RecordSpanStart || rec.Record == tracelog.RecordSpanEnd {
			continue
		}
		requireWellFormed(t, rec)
		recs = append(recs, rec)
	}
	return recs
}

// readSpanLog reads every record of the log file at path, in order:
// command records, span_start and span_end. Each span_start and span_end is
// required to be well formed (see requireWellFormedSpan); command records
// are checked by readRecords.
func readSpanLog(t *testing.T, path string) []tracelog.Record {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var recs []tracelog.Record
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var rec tracelog.Record
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "line %q", line)
		if rec.Record == tracelog.RecordSpanStart || rec.Record == tracelog.RecordSpanEnd {
			requireWellFormedSpan(t, rec)
		}
		recs = append(recs, rec)
	}
	return recs
}

// requireWellFormedSpan checks the fields every span_start and span_end
// record carries: a span_start has no end time and no status; a span_end
// has both and an end reason.
func requireWellFormedSpan(t *testing.T, rec tracelog.Record) {
	t.Helper()
	require.Regexp(t, traceIDPattern, rec.TraceID)
	require.Regexp(t, spanIDPattern, rec.SpanID)
	if rec.ParentSpanID != "" {
		require.Regexp(t, spanIDPattern, rec.ParentSpanID)
	}
	require.NotEmpty(t, rec.Name)
	require.Equal(t, tracelog.KindInternal, rec.Kind)
	require.False(t, rec.StartTime.IsZero(), "start_time")
	require.Contains(t, rec.Attributes, tracelog.AttrSpanType)
	require.Contains(t, rec.Attributes, tracelog.AttrAgent)
	require.Equal(t, tracelog.ServiceName, rec.Resource[tracelog.ResourceServiceName])
	require.Equal(t, version, rec.Resource[tracelog.ResourceServiceVersion])
	switch rec.Record {
	case tracelog.RecordSpanStart:
		require.True(t, rec.EndTime.IsZero(), "a span_start has no end_time")
		require.Equal(t, tracelog.Status{}, rec.Status, "a span_start has no status")
		require.NotContains(t, rec.Attributes, tracelog.AttrEndReason)
	case tracelog.RecordSpanEnd:
		require.False(t, rec.EndTime.IsZero(), "end_time")
		require.False(t, rec.EndTime.Before(rec.StartTime), "a span ends no earlier than it starts")
		require.Contains(t, []string{tracelog.StatusOK, tracelog.StatusError}, rec.Status.Code)
		require.Contains(t, rec.Attributes, tracelog.AttrEndReason)
	}
}

// rawLogSnapshot maps every file in the log directory dir to its content.
// Unlike the shared snapshot helpers it does not skip log paths.
func rawLogSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	snap := map[string]string{}
	for _, e := range entries {
		snap[e.Name()] = string(readBytes(t, filepath.Join(dir, e.Name())))
	}
	return snap
}

// allRecords reads every record of every log file in dir.
func allRecords(t *testing.T, dir string) []tracelog.Record {
	t.Helper()
	var recs []tracelog.Record
	for _, f := range logFiles(t, dir) {
		recs = append(recs, readRecords(t, f)...)
	}
	return recs
}

// soleRecord requires the log directory dir to hold exactly one file with
// exactly one record, and returns it.
func soleRecord(t *testing.T, dir string) tracelog.Record {
	t.Helper()
	files := logFiles(t, dir)
	require.Len(t, files, 1, "expected exactly one log file in %s", dir)
	recs := readRecords(t, files[0])
	require.Len(t, recs, 1)
	return recs[0]
}

// requireWellFormed checks the fields every record carries.
func requireWellFormed(t *testing.T, rec tracelog.Record) {
	t.Helper()
	require.Equal(t, tracelog.RecordSpan, rec.Record)
	require.Regexp(t, traceIDPattern, rec.TraceID)
	require.Regexp(t, spanIDPattern, rec.SpanID)
	require.NotEmpty(t, rec.Name)
	require.Equal(t, tracelog.KindInternal, rec.Kind)
	require.False(t, rec.StartTime.IsZero(), "start_time")
	require.False(t, rec.EndTime.IsZero(), "end_time")
	require.Contains(t, []string{tracelog.StatusOK, tracelog.StatusError}, rec.Status.Code)
	require.Equal(t, tracelog.TypeCommand, rec.Attributes[tracelog.AttrSpanType])
	require.Contains(t, rec.Attributes, tracelog.AttrAgent)
	require.Contains(t, rec.Attributes, tracelog.AttrCommandArgs)
	require.Contains(t, rec.Attributes, tracelog.AttrExitCode)
	require.Equal(t, tracelog.ServiceName, rec.Resource[tracelog.ResourceServiceName])
	require.Equal(t, version, rec.Resource[tracelog.ResourceServiceVersion])
}

// requireNoVerboseAttrs requires rec to carry neither the response nor the
// state snapshots, as a standard-level record does.
func requireNoVerboseAttrs(t *testing.T, rec tracelog.Record) {
	t.Helper()
	for _, k := range []string{tracelog.AttrResponse, tracelog.AttrStateBefore, tracelog.AttrStateAfter} {
		require.NotContains(t, rec.Attributes, k)
	}
}

// stateAttr returns a verbose record's state snapshot attribute as a map,
// requiring it to be present and non-null.
func stateAttr(t *testing.T, rec tracelog.Record, key string) map[string]any {
	t.Helper()
	m, ok := rec.Attributes[key].(map[string]any)
	require.Truef(t, ok, "%s must be a state snapshot, got %#v", key, rec.Attributes[key])
	return m
}

// argsAttr is the record's process.command_args as strings.
func argsAttr(t *testing.T, rec tracelog.Record) []string {
	t.Helper()
	raw, ok := rec.Attributes[tracelog.AttrCommandArgs].([]any)
	require.True(t, ok)
	out := make([]string, len(raw))
	for i, a := range raw {
		out[i] = a.(string)
	}
	return out
}

// runLogged runs args through runRoot with os.Args set to match (see
// runDebugCmd), returning stdout and the exit code without requiring
// success.
func runLogged(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"spektacular"}, args...)
	defer func() { os.Args = saved }()
	stdout, stderr, code := runRootCmd(t, args...)
	require.Empty(t, stderr)
	return stdout, code
}

// readStateMap reads .spektacular/state.json under dir as a generic map.
func readStateMap(t *testing.T, dir string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(readBytes(t, filepath.Join(dir, ".spektacular", "state.json")), &m))
	return m
}

// legacySpecState is a spec workflow's state.json as the previous version
// wrote it, before run ids existed: in progress at step new, for the spec
// 000001_billing.
const legacySpecState = `{
  "kind": "spec",
  "current_step": "new",
  "completed_steps": [],
  "created_at": "2026-10-01T09:00:00.000000001Z",
  "updated_at": "2026-10-01T09:00:00.000000002Z",
  "data": {
    "name": "000001_billing"
  }
}`

// legacySpecProject lays out a project (counter spec ids, logging at level)
// whose spec 000001_billing was started by the previous version: its spec
// exists and state.json is in progress without a run id. The spec is created
// with logging off, then the state is replaced with the legacy shape and the
// configured level switched on.
func legacySpecProject(t *testing.T, level string) (dir string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  level: off\n")
	resetRootCmd(t)
	runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".spektacular", "state.json"), []byte(legacySpecState), 0o644))
	require.NoDirExists(t, logsDir(dir))
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  level: "+level+"\n")
	return dir
}

// With log.level off nothing is written: no logs directory appears, for a
// workflow command, a rejected one or a standalone one.
func TestLog_OffWritesNothing(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: off\n")
	resetRootCmd(t)

	runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	_, code := runLogged(t, "spec", "goto", "--data", `{"step":"bogus"}`)
	require.Equal(t, 1, code)
	runDebugCmd(t, "skill", "list")

	require.NoDirExists(t, logsDir(dir))
}

// logLevel takes the level from the project's configuration alone: empty
// means standard, outside a project, with a config that does not load, or
// for an exempt command it is off, and the suite's forced level wins inside
// a project.
func TestLogLevel(t *testing.T) {
	useConfiguredLogLevel(t)
	for _, tc := range []struct {
		name       string
		forced     string
		configured string
		configOK   bool
		inProject  bool
		exempt     bool
		want       string
	}{
		{"configured off", "", "off", true, true, false, "off"},
		{"configured standard", "", "standard", true, true, false, "standard"},
		{"configured verbose", "", "verbose", true, true, false, "verbose"},
		{"empty means standard", "", "", true, true, false, "standard"},
		{"outside a project", "", "verbose", true, false, false, "off"},
		{"config not OK", "", "verbose", false, true, false, "off"},
		{"exempt", "", "verbose", true, true, true, "off"},
		{"forced off wins over verbose", "off", "verbose", true, true, false, "off"},
		{"forced verbose wins over off", "verbose", "off", true, true, false, "verbose"},
		{"forced standard wins over empty", "standard", "", true, true, false, "standard"},
		{"forced level outside a project", "verbose", "verbose", true, false, false, "off"},
		{"forced level with config not OK", "verbose", "", false, true, false, "off"},
		{"forced level for an exempt command", "verbose", "verbose", true, true, true, "off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forcedLogLevel = tc.forced
			require.Equal(t, tc.want, logLevel(tc.configured, tc.configOK, tc.inProject, tc.exempt))
		})
	}
}

// SPEKTACULAR_SESSION_LOG is no longer read: set to off it does not keep a
// command out of a standard-level log, set to on it records nothing with
// logging off or outside a project.
func TestLog_SessionLogEnvChangesNothing(t *testing.T) {
	useConfiguredLogLevel(t)
	for _, env := range []string{"off", "on"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("SPEKTACULAR_SESSION_LOG", env)

			t.Run("standard level still records", func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, "")
				resetRootCmd(t)
				runDebugCmd(t, "skill", "list")
				require.Equal(t, "skill list", soleRecord(t, logsDir(dir)).Name)
			})

			t.Run("level off still records nothing", func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, "log:\n  level: off\n")
				resetRootCmd(t)
				runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
				require.NoDirExists(t, logsDir(dir))
			})

			t.Run("outside a project nothing is written", func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				resetRootCmd(t)
				runLogged(t, "version")
				runLogged(t, "skill", "list")
				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		})
	}
}

// Nothing is ever written outside a project, even with the level forced to
// verbose.
func TestLog_OutsideAProjectWritesNothingEvenWhenForced(t *testing.T) {
	useConfiguredLogLevel(t)
	forcedLogLevel = config.LogLevelVerbose
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	runLogged(t, "version")
	_, code := runLogged(t, "spec", "file", "list")
	require.Equal(t, 1, code, "no project: the command is refused")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

// At the default (standard) level a successful command and a rejected one
// each leave a record holding the command, its arguments, timing and exit
// status, and no response or state; the rejected one has an error status, a
// non-zero exit code and the error's message.
func TestLog_StandardLevelRecordsSuccessAndRejection(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	resetRootCmd(t)

	newArgs := []string{"spec", "new", "--data", `{"name":"billing"}`}
	runDebugCmd(t, newArgs...)
	gotoArgs := []string{"spec", "goto", "--data", `{"step":"bogus"}`}
	stdout, code := runLogged(t, gotoArgs...)
	require.Equal(t, 1, code)
	er := errorEnvelope(t, stdout)

	files := logFiles(t, logsDir(dir))
	require.Len(t, files, 1)
	require.True(t, strings.HasSuffix(files[0], "_unknown_spec_000001_billing.jsonl"), files[0])
	recs := readRecords(t, files[0])
	require.Len(t, recs, 2)

	ok := recs[0]
	require.Equal(t, "spec new", ok.Name)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusOK}, ok.Status)
	require.Equal(t, float64(0), ok.Attributes[tracelog.AttrExitCode])
	require.Equal(t, newArgs, argsAttr(t, ok))
	requireNoVerboseAttrs(t, ok)

	bad := recs[1]
	require.Equal(t, "spec goto", bad.Name)
	require.Equal(t, tracelog.Status{Code: tracelog.StatusError, Message: `cannot run step "bogus" from the current step "new"`}, bad.Status)
	require.Equal(t, er.Message, bad.Status.Message)
	require.Equal(t, float64(1), bad.Attributes[tracelog.AttrExitCode])
	require.Equal(t, gotoArgs, argsAttr(t, bad))
	require.Equal(t, "new", bad.Attributes[tracelog.AttrStep])
	require.NotContains(t, bad.Attributes, tracelog.AttrStepTo, "a rejected goto does not move the run")
	requireNoVerboseAttrs(t, bad)
}

// A freshly initialised project with no settings changed logs by default:
// `init claude` itself runs before any project exists and is not logged,
// and the next command leaves one well-formed record in .spektacular/logs.
func TestLog_FreshInitProjectLogsByDefault(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	runDebugCmd(t, "init", "claude")
	require.NoDirExists(t, logsDir(dir), "init runs before the project exists")

	runDebugCmd(t, "spec", "file", "list")
	rec := soleRecord(t, logsDir(dir))
	require.Equal(t, "spec file list", rec.Name)
	require.Equal(t, "claude", rec.Attributes[tracelog.AttrAgent])
	require.Equal(t, []string{"spec", "file", "list"}, argsAttr(t, rec))
	require.Equal(t, tracelog.Status{Code: tracelog.StatusOK}, rec.Status)
	requireNoVerboseAttrs(t, rec)
	require.True(t, strings.HasPrefix(filepath.Base(logFiles(t, logsDir(dir))[0]), "standalone_"))
	require.Equal(t, "*\n", string(readBytes(t, filepath.Join(logsDir(dir), ".gitignore"))))
}

// A `spec goto` record carries the configured agent (or unknown when none is
// set), the workflow kind and name, the step it ran in and the step it moved
// to; the run's file is named after the agent, kind and name.
func TestLog_GotoRecordCarriesAgentWorkflowAndSteps(t *testing.T) {
	useConfiguredLogLevel(t)
	for _, tc := range []struct {
		name, agentConfig, want string
	}{
		{"agent claude", "agent: claude\n", "claude"},
		{"no agent", "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, tc.agentConfig+"spec:\n  id_method: counter\n")
			resetRootCmd(t)

			runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)

			files := logFiles(t, logsDir(dir))
			require.Len(t, files, 1)
			require.Regexp(t, `^\d{8}T\d{6}\.\d{9}Z_`+tc.want+`_spec_000001_billing\.jsonl$`, filepath.Base(files[0]))
			recs := readRecords(t, files[0])
			require.Len(t, recs, 2)
			g := recs[1]
			require.Equal(t, "spec goto", g.Name)
			require.Equal(t, tc.want, g.Attributes[tracelog.AttrAgent])
			require.Equal(t, "spec", g.Attributes[tracelog.AttrWorkflowKind])
			require.Equal(t, "000001_billing", g.Attributes[tracelog.AttrWorkflowName])
			require.Equal(t, "new", g.Attributes[tracelog.AttrStep])
			require.Equal(t, "interview", g.Attributes[tracelog.AttrStepTo])
			require.Equal(t, tc.want, recs[0].Attributes[tracelog.AttrAgent])
		})
	}
}

// A spec run's start, gotos, resume report and later gotos share one trace
// id, the state's run id, in one file; a `--force` restart gets a new trace
// id and a new file, and the old file is untouched.
func TestLog_SpecRunSharesOneTraceID(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")
	resetRootCmd(t)

	runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"overview"}`)
	resume, code := runLogged(t, "spec", "new")
	require.Equal(t, 1, code)
	require.Equal(t, "workflow_in_progress", errorEnvelope(t, resume).Code, "spec new with no data reports the run to resume")
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"requirements"}`)

	runID, _ := readStateMap(t, dir)["run_id"].(string)
	require.Regexp(t, traceIDPattern, runID)
	files := logFiles(t, logsDir(dir))
	require.Len(t, files, 1)
	first := files[0]
	recs := readRecords(t, first)
	require.Equal(t, []string{"spec new", "spec goto", "spec goto", "spec new", "spec goto"}, recordNames(recs))
	require.Equal(t, tracelog.StatusError, recs[3].Status.Code, "the resume report is a refusal")
	require.Equal(t, "overview", recs[3].Attributes[tracelog.AttrStep])
	for _, r := range recs {
		require.Equal(t, runID, r.TraceID)
	}
	firstBytes := readBytes(t, first)

	runDebugCmd(t, "spec", "new", "--force", "--data", `{"name":"billing"}`)
	restarted, _ := readStateMap(t, dir)["run_id"].(string)
	require.Regexp(t, traceIDPattern, restarted)
	require.NotEqual(t, runID, restarted)
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)

	files = logFiles(t, logsDir(dir))
	require.Len(t, files, 2)
	require.Contains(t, files, first)
	// The restart only closes the abandoned run's open step span: its
	// file keeps every line it had and gains span_end records alone.
	after := readBytes(t, first)
	require.True(t, bytes.HasPrefix(after, firstBytes), "the abandoned run's records are untouched")
	tail := filepath.Join(t.TempDir(), "tail.jsonl")
	require.NoError(t, os.WriteFile(tail, after[len(firstBytes):], 0o644))
	for _, r := range readSpanLog(t, tail) {
		require.Equal(t, tracelog.RecordSpanEnd, r.Record, "the restart adds only span_end records to the old run")
		require.Equal(t, runID, r.TraceID)
	}
	var newest string
	for _, f := range files {
		if f != first {
			newest = f
		}
	}
	recs = readRecords(t, newest)
	require.Equal(t, []string{"spec new", "spec goto"}, recordNames(recs))
	for _, r := range recs {
		require.Equal(t, restarted, r.TraceID)
	}
}

func recordNames(recs []tracelog.Record) []string {
	names := make([]string, len(recs))
	for i, r := range recs {
		names[i] = r.Name
	}
	return names
}

// A spec workflow whose state was written by the previous version (no run
// id) resumes without error: the first logged command gives it a run id,
// every other field is kept, and every record from then on carries that id.
func TestLog_LegacyStateResumesWithOneTraceID(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := legacySpecProject(t, "standard")
	resetRootCmd(t)

	runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	state := readStateMap(t, dir)
	runID, _ := state["run_id"].(string)
	require.Regexp(t, traceIDPattern, runID)
	require.Equal(t, "2026-10-01T09:00:00.000000001Z", state["created_at"], "the backfill keeps every other field")

	runDebugCmd(t, "spec", "goto", "--data", `{"step":"overview"}`)
	_, code := runLogged(t, "spec", "new")
	require.Equal(t, 1, code, "the resume report")
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"requirements"}`)
	require.Equal(t, runID, readStateMap(t, dir)["run_id"])

	files := logFiles(t, logsDir(dir))
	require.Len(t, files, 1)
	require.Equal(t, "20261001T090000.000000001Z_unknown_spec_000001_billing.jsonl", filepath.Base(files[0]))
	recs := readRecords(t, files[0])
	require.Len(t, recs, 4)
	for _, r := range recs {
		require.Equal(t, runID, r.TraceID)
	}
	require.Equal(t, "new", recs[0].Attributes[tracelog.AttrStep])
	require.Equal(t, "interview", recs[0].Attributes[tracelog.AttrStepTo])
}

// Two standalone `knowledge search` commands with no workflow in progress
// leave two records with different trace ids in the same day's log.
func TestLog_StandaloneCommandsShareTheDayFile(t *testing.T) {
	useConfiguredLogLevel(t)
	root, _, _ := twoScopeProject(t)
	resetRootCmd(t)

	require.Contains(t, runDebugCmd(t, "knowledge", "search", "compass"), "compass")
	runDebugCmd(t, "knowledge", "search", "widgets")

	files := logFiles(t, logsDir(root))
	recs := allRecords(t, logsDir(root))
	require.Len(t, recs, 2)
	if recs[0].StartTime.UTC().Format("20060102") != recs[1].StartTime.UTC().Format("20060102") {
		t.Skip("the two commands straddled midnight UTC")
	}
	require.Len(t, files, 1)
	require.Equal(t, "standalone_"+recs[0].StartTime.UTC().Format("20060102")+".jsonl", filepath.Base(files[0]))
	require.NotEqual(t, recs[0].TraceID, recs[1].TraceID)
	for _, r := range recs {
		require.Equal(t, "knowledge search", r.Name)
		require.Equal(t, "claude", r.Attributes[tracelog.AttrAgent])
		require.NotContains(t, r.Attributes, tracelog.AttrWorkflowKind)
		require.NotContains(t, r.Attributes, tracelog.AttrWorkflowName)
		require.NotContains(t, r.Attributes, tracelog.AttrStep)
		requireNoVerboseAttrs(t, r)
	}
	require.Equal(t, []string{"knowledge", "search", "compass"}, argsAttr(t, recs[0]))
	require.Equal(t, []string{"knowledge", "search", "widgets"}, argsAttr(t, recs[1]))
}

// largeSpecProject lays out a project whose spec store holds feature.md, a
// spec body of several kilobytes carrying a marker, so `spec file read
// feature` has a large response. It returns the project root and the
// marker.
func largeSpecProject(t *testing.T, logConfig string) (dir, marker string) {
	t.Helper()
	dir = t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n"+logConfig)
	marker = "large-response-marker"
	body := "# Feature\n\n" + strings.Repeat("A line of the large spec body, "+marker+".\n", 200)
	require.Greater(t, len(body), 8000)
	path := filepath.Join(dir, ".spektacular", "specs", "feature.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return dir, marker
}

// The standard level omits a several-kilobyte response and the state
// snapshots; the verbose level records the exact output and the state
// before and after.
func TestLog_ResponseAndStateOnlyAtVerbose(t *testing.T) {
	useConfiguredLogLevel(t)

	t.Run("standard", func(t *testing.T) {
		dir, marker := largeSpecProject(t, "")
		resetRootCmd(t)
		out := runDebugCmd(t, "spec", "file", "read", "feature")
		require.Contains(t, out, marker)
		require.Greater(t, len(out), 8000)

		rec := soleRecord(t, logsDir(dir))
		require.Equal(t, "spec file read", rec.Name)
		requireNoVerboseAttrs(t, rec)
		require.NotContains(t, string(readBytes(t, logFiles(t, logsDir(dir))[0])), marker)

		runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
		for _, r := range allRecords(t, logsDir(dir)) {
			requireNoVerboseAttrs(t, r)
		}
	})

	t.Run("verbose", func(t *testing.T) {
		dir, marker := largeSpecProject(t, "log:\n  level: verbose\n")
		resetRootCmd(t)
		out := runDebugCmd(t, "spec", "file", "read", "feature")
		require.Contains(t, out, marker)

		rec := soleRecord(t, logsDir(dir))
		require.Equal(t, out, rec.Attributes[tracelog.AttrResponse], "the response is the exact stdout")
		require.Contains(t, rec.Attributes, tracelog.AttrStateBefore)
		require.Contains(t, rec.Attributes, tracelog.AttrStateAfter)
		require.Nil(t, rec.Attributes[tracelog.AttrStateBefore], "no workflow state exists yet")
		require.Nil(t, rec.Attributes[tracelog.AttrStateAfter])

		runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		gotoOut := runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
		runID, _ := readStateMap(t, dir)["run_id"].(string)

		var g tracelog.Record
		newSpan := ""
		for _, f := range logFiles(t, logsDir(dir)) {
			if strings.HasSuffix(f, "_spec_000001_billing.jsonl") {
				recs := readRecords(t, f)
				require.Len(t, recs, 2)
				g = recs[1]
				for _, r := range readSpanLog(t, f) {
					if r.Record == tracelog.RecordSpanStart && r.Name == "new" {
						newSpan = r.SpanID
					}
				}
			}
		}
		require.Regexp(t, spanIDPattern, newSpan)
		interviewSpan := stateStepSpan(t, dir)
		require.Regexp(t, spanIDPattern, interviewSpan)
		require.Equal(t, "spec goto", g.Name)
		require.Equal(t, gotoOut, g.Attributes[tracelog.AttrResponse])
		require.Equal(t, map[string]any{
			"kind":            "spec",
			"name":            "000001_billing",
			"run_id":          runID,
			"current_step":    "new",
			"completed_steps": []any{},
			"step_span":       newSpan,
		}, stateAttr(t, g, tracelog.AttrStateBefore))
		require.Equal(t, map[string]any{
			"kind":            "spec",
			"name":            "000001_billing",
			"run_id":          runID,
			"current_step":    "interview",
			"completed_steps": []any{"new"},
			"step_span":       interviewSpan,
		}, stateAttr(t, g, tracelog.AttrStateAfter))
	})
}

// statusInvocations are the forms of `status` a caller polls with.
func statusInvocations(name string) [][]string {
	return [][]string{
		{"status"},
		{"status", "--format", "json"},
		{"status", name},
		{"status", name, "--format", "json"},
	}
}

// At the standard and verbose levels `status`, readable or JSON, with or
// without a name, adds no record and creates no log file, and the next
// workflow command is still recorded.
func TestLog_StatusIsNeverLogged(t *testing.T) {
	useConfiguredLogLevel(t)
	for _, level := range []string{"standard", "verbose"} {
		t.Run(level, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  level: "+level+"\n")
			resetRootCmd(t)

			// No workflow yet: the logs directory never appears.
			for _, args := range statusInvocations("000001_billing")[:2] {
				_, code := runLogged(t, args...)
				require.Equal(t, 0, code, "%v", args)
			}
			require.NoDirExists(t, logsDir(dir))

			runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			require.Len(t, allRecords(t, logsDir(dir)), 1)
			before := rawLogSnapshot(t, logsDir(dir))

			for _, args := range statusInvocations("000001_billing") {
				_, code := runLogged(t, args...)
				require.Equal(t, 0, code, "%v", args)
			}
			// Driven through SetArgs alone, so only the post-command check
			// sees the command is status.
			runOK(t, "status", "--format", "json")
			require.Equal(t, before, rawLogSnapshot(t, logsDir(dir)), "status adds no record and no file")

			runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
			recs := allRecords(t, logsDir(dir))
			require.Equal(t, []string{"spec new", "spec goto"}, recordNames(recs))
		})
	}
}

// `status` on a run started by the previous version does not give it a run
// id: state.json stays byte-identical and no log appears; the next workflow
// command gives it one and is recorded under it.
func TestLog_StatusDoesNotBackfillALegacyRun(t *testing.T) {
	useConfiguredLogLevel(t)
	for _, level := range []string{"standard", "verbose"} {
		t.Run(level, func(t *testing.T) {
			dir := legacySpecProject(t, level)
			resetRootCmd(t)
			statePath := filepath.Join(dir, ".spektacular", "state.json")

			for _, args := range statusInvocations("000001_billing") {
				_, code := runLogged(t, args...)
				require.Equal(t, 0, code, "%v", args)
			}
			require.Equal(t, legacySpecState, string(readBytes(t, statePath)))
			require.NoDirExists(t, logsDir(dir))

			runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
			runID, _ := readStateMap(t, dir)["run_id"].(string)
			require.Regexp(t, traceIDPattern, runID)
			rec := soleRecord(t, logsDir(dir))
			require.Equal(t, "spec goto", rec.Name)
			require.Equal(t, runID, rec.TraceID)
		})
	}
}

// logExempted walks the command's parents for the logging-exempt
// annotation, whose key and value are named constants another command group
// can set.
func TestLogExempted(t *testing.T) {
	require.Equal(t, "log", logAnnotation)
	require.Equal(t, "exempt", logExempt)

	parent := &cobra.Command{Use: "group", Annotations: map[string]string{logAnnotation: logExempt}}
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)
	unmarked := &cobra.Command{Use: "plain"}
	other := &cobra.Command{Use: "other", Annotations: map[string]string{logAnnotation: "something-else"}}

	require.True(t, logExempted(parent), "a marked command")
	require.True(t, logExempted(child), "a child of a marked command, through the walk")
	require.False(t, logExempted(unmarked))
	require.False(t, logExempted(other), "only the exempt value exempts")
	require.False(t, logExempted(nil))
	require.True(t, logExempted(statusCmd))
	require.False(t, logExempted(specCmd))
	require.False(t, logExempted(rootCmd))
}

// An unknown log.provider makes config loading fail with config_invalid,
// naming the file logger as the fix, and nothing is logged.
func TestLog_UnknownProviderIsAConfigError(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  provider: bogus\n")
	resetRootCmd(t)

	stdout, code := runLogged(t, "spec", "file", "list")
	require.Equal(t, 1, code)
	er := errorEnvelope(t, stdout)
	require.Equal(t, "config_invalid", er.Code)
	require.Contains(t, er.NextAction, "file")

	stdout, code = runLogged(t, "spec", "new", "--data", `{"name":"billing"}`)
	require.Equal(t, 1, code)
	require.Equal(t, "config_invalid", errorEnvelope(t, stdout).Code)
	require.NoDirExists(t, logsDir(dir))
	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "state.json"))
}

// A custom log.config.directory, relative to the .spektacular folder or
// absolute, receives the records and the default directory stays absent.
func TestLog_CustomDirectoryReceivesTheRecords(t *testing.T) {
	useConfiguredLogLevel(t)
	abs := t.TempDir()
	for _, tc := range []struct {
		name, directory string
		want            func(dir string) string
	}{
		{"relative", "../custom-logs", func(dir string) string { return filepath.Join(dir, "custom-logs") }},
		{"absolute", abs, func(string) string { return abs }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nlog:\n  config:\n    directory: "+tc.directory+"\n")
			resetRootCmd(t)

			runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
			runDebugCmd(t, "skill", "list")

			got := tc.want(dir)
			recs := allRecords(t, got)
			require.ElementsMatch(t, []string{"spec new", "skill list"}, recordNames(recs))
			require.Equal(t, "*\n", string(readBytes(t, filepath.Join(got, ".gitignore"))))
			require.NoDirExists(t, logsDir(dir))
		})
	}
}

// porcelainLogEntries returns the entries of `git status --porcelain` that
// name a log file, listing every untracked file individually.
func porcelainLogEntries(t *testing.T, dir string) []string {
	t.Helper()
	var hits []string
	for _, line := range strings.Split(gittest.RunGit(t, dir, "status", "--porcelain", "--untracked-files=all", "--ignored=no"), "\n") {
		if strings.Contains(line, "logs/") || strings.Contains(line, ".jsonl") {
			hits = append(hits, line)
		}
	}
	return hits
}

// After `init` in a new git repository, and after `migrate` of a format-4
// project, running commands leaves git status free of log files.
func TestLog_LogsStayOutOfGit(t *testing.T) {
	gittest.RequireGit(t)
	useConfiguredLogLevel(t)

	t.Run("init", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)
		gittest.RunGit(t, dir, "init")

		runDebugCmd(t, "init", "claude")
		runDebugCmd(t, "skill", "list")
		runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)

		require.Len(t, logFiles(t, logsDir(dir)), 2, "a run file and a day file")
		require.Empty(t, porcelainLogEntries(t, dir))
	})

	t.Run("migrate of a format-4 project", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)
		writeFormat4Project(t, dir)
		gittest.RunGit(t, dir, "init")
		gittest.RunGit(t, dir, "add", "-A")
		gittest.RunGit(t, dir, "commit", "-m", "format 4")

		runDebugCmd(t, "migrate")
		runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
		runDebugCmd(t, "skill", "list")

		require.NotEmpty(t, logFiles(t, logsDir(dir)))
		require.Empty(t, porcelainLogEntries(t, dir))
	})
}

// writeFormat4Project lays out a project as a format-4 Spektacular left it:
// debug off, a .gitignore that does not know the logs folder, and an old
// debug folder of session logs.
func writeFormat4Project(t *testing.T, dir string) {
	t.Helper()
	writeSettingsFile(t, dir, "config.yaml", `schema: 4
written_by: 9.9.9
skills_version: 9.9.9
name: proj
command: spektacular
agent: claude
debug:
    enabled: false
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", "schema: 2\nwritten_by: 9.9.9\n")
	writeSettingsFile(t, dir, ".gitignore", "*.tmp\n*.log\nrepos/\n")
	writeProjectFile(t, dir, ".spektacular/debug/20261001T000000.000000000Z_claude_spec_x.jsonl", "{\"seq\":0}\n{\"seq\":1}\n")
	writeProjectFile(t, dir, ".spektacular/debug/20261002T000000.000000000Z_claude_no-active-workflow.jsonl", "{\"seq\":0}\n")
}

// Files in an old .spektacular/debug folder are byte-identical after
// `migrate` and further commands.
func TestLog_OldDebugFolderIsUntouched(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)
	writeFormat4Project(t, dir)
	debugDir := filepath.Join(dir, ".spektacular", "debug")
	before := hashStore(t, debugDir)
	require.Len(t, before, 2)

	runDebugCmd(t, "migrate")
	require.Equal(t, before, hashStore(t, debugDir))
	runDebugCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
	runDebugCmd(t, "spec", "goto", "--data", `{"step":"interview"}`)
	runDebugCmd(t, "skill", "list")
	runLogged(t, "status")

	require.NotEmpty(t, logFiles(t, logsDir(dir)))
	require.Equal(t, before, hashStore(t, debugDir))
}

// timestampPattern matches the clock times a response carries (such as a
// workflow's updated_at), which differ between two otherwise identical runs.
var timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)

// parityRun is one command's caller-visible outcome.
type parityRun struct {
	Args   []string
	Stdout string
	Code   int
}

// runParitySet runs a representative command set — a success, a rejection,
// a large output, and status readable and JSON — in a fresh project logging
// at level, with the logs directory unwritable when readOnlyLogs is set, and
// returns each command's output with the project's path and clock times
// normalised.
func runParitySet(t *testing.T, level string, readOnlyLogs bool) []parityRun {
	t.Helper()
	dir, marker := largeSpecProject(t, "agent: claude\nlog:\n  level: "+level+"\n")
	resetRootCmd(t)
	if readOnlyLogs {
		logs := logsDir(dir)
		require.NoError(t, os.MkdirAll(logs, 0o755))
		require.NoError(t, os.Chmod(logs, 0o500))
		t.Cleanup(func() { _ = os.Chmod(logs, 0o755) })
	}

	var runs []parityRun
	for _, args := range [][]string{
		{"spec", "new", "--data", `{"name":"billing"}`},
		{"spec", "goto", "--data", `{"step":"bogus"}`},
		{"spec", "file", "read", "feature"},
		{"spec", "goto", "--data", `{"step":"interview"}`},
		{"status"},
		{"status", "--format", "json"},
		{"status", "000001_billing"},
		{"knowledge", "search", "nothing-matches-this"},
	} {
		stdout, code := runLogged(t, args...)
		normalised := strings.ReplaceAll(stdout, dir, "TESTDIR")
		normalised = timestampPattern.ReplaceAllString(normalised, "TIMESTAMP")
		runs = append(runs, parityRun{Args: args, Stdout: normalised, Code: code})
	}
	require.Contains(t, runs[2].Stdout, marker)
	require.Equal(t, 1, runs[1].Code, "the set includes a rejection")

	switch {
	case level == config.LogLevelOff:
		require.NoDirExists(t, logsDir(dir))
	case readOnlyLogs:
		require.Empty(t, logFiles(t, logsDir(dir)), "an unwritable logs directory receives nothing")
	default:
		require.Len(t, allRecords(t, logsDir(dir)), 5, "status is not logged")
	}
	return runs
}

// Every command's output and exit code are identical with logging off,
// standard and verbose, including when the logs directory cannot be
// written.
func TestLog_OutputParityAcrossLevels(t *testing.T) {
	useConfiguredLogLevel(t)
	want := runParitySet(t, config.LogLevelOff, false)
	for _, level := range []string{config.LogLevelStandard, config.LogLevelVerbose} {
		t.Run(level, func(t *testing.T) {
			require.Equal(t, want, runParitySet(t, level, false))
		})
		t.Run(level+" with an unwritable logs directory", func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("root can write a read-only directory")
			}
			require.Equal(t, want, runParitySet(t, level, true))
		})
	}
}

// The verbose tee never leaks into the command's output stream: what the
// caller receives is the response once, not twice.
func TestLog_VerboseResponseIsWrittenOnce(t *testing.T) {
	useConfiguredLogLevel(t)
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "log:\n  level: verbose\n")
	resetRootCmd(t)

	out := runDebugCmd(t, "skill", "list")
	var v map[string]any
	require.NoError(t, json.NewDecoder(bytes.NewReader([]byte(out))).Decode(&v))
	require.Equal(t, 1, strings.Count(out, `"skills"`))
	require.Equal(t, out, soleRecord(t, logsDir(dir)).Attributes[tracelog.AttrResponse])
}
