package tracelog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRecordsReadsRunFileInOrder writes a run file holding command, span
// start and end lines, a non-span line, garbage and a partly written last
// line, and checks the span records come back in file order.
func TestRecordsReadsRunFileInOrder(t *testing.T) {
	dir := t.TempDir()
	run := workflowRun(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC))
	path := runFilePath(dir, run)
	lines := `{"record":"span","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"0000000000000001","name":"spec new","kind":"internal"}
{"record":"span_start","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"0000000000000002","name":"new","kind":"internal"}
not json at all, "record":"span" but broken
{"record":"metric","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"0000000000000009"}
{"record":"span_end","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"0000000000000002","name":"new","kind":"internal"}
{"record":"span_start","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"0000000000000003","name":"interview","kind":"internal"}
{"record":"span_end","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00000000000000`
	require.NoError(t, os.WriteFile(path, []byte(lines), 0o644))

	l := newFileLogger(FileOptions{Directory: dir}).(*fileLogger)
	recs, err := l.Records(run)
	require.NoError(t, err)

	var got []string
	for _, r := range recs {
		got = append(got, r.Record+":"+r.SpanID)
	}
	require.Equal(t, []string{
		"span:0000000000000001",
		"span_start:0000000000000002",
		"span_end:0000000000000002",
		"span_start:0000000000000003",
	}, got)
}

// TestRecordsMissingFileIsEmpty checks a run with no log yet reads as no
// records, not an error.
func TestRecordsMissingFileIsEmpty(t *testing.T) {
	l := newFileLogger(FileOptions{Directory: t.TempDir()}).(*fileLogger)
	recs, err := l.Records(workflowRun(time.Now()))
	require.NoError(t, err)
	require.Empty(t, recs)
}

// TestRecordsStandaloneReadsTwoDayFilesByTrace checks a standalone run's
// records come from the day before and its own day, filtered to its trace.
func TestRecordsStandaloneReadsTwoDayFilesByTrace(t *testing.T) {
	dir := t.TempDir()
	trace := "4bf92f3577b34da6a3ce929d0e0e4736"
	other := "11111111111111111111111111111111"
	yesterday := `{"record":"span_start","trace_id":"` + trace + `","span_id":"0000000000000001","name":"a"}
{"record":"span_start","trace_id":"` + other + `","span_id":"0000000000000002","name":"b"}
`
	today := `{"record":"span_end","trace_id":"` + trace + `","span_id":"0000000000000001","name":"a"}
{"record":"span","trace_id":"` + other + `","span_id":"0000000000000003","name":"c"}
`
	older := `{"record":"span_start","trace_id":"` + trace + `","span_id":"0000000000000004","name":"too old"}
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "standalone_20261023.jsonl"), []byte(yesterday), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "standalone_20261024.jsonl"), []byte(today), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "standalone_20261022.jsonl"), []byte(older), 0o644))

	l := newFileLogger(FileOptions{Directory: dir}).(*fileLogger)
	recs, err := l.Records(Run{TraceID: trace, StartedAt: time.Date(2026, 10, 24, 0, 5, 0, 0, time.UTC)})
	require.NoError(t, err)

	var got []string
	for _, r := range recs {
		got = append(got, r.Record+":"+r.Name)
	}
	require.Equal(t, []string{"span_start:a", "span_end:a"}, got)
}

// TestFileLoggerIsAReader pins that the shipped logger has the read side.
func TestFileLoggerIsAReader(t *testing.T) {
	l, err := New(ProviderFile, FileOptions{Directory: t.TempDir()})
	require.NoError(t, err)
	_, ok := l.(Reader)
	require.True(t, ok)
}
