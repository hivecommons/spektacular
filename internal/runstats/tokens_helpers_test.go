package runstats

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/hivecommons/spektacular/internal/tracelog"
	"github.com/stretchr/testify/require"
)

func TestTokensLateHelper(t *testing.T) {
	// research is launched in discovery and its transcript runs to 09:07,
	// after architecture began at 09:06, with a request either side.
	run := tokPlan(t, helperSpan("h1", "research", 2, 7))
	s := planSession(
		[]agentusage.Launch{launch("toolu_1", "research", "Research the codebase.", 2.005)},
		helperT("agent-1", "toolu_1", "", "research", 2.01, 7, req(3, "opus", use(100, 10, 0, 0)), req(6.5, "opus", use(200, 20, 0, 0))),
	)
	rep, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Equal(t, use(300, 30, 0, 0), sums(t, rows.helpers["h1"]), "all its usage on its launch")
	require.Equal(t, use(301, 31, 0, 0), sums(t, rows.steps["discovery"]), "and in the step it was launched in")
	require.Equal(t, use(2, 2, 0, 0), sums(t, rows.steps["architecture"]), "none in the next step")
	require.Equal(t, use(303, 33, 0, 0), sums(t, sec.Total))
	require.Empty(t, sec.UnmatchedHelpers)
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
}

func TestTokensHelperMatching(t *testing.T) {
	const b1, b2 = "00000000000000b1", "00000000000000b2"
	const c0, c1, c2 = "00000000000000c0", "00000000000000c1", "00000000000000c2"
	brief := func(span string) string {
		return "Your parent span is " + tokPlanTrace + "-" + span + ". Do the work."
	}
	helpers := func() []agentusage.Transcript {
		return []agentusage.Transcript{
			helperT("agent-1", "toolu_1", "", "first", 3.01, 4, req(3.5, "opus", use(100, 10, 0, 0))),
			helperT("agent-2", "toolu_2", "", "second", 3.51, 4.5, req(4, "opus", use(200, 20, 0, 0))),
		}
	}
	cases := []struct {
		name     string
		spans    [][]tracelog.Record
		launches []agentusage.Launch
		want     map[string]agentusage.Usage
		none     []string // spans with no figure
	}{
		{
			// By launch order the 09:03 launch would take b2, the latest
			// span before it; its brief names b1.
			name:  "byBriefID",
			spans: [][]tracelog.Record{helperSpan(b1, "first", 2, 4), helperSpan(b2, "second", 2.5, 4.5)},
			launches: []agentusage.Launch{
				launch("toolu_1", "first", brief(b1), 3),
				launch("toolu_2", "second", brief(b2), 3.5),
			},
			want: map[string]agentusage.Usage{b1: use(100, 10, 0, 0), b2: use(200, 20, 0, 0)},
		},
		{
			// Two launches in one response take the two spans started just
			// before, in start order; the older c0 is left.
			name: "byLaunchOrder",
			spans: [][]tracelog.Record{
				helperSpan(c0, "old", 1.5, 2), helperSpan(c1, "first", 2.9, 4), helperSpan(c2, "second", 2.95, 4.5),
			},
			launches: []agentusage.Launch{
				launch("toolu_1", "first", "Do the first thing.", 3),
				launch("toolu_2", "second", "Do the second thing.", 3),
			},
			want: map[string]agentusage.Usage{c1: use(100, 10, 0, 0), c2: use(200, 20, 0, 0)},
			none: []string{c0},
		},
		{
			// A brief naming a span of another run is no match: launch
			// order decides.
			name:  "briefForAnotherRun",
			spans: [][]tracelog.Record{helperSpan(c1, "first", 2.9, 4), helperSpan(c2, "second", 2.95, 4.5)},
			launches: []agentusage.Launch{
				launch("toolu_1", "first", "Your parent span is "+tokSpecTrace+"-"+c2+".", 3),
				launch("toolu_2", "second", "Your parent span is "+tokSpecTrace+"-"+c1+".", 3),
			},
			want: map[string]agentusage.Usage{c1: use(100, 10, 0, 0), c2: use(200, 20, 0, 0)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := tokPlan(t, tc.spans...)
			_, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{planSession(tc.launches, helpers()...)}})
			require.Len(t, rows.helpers, len(tc.want))
			for id, u := range tc.want {
				require.Equal(t, u, sums(t, rows.helpers[id]), id)
			}
			for _, id := range tc.none {
				require.NotContains(t, rows.helpers, id, "a span no transcript matched has unknown tokens")
			}
			require.Empty(t, sec.UnmatchedHelpers)
			require.Equal(t, use(303, 33, 0, 0), sums(t, sec.Total))
		})
	}
}

