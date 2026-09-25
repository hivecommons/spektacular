package implement

import (
	"errors"
	"fmt"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
)

// Steps returns the ordered step configs for an implement workflow.
//
// The FSM uses a multi-source transition on analyze: both `read_plan` and
// `update_changelog` can lead into `analyze`. This encodes the phase-loop
// directly in the FSM declaration — when `update_changelog` detects remaining
// unchecked phases in the plan, it advances back to `analyze`; otherwise it
// advances to `test_plan`.
//
// A single-task run (workflow data "task") never loops. `update_changelog`
// either continues into the feature wrap-up, when the run completed the
// plan's last open task, or goes straight to `finished`, leaving the wrap-up
// to the run that completes the last one.
func Steps() []workflow.StepConfig {
	return []workflow.StepConfig{
		{Name: "new", Src: []string{"start"}, Dst: "new", Callback: newStep()},
		{Name: "read_plan", Src: []string{"new"}, Dst: "read_plan", Callback: readPlan()},
		{Name: "analyze", Src: []string{"read_plan", "update_changelog"}, Dst: "analyze", Callback: analyze()},
		{Name: "implement", Src: []string{"analyze"}, Dst: "implement", Callback: implementStep()},
		{Name: "test", Src: []string{"implement"}, Dst: "test", Callback: testStep()},
		{Name: "verify", Src: []string{"test"}, Dst: "verify", Callback: verify()},
		{Name: "update_plan", Src: []string{"verify"}, Dst: "update_plan", Callback: updatePlan()},
		{Name: "update_changelog", Src: []string{"update_plan"}, Dst: "update_changelog", Callback: updateChangelog()},
		{Name: "test_plan", Src: []string{"update_changelog"}, Dst: "test_plan", Callback: testPlan()},
		{Name: "update_feature_changelog", Src: []string{"test_plan"}, Dst: "update_feature_changelog", Callback: updateFeatureChangelog()},
		{Name: "reconcile_spec", Src: []string{"update_feature_changelog"}, Dst: "reconcile_spec", Callback: reconcileSpec()},
		{Name: "finished", Src: []string{"reconcile_spec", "update_changelog"}, Dst: "finished", Callback: finished()},
	}
}

// buildResult is the stepkit.ResultBuilder for the implement workflow.
func buildResult(stepName, instanceName, primaryPath, instruction string) any {
	return Result{
		Step:         stepName,
		PlanPath:     primaryPath,
		PlanName:     instanceName,
		PlanDocument: "plan",
		Instruction:  instruction,
	}
}

// writeStep is a thin wrapper around stepkit.WriteStepResult pre-applied with
// the implement strategy and result builder.
func writeStep(stepName, nextStep, templatePath string, data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config, extra map[string]any) error {
	return stepkit.WriteStepResult(
		stepkit.StepRequest{
			StepName:     stepName,
			NextStep:     nextStep,
			TemplatePath: templatePath,
			Strategy:     strategy{planDir: cfg.PlanDir},
			Extra:        extra,
		},
		data, out, st, cfg,
		buildResult,
	)
}

// newStep initializes state only — auto-advances to read_plan without writing
// anything. Mirrors the plan workflow's `new` callback.
func newStep() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "read_plan", nil
	}
}

func readPlan() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("read_plan", "analyze", "steps/implement/01-read_plan.md", data, out, st, cfg, extra)
	}
}

func analyze() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("analyze", "implement", "steps/implement/02-analyze.md", data, out, st, cfg, extra)
	}
}

func implementStep() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("implement", "test", "steps/implement/03-implement.md", data, out, st, cfg, extra)
	}
}

func testStep() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("test", "verify", "steps/implement/04-test.md", data, out, st, cfg, extra)
	}
}

func verify() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("verify", "update_plan", "steps/implement/05-verify.md", data, out, st, cfg, extra)
	}
}

func updatePlan() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		extra, err := taskExtra(data, st, cfg)
		if err != nil {
			return "", err
		}
		return "", writeStep("update_plan", "update_changelog", "steps/implement/06-update_plan.md", data, out, st, cfg, extra)
	}
}

// updateChangelog has two legal exits in a whole-plan run, encoded in the
// template:
//   - goto analyze (loop back) when unchecked phases remain
//   - goto test_plan when no unchecked phases remain
//
// NextStep is set to "test_plan" for the default advance path; the template
// instructs the agent to branch based on plan-file state.
//
// In a single-task run the decision is made here, from the plan, and the
// template names the one exit: test_plan when no open task remains, so this
// run does the feature wrap-up, and finished otherwise.
func updateChangelog() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		task, p, ok, err := taskRun(data, st, cfg)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", writeStep("update_changelog", "test_plan", "steps/implement/07-update_changelog.md", data, out, st, cfg, nil)
		}
		last := len(p.OpenTasks()) == 0
		next := "finished"
		if last {
			next = "test_plan"
		}
		return "", writeStep("update_changelog", next, "steps/implement/07-update_changelog.md", data, out, st, cfg, map[string]any{
			"task":      taskVars(task),
			"last_task": last,
		})
	}
}

