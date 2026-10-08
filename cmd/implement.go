package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/status"
	"github.com/hivecommons/spektacular/internal/steps/implement"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/spf13/cobra"
)

var implementResultOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"step":          {Type: "string"},
		"plan_path":     {Type: "string", Description: "the plan's location relative to the folder holding config.yaml"},
		"plan_document": {Type: "string", Description: `the plan's document name, always "plan"`},
		"plan_name":     {Type: "string"},
		"instruction":   {Type: "string"},
	},
}

var implementCmd = &cobra.Command{
	Use:   "implement",
	Short: "Manage implement workflow",
	RunE:  runUnknownSubcommand,
}

var implementNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new implement workflow for an existing spec",
	RunE:  runImplementNew,
}

var implementGotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Jump to a named step",
	RunE:  runImplementGoto,
}

var implementStepsCmd = &cobra.Command{
	Use:   "steps",
	Short: "List available workflow step names",
	RunE:  runImplementSteps,
}

func runImplementNew(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name":                  {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the spec to implement (its plan shares the name)"},
					"task":                  {Type: "string"},
					"orchestrated":          {Type: "boolean", Description: "true when an epic orchestrator starts the run: it keeps its own lane under .spektacular/workflows/ and skips the uncommitted-changes question"},
					"override_dependencies": {Type: "boolean", Description: "start even though a spec this one depends on in its epic is not implemented yet; set only after the user agrees, and refused under epic.strict_dependencies"},
				},
				Required: []string{"name"},
			},
			Output: implementResultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	dataDir, err := dataDir()
	if err != nil {
		return err
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Check for an in-progress workflow BEFORE requiring a name — mirrors
	// spec new so the driving agent can offer resume without first
	// prompting the user for a plan name.
	statePath := stateFilePath(dataDir)
	laneName, lane, orchestrated, err := implementLaneStart(cfg, dataStr, dryRun)
	if err != nil {
		return err
	}
	if lane {
		// A lane run keeps its own state, so it probes only that lane for a
		// resume: another spec's workflow never blocks it.
		statePath = workflow.LaneStatePath(dataDir, "implement", laneName)
	}
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		if lane && !orchestrated {
			// An epic orchestrator's run of this spec is its to resume, not
			// the user's: refuse as the shared slot always has.
			if existing, err := workflow.ReadLane(dataDir, "implement", laneName); err != nil {
				return err
			} else if existing != nil && existing.InProgress() {
				if o, _ := existing.Data["orchestrated"].(bool); o && !force {
					return refuseLaneInProgress(dataDir, cfg.Command, "implement", laneName)
				}
			}
			// A run of this same spec still in the shared slot, from before
			// worktrees were on, is resumed there rather than orphaned.
			if shared, err := detectInProgress(stateFilePath(dataDir)); err != nil {
				return err
			} else if shared != nil && shared.Kind == "implement" {
				if sharedName, _ := shared.Data["name"].(string); sharedName == laneName {
					handled, err := probeResume(stateFilePath(dataDir), cfg.Command, "implement", force)
					if handled || err != nil {
						return err
					}
				}
			}
		}
		handled, err := probeResume(statePath, cfg.Command, "implement", force)
		if err != nil {
			return err
		}
		if handled {
			return err
		}
	}

	// No workflow to resume — starting fresh requires a name.
	if dataStr == "" {
		next := `specify the spec to implement with --data '{"name":"<spec_name>"}'; the spec must have a plan, so if it has none run "plan new" for it first; to see existing specs, run "spec file list"`
		if lanes := inProgressLanes(dataDir, cfg.Command, "implement"); len(lanes) > 0 {
			next += "; implement workflows in progress in their own lanes: " + strings.Join(lanes, ", ")
		}
		return output.NewError("name_required", "no spec name was provided").WithNextAction(next)
	}
	var input struct {
		Name                 string `json:"name"`
		Task                 string `json:"task"`
		OverrideDependencies bool   `json:"override_dependencies"`
	}
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	if input.Name == "" || !nameRegexp.MatchString(input.Name) || len(input.Name) > 64 {
		return fmt.Errorf("name must match ^[a-z0-9_-]+$ and be at most 64 characters")
	}
	if !lane && !dryRun {
		if err := refuseLaneInProgress(dataDir, cfg.Command, "implement", input.Name); err != nil {
			return err
		}
	}

	// Precondition: the plan file must exist before an implement workflow
	// can run against it. The workflow operates on an already-approved plan.
	// The check goes through the store, so a backend that is not a local
	// directory answers it too; Root() is used only to render the path.
	projectStore := store.NewSourceStore(root, "project")
	planRel := implement.PlanFilePath(cfg.Plan.Config.Directory, input.Name)
	if _, statErr := projectStore.Stat(planRel); statErr != nil {
		return fmt.Errorf("spec %q has no plan at %s — run 'plan new' for it first, or check the spec name", input.Name, filepath.Join(root, planRel))
	}
	if err := refuseStalePlan(cfg, projectStore, input.Name); err != nil {
		return err
	}
	if input.Task != "" {
		if err := refuseUnstartableTask(cfg, projectStore, input.Name, input.Task); err != nil {
			return err
		}
	}
	override, err := refuseUnmetDependencies(cfg, root, projectStore, input.Name, dataStr, input.OverrideDependencies)
	if err != nil {
		return err
	}

	// The uncommitted-changes gate runs once the plan is known to exist, so a
	// refusal here never precedes a plan-not-found error, and before
	// clearState — the first thing this command writes.
	// An orchestrated run skips it: the orchestrator raises uncommitted work
	// once, at the start of the whole run.
	if !orchestrated {
		if err := startGate(cfg, root, "implement", input.Name, dataStr, dryRun); err != nil {
			return err
		}
	}
	if !dryRun {
		clearState(statePath)
	}

	if err := ensureImplementWorktrees(root, cfg, projectStore, input.Name, orchestrated, dryRun); err != nil {
		return err
	}
	codeRoots, err := codeRootsFor(root, cfg, input.Name)
	if err != nil {
		return err
	}
	wfCfg := workflow.Config{Command: cfg.Command, Kind: "implement", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, ChangelogDir: cfg.Changelog.Config.Directory, AutoCommit: cfg.AutoCommitMode(), CodeRoots: codeRoots}
	steps := implement.Steps()
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(steps, statePath, wfCfg, projectStore, out)
	wf.SetData("name", input.Name)
	if orchestrated {
		wf.SetData("orchestrated", true)
	} else if lane {
		// A lane of its own, but an interactive run: it never hands back to
		// an orchestrator.
		wf.SetData("lane", true)
	}
	if input.Task != "" {
		wf.SetData("task", input.Task)
	}
	if len(override) > 0 {
		wf.SetData("dependency_override", override)
	}

	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	if err := wf.Next(); err != nil {
		return err
	}
	return nil
}

