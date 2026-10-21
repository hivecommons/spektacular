package goal

import (
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

var threeCriteria = Goal{Item: "t", Criteria: []string{"one", "two", "three"}}

func TestValidateReply_AcceptsOneVerdictWithEvidencePerCriterion(t *testing.T) {
	for name, raw := range map[string]string{
		"object":        `{"verdicts":[{"criterion":3,"verdict":"impossible","evidence":"contradicts 1"},{"criterion":1,"verdict":"met","evidence":" a.go:1 "},{"criterion":2,"verdict":"not_met","evidence":"no README line"}]}`,
		"bare array":    `[{"criterion":1,"verdict":"met","evidence":"a.go:1"},{"criterion":2,"verdict":"not_met","evidence":"no README line"},{"criterion":3,"verdict":"impossible","evidence":"contradicts 1"}]`,
		"fenced object": "```json\n{\"verdicts\":[{\"criterion\":1,\"verdict\":\"met\",\"evidence\":\"a.go:1\"},{\"criterion\":2,\"verdict\":\"not_met\",\"evidence\":\"no README line\"},{\"criterion\":3,\"verdict\":\"impossible\",\"evidence\":\"contradicts 1\"}]}\n```",
	} {
		t.Run(name, func(t *testing.T) {
			v, err := ValidateReply(threeCriteria, []byte(raw))
			require.NoError(t, err)
			require.Equal(t, []Verdict{
				{1, VerdictMet, "a.go:1"},
				{2, VerdictNotMet, "no README line"},
				{3, VerdictImpossible, "contradicts 1"},
			}, v, "verdicts come back in criterion order with trimmed evidence")
		})
	}
}

func TestValidateReply_RefusesEveryMalformedReply(t *testing.T) {
	cases := map[string]struct {
		raw  string
		want []string
	}{
		"not json":           {`the work looks fine`, []string{"not valid JSON"}},
		"missing":            {`{"verdicts":[{"criterion":1,"verdict":"met","evidence":"x"},{"criterion":2,"verdict":"met","evidence":"x"}]}`, []string{"criterion 3 has no verdict"}},
		"duplicate":          {`{"verdicts":[{"criterion":1,"verdict":"met","evidence":"x"},{"criterion":1,"verdict":"met","evidence":"x"},{"criterion":2,"verdict":"met","evidence":"x"},{"criterion":3,"verdict":"met","evidence":"x"}]}`, []string{"criterion 1 has more than one verdict"}},
		"out of range":       {`{"verdicts":[{"criterion":0,"verdict":"met","evidence":"x"},{"criterion":4,"verdict":"met","evidence":"x"},{"criterion":1,"verdict":"met","evidence":"x"},{"criterion":2,"verdict":"met","evidence":"x"},{"criterion":3,"verdict":"met","evidence":"x"}]}`, []string{"names criterion 0", "names criterion 4"}},
		"unknown verdict":    {`{"verdicts":[{"criterion":1,"verdict":"partly","evidence":"x"},{"criterion":2,"verdict":"met","evidence":"x"},{"criterion":3,"verdict":"met","evidence":"x"}]}`, []string{`criterion 1 has the unknown verdict "partly"`}},
		"empty evidence":     {`{"verdicts":[{"criterion":1,"verdict":"met","evidence":"  "},{"criterion":2,"verdict":"met"},{"criterion":3,"verdict":"met","evidence":"x"}]}`, []string{"criterion 1 has no evidence", "criterion 2 has no evidence"}},
		"no verdicts at all": {`{}`, []string{"criterion 1 has no verdict", "criterion 2 has no verdict", "criterion 3 has no verdict"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ValidateReply(threeCriteria, []byte(tc.raw))
			var oe *output.ErrorResponse
			require.ErrorAs(t, err, &oe)
			require.Equal(t, "goal_reply_invalid", oe.Code)
			for _, w := range tc.want {
				require.Contains(t, oe.Message, w)
			}
			require.Contains(t, oe.NextAction, `{"verdicts":[`, "the fix names the reply shape")
		})
	}
}

func verdicts(vs ...string) []Verdict {
	out := make([]Verdict, len(vs))
	for i, v := range vs {
		out[i] = Verdict{Criterion: i + 1, Verdict: v, Evidence: "e"}
	}
	return out
}

func record(t *testing.T, it ItemOutcome, vs []Verdict) ItemOutcome {
	t.Helper()
	c, err := NewCheck(it, threeCriteria, vs)
	require.NoError(t, err)
	it.Checks = append(it.Checks, c)
	return derive(it)
}

func TestDecisionRule_AllMetIsAccepted(t *testing.T) {
	it := record(t, ItemOutcome{Item: "t"}, verdicts(VerdictMet, VerdictMet, VerdictMet))
	last := it.Checks[0]
	require.Equal(t, CheckAccept, last.Decision)
	require.Equal(t, 1, last.Round)
	require.Equal(t, 1, last.Attempt)
	require.Equal(t, StateAccepted, it.State)
	require.True(t, Settled(it))
	require.Equal(t, "update_plan", NextStep(last.Decision))
}

func TestDecisionRule_ImpossibleStopsOnTheFirstCheckWithNoRetry(t *testing.T) {
	it := record(t, ItemOutcome{Item: "t"}, verdicts(VerdictNotMet, VerdictImpossible, VerdictMet))
	require.Equal(t, CheckImpossible, it.Checks[0].Decision, "impossible wins over not met")
	require.Equal(t, 1, it.Attempts, "no retry is counted")
	require.Equal(t, StateAwaitingDecision, it.State)
	require.False(t, Settled(it))
	require.Equal(t, "check_goal", NextStep(CheckImpossible))

	_, _, err := NextAttempt(it)
	var oe *output.ErrorResponse
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_decision_due", oe.Code, "no further check until the user decides")
}

func TestDecisionRule_NotMetIsRetriedTwiceThenExhausted(t *testing.T) {
	it := ItemOutcome{Item: "t"}
	notMet := verdicts(VerdictMet, VerdictNotMet, VerdictMet)

	it = record(t, it, notMet)
	require.Equal(t, CheckRetry, it.Checks[0].Decision)
	require.Equal(t, 2, RetriesLeft(it.Checks[0]))
	require.Equal(t, StateRetrying, it.State)
	require.Equal(t, "implement", NextStep(CheckRetry))
	require.Equal(t, []Verdict{{2, VerdictNotMet, "e"}}, Shortfall(it))

	it = record(t, it, notMet)
	require.Equal(t, CheckRetry, it.Checks[1].Decision)
	require.Equal(t, 2, it.Checks[1].Attempt)
	require.Equal(t, 1, RetriesLeft(it.Checks[1]))

	it = record(t, it, notMet)
	require.Equal(t, CheckExhausted, it.Checks[2].Decision, "two retries after the first check, then stop")
	require.Equal(t, 3, it.Checks[2].Attempt)
	require.Equal(t, 0, RetriesLeft(it.Checks[2]))
	require.Equal(t, 3, it.Attempts)
	require.Equal(t, StateAwaitingDecision, it.State)
	require.Equal(t, "check_goal", NextStep(CheckExhausted))
}

func exhausted(t *testing.T) ItemOutcome {
	it := ItemOutcome{Item: "t"}
	for range 3 {
		it = record(t, it, verdicts(VerdictMet, VerdictNotMet, VerdictMet))
	}
	require.Equal(t, StateAwaitingDecision, it.State)
	return it
}

func TestDecisions_KeepFixingOpensANewRoundWithTwoMoreRetries(t *testing.T) {
	it, next, err := ApplyDecision(exhausted(t), Decision{Decision: DecideKeepFixing, Reason: "almost there"})
	require.NoError(t, err)
	require.Equal(t, "implement", next)
	require.Equal(t, StateRetrying, it.State)
	require.Equal(t, Decision{Round: 1, Decision: DecideKeepFixing, Reason: "almost there"}, it.Decisions[0])

	notMet := verdicts(VerdictMet, VerdictNotMet, VerdictMet)
	it = record(t, it, notMet)
	require.Equal(t, 2, it.Checks[3].Round)
	require.Equal(t, 1, it.Checks[3].Attempt)
	require.Equal(t, CheckRetry, it.Checks[3].Decision)
	it = record(t, it, notMet)
	require.Equal(t, CheckRetry, it.Checks[4].Decision)
	it = record(t, it, notMet)
	require.Equal(t, CheckExhausted, it.Checks[5].Decision, "the new round has its own two retries")
	require.Equal(t, 6, it.Attempts)
}

func TestDecisions_RecheckRunsTheCheckAgainInANewRound(t *testing.T) {
	stopped := record(t, ItemOutcome{Item: "t"}, verdicts(VerdictMet, VerdictImpossible, VerdictMet))
	it, next, err := ApplyDecision(stopped, Decision{Decision: DecideRecheck})
	require.NoError(t, err)
	require.Equal(t, "check_goal", next)
	require.Equal(t, StateRetrying, it.State)

	it = record(t, it, verdicts(VerdictMet, VerdictMet, VerdictMet))
	require.Equal(t, 2, it.Checks[1].Round)
	require.Equal(t, StateAccepted, it.State)
}

func TestDecisions_AcceptAndSkipSettleTheItem(t *testing.T) {
	accepted, next, err := ApplyDecision(exhausted(t), Decision{Decision: DecideAccept})
	require.NoError(t, err)
	require.Equal(t, "update_plan", next)
	require.Equal(t, StateAcceptedShortfall, accepted.State)
	require.True(t, Settled(accepted))
	require.Equal(t, []int{1, 3}, MetCriteria(accepted), "only the criteria judged met")

	skipped, next, err := ApplyDecision(exhausted(t), Decision{Decision: DecideSkip})
	require.NoError(t, err)
	require.Equal(t, "update_plan", next)
	require.Equal(t, StateSkipped, skipped.State)
	require.True(t, Settled(skipped))
}

func TestDecisions_RefusedWhenNotDueOrUnknown(t *testing.T) {
	var oe *output.ErrorResponse

	_, _, err := ApplyDecision(ItemOutcome{Item: "t"}, Decision{Decision: DecideAccept})
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_decision_not_due", oe.Code)
	require.Contains(t, oe.NextAction, "implement goal show")

	retrying := record(t, ItemOutcome{Item: "t"}, verdicts(VerdictNotMet, VerdictMet, VerdictMet))
	_, _, err = ApplyDecision(retrying, Decision{Decision: DecideSkip})
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_decision_not_due", oe.Code, "a retry still in budget is not a stop")

	_, _, err = ApplyDecision(exhausted(t), Decision{Decision: "give_up"})
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_decision_invalid", oe.Code)
	require.Contains(t, oe.NextAction, "keep_fixing, accept, skip, recheck")
}

func TestState_PendingWithoutAnyCheck(t *testing.T) {
	require.Equal(t, StatePending, State(ItemOutcome{}))
	require.Equal(t, []int{}, MetCriteria(ItemOutcome{}))
	require.Nil(t, Shortfall(ItemOutcome{}))
}
