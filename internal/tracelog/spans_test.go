package tracelog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// writeOnly is a logger with no read side.
type writeOnly struct{ recs []Record }

func (w *writeOnly) Write(_ Run, rec Record) { w.recs = append(w.recs, rec) }

func fileSpans(t *testing.T) (Spans, *fileLogger) {
	t.Helper()
	l := newFileLogger(FileOptions{Directory: t.TempDir()}).(*fileLogger)
	return Spans{
		Log:      l,
		Run:      workflowRun(time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)),
		Base:     map[string]any{AttrAgent: "claude", AttrWorkflowKind: "spec"},
		Resource: Resource("0.22.0"),
	}, l
}

func readAll(t *testing.T, sp Spans) []Record {
	t.Helper()
	recs, err := sp.Log.(Reader).Records(sp.Run)
	require.NoError(t, err)
	return recs
}

// TestSpansStartAndEnd checks a start is written with the base attributes
// and type, and its end repeats it with the end reason and status.
func TestSpansStartAndEnd(t *testing.T) {
	sp, _ := fileSpans(t)
	t0 := sp.Run.StartedAt
	st := sp.Start("test-writer", TypeSubAgent, "a3ce929d0e0e4736", t0, map[string]any{AttrStep: "test"})
	require.Len(t, st.SpanID, 16)
	sp.End(st, t0.Add(time.Second), Status{Code: StatusOK}, EndStopped)

	recs := readAll(t, sp)
	require.Len(t, recs, 2)
	require.Equal(t, RecordSpanStart, recs[0].Record)
	require.Equal(t, RecordSpanEnd, recs[1].Record)
	for _, r := range recs {
		require.Equal(t, st.SpanID, r.SpanID)
		require.Equal(t, "a3ce929d0e0e4736", r.ParentSpanID)
		require.Equal(t, "test-writer", r.Name)
		require.Equal(t, sp.Run.TraceID, r.TraceID)
		require.Equal(t, TypeSubAgent, r.Attributes[AttrSpanType])
		require.Equal(t, "claude", r.Attributes[AttrAgent])
		require.Equal(t, "test", r.Attributes[AttrStep])
	}
	require.True(t, recs[0].EndTime.IsZero())
	require.Equal(t, t0.Add(time.Second), recs[1].EndTime)
	require.Equal(t, StatusOK, recs[1].Status.Code)
	require.Equal(t, EndStopped, recs[1].Attributes[AttrEndReason])

	start, open, found := sp.Find(st.SpanID)
	require.True(t, found)
	require.False(t, open)
	require.Equal(t, RecordSpanStart, start.Record)
}

// TestSpansEndUnderClosesDescendantsOnly checks ending a step closes its
// child and grandchild, deepest first, and not a span under another step.
func TestSpansEndUnderClosesDescendantsOnly(t *testing.T) {
	sp, _ := fileSpans(t)
	t0 := sp.Run.StartedAt
	stepA := sp.StartWithID("aaaaaaaaaaaaaaaa", "a", TypeStep, "", t0, nil)
	child := sp.Start("helper", TypeSubAgent, stepA.SpanID, t0, nil)
	grand := sp.Start("helper-own", TypeWork, child.SpanID, t0, nil)
	stepB := sp.StartWithID("bbbbbbbbbbbbbbbb", "b", TypeStep, "", t0, nil)
	sibling := sp.Start("other", TypeSubAgent, stepB.SpanID, t0, nil)

	at := t0.Add(time.Minute)
	sp.EndUnder(stepA.SpanID, at, EndAbandoned)

	recs := readAll(t, sp)
	var ended []string
	for _, r := range recs {
		if r.Record == RecordSpanEnd {
			ended = append(ended, r.SpanID)
			require.Equal(t, at, r.EndTime)
			require.Equal(t, Status{Code: StatusError, Message: "abandoned"}, r.Status)
			require.Equal(t, EndAbandoned, r.Attributes[AttrEndReason])
		}
	}
	require.Equal(t, []string{grand.SpanID, child.SpanID}, ended, "deepest first, and nothing else")

	ix, ok := sp.Index()
	require.True(t, ok)
	require.True(t, ix.Open(stepA.SpanID), "EndUnder leaves the parent itself open")
	require.True(t, ix.Open(sibling.SpanID))
}

// TestSpansEndAllClosesEveryOpenSpan checks ending a run closes every span
// still open in it and leaves closed ones alone.
func TestSpansEndAllClosesEveryOpenSpan(t *testing.T) {
	sp, _ := fileSpans(t)
	t0 := sp.Run.StartedAt
	step := sp.StartWithID("aaaaaaaaaaaaaaaa", "a", TypeStep, "", t0, nil)
	done := sp.Start("done", TypeWork, step.SpanID, t0, nil)
	sp.End(done, t0, Status{Code: StatusOK}, EndStopped)
	sp.Start("left", TypeSubAgent, step.SpanID, t0, nil)

	sp.EndAll(t0.Add(time.Minute), EndAbandoned)

	ix, ok := sp.Index()
	require.True(t, ok)
	require.Empty(t, ix.OpenAll())
	ends := 0
	for _, r := range readAll(t, sp) {
		if r.Record == RecordSpanEnd {
			ends++
		}
	}
	require.Equal(t, 3, ends, "one stop, then the open helper and step")
}

// TestSpansLastCommandUnder reads a step's last command from the log.
func TestSpansLastCommandUnder(t *testing.T) {
	sp, l := fileSpans(t)
	t0 := sp.Run.StartedAt
	l.Write(sp.Run, Record{Record: RecordSpan, TraceID: sp.Run.TraceID, SpanID: "0000000000000001", ParentSpanID: "aaaaaaaaaaaaaaaa", Name: "plan file read", StartTime: t0, EndTime: t0.Add(time.Minute), Status: Status{Code: StatusOK}})
	l.Write(sp.Run, Record{Record: RecordSpan, TraceID: sp.Run.TraceID, SpanID: "0000000000000002", ParentSpanID: "aaaaaaaaaaaaaaaa", Name: "plan new", StartTime: t0, EndTime: t0.Add(time.Hour), Status: Status{Code: StatusOK}})

	last, ok := sp.LastCommandUnder("aaaaaaaaaaaaaaaa")
	require.True(t, ok)
	require.Equal(t, t0.Add(time.Minute), last)
}

// TestSpansWithoutReaderStillWrites checks a logger with no read side has
// its spans written, while sweeps and queries do nothing.
func TestSpansWithoutReaderStillWrites(t *testing.T) {
	w := &writeOnly{}
	sp := Spans{Log: w, Run: workflowRun(time.Now())}
	st := sp.Start("x", TypeWork, "", time.Now(), nil)
	sp.EndUnder("", time.Now(), EndAbandoned)
	sp.EndAll(time.Now(), EndAbandoned)
	require.Len(t, w.recs, 1)

	_, _, found := sp.Find(st.SpanID)
	require.False(t, found)
	_, ok := sp.LastCommandUnder("")
	require.False(t, ok)
}

// TestSpansNilLoggerIsSafe checks a Spans with no logger never panics.
func TestSpansNilLoggerIsSafe(t *testing.T) {
	sp := Spans{}
	require.NotPanics(t, func() {
		st := sp.Start("x", TypeWork, "", time.Now(), nil)
		sp.End(st, time.Now(), Status{Code: StatusOK}, EndStopped)
		sp.EndAll(time.Now(), EndAbandoned)
	})
}