// testPlan runs once at the end, after all phases are implemented and their
// changelog entries are written. By this point the implementation is complete, so the
// manual test plan it produces can reference real endpoints, commands, and
// thresholds. It writes `.spektacular/plans/<name>/test-plan.md` for success
// metrics that cannot be covered by an automated behavioural test.
func testPlan() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("test_plan", "update_feature_changelog", "steps/implement/09-test_plan.md", data, out, st, cfg, nil)
	}
}

// updateFeatureChangelog runs last, after every phase is implemented and
// verified and the test plan is written. It authors a durable, self-contained
// changelog record for the feature into the changelog store, grounded in what
// was actually built rather than what was originally planned.
func updateFeatureChangelog() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("update_feature_changelog", "reconcile_spec", "steps/implement/10-update_feature_changelog.md", data, out, st, cfg, nil)
	}
}

// reconcileSpec runs after the feature changelog record is written. It
// compares the specification's Requirements and Acceptance Criteria against
// the plan's accumulated changelog record and commits an updated spec with
// satisfied items checked off, so the finished report can read it back.
func reconcileSpec() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		return "", writeStep("reconcile_spec", "finished", "steps/implement/11-reconcile_spec.md", data, out, st, cfg, nil)
	}
}

func finished() workflow.StepCallback {
	return func(data workflow.Data, out workflow.ResultWriter, st store.Store, cfg workflow.Config) (string, error) {
		// A single-task run that left tasks open ends here without the
		// feature wrap-up: there is no test plan or feature changelog to
		// close, and none is required. The run that completes the last open
		// task produces them.
		task, p, ok, err := taskRun(data, st, cfg)
		if err != nil {
			return "", err
		}
		if ok {
			if open := len(p.OpenTasks()); open > 0 {
				return "", writeStep("finished", "", "steps/implement/12-finished.md", data, out, st, cfg, map[string]any{
					"task":       taskVars(task),
					"task_run":   true,
					"open_tasks": open,
				})
			}
		}

		if !cfg.DryRun && st != nil {
			planName := stepkit.GetString(data, "name")

			// Test plan: tolerate absence, close if present.
			testPlanPath := PlanDocumentPath(cfg.PlanDir, planName, "test-plan")
			if err := metadata.Close(st, testPlanPath, metadata.StatusFinal); err != nil && !errors.Is(err, store.ErrNotFound) {
				return "", err
			}

			// Project-level changelog: required artifact of the implement
			// workflow. If update_feature_changelog silently skipped its write
			// (a real failure mode we've observed), the FSM must surface it here
			// rather than mark the workflow finished with no record on disk.
			changelogPath := ChangelogFilePath(cfg.ChangelogDir, planName)
			if err := metadata.Close(st, changelogPath, metadata.StatusFinal); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return "", output.NewError(
						"changelog_missing",
						fmt.Sprintf("project-level changelog record %q was never written by update_feature_changelog", changelogPath),
					).WithResource(changelogPath).
						WithNextAction(fmt.Sprintf("re-run the update_feature_changelog step: `%s implement goto --data '{\"step\":\"update_feature_changelog\"}'`", cfg.Command))
				}
				return "", err
			}
		}
		return "", writeStep("finished", "", "steps/implement/12-finished.md", data, out, st, cfg, nil)
	}
}

// taskRun reads the plan for a single-task run. ok is false for a whole-plan
// run, which has no "task" in its workflow data. The task comes back with
// only its id when the plan cannot be read (no store, as in a dry run).
func taskRun(data workflow.Data, st store.Store, cfg workflow.Config) (plantask.Task, plantask.Plan, bool, error) {
	id := stepkit.GetString(data, "task")
	if id == "" {
		return plantask.Task{}, plantask.Plan{}, false, nil
	}
	if st == nil {
		return plantask.Task{ID: id, Title: id}, plantask.Plan{}, true, nil
	}
	raw, err := st.Read(PlanFilePath(cfg.PlanDir, stepkit.GetString(data, "name")))
	if err != nil {
		return plantask.Task{}, plantask.Plan{}, false, err
	}
	_, body, err := metadata.Split(raw)
	if err != nil {
		return plantask.Task{}, plantask.Plan{}, false, err
	}
	p := plantask.Parse(body)
	task, found := p.Task(id)
	if !found {
		task = plantask.Task{ID: id, Title: id}
	}
	return task, p, true, nil
}

// taskExtra is the template data scoping a step to the selected task in a
// single-task run, and nil in a whole-plan run.
func taskExtra(data workflow.Data, st store.Store, cfg workflow.Config) (map[string]any, error) {
	task, _, ok, err := taskRun(data, st, cfg)
	if err != nil || !ok {
		return nil, err
	}
	return map[string]any{"task": taskVars(task)}, nil
}

// taskVars is the template value for the selected task.
func taskVars(t plantask.Task) map[string]any {
	return map[string]any{"id": t.ID, "title": t.Title}
}
