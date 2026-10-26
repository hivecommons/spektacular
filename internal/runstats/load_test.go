package runstats

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

const (
	traceA = "aaaaaaaa000000000000000000000001"
	traceS = "5555555500000000000000000000000f"
)

func specRun() []tracelog.Record {
	a := fx{trace: traceA, kind: "spec", name: "000001_demo", agent: "claude"}
	return recs(
		a.cmd("spec new", "", at(0), at(0.1)),
		a.span("s1", "overview", tracelog.TypeStep, "", at(0.1), at(5), tracelog.EndMoved),
		a.cmd("spec goto", "s1", at(4.9), at(5)),
		a.span("s2", "finished", tracelog.TypeStep, "", at(5), at(5), tracelog.EndFinished),
	)
}

func TestLoadGroupsRunFilesAndDayFiles(t *testing.T) {
	dir := t.TempDir()
	all := specRun()
	// The run's last command went to the day file: records group by trace,
	// whatever file they are in.
	writeLog(t, dir, "20261025T090000.000000000Z_claude_spec_000001_demo.jsonl", all[:len(all)-3])
	s := fx{trace: traceS, agent: "claude"}
	writeLog(t, dir, "standalone_20261025.jsonl", append(all[len(all)-3:], s.cmd("status", "", at(1), at(1.1))))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.jsonl"), 0o755))

	logs, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, dir, logs.Dir)
	require.Zero(t, logs.Skipped)
	require.Len(t, logs.Runs, 1, "the standalone trace is not a run")
	r := logs.Runs[0]
	require.Equal(t, traceA, r.ID)
	require.Equal(t, "spec", r.Kind)
	require.Equal(t, "000001_demo", r.Name)
	require.Equal(t, "claude", r.Agent)
	require.Equal(t, "20261025T090000.000000000Z_claude_spec_000001_demo.jsonl", r.LogFile)
	require.Len(t, r.Commands, 2)
	require.Len(t, r.Spans, 2)
	require.Equal(t, at(0), r.Start)
	require.Equal(t, at(5), r.End)
}

func TestLoadSkipsAndCountsDamagedLines(t *testing.T) {
	clean := t.TempDir()
	writeLog(t, clean, "run.jsonl", specRun())
	want, err := Load(clean)
	require.NoError(t, err)

	dir := t.TempDir()
	rs := specRun()
	noTrace := rs[0]
	noTrace.TraceID = ""
	noSpan := rs[1]
	noSpan.SpanID = ""
	noStart := rs[0]
	noStart.StartTime = time.Time{}
	badKind := rs[0]
	badKind.Record = "event"
	endNoTime := rs[2]
	endNoTime.EndTime = time.Time{}
	writeLog(t, dir, "run.jsonl", append(rs, noTrace, noSpan, noStart, badKind, endNoTime),
		"{not json\n",
		"\n   \n", // blank lines are not counted
		`{"record":"span","trace_id":"`+traceA+`","span_id":"x","start_ti`, // partly written last line
	)

	logs, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, 7, logs.Skipped)
	require.Len(t, logs.Runs, 1)
	got := logs.Runs[0]
	require.Equal(t, want.Runs[0].Start, got.Start)
	require.Equal(t, want.Runs[0].End, got.End)
	require.Equal(t, want.Runs[0].Status, got.Status)
	require.Equal(t, want.Runs[0].Spans, got.Spans)
	require.Equal(t, want.Runs[0].Commands, got.Commands)
	require.Equal(t, want.Runs[0].Events, got.Events)
}

func TestLoadReadsLastLineWithoutNewlineAndLongLines(t *testing.T) {
	dir := t.TempDir()
	rs := specRun()
	rs[0].Attributes[tracelog.AttrResponse] = strings.Repeat("x", 2<<20)
	writeLog(t, dir, "run.jsonl", rs)
	data, err := os.ReadFile(filepath.Join(dir, "run.jsonl"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "run.jsonl"), []byte(strings.TrimSuffix(string(data), "\n")), 0o644))

	logs, err := Load(dir)
	require.NoError(t, err)
	require.Zero(t, logs.Skipped)
	require.Len(t, logs.Runs, 1)
	require.Len(t, logs.Runs[0].Commands, 2, "the 2 MB line and the unterminated last line are both read")
	require.Len(t, logs.Runs[0].Spans, 2)
}

func TestLoadMissingDirectory(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope"))
	require.ErrorIs(t, err, ErrNoLogDir)
	require.True(t, errors.Is(err, fs.ErrNotExist))
}

func TestLoadEmptyDirectory(t *testing.T) {
	logs, err := Load(t.TempDir())
	require.NoError(t, err)
	require.Empty(t, logs.Runs)
	require.Zero(t, logs.Skipped)
}

func TestLoadNeverChangesTheLogs(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "20261025T090000.000000000Z_claude_spec_000001_demo.jsonl", specRun(), "{broken\n")
	writeLog(t, dir, "standalone_20261025.jsonl", recs(fx{trace: traceS}.cmd("status", "", at(1), at(2))))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644))
	before := snapshot(t, dir)
	infoBefore := map[string]time.Time{}
	for name := range before {
		fi, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		infoBefore[name] = fi.ModTime()
	}

	_, err := Load(dir)
	require.NoError(t, err)

	require.Equal(t, before, snapshot(t, dir), "no file created, changed or removed")
	for name, mt := range infoBefore {
		fi, err := os.Stat(filepath.Join(dir, name))
		require.NoError(t, err)
		require.Equal(t, mt, fi.ModTime(), name)
	}
}
