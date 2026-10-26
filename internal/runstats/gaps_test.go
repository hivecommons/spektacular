package runstats

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

func TestGapsNeverCountACommandsOwnTime(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	r := Assemble(recs(
		a.cmd("spec new", "", at(0), at(10)),
		a.cmd("spec status", "", at(2), at(3)), // inside the long command
		a.cmd("spec goto", "", at(12), at(13)),
	))[0]
	require.Equal(t, []gap{{
		from: at(10), to: at(12),
		after:  GapEdge{Event: EdgeEnd, Type: tracelog.TypeCommand, Name: "spec new"},
		before: GapEdge{Event: EdgeStart, Type: tracelog.TypeCommand, Name: "spec goto"},
	}}, gaps(r), "the long command covers its own time, and the one nested in it does not label the gap")
}

func TestGapsAreLabelledBySpanEdges(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	r := Assemble(recs(
		a.cmd("spec new", "", at(0), at(1)),
		a.open("s1", "interview", tracelog.TypeStep, "", at(3)),
		a.span("h1", "research", tracelog.TypeSubAgent, "s1", at(5), at(6), tracelog.EndStopped),
		a.cmd("spec goto", "s1", at(8), at(9)),
	))[0]
	cmdEnd := GapEdge{Event: EdgeEnd, Type: tracelog.TypeCommand, Name: "spec new"}
	stepStart := GapEdge{Event: EdgeStart, Type: tracelog.TypeStep, Name: "interview"}
	helperStart := GapEdge{Event: EdgeStart, Type: tracelog.TypeSubAgent, Name: "research"}
	helperEnd := GapEdge{Event: EdgeEnd, Type: tracelog.TypeSubAgent, Name: "research"}
	gotoStart := GapEdge{Event: EdgeStart, Type: tracelog.TypeCommand, Name: "spec goto"}
	require.Equal(t, []gap{
		{from: at(1), to: at(3), after: cmdEnd, before: stepStart},
		{from: at(3), to: at(5), after: stepStart, before: helperStart},
		{from: at(5), to: at(6), after: helperStart, before: helperEnd},
		{from: at(6), to: at(8), after: helperEnd, before: gotoStart},
	}, gaps(r))
}

func TestGapsNoneInASingleCommandRun(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	r := Assemble(recs(a.cmd("spec new", "", at(0), at(30))))[0]
	require.Empty(t, gaps(r))

	rep := BuildReport(r, DefaultOptions)
	require.Empty(t, rep.Gaps)
	require.Zero(t, rep.Totals.GapMS)
	require.Zero(t, rep.Totals.GapsOverThreshold)
}

// twentyMinutePause is a walkthrough step whose commands are separated by
// gaps of 1, 20, 6, 1 and 7 minutes, with a 1-minute gap before the first.
func twentyMinutePause() []tracelog.Record {
	a := fx{trace: traceA, kind: "plan", name: "000007_pause", agent: "claude"}
	return recs(
		a.cmd("plan new", "", at(0), at(1)),
		a.span("s1", "walkthrough", tracelog.TypeStep, "", at(1), at(41), tracelog.EndMoved),
		a.cmd("plan goto", "s1", at(2), at(3)),
		a.cmd("plan goto", "s1", at(23), at(24)),
		a.cmd("plan status", "s1", at(30), at(31)),
		a.cmd("plan status", "s1", at(32), at(33)),
		a.cmd("plan goto", "s1", at(40), at(41)),
	)
}

