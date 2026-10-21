package implement

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/goal"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/stretchr/testify/require"
)

// goalTaskPlan has two tasks; the first has three criteria.
const goalTaskPlan = "# Plan: test\n\n## Milestones & Tasks\n\n### Milestone 1: M\n\n" +
	"#### - [ ] Task: Count items\n**Id:** t-1\n**Repo:** app\n**Depends on:** none\n**Execution:** agent\n\n" +
	"**Acceptance criteria**:\n- [ ] `todo count` prints the number of items\n- [ ] unit tests cover it\n- [ ] the README documents `todo count`\n\n" +
	"#### - [ ] Task: Another task\n**Id:** t-2\n**Repo:** app\n**Depends on:** none\n**Execution:** agent\n\n" +
	"**Acceptance criteria**:\n- [ ] other work\n"

const goalTaskContext = "# Context\n\n### Task: Count items\n\n- `todo/cli.py` — the command\n- `README.md` — the docs\n"

const goalLegacyPlan = "# Plan: test\n\n## Milestones & Phases\n\n### Milestone 1: M\n\n" +
	"#### - [ ] Phase 1.1: Old style\n\n**Acceptance criteria**:\n- [ ] it works\n- [ ] it is documented\n"

// goalFixture is a store holding a plan, and the config and data of a run
// over it with the goal check on.
type goalFixture struct {
	st   store.Store
	cfg  workflow.Config
	data map[string]any
}

func newGoalFixture(t *testing.T, plan, context string) goalFixture {
	t.Helper()
	st := store.NewFileStore(t.TempDir(), "project")
	require.NoError(t, st.Write(filepath.Join("plans", "test", "plan.md"), []byte(plan)))
	if context != "" {
		require.NoError(t, st.Write(filepath.Join("plans", "test", "context.md"), []byte(context)))
	}
	return goalFixture{
		st:   st,
		cfg:  workflow.Config{Command: "spektacular", PlanDir: "plans", GoalCheck: true, Repos: []string{"app"}},
		data: map[string]any{"name": "test"},
	}
}

func (f goalFixture) render(t *testing.T, cb workflow.StepCallback) (Result, error) {
	t.Helper()
	writer := &captureWriter{}
	_, err := cb(&testData{values: f.data}, writer, f.st, f.cfg)
	return writer.result, err
}

func (f goalFixture) instruction(t *testing.T, cb workflow.StepCallback) string {
	t.Helper()
	res, err := f.render(t, cb)
	require.NoError(t, err)
	return res.Instruction
}

// record appends a check of item with the given verdicts, as `implement goal
// record` does.
func (f goalFixture) record(t *testing.T, item string, verdicts ...string) goal.Check {
	t.Helper()
	plan, err := f.st.Read("plans/test/plan.md")
	require.NoError(t, err)
	ctx, _ := f.st.Read("plans/test/context.md")
	g, err := goal.Build(plan, ctx, item, f.cfg.Repos)
	require.NoError(t, err)
	vs := make([]goal.Verdict, len(verdicts))
	for i, v := range verdicts {
		vs[i] = goal.Verdict{Criterion: i + 1, Verdict: v, Evidence: "evidence for " + string(rune('1'+i))}
	}
	o, err := goal.Load(f.st, "plans", "test")
	require.NoError(t, err)
	c, err := goal.NewCheck(o.Item(item), g, vs)
	require.NoError(t, err)
	_, err = goal.AppendCheck(f.st, "plans", "test", c, time.Now())
	require.NoError(t, err)
	return c
}

func (f goalFixture) decide(t *testing.T, item, decision string) {
	t.Helper()
	_, _, err := goal.AppendDecision(f.st, "plans", "test", item, goal.Decision{Decision: decision}, time.Now())
	require.NoError(t, err)
}

func stepConfig(name string) workflow.StepConfig {
	for _, s := range Steps() {
		if s.Name == name {
			return s
		}
	}
	return workflow.StepConfig{}
}

