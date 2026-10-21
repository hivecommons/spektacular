package goal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hivecommons/spektacular/internal/output"
)

// reply is the reviewer's verdict list exactly as it returns it.
type reply struct {
	Verdicts []Verdict `json:"verdicts"`
}

// ValidateReply reads a reviewer's reply for g and returns its verdicts in
// criterion order. A reply is valid only when it holds exactly one verdict per
// criterion, each one of met, not_met or impossible and each with non-empty
// evidence. The reply is `{"verdicts":[...]}`; a bare verdict array, or a
// reply wrapped in one Markdown code fence, is read the same way. Every
// problem found is named in the refusal.
func ValidateReply(g Goal, raw []byte) ([]Verdict, error) {
	body := unfence(bytes.TrimSpace(raw))
	var r reply
	var err error
	if bytes.HasPrefix(body, []byte("[")) {
		err = json.Unmarshal(body, &r.Verdicts)
	} else {
		err = json.Unmarshal(body, &r)
	}
	if err != nil {
		return nil, replyInvalid([]string{fmt.Sprintf("it is not valid JSON (%v)", err)})
	}

	n := len(g.Criteria)
	var problems []string
	seen := map[int]bool{}
	for i, v := range r.Verdicts {
		switch {
		case v.Criterion < 1 || v.Criterion > n:
			problems = append(problems, fmt.Sprintf("verdict %d names criterion %d, but the goal has criteria 1 to %d", i+1, v.Criterion, n))
			continue
		case seen[v.Criterion]:
			problems = append(problems, fmt.Sprintf("criterion %d has more than one verdict", v.Criterion))
			continue
		}
		seen[v.Criterion] = true
		if !slices.Contains([]string{VerdictMet, VerdictNotMet, VerdictImpossible}, v.Verdict) {
			problems = append(problems, fmt.Sprintf("criterion %d has the unknown verdict %q (use met, not_met or impossible)", v.Criterion, v.Verdict))
		}
		if strings.TrimSpace(v.Evidence) == "" {
			problems = append(problems, fmt.Sprintf("criterion %d has no evidence", v.Criterion))
		}
	}
	for c := 1; c <= n; c++ {
		if !seen[c] {
			problems = append(problems, fmt.Sprintf("criterion %d has no verdict", c))
		}
	}
	if len(problems) > 0 {
		return nil, replyInvalid(problems)
	}

	out := slices.Clone(r.Verdicts)
	sort.Slice(out, func(i, j int) bool { return out[i].Criterion < out[j].Criterion })
	for i := range out {
		out[i].Evidence = strings.TrimSpace(out[i].Evidence)
	}
	return out, nil
}

// unfence strips one surrounding Markdown code fence (``` or ```json).
func unfence(b []byte) []byte {
	if !bytes.HasPrefix(b, []byte("```")) || !bytes.HasSuffix(b, []byte("```")) || len(b) < 6 {
		return b
	}
	inner := b[3 : len(b)-3]
	if nl := bytes.IndexByte(inner, '\n'); nl >= 0 {
		inner = inner[nl+1:]
	}
	return bytes.TrimSpace(inner)
}

func replyInvalid(problems []string) error {
	return output.NewError("goal_reply_invalid",
		"the reviewer's reply cannot be recorded: "+strings.Join(problems, "; ")).
		WithNextAction(`ask the reviewer again for exactly one verdict per criterion, as {"verdicts":[{"criterion":1,"verdict":"met|not_met|impossible","evidence":"<where it looked, or why>"}]}, stage its reply unchanged and record it again`)
}

// Decide applies the decision rule to one check's verdicts at the given
// attempt of its round: any impossible criterion stops at once; all met is
// accepted; anything not yet met is retried while the round has retries left,
// and otherwise stops as exhausted.
func Decide(verdicts []Verdict, attempt int) string {
	allMet := true
	for _, v := range verdicts {
		switch v.Verdict {
		case VerdictImpossible:
			return CheckImpossible
		case VerdictNotMet:
			allMet = false
		}
	}
	switch {
	case allMet:
		return CheckAccept
	case attempt < 1+MaxRetries:
		return CheckRetry
	default:
		return CheckExhausted
	}
}

// NextAttempt is the round and attempt the item's next check counts as. A
// retry continues its round; a keep_fixing or recheck decision, or a check of
// an item that was already settled, opens a new round. An item awaiting the
// user's decision cannot be checked again until the decision is recorded.
func NextAttempt(it ItemOutcome) (round, attempt int, err error) {
	it = derive(it)
	if len(it.Checks) == 0 {
		return 1, 1, nil
	}
	last := it.Checks[len(it.Checks)-1]
	switch {
	case it.State == StateAwaitingDecision:
		return 0, 0, output.NewError("goal_decision_due",
			fmt.Sprintf("the goal check of %s stopped (%s) and is waiting for the user's decision", it.Item, last.Decision)).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf(`record the user's answer with spektacular implement goal decide --data '{"name":"<plan>","item":"%s","decision":"keep_fixing|accept|skip|recheck"}'`, it.Item))
	case last.Decision == CheckRetry:
		return last.Round, last.Attempt + 1, nil
	default:
		return last.Round + 1, 1, nil
	}
}

