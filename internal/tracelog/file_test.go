package tracelog

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func workflowRun(started time.Time) Run {
	return Run{
		TraceID:   "4bf92f3577b34da6a3ce929d0e0e4736",
		Kind:      "spec",
		Name:      "000073_log-mode",
		StartedAt: started,
		Agent:     "claude",
	}
}

func readRecordLines(t *testing.T, path string) []Record {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var recs []Record
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		var r Record
		require.NoError(t, json.Unmarshal([]byte(line), &r))
		recs = append(recs, r)
	}
	return recs
}

// TestRunFileNames pins the per-run and per-day file names.
func TestRunFileNames(t *testing.T) {
	started := time.Date(2026, 10, 24, 9, 3, 34, 911373161, time.FixedZone("x", 3600))
	tests := []struct {
		name string
		run  Run
		want string
	}{
		{
			name: "workflow run named from its UTC start, agent, kind and name",
			run:  workflowRun(started),
			want: "20261024T080334.911373161Z_claude_spec_000073_log-mode.jsonl",
		},
		{
			name: "empty agent becomes unknown and unsafe characters are replaced",
			run:  Run{Kind: "plan", Name: "a/b:c", StartedAt: started},
			want: "20261024T080334.911373161Z_unknown_plan_a_b_c.jsonl",
		},
		{
			name: "standalone command goes to its UTC day file",
			run:  Run{TraceID: "x", StartedAt: time.Date(2026, 10, 24, 23, 59, 0, 0, time.UTC)},
			want: "standalone_20261024.jsonl",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, runFileName(tc.run))
		})
	}
}

// TestWriteWorkflowRunAppendsOneLinePerRecord checks each write appends one
// line to the run's file, that the second write leaves the earlier bytes as
// they were, and that the directory holds a self-ignoring .gitignore.
func TestWriteWorkflowRunAppendsOneLinePerRecord(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	l := newFileLogger(FileOptions{Directory: dir, Retention: 10})
	run := workflowRun(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))

	first := sampleRecord()
	l.Write(run, first)
	path := filepath.Join(dir, "20261024T090000.000000000Z_claude_spec_000073_log-mode.jsonl")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, 1, bytes.Count(before, []byte("\n")))

	second := sampleRecord()
	second.SpanID = "1111111111111111"
	l.Write(run, second)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(after, before), "earlier contents must be kept byte for byte")
	require.Equal(t, 2, bytes.Count(after, []byte("\n")))

	recs := readRecordLines(t, path)
	require.Len(t, recs, 2)
	require.Equal(t, "00f067aa0ba902b7", recs[0].SpanID)
	require.Equal(t, "1111111111111111", recs[1].SpanID)

	ignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, "*\n", string(ignore))
}

// TestWriteDoesNotReadEarlierContents seeds the run's file with bytes that
// are not JSON: a writer that read or parsed earlier lines would fail or
// rewrite them, while an append leaves them in place with one line added.
func TestWriteDoesNotReadEarlierContents(t *testing.T) {
	dir := t.TempDir()
	run := workflowRun(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	path := filepath.Join(dir, runFileName(run))
	seed := bytes.Repeat([]byte("not json at all\n"), 1000)
	require.NoError(t, os.WriteFile(path, seed, 0o644))

	newFileLogger(FileOptions{Directory: dir, Retention: 10}).Write(run, sampleRecord())

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(after, seed))
	tail := after[len(seed):]
	require.Equal(t, 1, bytes.Count(tail, []byte("\n")))
	var r Record
	require.NoError(t, json.Unmarshal(bytes.TrimRight(tail, "\n"), &r))
	require.Equal(t, RecordSpan, r.Record)
}

// TestWriteStandaloneGoesToDayFile checks two standalone commands of one
// day land in that day's file, each with its own trace id.
func TestWriteStandaloneGoesToDayFile(t *testing.T) {
	dir := t.TempDir()
	l := newFileLogger(FileOptions{Directory: dir, Retention: 10})
	day := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)

	for i, id := range []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"} {
		rec := sampleRecord()
		rec.TraceID = id
		l.Write(Run{TraceID: id, StartedAt: day.Add(time.Duration(i) * time.Hour)}, rec)
	}

	recs := readRecordLines(t, filepath.Join(dir, "standalone_20261024.jsonl"))
	require.Len(t, recs, 2)
	require.NotEqual(t, recs[0].TraceID, recs[1].TraceID)
}

