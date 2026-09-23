package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/steps/plan"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

var planResultOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"step":        {Type: "string"},
		"plan_path":   {Type: "string"},
		"plan_name":   {Type: "string"},
		"instruction": {Type: "string"},
	},
}

var planStatusOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"plan_name":       {Type: "string"},
		"plan_path":       {Type: "string"},
		"current_step":    {Type: "string"},
		"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
		"total_steps":     {Type: "integer"},
		"progress":        {Type: "string"},
		"steps":           {Type: "array"},
	},
}

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage plan workflow",
	RunE:  runUnknownSubcommand,
}

var planNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new plan workflow",
	RunE:  runPlanNew,
}

var planGotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Jump to a named step",
	RunE:  runPlanGoto,
}

var planStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current workflow progress",
	RunE:  runPlanStatus,
}

var planStepsCmd = &cobra.Command{
	Use:   "steps",
	Short: "List available workflow step names",
	RunE:  runPlanSteps,
}

func runPlanNew(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64},
				},
				Required: []string{"name"},
			},
			Output: planResultOutputSchema,
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
	// prompting the user for a spec name.
	statePath := stateFilePath(dataDir)
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		handled, err := probeResume(statePath, cfg.Command, "plan", force)
		if err != nil {
			return err
		}
		if handled {
			return err
		}
	}

	// No workflow to resume — starting fresh requires a name.
	if dataStr == "" {
		return output.NewError("name_required", "no spec name was provided").
			WithNextAction(`specify the spec to plan against with --data '{"name":"<spec_name>"}'; to see existing specs, run "spec file list"`)
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	if input.Name == "" || !nameRegexp.MatchString(input.Name) || len(input.Name) > 64 {
		return fmt.Errorf("name must match ^[a-z0-9_-]+$ and be at most 64 characters")
	}

	// The uncommitted-changes gate runs once the name is validated, so its
	// pre-workflow commit message can name the real spec, and before
	// clearState — the first thing this command writes.
	if err := startGate(cfg, root, "plan", input.Name, dataStr, dryRun); err != nil {
		return err
	}
	if !dryRun {
		clearState(statePath)
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "plan", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, AutoCommit: cfg.AutoCommitMode()}
	steps := plan.Steps()
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(steps, statePath, wfCfg, store.NewSourceStore(root, "project"), out)
	wf.SetData("name", input.Name)

	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	if err := wf.Next(); err != nil {
		return err
	}
	return nil
}

func runPlanGoto(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"step": {Type: "string", Enum: workflow.New(plan.Steps(), "", workflow.Config{}, nil, nil).StepNames()},
				},
				Required: []string{"step"},
			},
			Output: planResultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dataStr == "" {
		return output.NewError("step_required", "no step was provided").
			WithNextAction(`specify the step with --data '{"step":"<step_name>"}'; run "plan steps" to see valid step names`)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	stepVal, _ := input["step"].(string)
	if stepVal == "" {
		return output.NewError("step_required", `"step" is missing or empty in --data`).
			WithNextAction(`include a non-empty "step" in --data, e.g. --data '{"step":"<step_name>"}'; run "plan steps" to see valid step names`)
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
	// spec); resuming it from here would apply plan steps to a spec's state.
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "plan"); err != nil {
		return err
	} else if handled {
		return err
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "plan", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, AutoCommit: cfg.AutoCommitMode()}
	return gotoWithAutoCommit(cmd, cfg, root, stateFilePath(dataDir), "plan",
		plan.Steps(), wfCfg, input, stepVal, "no active plan found — run 'plan new' first")
}

func runPlanStatus(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{Input: nil, Output: planStatusOutputSchema}
		return output.Write(cmd.OutOrStdout(), s, "")
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

	// Refuse to report on an in-progress workflow of a different kind — its
	// steps and counts would be meaningless under the plan step list.
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "plan"); err != nil {
		return err
	} else if handled {
		return err
	}

	steps := plan.Steps()
	wf := workflow.New(steps, stateFilePath(dataDir), workflow.Config{}, nil, nil)
	st := wf.State()

	nameVal, ok := wf.GetData("name")
	if !ok {
		return fmt.Errorf("no active plan found — run 'plan new' first")
	}
	planName := fmt.Sprintf("%v", nameVal)
	planPath := filepath.Join(root, plan.PlanFilePath(cfg.Plan.Config.Directory, planName))

	stepInfos := wf.StepStatus()
	entries := make([]plan.StepEntry, len(stepInfos))
	for i, info := range stepInfos {
		entries[i] = plan.StepEntry{Name: info.Name, Status: info.Status}
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(plan.StatusResult{
		PlanName:       planName,
		PlanPath:       planPath,
		CurrentStep:    wf.Current(),
		CompletedSteps: st.CompletedSteps,
		TotalSteps:     len(steps),
		Progress:       fmt.Sprintf("%d/%d", len(st.CompletedSteps), len(steps)),
		Steps:          entries,
	})
}

func runPlanSteps(cmd *cobra.Command, _ []string) error {
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

	wf := workflow.New(plan.Steps(), "", workflow.Config{}, nil, nil)
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(plan.StepsResult{Steps: wf.StepNames()})
}

func init() {
	planCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	planCmd.PersistentFlags().BoolP("dry-run", "n", false, "Validate and preview without writing any files or persisting state")

	planNewCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature"}')`)
	planNewCmd.Flags().Bool("force", false, "Overwrite any in-progress workflow and start fresh")
	planNewCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	planNewCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")
	planGotoCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"step":"discovery"}')`)
	planGotoCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	planGotoCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	planCmd.AddCommand(planNewCmd, planGotoCmd, planStatusCmd, planStepsCmd)
}