func runImplementGoto(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"step": {Type: "string", Enum: workflow.New(implement.Steps(), "", workflow.Config{}, nil, nil).StepNames()},
					"name": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64, Description: "the spec whose workflow to advance; routes to its orchestrated lane when it has one"},
				},
				Required: []string{"step"},
			},
			Output: implementResultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dataStr == "" {
		return output.NewError("step_required", "no step was provided").
			WithNextAction(`specify the step with --data '{"step":"<step_name>"}'; run "implement steps" to see valid step names`)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	stepVal, _ := input["step"].(string)
	if stepVal == "" {
		return output.NewError("step_required", `"step" is missing or empty in --data`).
			WithNextAction(`include a non-empty "step" in --data, e.g. --data '{"step":"<step_name>"}'; run "implement steps" to see valid step names`)
	}

	dataDir, err := dataDir()
	if err != nil {
		return err
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Refuse to operate on an in-progress workflow of a different kind (e.g. a
	// spec or plan); resuming it from here would apply implement steps to it.
	// The spec name routes the goto to that spec's workflow — its lane, or
	// the shared record when that holds it. It is a routing key, never
	// workflow data, so it is removed before the rest is copied in.
	gotoName, _ := input["name"].(string)
	delete(input, "name")
	slot, err := resolveGotoSlot(dataDir, cfg.Command, "implement", gotoName)
	if err != nil {
		return err
	}

	if handled, err := guardKind(slot.StatePath, cfg.Command, "implement"); err != nil {
		return err
	} else if handled {
		return err
	}
	wf := workflow.New(implement.Steps(), slot.StatePath, workflow.Config{}, nil, nil)
	var codeRoots []workflow.CodeRoot
	if nameVal, ok := wf.GetData("name"); ok {
		projectStore := store.NewSourceStore(root, "project")
		if err := refuseStalePlan(cfg, projectStore, fmt.Sprintf("%v", nameVal)); err != nil {
			return err
		}
		if codeRoots, err = codeRootsFor(root, cfg, fmt.Sprintf("%v", nameVal)); err != nil {
			return err
		}
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "implement", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, ChangelogDir: cfg.Changelog.Config.Directory, AutoCommit: cfg.AutoCommitMode(), CodeRoots: codeRoots}
	return gotoWithAutoCommit(cmd, cfg, root, slot.StatePath, "implement",
		implement.Steps(), wfCfg, input, stepVal, "no active implement workflow found — run 'implement new' first")
}

// ensureImplementWorktrees gives a run the user starts its spec's worktrees,
// one in every repo its plan touches, before the workflow starts, so every
// step builds there. It does nothing for an orchestrated child, whose
// orchestrator made them; for a dry run; with implement.worktrees off; or
// when the spec already has a worktree record, from an earlier task run, a
// resume or `epic worktree`. Those checks read files only, so no git runs
// unless worktrees are actually made.
func ensureImplementWorktrees(root string, cfg config.Config, st store.Store, spec string, orchestrated, dryRun bool) error {
	if orchestrated || dryRun || !cfg.Implement.Worktrees {
		return nil
	}
	if _, ok, err := worktree.ReadRecord(root, spec); err != nil || ok {
		return err
	}
	touched, err := worktree.TouchedRepos(cfg, st, spec)
	if err != nil {
		return err
	}
	m, _, _, err := worktreeManager()
	if err != nil {
		return err
	}
	_, _, err = m.Ensure(spec, touched)
	return err
}

// unmergedFn reports whether a spec's worktrees are still unmerged: its
// worktree record exists, and only a successful merge removes it. It reads a
// file and never runs git. Every status view and the implement dependency
// check use it, so they always agree on whether a dependency is met.
func unmergedFn(root string) func(string) bool {
	return func(spec string) bool {
		_, ok, err := worktree.ReadRecord(root, spec)
		return ok && err == nil
	}
}

// codeRootsFor reads the spec's worktree record from the project and lists
// each recorded repo's code root, in registry order. A spec built without
// worktrees has no record, and so no code roots. It never runs git.
func codeRootsFor(root string, cfg config.Config, spec string) ([]workflow.CodeRoot, error) {
	rec, ok, err := worktree.ReadRecord(root, spec)
	if err != nil || !ok {
		return nil, err
	}
	var roots []workflow.CodeRoot
	for _, e := range cfg.Repos {
		if dir, ok := rec.Repos[e.Name]; ok {
			roots = append(roots, workflow.CodeRoot{Repo: e.Name, Root: dir})
		}
	}
	return roots, nil
}

func refuseStalePlan(cfg config.Config, st store.Store, planName string) error {
	if !cfg.Plan.StrictSpecChanges {
		return nil
	}
	raw, err := st.Read(implement.PlanFilePath(cfg.Plan.Config.Directory, planName))
	if err != nil {
		return err
	}
	fm, _, err := metadata.Split(raw)
	if err != nil {
		return err
	}
	if !status.PlanIsStale(cfg, st, planName, fm) {
		return nil
	}
	return output.NewError("plan_stale",
		fmt.Sprintf("plan %q is stale because its spek changed after approval", planName)).
		WithResource(planName).
		WithNextAction("re-run the plan workflow against the updated spek and approve the fresh plan before implementing")
}

// refuseUnmetDependencies checks the spec's direct dependencies in its epic
// before implementation starts. A standalone spec, or one whose dependencies
// are all implemented, passes silently. An unmet dependency refuses the run
// unless the caller asked to override it, and epic.strict_dependencies refuses
// even then. It runs before any workflow state is written, so a refusal
// starts nothing. On an accepted override it returns the unmet dependencies
// and their states, for the workflow to record in the changelog.
func refuseUnmetDependencies(cfg config.Config, root string, st store.Store, specName, dataStr string, override bool) ([]map[string]any, error) {
	deps, err := status.DependenciesOf(status.Options{Config: cfg, Store: st, Unmerged: unmergedFn(root)}, specName)
	if err != nil {
		return nil, err
	}
	unmet := deps.Unmet()
	if len(unmet) == 0 {
		return nil, nil
	}

	lines := make([]string, len(unmet))
	for i, dep := range unmet {
		lines[i] = fmt.Sprintf("%s depends on %s, which is %s", specName, dep.Name, dep.Description)
	}
	message := strings.Join(lines, "; ")

	implementReady := "no unmet dependency is ready to implement yet, because each still waits on its own dependencies; run `" + cfg.Command + " status " + specName + "` to see the epic's order"
	for _, dep := range unmet {
		if dep.Unmerged {
			// Its work is done: what it needs is merging back, which is the
			// user's call to make, not another implement run.
			implementReady = fmt.Sprintf(`offer to merge the implemented dependency back first: %s implement merge --data '{"name":"%s"}'`, cfg.Command, dep.Name)
			break
		}
		if dep.Ready {
			implementReady = fmt.Sprintf(`implement the first ready dependency instead: %s implement new --data '{"name":"%s"}'`, cfg.Command, dep.Name)
			break
		}
	}

	if cfg.Epic.StrictDependencies {
		if override {
			return nil, output.NewError("dependency_override_refused",
				message+"; epic.strict_dependencies is on, so implementation cannot start past an unmet dependency").
				WithResource(specName).
				WithNextAction("tell the user each unmet dependency and its state; " + implementReady)
		}
		return nil, output.NewError("dependencies_unmet", message).
			WithResource(specName).
			WithNextAction("tell the user each unmet dependency and its state; epic.strict_dependencies is on, so this spec cannot be implemented until they are; " + implementReady)
	}

	if !override {
		return nil, output.NewError("dependencies_unmet", message).
			WithResource(specName).
			WithNextAction(fmt.Sprintf("tell the user each unmet dependency and its state, and ask whether to continue anyway; if they choose to continue, re-run %s implement new --data '%s'; otherwise %s",
				cfg.Command, withOverride(dataStr), implementReady))
	}

	recorded := make([]map[string]any, len(unmet))
	for i, dep := range unmet {
		recorded[i] = map[string]any{"name": dep.Name, "state": dep.Description}
	}
	return recorded, nil
}

// withOverride returns the implement new --data payload with
// "override_dependencies": true added, keeping every other field the caller
// sent.
func withOverride(dataStr string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(dataStr), &fields); err != nil || fields == nil {
		fields = map[string]any{}
	}
	fields["override_dependencies"] = true
	raw, err := json.Marshal(fields)
	if err != nil {
		return dataStr
	}
	return string(raw)
}

// refuseUnstartableTask refuses a single-task run whose task cannot start:
// the plan has no task structure, the task is not in it, it is already
// complete, a dependency is still open, or a person must do it. It runs
// before any workflow state is written, so a refusal starts nothing.
func refuseUnstartableTask(cfg config.Config, st store.Store, planName, taskID string) error {
	raw, err := st.Read(implement.PlanFilePath(cfg.Plan.Config.Directory, planName))
	if err != nil {
		return err
	}
	_, body, err := metadata.Split(raw)
	if err != nil {
		return err
	}
	p := plantask.Parse(body)
	if err := p.RequireTasks(); err != nil {
		return err
	}

	listTasks := fmt.Sprintf("run `%s status %s --format json` to see the spec's tasks and their ids", cfg.Command, planName)
	task, ok := p.Task(taskID)
	if !ok {
		return output.NewError("task_not_found", fmt.Sprintf("plan %q has no task with id %s", planName, taskID)).
			WithResource(taskID).
			WithNextAction(listTasks)
	}
	if task.Completed {
		return output.NewError("task_completed", fmt.Sprintf("task %q (%s) is already completed", task.Title, taskID)).
			WithResource(taskID).
			WithNextAction("choose a task that is not yet completed; " + listTasks)
	}

	var open []plantask.Task
	for _, id := range task.DependsOn {
		if dep, ok := p.Task(id); ok && !dep.Completed {
			open = append(open, dep)
		}
	}
	if len(open) > 0 {
		names := make([]string, len(open))
		for i, dep := range open {
			names[i] = fmt.Sprintf("%s (%q)", dep.ID, dep.Title)
		}
		return output.NewError("task_dependencies_incomplete",
			fmt.Sprintf("task %q (%s) depends on tasks that are not completed: %s", task.Title, taskID, strings.Join(names, ", "))).
			WithResource(taskID).
			WithNextAction(fmt.Sprintf(`implement these first, starting with: %s implement new --data '{"name":"%s","task":"%s"}'`, cfg.Command, planName, open[0].ID))
	}
	if task.Execution.Type == "human" {
		return output.NewError("task_requires_human",
			fmt.Sprintf("task %q (%s) must be carried out by a person: %s", task.Title, taskID, task.Execution.Reason)).
			WithResource(taskID).
			WithNextAction("have a person carry the task out and tick it in plan.md; " + listTasks + " to find work an agent can do")
	}
	return nil
}

func runImplementSteps(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: nil,
			Output: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"steps": {Type: "array", Items: &schemaProp{Type: "string"}},
				},
			},
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	wf := workflow.New(implement.Steps(), "", workflow.Config{}, nil, nil)
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(implement.StepsResult{Steps: wf.StepNames()})
}

func init() {
	implementCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	implementCmd.PersistentFlags().BoolP("dry-run", "n", false, "Validate and preview without writing any files or persisting state")

	implementNewCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature"}')`)
	implementNewCmd.Flags().Bool("force", false, "Overwrite any in-progress workflow and start fresh")
	implementNewCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	implementNewCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")
	implementGotoCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"step":"analyze"}')`)
	implementGotoCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	implementGotoCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	implementCmd.AddCommand(implementNewCmd, implementGotoCmd, implementStepsCmd)
}