func TestGoalCheckStepWiring(t *testing.T) {
	require.Equal(t, []string{"verify"}, stepConfig("check_goal").Src, "check_goal follows verification")
	require.ElementsMatch(t, []string{"analyze", "check_goal"}, stepConfig("implement").Src,
		"a not-yet-met check loops back to implement")
	require.ElementsMatch(t, []string{"verify", "check_goal"}, stepConfig("update_plan").Src,
		"update_plan follows the goal check, or verification when the check is off")
}

func TestVerifyNextStepFollowsTheGoalCheckSwitch(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, "")
	on := f.instruction(t, verify())
	require.Contains(t, on, `implement goto --data '{"step":"check_goal","name":"test"}'`)
	require.NotContains(t, on, `"step":"update_plan"`)

	f.cfg.GoalCheck = false
	off := f.instruction(t, verify())
	require.Contains(t, off, `implement goto --data '{"step":"update_plan","name":"test"}'`)
	require.NotContains(t, off, "check_goal")
}

func TestCheckGoalRendersOnlyTheCurrentTasksGoal(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	out := f.instruction(t, checkGoal())

	require.Contains(t, out, "GOAL t-1: Count items\nAcceptance criteria (one verdict each):\n"+
		"1. `todo count` prints the number of items\n2. unit tests cover it\n3. the README documents `todo count`\n"+
		"Scope:\n- Repo: app\n- Files this item may change:\n  - app: todo/cli.py\n  - app: README.md\n"+
		"- Rule: "+goal.ScopeRule+"\nEND GOAL")
	require.NotContains(t, out, "other work", "nothing from any other task")
	require.Contains(t, out, "This is attempt 1 of round 1.")
	require.NotContains(t, out, "&#", "criterion text is never HTML-escaped")
}

func TestCheckGoalLaunchesAFreshReadOnlyReviewerWithNoConversation(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	out := f.instruction(t, checkGoal())

	for _, want := range []string{
		"helper named `spek-goal-reviewer`",
		"Bob's built-in read-only `explore` helper",
		"any sub-agent that starts with a fresh context",
		"spektacular skill review-task-goal",
		"read and search tools only",
		"the goal block from Step 1, verbatim",
		"the verify step's pass/fail summary",
		"pointers to the work",
		"Give it nothing from your own conversation: no summary of what you did",
		"no claim that the work is complete or that any criterion is met",
		"git -C <root> diff HEAD",
		"git -C <root> status --porcelain --untracked-files=all",
		".spektacular/tmp/test/goal-check/<repo>.diff",
	} {
		require.Contains(t, out, want)
	}
	require.Contains(t, out, `spektacular implement goal record --data '{"name":"test","item":"t-1"}' --from .spektacular/tmp/test/goal-check/reply.json`)
	require.Contains(t, out, "Write the reviewer's reply **unchanged**")
}

func TestCheckGoalNamesBothStopsAndTheirOptions(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	out := f.instruction(t, checkGoal())

	require.Contains(t, out, "**`exhausted`**: criteria are still not met after the round's last retry. STOP.")
	require.Contains(t, out, "**`impossible`**: a criterion cannot be met as written. STOP at once, without retrying.")
	for _, option := range []string{"(`keep_fixing`)", "(`accept`)", "(`skip`)", "(`recheck`)"} {
		require.Contains(t, out, option)
	}
	require.Contains(t, out, `spektacular implement goal decide --data '{"name":"test","item":"t-1","decision":"<keep_fixing|accept|skip|recheck>"`)
	require.Contains(t, out, specConflictHeading)
	for _, word := range []string{"orchestrator", "QUESTION:"} {
		require.NotContains(t, out, word, "a standalone render carries no orchestrator wording")
	}
}