func TestReportTwentyMinutePauseIsTheLongestGap(t *testing.T) {
	run := Assemble(twentyMinutePause())[0]
	rep := BuildReport(run, Options{Gaps: 3, GapThreshold: 5 * time.Minute})

	walk := str("walkthrough")
	gotoEnd := GapEdge{Event: EdgeEnd, Type: tracelog.TypeCommand, Name: "plan goto"}
	gotoStart := GapEdge{Event: EdgeStart, Type: tracelog.TypeCommand, Name: "plan goto"}
	statusEnd := GapEdge{Event: EdgeEnd, Type: tracelog.TypeCommand, Name: "plan status"}
	statusStart := GapEdge{Event: EdgeStart, Type: tracelog.TypeCommand, Name: "plan status"}
	require.Equal(t, []Gap{
		{StartedAt: at(3), EndedAt: at(23), DurationMS: 1200000, Step: walk, After: gotoEnd, Before: gotoStart},
		{StartedAt: at(33), EndedAt: at(40), DurationMS: 420000, Step: walk, After: statusEnd, Before: gotoStart},
		{StartedAt: at(24), EndedAt: at(30), DurationMS: 360000, Step: walk, After: gotoEnd, Before: statusStart},
	}, rep.Gaps, "at most 3 gaps, longest first, the 20-minute pause first")
	require.EqualValues(t, 300000, rep.GapThresholdMS)
	require.EqualValues(t, 1980000, rep.Totals.GapMS, "20 + 7 + 6 minutes are over 5 minutes")
	require.Equal(t, 3, rep.Totals.GapsOverThreshold)
}

func TestReportGapThreshold(t *testing.T) {
	cases := []struct {
		name      string
		threshold time.Duration
		gapMS     int64
		over      int
	}{
		{"default 5 minutes", DefaultOptions.GapThreshold, 1980000, 3},
		{"a gap equal to the threshold does not count", 6 * time.Minute, 1620000, 2},
		{"30 minutes leaves the pause out", 30 * time.Minute, 0, 0},
	}
	run := Assemble(twentyMinutePause())[0]
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := BuildReport(run, Options{Gaps: 3, GapThreshold: c.threshold})
			require.Equal(t, c.gapMS, rep.Totals.GapMS)
			require.Equal(t, c.over, rep.Totals.GapsOverThreshold)
			require.Equal(t, c.threshold.Milliseconds(), rep.GapThresholdMS)
			require.Len(t, rep.Gaps, 3, "the threshold does not change which gaps are listed")
			require.EqualValues(t, 1200000, rep.Gaps[0].DurationMS)
		})
	}
}

func TestReportGapTotalsCoverUnlistedGaps(t *testing.T) {
	rep := BuildReport(Assemble(twentyMinutePause())[0], Options{Gaps: 1, GapThreshold: 5 * time.Minute})
	require.Len(t, rep.Gaps, 1)
	require.EqualValues(t, 1200000, rep.Gaps[0].DurationMS)
	require.EqualValues(t, 1980000, rep.Totals.GapMS)
	require.Equal(t, 3, rep.Totals.GapsOverThreshold)
}

func TestReportZeroGapsListsEveryGapLongestFirst(t *testing.T) {
	rep := BuildReport(Assemble(twentyMinutePause())[0], Options{Gaps: 0, GapThreshold: 5 * time.Minute})
	type span struct {
		from, to time.Time
		ms       int64
	}
	var got []span
	for _, g := range rep.Gaps {
		got = append(got, span{g.StartedAt, g.EndedAt, g.DurationMS})
	}
	require.Equal(t, []span{
		{at(3), at(23), 1200000},
		{at(33), at(40), 420000},
		{at(24), at(30), 360000},
		{at(1), at(2), 60000}, // equal lengths: earliest first
		{at(31), at(32), 60000},
	}, got)
}

func TestGapsUnderAMillisecondAreNotGaps(t *testing.T) {
	a := fx{trace: traceA, kind: "spec", name: "n", agent: "claude"}
	end := at(1)
	r := Assemble(recs(
		a.cmd("spec new", "", at(0), end),
		a.cmd("spec goto", "", end.Add(500*time.Microsecond), at(2)),
		a.cmd("spec goto", "", at(2).Add(time.Millisecond), at(3)),
	))[0]
	got := gaps(r)
	require.Len(t, got, 1)
	require.Equal(t, at(2), got[0].from)
	require.Equal(t, time.Millisecond, got[0].to.Sub(got[0].from))
}
