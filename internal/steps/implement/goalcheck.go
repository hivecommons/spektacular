package implement

import (
	"errors"
	"fmt"

	"github.com/hivecommons/spektacular/internal/goal"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// goalView is what the goal-check steps read: the plan, its context, the
// recorded outcome and the current item.
type goalView struct {
	plan        plantask.Plan
	planBody    []byte
	contextBody []byte
	outcome     goal.Outcome
	item        string // the current item; "" when the plan has none open
	// missing says the plan could not be read because it is not in the store
	// (a dry run's preview of a plan that was never written).
	missing bool
}

// loadGoalView reads the plan, its context and the outcome record, and
// resolves the current item the way the CLI's `implement goal record` does.
func loadGoalView(data workflow.Data, st store.Store, cfg workflow.Config) (goalView, error) {
	name := stepkit.GetString(data, "name")
	var v goalView
	if st == nil {
		v.missing = true
		return v, nil
	}
	raw, err := st.Read(PlanFilePath(cfg.PlanDir, name))
	if errors.Is(err, store.ErrNotFound) {
		v.missing = true
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if _, v.planBody, err = metadata.Split(raw); err != nil {
		return v, err
	}
	if ctx, err := st.Read(ContextFilePath(cfg.PlanDir, name)); err == nil {
		v.contextBody = ctx
	} else if !errors.Is(err, store.ErrNotFound) {
		return v, err
	}
	v.plan = plantask.Parse(v.planBody)
	if v.outcome, err = goal.Load(st, cfg.PlanDir, name); err != nil {
		return v, err
	}
	v.item, _ = goal.Current(v.plan, v.outcome, stepkit.GetString(data, "task"))
	return v, nil
}

// goalItemKey is the workflow data key holding the item the last check_goal
// rendered, so update_plan acts on that item even once a skip has taken it
// out of the current-item rule. analyze clears it as each new item starts.
const goalItemKey = "goal_item"

// clearGoalItem forgets the item the last check was for, as a new item starts.
func clearGoalItem(data workflow.Data, cfg workflow.Config) {
	if cfg.GoalCheck {
		data.Set(goalItemKey, "")
	}
}

// goalTmpDir is the project-relative folder the check_goal step stages the
// work and the reviewer's reply in.
func goalTmpDir(plan string) string {
	return ".spektacular/tmp/" + plan + "/goal-check"
}

// checkGoal renders the check_goal step: the current item's goal, how to
// stage the work and launch the reviewer, and the record command whose answer
// decides the next step. A check that already stopped renders the stop again
// until the user's decision is recorded.
func checkGoal() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		name := stepkit.GetString(data, "name")
		if !cfg.GoalCheck {
			return "", output.NewError("goal_check_disabled", "the goal check is switched off for this project (implement.goal_check.enabled is false)").
				WithNextAction(fmt.Sprintf("go on to the plan update: %s implement goto --data '{\"step\":\"update_plan\",\"name\":%q}', or set implement.goal_check.enabled: true in .spektacular/config.yaml", cfg.Command, name))
		}
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		extra = copyExtra(extra)
		extra["goal_tmp_dir"] = goalTmpDir(name)

		v, err := loadGoalView(data, st, cfg)
		if err != nil {
			return "", err
		}
		if v.missing {
			// A preview of a plan that is not in the store: render the step's
			// shape with a placeholder goal.
			extra["goal"] = map[string]any{"item": "<item>", "title": "<the current task>", "criteria": []map[string]any{}, "repo": "<repo>", "rule": goal.ScopeRule}
			extra["attempt"], extra["round"], extra["retries_left"] = 1, 1, goal.MaxRetries
			return "", writeStep("check_goal", "update_plan", "steps/implement/05b-check_goal.md", data, out, st, cfg, extra)
		}
		if v.item == "" {
			return "", output.NewError("goal_item_not_found", "the plan has no open task or phase left to check").
				WithNextAction(fmt.Sprintf("read the plan with `%s plan file read %s plan`; when every item is ticked or skipped, advance to the plan update: %s implement goto --data '{\"step\":\"update_plan\",\"name\":%q}'", cfg.Command, name, cfg.Command, name))
		}
		g, err := goal.Build(v.planBody, v.contextBody, v.item, cfg.Repos)
		if err != nil {
			return "", err
		}
		extra["goal"] = goalVars(g)
		// The plan update that follows ticks this item's criteria even when
		// the user skips it, which takes it out of the current-item rule.
		data.Set(goalItemKey, v.item)

		it := v.outcome.Item(v.item)
		round, attempt, err := goal.NextAttempt(it)
		if err != nil {
			// The last check stopped the run and the user has not decided:
			// show the stop again.
			last := it.Checks[len(it.Checks)-1]
			extra["awaiting_decision"] = true
			extra["stopped_exhausted"] = last.Decision == goal.CheckExhausted
			extra["stopped_impossible"] = last.Decision == goal.CheckImpossible
			extra["stop_verdicts"] = verdictVars(g, goal.Shortfall(it))
			round, attempt = last.Round, last.Attempt
		}
		extra["round"], extra["attempt"] = round, attempt
		extra["retries_left"] = 1 + goal.MaxRetries - attempt
		return "", writeStep("check_goal", "update_plan", "steps/implement/05b-check_goal.md", data, out, st, cfg, extra)
	}
}

// goalVars is the template value of a goal: its item, title, numbered
// criteria, repo, files and scope rule.
func goalVars(g goal.Goal) map[string]any {
	criteria := make([]map[string]any, 0, len(g.Criteria))
	for i, c := range g.Criteria {
		criteria = append(criteria, map[string]any{"n": i + 1, "text": c})
	}
	files := make([]map[string]any, 0, len(g.Scope.Files))
	for _, f := range g.Scope.Files {
		files = append(files, map[string]any{"repo": f.Repo, "path": f.Path})
	}
	return map[string]any{
		"item":      g.Item,
		"title":     g.Title,
		"criteria":  criteria,
		"repo":      g.Scope.Repo,
		"files":     files,
		"has_files": len(files) > 0,
		"rule":      g.Scope.Rule,
	}
}