func TestCheckGoalRefusesWhenSwitchedOff(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, "")
	f.cfg.GoalCheck = false
	_, err := f.render(t, checkGoal())
	var oe *output.ErrorResponse
	require.ErrorAs(t, err, &oe)
	require.Equal(t, "goal_check_disabled", oe.Code)
	require.Contains(t, oe.NextAction, `"step":"update_plan"`)
}

func TestUpdatePlanRefusesUntilTheGoalCheckIsSettled(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	var oe *output.ErrorResponse

	_, err := f.render(t, updatePlan())
	require.ErrorAs(t, err, &oe, "nothing is ticked before the check has run")
	require.Equal(t, "goal_check_not_settled", oe.Code)
	require.Contains(t, oe.Message, "has not run for t-1")
	require.Contains(t, oe.NextAction, `"step":"check_goal"`)
	require.Contains(t, oe.NextAction, "implement.goal_check.enabled: false", "a run upgraded mid-task has a way out")

	f.record(t, "t-1", "met", "met", "not_met")
	_, err = f.render(t, updatePlan())
	require.ErrorAs(t, err, &oe)
	require.Contains(t, oe.NextAction, `"step":"implement"`, "a retry goes back to implementing")

	f.record(t, "t-1", "met", "impossible", "met")
	_, err = f.render(t, updatePlan())
	require.ErrorAs(t, err, &oe)
	require.Contains(t, oe.NextAction, "implement goal decide", "a stop waits for the user's decision")
}

func TestImplementCarriesTheReviewersReasonsOnARetry(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	before := f.instruction(t, implementStep())
	require.NotContains(t, before, "The goal check sent this task back")

	f.record(t, "t-1", "met", "met", "not_met")
	out := f.instruction(t, implementStep())
	require.Contains(t, out, "### The goal check sent this task back")
	require.Contains(t, out, "(round 1, attempt 1)")
	require.Contains(t, out, "- Criterion 3 (not_met): the README documents `todo count`. Reviewer's evidence: evidence for 3")
	require.NotContains(t, out, "Criterion 1 (", "only the criteria not yet met")

	retry := f.instruction(t, checkGoal())
	require.Contains(t, retry, "This is attempt 2 of round 1.", "the next check follows, as attempt 2")
}

func TestUpdatePlanTicksExactlyTheCriteriaJudgedMet(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	f.record(t, "t-1", "met", "met", "met")
	accepted := f.instruction(t, updatePlan())
	require.Contains(t, accepted, "Tick exactly these acceptance criteria, which the goal check judged met, and no other:\n"+
		"   - criterion 1: `todo count` prints the number of items\n   - criterion 2: unit tests cover it\n   - criterion 3: the README documents `todo count`\n")
	require.NotContains(t, accepted, "passed verification in the previous step")

	// A mix the user accepts: only the met criteria are ticked.
	mixed := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	for range 3 {
		mixed.record(t, "t-1", "met", "not_met", "met")
	}
	mixed.decide(t, "t-1", goal.DecideAccept)
	out := mixed.instruction(t, updatePlan())
	require.Contains(t, out, "   - criterion 1: `todo count` prints the number of items\n   - criterion 3: the README documents `todo count`\n")
	require.NotContains(t, out, "criterion 2:")
	require.Contains(t, out, "Leave every other criterion of the task as `- [ ]`, whatever verification said.")
	require.Contains(t, out, "The user accepted this shortfall")
}