// TestWriteKeepsExistingGitignore checks an existing .gitignore is not
// overwritten.
func TestWriteKeepsExistingGitignore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("custom\n"), 0o644))
	newFileLogger(FileOptions{Directory: dir, Retention: 10}).Write(workflowRun(time.Now()), sampleRecord())
	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, "custom\n", string(got))
}

// captureStd redirects stdout and stderr while fn runs and returns what was
// written to them.
func captureStd(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()

	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	fn()
	require.NoError(t, w.Close())
	os.Stdout, os.Stderr = origOut, origErr
	return string(<-done)
}

// TestWriteUnwritableDirectoryIsContained checks that a log directory that
// cannot be created makes Write return normally, with no panic, nothing
// printed and nothing written.
func TestWriteUnwritableDirectoryIsContained(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	parent := t.TempDir()
	require.NoError(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })
	dir := filepath.Join(parent, "logs")

	l := newFileLogger(FileOptions{Directory: dir, Retention: 10})
	printed := captureStd(t, func() {
		require.NotPanics(t, func() { l.Write(workflowRun(time.Now()), sampleRecord()) })
	})
	require.Empty(t, printed)
	_, err := os.Stat(dir)
	require.True(t, os.IsNotExist(err))
}

// TestWriteUnwritableFileIsContained checks that an existing log directory
// whose files cannot be created makes Write return normally.
func TestWriteUnwritableFileIsContained(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	l := newFileLogger(FileOptions{Directory: dir, Retention: 10})
	printed := captureStd(t, func() {
		require.NotPanics(t, func() { l.Write(Run{TraceID: "x", StartedAt: time.Now()}, sampleRecord()) })
	})
	require.Empty(t, printed)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

// TestWriteUnmarshalableRecordIsContained checks a record that cannot be
// encoded is dropped without a panic.
func TestWriteUnmarshalableRecordIsContained(t *testing.T) {
	dir := t.TempDir()
	rec := sampleRecord()
	rec.Attributes["bad"] = make(chan int)
	require.NotPanics(t, func() {
		newFileLogger(FileOptions{Directory: dir, Retention: 10}).Write(workflowRun(time.Now()), rec)
	})
}

// writeRun writes one record for a workflow run that started at started.
func writeRun(l Logger, name string, started time.Time) {
	l.Write(Run{TraceID: NewTraceID(), Kind: "spec", Name: name, StartedAt: started, Agent: "claude"}, sampleRecord())
}

// jsonlNames lists the .jsonl entries of dir, sorted.
func jsonlNames(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	require.NoError(t, err)
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	return names
}

var retentionBase = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)

// TestSweepKeepsTheNewestRuns checks 12 runs leave the 10 newest with the
// default retention, and a smaller retention keeps that many.
func TestSweepKeepsTheNewestRuns(t *testing.T) {
	for _, tc := range []struct {
		retention int
		want      []string
	}{
		{retention: 10, want: []string{
			"20261001T110000.000000000Z_claude_spec_r02.jsonl",
			"20261001T120000.000000000Z_claude_spec_r03.jsonl",
			"20261001T130000.000000000Z_claude_spec_r04.jsonl",
			"20261001T140000.000000000Z_claude_spec_r05.jsonl",
			"20261001T150000.000000000Z_claude_spec_r06.jsonl",
			"20261001T160000.000000000Z_claude_spec_r07.jsonl",
			"20261001T170000.000000000Z_claude_spec_r08.jsonl",
			"20261001T180000.000000000Z_claude_spec_r09.jsonl",
			"20261001T190000.000000000Z_claude_spec_r10.jsonl",
			"20261001T200000.000000000Z_claude_spec_r11.jsonl",
		}},
		{retention: 3, want: []string{
			"20261001T180000.000000000Z_claude_spec_r09.jsonl",
			"20261001T190000.000000000Z_claude_spec_r10.jsonl",
			"20261001T200000.000000000Z_claude_spec_r11.jsonl",
		}},
	} {
		dir := t.TempDir()
		l := newFileLogger(FileOptions{Directory: dir, Retention: tc.retention})
		for i := range 12 {
			writeRun(l, "r"+twoDigits(i), retentionBase.Add(time.Duration(i)*time.Hour))
		}
		require.Equal(t, tc.want, jsonlNames(t, dir), "retention %d", tc.retention)
	}
}

func twoDigits(i int) string {
	return string([]byte{byte('0' + i/10), byte('0' + i%10)})
}

