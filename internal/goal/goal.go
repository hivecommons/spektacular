// Package goal owns the implement workflow's goal check: a plan work item's
// goal (its acceptance criteria plus its scope limits), the verdicts an
// independent reviewer returns for it, the rule that decides what happens
// next, and the per-plan outcome record that keeps every check.
//
// An item is a task, addressed by its **Id:**, or — in a plan written before
// tasks — a legacy phase, addressed by its "Phase N.M" label.
//
// The CLI verbs (`implement goal record`, `decide`, `show`) and the implement
// step callbacks both call this package, so the goal a reviewer is shown, the
// goal that is recorded and the decision that follows can never disagree.
package goal

import (
	"fmt"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
)

// MaxRetries is how many times a not-yet-met item goes back to implementing
// within one round, after the round's first check.
const MaxRetries = 2

// ScopeRule is the scope limit every goal carries beside its repo and files.
const ScopeRule = "Change only the files that belong to this item; change nothing that belongs to another task."

// Verdicts a reviewer may return for one criterion.
const (
	VerdictMet        = "met"
	VerdictNotMet     = "not_met"
	VerdictImpossible = "impossible"
)

// Decisions the CLI takes after a check.
const (
	CheckAccept     = "accept"
	CheckRetry      = "retry"
	CheckExhausted  = "exhausted"
	CheckImpossible = "impossible"
)

// Decisions the user takes after a check stops the run.
const (
	DecideKeepFixing = "keep_fixing"
	DecideAccept     = "accept"
	DecideSkip       = "skip"
	DecideRecheck    = "recheck"
)

// UserDecisions are the answers `implement goal decide` accepts, in the order
// they are offered.
var UserDecisions = []string{DecideKeepFixing, DecideAccept, DecideSkip, DecideRecheck}

// Item states, derived from an item's latest check and decision.
const (
	StatePending           = "pending"
	StateRetrying          = "retrying"
	StateAwaitingDecision  = "awaiting_decision"
	StateAccepted          = "accepted"
	StateAcceptedShortfall = "accepted_shortfall"
	StateSkipped           = "skipped"
)

// Goal is one work item's goal: its acceptance criteria, in plan order, and
// the scope limits the reviewer judges them within.
type Goal struct {
	Item     string   `json:"item"`     // task id, or "Phase N.M"
	Title    string   `json:"title"`    // the item's heading title
	Criteria []string `json:"criteria"` // in plan order; verdict i judges Criteria[i-1]
	Scope    Scope    `json:"scope"`
}

// Scope is what an item may change.
type Scope struct {
	Repo  string             `json:"repo"`  // the task's **Repo:**, or the only registered repo
	Files []plantask.FileRef `json:"files"` // from the item's context.md section; may be empty
	Rule  string             `json:"rule"`  // ScopeRule
}

// Verdict is a reviewer's judgement of one criterion.
type Verdict struct {
	Criterion int    `json:"criterion"` // 1-based index into Goal.Criteria
	Verdict   string `json:"verdict"`   // "met" | "not_met" | "impossible"
	Evidence  string `json:"evidence"`  // non-empty: where it looked, or why
}

// Check is one recorded goal check.
type Check struct {
	At            string    `json:"at"`             // RFC3339 UTC, stamped by the CLI
	Round         int       `json:"round"`          // 1-based; a keep_fixing or recheck decision opens a new round
	Attempt       int       `json:"attempt"`        // 1-based within the round; at most 1 + MaxRetries
	Goal          Goal      `json:"goal"`           // snapshot at check time
	Verdicts      []Verdict `json:"verdicts"`       // exactly len(Goal.Criteria)
	Decision      string    `json:"decision"`       // "accept" | "retry" | "exhausted" | "impossible"
	TreeUnchanged bool      `json:"tree_unchanged"` // the code roots matched the diff/status staged just before the reviewer ran
}

// Decision is the user's answer after a check stopped the run.
type Decision struct {
	At       string `json:"at"`
	Round    int    `json:"round"`    // the round whose stop it answers
	Decision string `json:"decision"` // "keep_fixing" | "accept" | "skip" | "recheck"
	Reason   string `json:"reason,omitempty"`
}

// ItemOutcome is every check and decision recorded for one item.
type ItemOutcome struct {
	Item      string     `json:"item"`
	Title     string     `json:"title"`
	Checks    []Check    `json:"checks"`
	Decisions []Decision `json:"decisions"`
	Attempts  int        `json:"attempts"` // total checks across rounds (derived on read)
	State     string     `json:"state"`    // derived on read; see the State* constants
}

// Outcome is a plan's goal-check record.
type Outcome struct {
	Plan  string        `json:"plan"`
	Items []ItemOutcome `json:"items"` // in first-checked order
}

// Item returns the recorded outcome of one item, or a pending outcome when it
// has never been checked.
func (o Outcome) Item(key string) ItemOutcome {
	for _, it := range o.Items {
		if it.Item == key {
			return it
		}
	}
	return derive(ItemOutcome{Item: key, Checks: []Check{}, Decisions: []Decision{}})
}

// put replaces the item's outcome in o, or appends it on first sight.
func (o *Outcome) put(it ItemOutcome) {
	it = derive(it)
	for i := range o.Items {
		if o.Items[i].Item == it.Item {
			o.Items[i] = it
			return
		}
	}
	o.Items = append(o.Items, it)
}

// Build reads one item's goal from a plan and its context document. item is a
// task id or, for a plan written before tasks, a "Phase N.M" label. repos are
// the registered repo names, used to attribute the item's files and as the
// repo of a legacy phase that names none.
func Build(planBody, contextBody []byte, item string, repos []string) (Goal, error) {
	p := plantask.Parse(planBody)
	if t, ok := p.Task(item); ok && item != "" {
		return Goal{
			Item:     t.ID,
			Title:    t.Title,
			Criteria: criteriaText(t.Criteria),
			Scope: Scope{
				Repo:  scopeRepo(t.Repo, repos),
				Files: files(plantask.TaskFiles(planBody, contextBody, repos)[t.Title]),
				Rule:  ScopeRule,
			},
		}, nil
	}
	if ph, ok := p.Phase(item); ok {
		return Goal{
			Item:     ph.Label,
			Title:    ph.Title,
			Criteria: criteriaText(ph.Criteria),
			Scope: Scope{
				Repo:  scopeRepo(ph.Repo, repos),
				Files: files(plantask.PhaseFiles(planBody, contextBody, repos)[ph.Label]),
				Rule:  ScopeRule,
			},
		}, nil
	}
	return Goal{}, output.NewError("goal_item_not_found",
		fmt.Sprintf("the plan has no task with id %q and no phase labelled %q", item, item)).
		WithResource(item).
		WithNextAction("read the plan's work items with `spektacular plan file read <plan> plan` and name a task by its **Id:** or a phase by its \"Phase N.M\" label")
}

func criteriaText(c plantask.Criteria) []string {
	out := make([]string, 0, len(c.Items))
	for _, it := range c.Items {
		out = append(out, it.Text)
	}
	return out
}

func scopeRepo(repo string, repos []string) string {
	if repo == "" && len(repos) == 1 {
		return repos[0]
	}
	return repo
}

func files(refs []plantask.FileRef) []plantask.FileRef {
	if refs == nil {
		return []plantask.FileRef{}
	}
	return refs
}