func TestASkippedTaskTicksNothingAndIsPassedOver(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	f.instruction(t, checkGoal())
	f.record(t, "t-1", "met", "impossible", "met")
	f.decide(t, "t-1", goal.DecideSkip)

	out := f.instruction(t, updatePlan())
	require.Contains(t, out, "The user chose to **skip** this task. Tick nothing")
	require.NotContains(t, out, "Tick exactly these")

	// A whole-plan run moves on to the next task: the goal check now builds
	// the next task's goal, and the templates tell the agent to pass over it.
	next := f.instruction(t, checkGoal())
	require.Contains(t, next, "GOAL t-2: Another task")
	require.Contains(t, next, "Pass over any task or phase that the goal check skipped")
	changelog := f.instruction(t, updateChangelog())
	require.Contains(t, changelog, "Do not count any task or phase that `spektacular implement goal show test` reports as skipped.")
	require.Contains(t, changelog, "name that under **Deviations**")

	// A single-task run of the skipped task is the plan's last one when only
	// skipped tasks remain open.
	f.record(t, "t-2", "met")
	plan := strings.Replace(goalTaskPlan, "#### - [ ] Task: Another task", "#### - [x] Task: Another task", 1)
	require.NoError(t, f.st.Write("plans/test/plan.md", []byte(plan)))
	f.data["task"] = "t-1"
	single := f.instruction(t, updateChangelog())
	require.Contains(t, single, `"step":"test_plan"`, "a skipped task does not keep the plan open")
}

func TestLegacyPhaseIsJudgedTheSameWay(t *testing.T) {
	f := newGoalFixture(t, goalLegacyPlan, "")
	out := f.instruction(t, checkGoal())
	require.Contains(t, out, "GOAL Phase 1.1: Old style\nAcceptance criteria (one verdict each):\n1. it works\n2. it is documented\n")
	require.Contains(t, out, `"item":"Phase 1.1"`)

	_, err := f.render(t, updatePlan())
	require.Error(t, err, "a phase is not ticked before its check either")

	f.record(t, "Phase 1.1", "met", "met")
	plan := f.instruction(t, updatePlan())
	require.Contains(t, plan, "   - criterion 1: it works\n   - criterion 2: it is documented\n")
}

func TestCheckGoalShowsAStopAgainUntilItIsDecided(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	f.record(t, "t-1", "met", "impossible", "met")
	out := f.instruction(t, checkGoal())
	require.Contains(t, out, "### This check has already stopped")
	require.Contains(t, out, "- Criterion 2 (impossible): unit tests cover it. Evidence: evidence for 2")
	require.Contains(t, out, "Do not check again.")
}

func TestFSMWalkWithTheGoalCheckOn(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	tmp := t.TempDir()
	writer := &captureWriter{}
	wf := workflow.New(Steps(), filepath.Join(tmp, "state.json"), f.cfg, f.st, writer)
	wf.SetData("name", "test")
	for _, step := range []string{"read_plan", "analyze", "implement", "test", "verify", "check_goal"} {
		require.NoError(t, advance(wf, step), step)
	}
	require.Error(t, wf.Goto("update_plan"), "update_plan is refused before a verdict is recorded")
	require.Equal(t, "check_goal", wf.Current())

	f.record(t, "t-1", "met", "met", "not_met")
	for _, step := range []string{"implement", "test", "verify", "check_goal"} {
		require.NoError(t, wf.Goto(step), step)
	}
	f.record(t, "t-1", "met", "met", "met")
	require.NoError(t, wf.Goto("update_plan"))
	require.NoError(t, wf.Goto("update_changelog"))
	require.Equal(t, "update_changelog", wf.Current())
}

func TestFSMWalkWithTheGoalCheckOff(t *testing.T) {
	f := newGoalFixture(t, goalTaskPlan, goalTaskContext)
	f.cfg.GoalCheck = false
	writer := &captureWriter{}
	wf := workflow.New(Steps(), filepath.Join(t.TempDir(), "state.json"), f.cfg, f.st, writer)
	wf.SetData("name", "test")
	for _, step := range []string{"read_plan", "analyze", "implement", "test", "verify", "update_plan"} {
		require.NoError(t, advance(wf, step), step)
	}
	require.Contains(t, writer.result.Instruction, "**only if** that criterion actually passed verification in the previous step")
	require.NotContains(t, writer.result.Instruction, "goal check")
	o, err := goal.Load(f.st, "plans", "test")
	require.NoError(t, err)
	require.Empty(t, o.Items, "with the check off no verdicts are recorded")
}