func TestTokensFewerSpansThanLaunches(t *testing.T) {
	// Two launches in one response and one span: the later launch takes it
	// and the earlier is unmatched, still counted.
	run := tokPlan(t, helperSpan("00000000000000c1", "second", 2.9, 4))
	s := planSession(
		[]agentusage.Launch{launch("toolu_1", "first", "One.", 3), launch("toolu_2", "second", "Two.", 3)},
		helperT("agent-1", "toolu_1", "", "first", 3.01, 4, req(3.5, "opus", use(100, 10, 0, 0))),
		helperT("agent-2", "toolu_2", "", "second", 3.01, 4, req(3.5, "opus", use(200, 20, 0, 0))),
	)
	_, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Equal(t, use(200, 20, 0, 0), sums(t, rows.helpers["00000000000000c1"]))
	require.Len(t, sec.UnmatchedHelpers, 1)
	require.Equal(t, "first", sec.UnmatchedHelpers[0].Name)
	require.Equal(t, use(100, 10, 0, 0), sums(t, sec.UnmatchedHelpers[0].Tokens))
	require.Equal(t, use(303, 33, 0, 0), sums(t, sec.Total))
}

func TestTokensUnmatchedHelpers(t *testing.T) {
	// The run recorded no helper spans. One helper is launched in
	// discovery with no description of its own, one in architecture.
	run := tokPlan(t)
	s := planSession(
		[]agentusage.Launch{
			launch("toolu_1", "lint the plan", "Lint.", 8),
			launch("toolu_2", "check refs", "Check.", 3),
		},
		helperT("agent-1", "toolu_1", "", "linter", 8.01, 9.01, req(8.5, "opus", use(5, 6, 8, 8))),
		helperT("agent-2", "toolu_2", "", "", 3.01, 3.51, req(3.2, "opus", use(10, 2, 20, 4))),
	)
	rep, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}, Prices: tokPrices})

	require.Empty(t, rows.helpers)
	require.Equal(t, []UnmatchedHelper{
		{
			Name: "check refs", Step: str("discovery"), StartedAt: at(3.01), EndedAt: at(3.51), DurationMS: 30000,
			Tokens: &TokenFigure{Input: 10, Output: 2, CacheRead: 20, CacheWrite: 4,
				// (10*15 + 2*75 + 20*1.5 + 4*18.75)/1e6
				Cost: f64(0.000405), CostComplete: true, UnpricedModels: []string{}, WorkingMSPer1kOutput: i64(15000000)},
		},
		{
			Name: "linter", Step: str("architecture"), StartedAt: at(8.01), EndedAt: at(9.01), DurationMS: 60000,
			Tokens: &TokenFigure{Input: 5, Output: 6, CacheRead: 8, CacheWrite: 8,
				// (5*15 + 6*75 + 8*1.5 + 8*18.75)/1e6
				Cost: f64(0.000687), CostComplete: true, UnpricedModels: []string{}, WorkingMSPer1kOutput: i64(10000000)},
		},
	}, sec.UnmatchedHelpers, "in launch order; named by the transcript, else the launch")

	// Still counted, in the step each was launched in.
	require.Equal(t, use(11, 3, 20, 4), sums(t, rows.steps["discovery"]))
	require.Equal(t, use(7, 8, 8, 8), sums(t, rows.steps["architecture"]))
	require.Equal(t, use(18, 11, 28, 12), sums(t, sec.Total))
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
}

