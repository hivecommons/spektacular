package goal

import (
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// taskPlan has two tasks in different repos; the expected goals below are
// written out by hand from it and taskContext.
const taskPlan = `# Plan: demo

## Milestones & Tasks

### Milestone 1: One

#### - [ ] Task: Add the reader
**Id:** aaaa-1
**Repo:** spektacular
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] reads criteria
- [x] keeps their order
- [ ] **Spec style**
  with a detail line

#### - [ ] Task: Document the reader
**Id:** bbbb-2
**Repo:** docs
**Depends on:** none
**Execution:** agent

**Acceptance criteria**:
- [ ] the docs page mentions it
`

const taskContext = "# Context\n\n" +
	"### Task: Add the reader\n\n" +
	"- `internal/plantask/plantask.go:73` — the reader\n" +
	"- `docs:site/reader.md` — a docs link\n\n" +
	"### Task: Document the reader\n\n" +
	"- `site/page.md` — the page\n"

const legacyPlan = `# Plan: old

## Milestones & Phases

### Milestone 1: First

#### - [x] Phase 1.1: Done already

**Acceptance criteria**:
- [x] old work

#### - [ ] Phase 1.2: Do the thing

**Acceptance criteria**:
- [ ] the thing is done
- [ ] the thing is tested

#### - [ ] Phase 1.3: Then the next
`

const legacyContext = "# Context\n\n### Phase 1.2: Do the thing\n\n- `cmd/thing.go` — the thing\n"

func TestBuild_TaskGoalHoldsOnlyThatTasksCriteriaAndScope(t *testing.T) {
	repos := []string{"spektacular", "docs"}
	g, err := Build([]byte(taskPlan), []byte(taskContext), "aaaa-1", repos)
	require.NoError(t, err)
	require.Equal(t, Goal{
		Item:     "aaaa-1",
		Title:    "Add the reader",
		Criteria: []string{"reads criteria", "keeps their order", "**Spec style** with a detail line"},
		Scope: Scope{
			Repo: "spektacular",
			Files: []plantask.FileRef{
				{Repo: "spektacular", Path: "internal/plantask/plantask.go"},
				{Repo: "docs", Path: "site/reader.md"},
			},
			Rule: ScopeRule,
		},
	}, g)
	require.NotContains(t, g.Criteria, "the docs page mentions it", "nothing from another task")

	other, err := Build([]byte(taskPlan), []byte(taskContext), "bbbb-2", repos)
	require.NoError(t, err)
	require.Equal(t, []string{"the docs page mentions it"}, other.Criteria)
	require.Equal(t, "docs", other.Scope.Repo)
	require.Equal(t, []plantask.FileRef{{Repo: "docs", Path: "site/page.md"}}, other.Scope.Files)
}

func TestBuild_LegacyPhaseGoalIsBuiltTheSameWay(t *testing.T) {
	g, err := Build([]byte(legacyPlan), []byte(legacyContext), "Phase 1.2", []string{"app"})
	require.NoError(t, err)
	require.Equal(t, Goal{
		Item:     "Phase 1.2",
		Title:    "Do the thing",
		Criteria: []string{"the thing is done", "the thing is tested"},
		Scope: Scope{
			Repo:  "app",
			Files: []plantask.FileRef{{Repo: "app", Path: "cmd/thing.go"}},
			Rule:  ScopeRule,
		},
	}, g)

	empty, err := Build([]byte(legacyPlan), nil, "Phase 1.3", []string{"app"})
	require.NoError(t, err)
	require.Equal(t, []string{}, empty.Criteria)
	require.Equal(t, []plantask.FileRef{}, empty.Scope.Files, "files are an empty list, never null")
}

func TestBuild_UnknownItemIsRefusedWithAFix(t *testing.T) {
	_, err := Build([]byte(taskPlan), nil, "nope", nil)
	var oe *output.ErrorResponse
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_item_not_found", oe.Code)
	require.Contains(t, oe.NextAction, "plan file read")
}

func TestCurrent_SelectedTaskThenFirstOpenUnskippedItem(t *testing.T) {
	p := plantask.Parse([]byte(taskPlan))
	empty := Outcome{Plan: "demo"}

	item, ok := Current(p, empty, "bbbb-2")
	require.True(t, ok)
	require.Equal(t, "bbbb-2", item, "a single-task run's selected task wins")

	item, ok = Current(p, empty, "")
	require.True(t, ok)
	require.Equal(t, "aaaa-1", item)
	require.Equal(t, 2, OpenUnskipped(p, empty))

	skipped := Outcome{Plan: "demo"}
	skipped.put(skippedItem("aaaa-1"))
	item, ok = Current(p, skipped, "")
	require.True(t, ok)
	require.Equal(t, "bbbb-2", item, "a skipped task is passed over")
	require.Equal(t, 1, OpenUnskipped(p, skipped))

	skipped.put(skippedItem("bbbb-2"))
	_, ok = Current(p, skipped, "")
	require.False(t, ok)
}

func TestCurrent_LegacyPlanUsesOpenPhases(t *testing.T) {
	p := plantask.Parse([]byte(legacyPlan))
	item, ok := Current(p, Outcome{}, "")
	require.True(t, ok)
	require.Equal(t, "Phase 1.2", item, "a ticked phase is not current")

	o := Outcome{}
	o.put(skippedItem("Phase 1.2"))
	item, ok = Current(p, o, "")
	require.True(t, ok)
	require.Equal(t, "Phase 1.3", item)
}

func skippedItem(key string) ItemOutcome {
	return ItemOutcome{
		Item:      key,
		Checks:    []Check{{Round: 1, Attempt: 3, Decision: CheckExhausted}},
		Decisions: []Decision{{Round: 1, Decision: DecideSkip}},
	}
}

func TestStore_RoundTripsAndDerivesAttemptsAndState(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	const planDir = ".spektacular/plans"

	o, err := Load(st, planDir, "demo")
	require.NoError(t, err)
	require.Equal(t, Outcome{Plan: "demo", Items: []ItemOutcome{}}, o, "a plan never checked has an empty record")
	require.Equal(t, ".spektacular/plans/demo/goals.json", FilePath(planDir, "demo"))

	g := Goal{Item: "aaaa-1", Title: "Add the reader", Criteria: []string{"a", "b"}, Scope: Scope{Repo: "spektacular", Files: []plantask.FileRef{}, Rule: ScopeRule}}
	now := time.Date(2026, 10, 21, 9, 30, 0, 0, time.FixedZone("x", 3600))

	first, err := NewCheck(o.Item("aaaa-1"), g, []Verdict{{1, VerdictMet, "plan.go:1"}, {2, VerdictNotMet, "nothing in README"}})
	require.NoError(t, err)
	it, err := AppendCheck(st, planDir, "demo", first, now)
	require.NoError(t, err)
	require.Equal(t, 1, it.Attempts)
	require.Equal(t, StateRetrying, it.State)
	require.Equal(t, "2026-10-21T08:30:00Z", it.Checks[0].At, "stamped in UTC")

	o, err = Load(st, planDir, "demo")
	require.NoError(t, err)
	second, err := NewCheck(o.Item("aaaa-1"), g, []Verdict{{1, VerdictMet, "plan.go:1"}, {2, VerdictMet, "README.md:4"}})
	require.NoError(t, err)
	require.Equal(t, 2, second.Attempt)
	_, err = AppendCheck(st, planDir, "demo", second, now)
	require.NoError(t, err)

	raw, err := st.Read(FilePath(planDir, "demo"))
	require.NoError(t, err)
	// A tampered derived field on disk is never trusted.
	require.NoError(t, st.Write(FilePath(planDir, "demo"), []byte(
		replaceOnce(t, string(raw), `"state": "accepted"`, `"state": "skipped"`))))

	o, err = Load(st, planDir, "demo")
	require.NoError(t, err)
	require.Len(t, o.Items, 1)
	got := o.Items[0]
	require.Equal(t, "Add the reader", got.Title)
	require.Equal(t, 2, got.Attempts)
	require.Equal(t, StateAccepted, got.State)
	require.Equal(t, g, got.Checks[1].Goal, "every check keeps its goal snapshot")
	require.Equal(t, "README.md:4", got.Checks[1].Verdicts[1].Evidence)
	require.Equal(t, []int{1, 2}, MetCriteria(got))
}

func TestStore_AppendDecisionIsRefusedWhenNotDue(t *testing.T) {
	st := store.NewFileStore(t.TempDir(), "project")
	_, _, err := AppendDecision(st, "plans", "demo", "aaaa-1", Decision{Decision: DecideAccept}, time.Now())
	var oe *output.ErrorResponse
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_decision_not_due", oe.Code)
	require.False(t, st.Exists(FilePath("plans", "demo")), "a refused decision writes nothing")
}

func replaceOnce(t *testing.T, s, old, new string) string {
	t.Helper()
	i := strings.Index(s, old)
	require.GreaterOrEqual(t, i, 0, "%q not found", old)
	return s[:i] + new + s[i+len(old):]
}
