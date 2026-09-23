package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/steps/implement"
	"github.com/jumppad-labs/spektacular/internal/store"
	"github.com/jumppad-labs/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

var uncheckedPhaseRegexp = regexp.MustCompile(`(?m)^#### - \[ \] Phase \d+\.\d+:`)

var implementResultOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"step":        {Type: "string"},
		"plan_path":   {Type: "string"},
		"plan_name":   {Type: "string"},
		"instruction": {Type: "string"},
	},
}

var implementStatusOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"plan_name":        {Type: "string"},
		"plan_path":        {Type: "string"},
		"current_step":     {Type: "string"},
		"completed_steps":  {Type: "array", Items: &schemaProp{Type: "string"}},
		"total_steps":      {Type: "integer"},
		"progress":         {Type: "string"},
		"steps":            {Type: "array"},
		"unchecked_phases": {Type: "integer"},
	},
}

var implementCmd = &cobra.Command{
	Use:   "implement",
	Short: "Manage implement workflow",
	RunE:  runUnknownSubcommand,
}

var implementNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new implement workflow against an existing plan",
	RunE:  runImplementNew,
}

var implementGotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Jump to a named step",
	RunE:  runImplementGoto,
}

var implementStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current workflow progress",
	RunE:  runImplementStatus,
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
					"name": {Type: "string", Pattern: "^[a-z0-9_-]+$", MaxLen: 64},
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
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		handled, err := resumeOrClear(statePath, cfg.Command, "implement", force)
		if err != nil {
			return err
		}
		if handled {
			return err
		}
	}

	// No workflow to resume — starting fresh requires a name.
	if dataStr == "" {
		return output.NewError("name_required", "no plan name was provided").
			WithNextAction(`specify the plan to implement with --data '{"name":"<plan_name>"}'; to see existing plans, run "plan file list"`)
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

	// Precondition: the plan file must exist before an implement workflow
	// can run against it. The workflow operates on an already-approved plan.
	// The check goes through the store, so a backend that is not a local
	// directory answers it too; Root() is used only to render the path.
	projectStore := store.NewSourceStore(root, "project")
	planRel := implement.PlanFilePath(cfg.Plan.Config.Directory, input.Name)
	if _, statErr := projectStore.Stat(planRel); statErr != nil {
		return fmt.Errorf("plan file not found at %s — run 'plan new' first or check the name", filepath.Join(root, planRel))
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "implement", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, ChangelogDir: cfg.Changelog.Config.Directory}
	steps := implement.Steps()
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(steps, statePath, wfCfg, projectStore, out)
	wf.SetData("name", input.Name)

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
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "implement"); err != nil {
		return err
	} else if handled {
		return err
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "implement", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, ChangelogDir: cfg.Changelog.Config.Directory}
	steps := implement.Steps()
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(steps, stateFilePath(dataDir), wfCfg, store.NewSourceStore(root, "project"), out)

	for k, v := range input {
		if k != "step" {
			wf.SetData(k, v)
		}
	}

	if _, ok := wf.GetData("name"); !ok {
		return fmt.Errorf("no active implement workflow found — run 'implement new' first")
	}

	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	if err := wf.Goto(stepVal); err != nil {
		return err
	}
	return nil
}

func runImplementStatus(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{Input: nil, Output: implementStatusOutputSchema}
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
	// steps and counts would be meaningless under the implement step list.
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "implement"); err != nil {
		return err
	} else if handled {
		return err
	}

	steps := implement.Steps()
	wf := workflow.New(steps, stateFilePath(dataDir), workflow.Config{}, nil, nil)
	st := wf.State()

	nameVal, ok := wf.GetData("name")
	if !ok {
		return fmt.Errorf("no active implement workflow found — run 'implement new' first")
	}
	planName := fmt.Sprintf("%v", nameVal)
	planRel := implement.PlanFilePath(cfg.Plan.Config.Directory, planName)
	planPath := filepath.Join(root, planRel)

	stepInfos := wf.StepStatus()
	entries := make([]implement.StepEntry, len(stepInfos))
	for i, info := range stepInfos {
		entries[i] = implement.StepEntry{Name: info.Name, Status: info.Status}
	}

	uncheckedPhases := 0
	if content, readErr := store.NewSourceStore(root, "project").Read(planRel); readErr == nil {
		uncheckedPhases = len(uncheckedPhaseRegexp.FindAllIndex(content, -1))
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(implement.StatusResult{
		PlanName:        planName,
		PlanPath:        planPath,
		CurrentStep:     wf.Current(),
		CompletedSteps:  st.CompletedSteps,
		TotalSteps:      len(steps),
		Progress:        fmt.Sprintf("%d/%d", len(st.CompletedSteps), len(steps)),
		Steps:           entries,
		UncheckedPhases: uncheckedPhases,
	})
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

	implementCmd.AddCommand(implementNewCmd, implementGotoCmd, implementStatusCmd, implementStepsCmd)
}
