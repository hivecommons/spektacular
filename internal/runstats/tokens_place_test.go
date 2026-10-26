package runstats

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/agentusage"
	"github.com/stretchr/testify/require"
)

// stepsPlusOutside is the steps' figures plus the outside figure.
func stepsPlusOutside(t *testing.T, rep Report, sec TokenSection, rows tokenRows) agentusage.Usage {
	t.Helper()
	got := sums(t, sec.OutsideSteps)
	for _, s := range rep.Steps {
		got = got.Add(sums(t, rows.steps[s.Step]))
	}
	return got
}

func TestTokensOneSession(t *testing.T) {
	rep, sec, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{tokSpecSession()}})
	require.Equal(t, TokensKnown, sec.Status)

	require.Equal(t, use(210, 21, 2100, 21000), sums(t, sec.Total), "every request, once")
	require.Equal(t, use(10, 1, 100, 1000), sums(t, sec.OutsideSteps), "the request before spec new")
	require.Equal(t, use(90, 9, 900, 9000), sums(t, rows.steps["overview"]))
	require.Equal(t, use(110, 11, 1100, 11000), sums(t, rows.steps["approach"]))
	require.Equal(t, use(0, 0, 0, 0), sums(t, rows.steps["finished"]))
	require.Len(t, rows.steps, 3, "one figure per step row")

	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows),
		"steps plus outside equal the total for each token type")
	require.Empty(t, rows.tasks)
	require.Empty(t, rows.helpers)
}

func TestTokensGotoResponse(t *testing.T) {
	// The response that issued the goto (09:04.85) came before the goto ran
	// (09:04.9): it is overview's, not approach's. A request at the step's
	// first moment is the new step's.
	s := tokSpecSession()
	s.Transcripts[0].Requests = []agentusage.Request{
		req(4.85, "opus", use(1, 0, 0, 0)),
		req(5, "opus", use(0, 1, 0, 0)),
	}
	_, _, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Equal(t, use(1, 0, 0, 0), sums(t, rows.steps["overview"]))
	require.Equal(t, use(0, 1, 0, 0), sums(t, rows.steps["approach"]))
}

func TestTokensRequestsAfterTheLastStepAreOutside(t *testing.T) {
	s := tokSpecSession()
	s.Transcripts[0].Requests = append(s.Transcripts[0].Requests, req(10.5, "opus", use(1000, 0, 0, 0)))
	rep, sec, rows := tokensOf(tokSpec(t, "claude"), &TokenInput{Sessions: []*agentusage.Session{s}})
	require.Equal(t, use(1010, 1, 100, 1000), sums(t, sec.OutsideSteps))
	require.Equal(t, use(1210, 21, 2100, 21000), sums(t, sec.Total))
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
}

func TestTokensTwoTasks(t *testing.T) {
	rep, sec, rows := tokensOf(tokImpl(t), &TokenInput{Sessions: []*agentusage.Session{tokImplSession()}, Prices: tokPrices})
	require.Len(t, rep.Tasks, 2)

	// Task 1: two requests and its helper; task 2: one request.
	require.Equal(t, use(250, 35, 1000, 0), sums(t, rows.tasks["1"]))
	require.Equal(t, use(300, 40, 0, 0), sums(t, rows.tasks["2"]))
	// (250*15 + 35*75 + 1000*1.5)/1e6 and (300*15 + 40*75)/1e6.
	require.Equal(t, f64(0.007875), rows.tasks["1"].Cost)
	require.Equal(t, f64(0.0075), rows.tasks["2"].Cost)
	require.True(t, rows.tasks["1"].CostComplete)

	// The requests before and after the tasks are in the step only.
	require.Equal(t, use(562, 87, 1000, 0), sums(t, rows.steps["tasks"]))
	require.Equal(t, use(562, 87, 1000, 0), sums(t, sec.Total))
	require.Equal(t, use(0, 0, 0, 0), sums(t, sec.OutsideSteps))
	require.Equal(t, sums(t, sec.Total), stepsPlusOutside(t, rep, sec, rows))
}

func TestTaskKey(t *testing.T) {
	require.Equal(t, "3", taskKey("3", "abc"))
	require.Equal(t, "span:abc", taskKey("", "abc"))
}

func TestLocatorStepOnlyNamesReportedSteps(t *testing.T) {
	run := tokSpec(t, "claude")
	rep := BuildReport(run, DefaultOptions)
	loc := newLocator(run, &rep)
	require.Equal(t, str("overview"), loc.stepAt(at(2)))
	require.Equal(t, str("approach"), loc.stepAt(at(5)), "a step entry holds its start")
	require.Nil(t, loc.stepAt(at(0.5)))
	require.Nil(t, loc.stepAt(at(10)), "the finished entry is empty")
	require.Nil(t, loc.step(str("nowhere")), "a step with no row is outside any step")
	require.Equal(t, str("overview"), loc.step(str("overview")))
	require.Nil(t, loc.step(nil))
	require.Nil(t, loc.taskAt(at(2)), "a spec run has no tasks")
}