// verdictVars pairs each verdict with its criterion's text.
func verdictVars(g goal.Goal, vs []goal.Verdict) []map[string]any {
	out := make([]map[string]any, 0, len(vs))
	for _, v := range vs {
		text := ""
		if v.Criterion >= 1 && v.Criterion <= len(g.Criteria) {
			text = g.Criteria[v.Criterion-1]
		}
		out = append(out, map[string]any{"n": v.Criterion, "text": text, "verdict": v.Verdict, "evidence": v.Evidence})
	}
	return out
}

// copyExtra returns a writable copy of a step's template variables.
func copyExtra(extra map[string]any) map[string]any {
	out := make(map[string]any, len(extra)+4)
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// withGoalCheck marks a step's template variables with "goal_check" when the
// check is switched on. Off, extra is returned unchanged, so every template
// renders exactly as it did before the check existed.
func withGoalCheck(cfg workflow.Config, extra map[string]any) map[string]any {
	if !cfg.GoalCheck {
		return extra
	}
	out := make(map[string]any, len(extra)+1)
	for k, v := range extra {
		out[k] = v
	}
	out["goal_check"] = true
	return out
}

// goalRetryExtra adds the reviewer's reasons to the implement step when the
// current item is being retried after a check judged criteria not yet met.
func goalRetryExtra(data workflow.Data, st store.Store, cfg workflow.Config, extra map[string]any) (map[string]any, error) {
	if !cfg.GoalCheck {
		return extra, nil
	}
	v, err := loadGoalView(data, st, cfg)
	if err != nil || v.missing || v.item == "" {
		return extra, err
	}
	it := v.outcome.Item(v.item)
	if it.State != goal.StateRetrying || len(it.Checks) == 0 {
		return extra, nil
	}
	last := it.Checks[len(it.Checks)-1]
	extra = copyExtra(extra)
	extra["goal_retry"] = map[string]any{
		"attempt":  last.Attempt,
		"round":    last.Round,
		"criteria": verdictVars(last.Goal, goal.Shortfall(it)),
	}
	return extra, nil
}

// goalUpdateExtra gates the plan update on the current item's goal check and
// lists exactly the criteria to tick. It refuses until the item's latest round
// is settled: accepted, accepted with a shortfall, or skipped.
func goalUpdateExtra(data workflow.Data, st store.Store, cfg workflow.Config, extra map[string]any) (map[string]any, error) {
	if !cfg.GoalCheck {
		return extra, nil
	}
	v, err := loadGoalView(data, st, cfg)
	if err != nil {
		return nil, err
	}
	extra = copyExtra(extra)
	item := stepkit.GetString(data, goalItemKey)
	if item == "" {
		item = v.item
	}
	if v.missing || item == "" {
		extra["goal_met"] = []map[string]any{}
		return extra, nil
	}
	name := stepkit.GetString(data, "name")
	it := v.outcome.Item(item)
	if !goal.Settled(it) {
		return nil, notSettled(cfg.Command, name, it)
	}
	last := it.Checks[len(it.Checks)-1]
	met := []map[string]any{}
	for _, n := range goal.MetCriteria(it) {
		met = append(met, map[string]any{"n": n, "text": last.Goal.Criteria[n-1]})
	}
	extra["goal_item"] = item
	extra["goal_met"] = met
	extra["has_goal_met"] = len(met) > 0
	extra["goal_skipped"] = it.State == goal.StateSkipped
	extra["goal_shortfall"] = it.State == goal.StateAcceptedShortfall
	return extra, nil
}

func notSettled(command, plan string, it goal.ItemOutcome) error {
	gotoCmd := func(step string) string {
		return fmt.Sprintf(`%s implement goto --data '{"step":"%s","name":"%s"}'`, command, step, plan)
	}
	switch it.State {
	case goal.StatePending:
		return output.NewError("goal_check_not_settled",
			fmt.Sprintf("the goal check has not run for %s, so none of its criteria can be ticked yet", it.Item)).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf("run the goal check first: %s; if this run was already at update_plan for this task before the goal check existed, finish this one task with implement.goal_check.enabled: false in .spektacular/config.yaml and switch it back on afterwards", gotoCmd("check_goal")))
	case goal.StateAwaitingDecision:
		return output.NewError("goal_check_not_settled",
			fmt.Sprintf("the goal check of %s stopped and is waiting for the user's decision", it.Item)).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf(`ask the user, record the answer with %s implement goal decide --data '{"name":"%s","item":"%s","decision":"keep_fixing|accept|skip|recheck"}' and run the goto it returns`, command, plan, it.Item))
	default:
		return output.NewError("goal_check_not_settled",
			fmt.Sprintf("the goal check of %s judged criteria not yet met, so the task goes back to implementing", it.Item)).
			WithResource(it.Item).
			WithNextAction(fmt.Sprintf("fix the criteria the reviewer judged not yet met: %s", gotoCmd("implement")))
	}
}

// openTasksLeft counts a single-task run's open tasks: every open task, less
// those the goal check skipped when it is switched on.
func openTasksLeft(p plantask.Plan, st store.Store, cfg workflow.Config, plan string) (int, error) {
	if !cfg.GoalCheck || st == nil {
		return len(p.OpenTasks()), nil
	}
	o, err := goal.Load(st, cfg.PlanDir, plan)
	if err != nil {
		return 0, err
	}
	return goal.OpenUnskipped(p, o), nil
}