func TestTokensUnmatchedHelperInATask(t *testing.T) {
	// A launch in task 2 with no span takes the task's id.
	s := tokImplSession()
	s.Transcripts[0].Launches = append(s.Transcripts[0].Launches, launch("toolu_u", "stray", "Stray.", 11))
	s.Transcripts = append(s.Transcripts, helperT("agent-u", "toolu_u", "", "", 11.01, 12, req(11.5, "opus", use(1, 1, 0, 0))))
	_, sec, rows := tokensOf(tokImpl(t), &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Len(t, sec.UnmatchedHelpers, 1)
	require.Equal(t, str("2"), sec.UnmatchedHelpers[0].TaskID)
	require.Equal(t, str("tasks"), sec.UnmatchedHelpers[0].Step)
	require.Equal(t, use(301, 41, 0, 0), sums(t, rows.tasks["2"]))
}

func TestTokensDepthTwo(t *testing.T) {
	// research starts a helper of its own, which starts another: both roll
	// into research.
	run := tokPlan(t, helperSpan("h1", "research", 2, 7))
	s := planSession(
		[]agentusage.Launch{launch("toolu_1", "research", "Research.", 2.005)},
		helperT("agent-1", "toolu_1", "", "research", 2.01, 7, req(3, "opus", use(100, 10, 0, 0))),
		helperT("agent-2", "toolu_inner", "agent-1", "deeper", 3.1, 4, req(3.5, "opus", use(1000, 100, 0, 0))),
		helperT("agent-3", "toolu_innermost", "agent-2", "deepest", 3.2, 3.9, req(3.6, "opus", use(10000, 1000, 0, 0))),
	)
	_, sec, rows := tokensOf(run, &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Equal(t, use(11100, 1110, 0, 0), sums(t, rows.helpers["h1"]))
	require.Equal(t, use(11101, 1111, 0, 0), sums(t, rows.steps["discovery"]))
	require.Empty(t, sec.UnmatchedHelpers)
	require.Equal(t, use(11103, 1113, 0, 0), sums(t, sec.Total))
}

// childDriver is a plan run whose helper drove an implement run of its
// own: the plan run, the implement run, and the one session.
func childDriver(t *testing.T) (plan, impl *Run, s *agentusage.Session) {
	t.Helper()
	plan = tokPlan(t, helperSpan("h1", "implement 000003_kid", 2, 7))
	m := fx{trace: tokImplTrace, kind: "implement", name: "000003_kid", agent: "claude"}
	impl = onlyRun(t, recs(
		m.cmd("implement new", "", at(2.5), at(2.6), argv(0, "implement", "new", "--data", `{"name":"000003_kid"}`)),
		m.span("r1", "read_plan", tracelog.TypeStep, "", at(2.6), at(4), tracelog.EndMoved),
		m.span("r2", "finished", tracelog.TypeStep, "", at(4), at(4), tracelog.EndFinished),
	))
	kid := helperT("agent-1", "toolu_1", "", "implement 000003_kid", 2.01, 7, req(3, "opus", use(100, 10, 0, 0)))
	kid.Calls = []agentusage.ShellCall{shell(`spektacular implement new --data '{"name":"000003_kid"}'`, 2.5, 2.6)}
	s = planSession([]agentusage.Launch{launch("toolu_1", "implement 000003_kid", "Implement it.", 2.005)}, kid)
	s.Totals = map[string]agentusage.Usage{"opus": use(1000, 1000, 0, 0)}
	return plan, impl, s
}

func TestTokensChildDriver(t *testing.T) {
	plan, impl, s := childDriver(t)
	in := &TokenInput{Sessions: []*agentusage.Session{s}, AllRuns: []*Run{plan, impl}}

	_, sec, rows := tokensOf(plan, in)
	require.NotContains(t, rows.helpers, "h1", "the helper that drove its own run is not the plan run's")
	require.Empty(t, sec.UnmatchedHelpers)
	require.Equal(t, use(3, 3, 0, 0), sums(t, sec.Total), "only the plan run's main requests")

	_, sec, rows = tokensOf(impl, in)
	require.Equal(t, []SessionRef{{SessionID: "plan-sess", Transcript: "agent-1", File: "/sessions/subagents/agent-1.jsonl"}}, sec.Sessions)
	require.Equal(t, use(100, 10, 0, 0), sums(t, sec.Total), "counted in its own run")
	require.Equal(t, use(100, 10, 0, 0), sums(t, rows.steps["read_plan"]))
}

func TestMatchHelpersEmpty(t *testing.T) {
	require.Empty(t, matchHelpers(tokPlan(t), nil))
}