// NewCheck builds the next check of it from a validated reply: its round and
// attempt, and the decision the rule takes.
func NewCheck(it ItemOutcome, g Goal, verdicts []Verdict) (Check, error) {
	round, attempt, err := NextAttempt(it)
	if err != nil {
		return Check{}, err
	}
	return Check{
		Round:    round,
		Attempt:  attempt,
		Goal:     g,
		Verdicts: verdicts,
		Decision: Decide(verdicts, attempt),
	}, nil
}

// RetriesLeft is how many more not-yet-met checks the item's current round
// allows before it stops as exhausted.
func RetriesLeft(c Check) int {
	if left := 1 + MaxRetries - c.Attempt; left > 0 {
		return left
	}
	return 0
}

// NextStep is the implement step a check's decision leads to: update_plan
// when accepted, implement for a retry, and check_goal (where the run stays to
// ask the user) when it stops.
func NextStep(checkDecision string) string {
	switch checkDecision {
	case CheckAccept:
		return "update_plan"
	case CheckRetry:
		return "implement"
	default:
		return "check_goal"
	}
}

// ApplyDecision records the user's answer to a stopped check and returns the
// updated outcome and the implement step it leads to: keep_fixing opens a new
// round back at implement, recheck opens one at check_goal, and accept or skip
// settle the item and go on to update_plan.
func ApplyDecision(it ItemOutcome, d Decision) (ItemOutcome, string, error) {
	if !slices.Contains(UserDecisions, d.Decision) {
		return it, "", output.NewError("goal_decision_invalid",
			fmt.Sprintf("%q is not a goal-check decision; use one of %s", d.Decision, strings.Join(UserDecisions, ", "))).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf(`record one of %s with spektacular implement goal decide --data '{"name":"<plan>","item":"%s","decision":"<decision>"}'`, strings.Join(UserDecisions, ", "), it.Item))
	}
	it = derive(it)
	if it.State != StateAwaitingDecision {
		return it, "", output.NewError("goal_decision_not_due",
			fmt.Sprintf("the goal check of %s is %s, not waiting for a decision", it.Item, strings.ReplaceAll(it.State, "_", " "))).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf("read the item's outcome with `spektacular implement goal show <plan> --item %s` and carry on with the step the run is at; a decision is recorded only after a check stops the run", it.Item))
	}
	d.Round = it.Checks[len(it.Checks)-1].Round
	it.Decisions = append(it.Decisions, d)
	it = derive(it)
	switch d.Decision {
	case DecideKeepFixing:
		return it, "implement", nil
	case DecideRecheck:
		return it, "check_goal", nil
	default:
		return it, "update_plan", nil
	}
}

// State derives an item's state from its latest check and the user's decision
// on that check's round.
func State(it ItemOutcome) string {
	if len(it.Checks) == 0 {
		return StatePending
	}
	last := it.Checks[len(it.Checks)-1]
	switch last.Decision {
	case CheckAccept:
		return StateAccepted
	case CheckRetry:
		return StateRetrying
	}
	for i := len(it.Decisions) - 1; i >= 0; i-- {
		d := it.Decisions[i]
		if d.Round != last.Round {
			continue
		}
		switch d.Decision {
		case DecideKeepFixing, DecideRecheck:
			return StateRetrying
		case DecideAccept:
			return StateAcceptedShortfall
		case DecideSkip:
			return StateSkipped
		}
	}
	return StateAwaitingDecision
}

// Settled reports whether an item's goal check lets the plan be updated:
// accepted, accepted with a shortfall, or skipped.
func Settled(it ItemOutcome) bool {
	switch State(it) {
	case StateAccepted, StateAcceptedShortfall, StateSkipped:
		return true
	}
	return false
}

// MetCriteria are the 1-based criteria the item's latest check judged met.
func MetCriteria(it ItemOutcome) []int {
	return criteriaWith(it, VerdictMet)
}

// Shortfall are the latest check's verdicts that are not met, in criterion
// order: what a retry must fix, or what the user is asked about.
func Shortfall(it ItemOutcome) []Verdict {
	if len(it.Checks) == 0 {
		return nil
	}
	var out []Verdict
	for _, v := range it.Checks[len(it.Checks)-1].Verdicts {
		if v.Verdict != VerdictMet {
			out = append(out, v)
		}
	}
	return out
}

// CriteriaWith are the 1-based criteria of a check carrying the given verdict.
func CriteriaWith(c Check, verdict string) []int {
	out := []int{}
	for _, v := range c.Verdicts {
		if v.Verdict == verdict {
			out = append(out, v.Criterion)
		}
	}
	return out
}

func criteriaWith(it ItemOutcome, verdict string) []int {
	if len(it.Checks) == 0 {
		return []int{}
	}
	return CriteriaWith(it.Checks[len(it.Checks)-1], verdict)
}

// derive fills an item's derived fields: its attempt count and state.
func derive(it ItemOutcome) ItemOutcome {
	if it.Checks == nil {
		it.Checks = []Check{}
	}
	if it.Decisions == nil {
		it.Decisions = []Decision{}
	}
	it.Attempts = len(it.Checks)
	it.State = State(it)
	return it
}
