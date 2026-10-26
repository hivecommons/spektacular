package runstats

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/stretchr/testify/require"
)

func TestTokensOverheadKnown(t *testing.T) {
	// Claude Code counted 7/8/9/10 opus tokens and 1/2/3/4 haiku tokens
	// more than the transcripts record.
	s := tokSpecSession()
	s.Totals = map[string]agentusage.Usage{
		"opus":  use(217, 29, 2109, 21010),
		"haiku": use(1, 2, 3, 4),
	}
	rep, sec, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}})

	require.Nil(t, sec.SessionOverheadReason)
	require.Equal(t, use(8, 10, 12, 14), sums(t, sec.SessionOverhead))
	require.Nil(t, sec.SessionOverhead.WorkingMSPer1kOutput, "overhead has no time of its own")
	require.Equal(t, use(218, 31, 2112, 21014), sums(t, sec.Total), "the run total equals Claude Code's totals")
	require.Equal(t, use(18, 11, 112, 1014), sums(t, sec.OutsideSteps), "overhead is outside any step")
	require.Equal(t, use(90, 9, 900, 9000), sums(t, rows.steps["overview"]), "steps are unchanged")
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
	require.Equal(t, []ModelTokens{
		{Model: "haiku", Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4},
		{Model: "opus", Input: 217, Output: 29, CacheRead: 2109, CacheWrite: 21010},
	}, sec.Models)
}

func TestTokensOverheadCountsHelpers(t *testing.T) {
	// The plan session: its main transcript (3/3) and a counted helper
	// (300/30), and totals one token of each kind higher.
	run := tokPlan(t, helperSpan("h1", "research", 2, 7))
	s := planSession(
		[]agentusage.Launch{launch("toolu_1", "research", "Research.", 2.005)},
		helperT("agent-1", "toolu_1", "", "research", 2.01, 7, req(3, "opus", use(100, 10, 0, 0)), req(6.5, "opus", use(200, 20, 0, 0))),
	)
	s.Totals = map[string]agentusage.Usage{"opus": use(304, 34, 1, 1)}
	_, sec, _ := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Nil(t, sec.SessionOverheadReason)
	require.Equal(t, use(1, 1, 1, 1), sums(t, sec.SessionOverhead))
	require.Equal(t, use(304, 34, 1, 1), sums(t, sec.Total))
}

func TestTokensOverheadResumed(t *testing.T) {
	// Each of the two sessions that drove the run has its own overhead.
	rs := tokSpecRequests()
	first := &agentusage.Session{ID: "sess-a", Totals: map[string]agentusage.Usage{"opus": use(101, 11, 1001, 10001)},
		Transcripts: []agentusage.Transcript{{
			ID: "main", Start: at(0), End: at(5.2),
			Calls:    []agentusage.ShellCall{shell(specNewCmd, 1, 1.1)},
			Requests: rs[:4],
		}}}
	second := &agentusage.Session{ID: "sess-b", Totals: map[string]agentusage.Usage{"opus": use(112, 13, 1102, 11002)},
		Transcripts: []agentusage.Transcript{{
			ID: "main", Start: at(5.5), End: at(11),
			Calls:    []agentusage.ShellCall{shell(specGotoFinished, 9.9, 10)},
			Requests: rs[4:],
		}}}
	_, sec, _ := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{first, second}})
	require.Equal(t, use(3, 3, 3, 3), sums(t, sec.SessionOverhead))
	require.Equal(t, use(213, 24, 2103, 21003), sums(t, sec.Total))
}

func TestTokensOverheadUnknown(t *testing.T) {
	cases := []struct {
		name   string
		totals map[string]agentusage.Usage
		reason string
	}{
		{name: "no_totals", totals: nil, reason: OverheadNoTotals},
		{name: "totals_below_transcript", totals: map[string]agentusage.Usage{"opus": use(209, 21, 2100, 21000)},
			reason: OverheadBelowTranscript},
		{name: "model_missing_from_totals", totals: map[string]agentusage.Usage{"haiku": use(1000, 1000, 1000, 1000)},
			reason: OverheadBelowTranscript},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := tokSpecSession()
			s.Totals = tc.totals
			_, sec, _ := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}})
			require.Nil(t, sec.SessionOverhead)
			require.Equal(t, str(tc.reason), sec.SessionOverheadReason)
			require.Equal(t, use(210, 21, 2100, 21000), sums(t, sec.Total), "the transcripts' requests alone")
			require.Equal(t, use(10, 1, 100, 1000), sums(t, sec.OutsideSteps))
		})
	}
}

func TestTokensOverheadSeveralRuns(t *testing.T) {
	// The plan session holds a helper that drove an implement run: the
	// totals cover both runs, so neither knows its overhead.
	plan, impl, s := childDriver(t)
	in := &TokenInput{Sessions: []*agentusage.Session{s}, AllRuns: []*Run{plan, impl}}

	_, sec, _ := tokensOf(plan, in)
	require.Nil(t, sec.SessionOverhead)
	require.Equal(t, str(OverheadSeveralRuns), sec.SessionOverheadReason)

	_, sec, _ = tokensOf(impl, in)
	require.Nil(t, sec.SessionOverhead)
	require.Equal(t, str(OverheadSeveralRuns), sec.SessionOverheadReason)
}

func TestTokensOverheadUncountedTranscript(t *testing.T) {
	// A helper transcript no owned launch started counts for no run: the
	// totals cannot be split.
	s := tokSpecSession()
	s.Transcripts = append(s.Transcripts, helperT("agent-x", "toolu_x", "", "stray", 2, 3, req(2.5, "opus", use(1, 1, 1, 1))))
	s.Totals = map[string]agentusage.Usage{"opus": use(1000, 1000, 10000, 100000)}
	_, sec, _ := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Nil(t, sec.SessionOverhead)
	require.Equal(t, str(OverheadSeveralRuns), sec.SessionOverheadReason)
	require.Equal(t, use(210, 21, 2100, 21000), sums(t, sec.Total))
}

// A transcript that drove two runs makes the overhead unknown for both, so
// its tokens are never counted twice.
func TestTokensOverheadOneTranscriptTwoRuns(t *testing.T) {
	spec, plan, in := twoRuns(t)
	in.Sessions[0].Totals = map[string]agentusage.Usage{"opus": {Input: 41}}

	for _, r := range []*Run{spec, plan} {
		_, sec, _ := tokensOf(r, in)
		require.Nil(t, sec.SessionOverhead, r.Kind)
		require.Equal(t, str(OverheadSeveralRuns), sec.SessionOverheadReason, r.Kind)
	}
}