// TestSweepCountsDaysSeparatelyFromRuns checks standalone commands on many
// days keep the newest day files and never remove a workflow run's log.
func TestSweepCountsDaysSeparatelyFromRuns(t *testing.T) {
	dir := t.TempDir()
	l := newFileLogger(FileOptions{Directory: dir, Retention: 10})
	for i := range 10 {
		writeRun(l, "r"+twoDigits(i), retentionBase.Add(time.Duration(i)*time.Hour))
	}
	for d := range 15 {
		l.Write(Run{TraceID: NewTraceID(), StartedAt: retentionBase.AddDate(0, 0, d)}, sampleRecord())
	}

	var runs, days []string
	for _, n := range jsonlNames(t, dir) {
		if strings.HasPrefix(n, standalonePrefix) {
			days = append(days, n)
		} else {
			runs = append(runs, n)
		}
	}
	require.Len(t, runs, 10, "standalone commands remove no run log")
	require.Equal(t, "standalone_20261006.jsonl", days[0])
	require.Equal(t, "standalone_20261015.jsonl", days[len(days)-1])
	require.Len(t, days, 10)
}

// TestSweepNeverDeletesTheFileItWrote checks a run whose start time sorts it
// among the oldest (a run started long ago) keeps its own new file.
func TestSweepNeverDeletesTheFileItWrote(t *testing.T) {
	dir := t.TempDir()
	l := newFileLogger(FileOptions{Directory: dir, Retention: 3})
	for i := range 3 {
		writeRun(l, "new"+twoDigits(i), retentionBase.Add(time.Duration(i)*time.Hour))
	}
	writeRun(l, "legacy", retentionBase.AddDate(-1, 0, 0))

	names := jsonlNames(t, dir)
	require.Len(t, names, 3)
	require.Contains(t, names, "20251001T090000.000000000Z_claude_spec_legacy.jsonl")
	require.Equal(t, 1, len(readRecordLines(t, filepath.Join(dir, "20251001T090000.000000000Z_claude_spec_legacy.jsonl"))))
}

// TestSweepOnlyOnNewFile checks appending to an existing run file does not
// sweep, so a directory over retention is trimmed only when a new run
// starts.
func TestSweepOnlyOnNewFile(t *testing.T) {
	dir := t.TempDir()
	for i := range 5 {
		name := retentionBase.Add(time.Duration(i)*time.Hour).Format(runFileTimestampFormat) + "_claude_spec_r" + twoDigits(i) + ".jsonl"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}\n"), 0o644))
	}
	l := newFileLogger(FileOptions{Directory: dir, Retention: 2})
	writeRun(l, "r04", retentionBase.Add(4*time.Hour))
	require.Len(t, jsonlNames(t, dir), 5, "an append to an existing run file sweeps nothing")

	writeRun(l, "r05", retentionBase.Add(5*time.Hour))
	require.Equal(t, []string{
		"20261001T130000.000000000Z_claude_spec_r04.jsonl",
		"20261001T140000.000000000Z_claude_spec_r05.jsonl",
	}, jsonlNames(t, dir))
}

// TestSweepSkipsALogThatCannotBeDeleted checks an old log that cannot be
// removed (here a non-empty directory named like one) is skipped: the write
// returns normally, prints nothing, the others are still trimmed, and the
// new record is written.
func TestSweepSkipsALogThatCannotBeDeleted(t *testing.T) {
	dir := t.TempDir()
	stuck := filepath.Join(dir, "20260101T000000.000000000Z_claude_spec_stuck.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Join(stuck, "inner"), 0o755))
	l := newFileLogger(FileOptions{Directory: dir, Retention: 2})
	for i := range 3 {
		writeRun(l, "r"+twoDigits(i), retentionBase.Add(time.Duration(i)*time.Hour))
	}

	printed := captureStd(t, func() {
		require.NotPanics(t, func() { writeRun(l, "r03", retentionBase.Add(3*time.Hour)) })
	})
	require.Empty(t, printed)
	require.DirExists(t, stuck)
	require.Equal(t, []string{
		"20260101T000000.000000000Z_claude_spec_stuck.jsonl",
		"20261001T110000.000000000Z_claude_spec_r02.jsonl",
		"20261001T120000.000000000Z_claude_spec_r03.jsonl",
	}, jsonlNames(t, dir))
}

// TestSweepNonPositiveRetentionUsesTheDefault checks a retention below 1
// keeps the default number of runs rather than deleting everything.
func TestSweepNonPositiveRetentionUsesTheDefault(t *testing.T) {
	dir := t.TempDir()
	l := newFileLogger(FileOptions{Directory: dir, Retention: 0})
	for i := range 12 {
		writeRun(l, "r"+twoDigits(i), retentionBase.Add(time.Duration(i)*time.Hour))
	}
	require.Len(t, jsonlNames(t, dir), defaultRetention)
}
