package tracelog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func startRec(id, parent string) Record {
	return Record{Record: RecordSpanStart, SpanID: id, ParentSpanID: parent, Name: id}
}

func endRec(id, parent string) Record {
	return Record{Record: RecordSpanEnd, SpanID: id, ParentSpanID: parent, Name: id}
}

func cmdRec(name, parent string, end time.Time) Record {
	return Record{Record: RecordSpan, SpanID: "c-" + name, ParentSpanID: parent, Name: name, EndTime: end}
}

func ids(recs []Record) []string {
	out := []string{}
	for _, r := range recs {
		out = append(out, r.SpanID)
	}
	return out
}

// TestIndexOpenAndFind covers open, closed, end-only and unknown spans.
func TestIndexOpenAndFind(t *testing.T) {
	ix := NewIndex([]Record{
		startRec("step", ""),
		startRec("helper", "step"),
		endRec("helper", "step"),
		endRec("orphan", ""),
	})

	require.True(t, ix.Open("step"))
	require.False(t, ix.Open("helper"))

	_, open, found := ix.Find("step")
	require.True(t, found)
	require.True(t, open)

	start, open, found := ix.Find("helper")
	require.True(t, found)
	require.False(t, open)
	require.Equal(t, RecordSpanStart, start.Record)

	start, open, found = ix.Find("orphan")
	require.True(t, found, "a span known only by its end is found")
	require.False(t, open)
	require.Equal(t, RecordSpanEnd, start.Record)

	_, _, found = ix.Find("nope")
	require.False(t, found)
}

// TestIndexOpenUnderIsTransitive checks descendants at any depth are found,
// including those under a closed child, while a sibling tree is not.
func TestIndexOpenUnderIsTransitive(t *testing.T) {
	ix := NewIndex([]Record{
		startRec("task", ""),
		startRec("stepA", "task"),
		startRec("child", "stepA"),
		startRec("grandchild", "child"),
		startRec("closedChild", "stepA"),
		startRec("underClosed", "closedChild"),
		endRec("closedChild", "stepA"),
		startRec("stepB", ""),
		startRec("other", "stepB"),
	})

	require.Equal(t, []string{"child", "grandchild", "underClosed"}, ids(ix.OpenUnder("stepA")))
	require.Equal(t, []string{"stepA", "child", "grandchild", "underClosed"}, ids(ix.OpenUnder("task")))
	require.Equal(t, []string{"other"}, ids(ix.OpenUnder("stepB")))
	require.Empty(t, ix.OpenUnder("grandchild"))
	require.Equal(t, []string{"task", "stepA", "child", "grandchild", "underClosed", "stepB", "other"}, ids(ix.OpenAll()))
}

// TestIndexLastCommandUnder checks the latest command under a step is found,
// leaving out resume probes, trace commands and commands of other steps.
func TestIndexLastCommandUnder(t *testing.T) {
	t0 := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	ix := NewIndex([]Record{
		cmdRec("spec goto", "step", t0.Add(1*time.Minute)),
		cmdRec("plan file read", "step", t0.Add(3*time.Minute)),
		cmdRec("knowledge search", "step", t0.Add(2*time.Minute)),
		cmdRec("spec new", "step", t0.Add(10*time.Minute)),
		cmdRec("implement new", "step", t0.Add(11*time.Minute)),
		cmdRec("trace stop", "step", t0.Add(12*time.Minute)),
		cmdRec("trace start", "step", t0.Add(13*time.Minute)),
		cmdRec("spec file read", "otherStep", t0.Add(20*time.Minute)),
	})

	last, ok := ix.LastCommandUnder("step")
	require.True(t, ok)
	require.Equal(t, t0.Add(3*time.Minute), last)

	_, ok = NewIndex([]Record{cmdRec("spec new", "s", t0), cmdRec("trace start", "s", t0)}).LastCommandUnder("s")
	require.False(t, ok, "only probes and trace commands is no activity")
}
