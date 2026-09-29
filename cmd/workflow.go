package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/hivecommons/spektacular/internal/customworkflow"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

var customWorkflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Run user-defined markdown/YAML workflows",
	RunE:  runUnknownSubcommand,
}

var workflowNewCmd = &cobra.Command{
	Use:   "new <workflow>",
	Short: "Start a user-defined workflow",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflowNew,
}

var workflowGotoCmd = &cobra.Command{
	Use:   "goto <workflow>",
	Short: "Jump to a user-defined workflow step",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflowGoto,
}

var workflowStatusCmd = &cobra.Command{
	Use:   "status <workflow>",
	Short: "Show user-defined workflow progress",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflowStatus,
}

var workflowStepsCmd = &cobra.Command{
	Use:   "steps <workflow>",
	Short: "List user-defined workflow steps",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkflowSteps,
}

var workflowResultOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"workflow":    {Type: "string"},
		"run_name":    {Type: "string"},
		"step":        {Type: "string"},
		"next_steps":  {Type: "array", Items: &schemaProp{Type: "string"}},
		"instruction": {Type: "string"},
	},
}

func runWorkflowNew(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type:       "object",
				Properties: map[string]*schemaProp{"name": {Type: "string", Pattern: identifierInputPattern}},
			},
			Output: workflowResultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	workflowName := args[0]
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
	loaded, err := customworkflow.Load(workflowName, root)
	if err != nil {
		return err
	}

	statePath := stateFilePath(dataDir)
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		handled, err := probeCustomWorkflowResume(statePath, cfg.Command, workflowName, force)
		if err != nil || handled {
			return err
		}
		clearState(statePath)
	}

	input := map[string]any{}
	if dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return fmt.Errorf("parsing --data: %w", err)
		}
	}
	if err := readInputIntoWorkflow(cmd, workflowDataBuffer(input)); err != nil {
		return err
	}
	if _, ok := input["name"]; !ok {
		input["name"] = workflowName
	}
	input["workflow"] = workflowName

	wfCfg := workflow.Config{Command: cfg.Command, Kind: customworkflow.Kind(workflowName), DryRun: dryRun}
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(loaded.Steps(), statePath, wfCfg, nil, out)
	for k, v := range input {
		wf.SetData(k, v)
	}
	return wf.Next()
}

func runWorkflowGoto(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type:       "object",
				Properties: map[string]*schemaProp{"step": {Type: "string"}},
				Required:   []string{"step"},
			},
			Output: workflowResultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	workflowName := args[0]
	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dataStr == "" {
		return output.NewError("step_required", "no step was provided").
			WithNextAction(`specify the step with --data '{"step":"<step_name>"}'; run "workflow steps <workflow>" to see valid step names`)
	}
	input := map[string]any{}
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	stepVal, _ := input["step"].(string)
	if stepVal == "" {
		return output.NewError("step_required", `"step" is missing or empty in --data`).
			WithNextAction(`include a non-empty "step" in --data, e.g. --data '{"step":"<step_name>"}'; run "workflow steps <workflow>" to see valid step names`)
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
	loaded, err := customworkflow.Load(workflowName, root)
	if err != nil {
		return err
	}
	if handled, err := guardCustomWorkflowKind(stateFilePath(dataDir), cfg.Command, workflowName); err != nil || handled {
		return err
	}
	if err := readInputIntoWorkflow(cmd, workflowDataBuffer(input)); err != nil {
		return err
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: customworkflow.Kind(workflowName), DryRun: dryRun}
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(loaded.Steps(), stateFilePath(dataDir), wfCfg, nil, out)
	for k, v := range input {
		if k != "step" {
			wf.SetData(k, v)
		}
	}
	return wf.Goto(stepVal)
}

func runWorkflowStatus(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: nil,
			Output: &schemaObj{Type: "object", Properties: map[string]*schemaProp{
				"workflow":        {Type: "string"},
				"run_name":        {Type: "string"},
				"current_step":    {Type: "string"},
				"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
				"total_steps":     {Type: "integer"},
				"progress":        {Type: "string"},
				"steps":           {Type: "array"},
			}},
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	workflowName := args[0]
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
	loaded, err := customworkflow.Load(workflowName, root)
	if err != nil {
		return err
	}
	if handled, err := guardCustomWorkflowKind(stateFilePath(dataDir), cfg.Command, workflowName); err != nil || handled {
		return err
	}

	state, err := readState(stateFilePath(dataDir))
	if err != nil {
		return err
	}
	if state == nil || state.Kind != customworkflow.Kind(workflowName) {
		return output.NewError("no_active_workflow", fmt.Sprintf("no active workflow %q", workflowName)).
			WithNextAction(fmt.Sprintf("run `%s workflow new %s` to start one", cfg.Command, workflowName))
	}

	wf := workflow.New(loaded.Steps(), stateFilePath(dataDir), workflow.Config{Kind: customworkflow.Kind(workflowName)}, nil, nil)
	stepInfos := wf.StepStatus()
	entries := make([]customworkflow.StepEntry, len(stepInfos))
	for i, info := range stepInfos {
		entries[i] = customworkflow.StepEntry{Name: info.Name, Status: info.Status}
	}
	runName, _ := wf.GetData("name")
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(customworkflow.StatusResult{
		Workflow:       workflowName,
		RunName:        fmt.Sprintf("%v", runName),
		CurrentStep:    wf.Current(),
		CompletedSteps: wf.State().CompletedSteps,
		TotalSteps:     len(loaded.Steps()),
		Progress:       fmt.Sprintf("%d/%d", len(wf.State().CompletedSteps), len(loaded.Steps())),
		Steps:          entries,
	})
}

func runWorkflowSteps(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{Input: nil, Output: &schemaObj{Type: "object", Properties: map[string]*schemaProp{"workflow": {Type: "string"}, "steps": {Type: "array", Items: &schemaProp{Type: "string"}}}}}
		return output.Write(cmd.OutOrStdout(), s, "")
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	loaded, err := customworkflow.Load(args[0], root)
	if err != nil {
		return err
	}
	wf := workflow.New(loaded.Steps(), "", workflow.Config{}, nil, nil)
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(customworkflow.StepsResult{Workflow: args[0], Steps: wf.StepNames()})
}

func probeCustomWorkflowResume(statePath, command, workflowName string, force bool) (bool, error) {
	if force {
		return false, nil
	}
	state, err := detectInProgress(statePath)
	if err != nil || state == nil {
		return false, err
	}
	expected := customworkflow.Kind(workflowName)
	name, _ := state.Data["name"].(string)
	if state.Kind == expected {
		return true, output.NewError("workflow_in_progress", fmt.Sprintf("workflow %q (%q) is already in progress at step %q", workflowName, name, state.CurrentStep)).
			WithResource(name).
			WithState(state.CurrentStep, nil).
			WithNextAction(fmt.Sprintf("run `%s workflow goto %s --data '{\"step\":\"%s\"}'` to resume, or `%s workflow new %s --force` to start fresh", command, workflowName, state.CurrentStep, command, workflowName))
	}
	return true, output.NewError("cross_kind_workflow_in_progress", fmt.Sprintf("a %s workflow (%q) is in progress at step %q; cannot start workflow %q while it is active", state.Kind, name, state.CurrentStep, workflowName)).
		WithResource(name).
		WithState(state.CurrentStep, nil).
		WithNextAction(fmt.Sprintf("finish the active workflow or run `%s workflow new %s --force` to replace it", command, workflowName))
}

func guardCustomWorkflowKind(statePath, command, workflowName string) (bool, error) {
	state, err := readState(statePath)
	if err != nil {
		return false, err
	}
	expected := customworkflow.Kind(workflowName)
	if state == nil || state.Kind == "" || state.Kind == expected {
		return false, nil
	}
	name, _ := state.Data["name"].(string)
	if state.InProgress() {
		return true, output.NewError("cross_kind_workflow_in_progress", fmt.Sprintf("a %s workflow (%q) is in progress at step %q; cannot run workflow %q", state.Kind, name, state.CurrentStep, workflowName)).
			WithResource(name).
			WithState(state.CurrentStep, nil).
			WithNextAction(fmt.Sprintf("finish the active workflow or run `%s workflow new %s --force` to replace it", command, workflowName))
	}
	return true, output.NewError("no_active_workflow", fmt.Sprintf("no active workflow %q — the last workflow recorded here was %s", workflowName, state.Kind)).
		WithNextAction(fmt.Sprintf("run `%s workflow new %s` to start one", command, workflowName))
}

func init() {
	customWorkflowCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	customWorkflowCmd.PersistentFlags().BoolP("dry-run", "n", false, "Validate and preview without writing any files or persisting state")
	workflowNewCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"campaign"}')`)
	workflowNewCmd.Flags().Bool("force", false, "Overwrite any in-progress workflow and start fresh")
	workflowNewCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	workflowNewCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")
	workflowGotoCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"step":"brief"}')`)
	workflowGotoCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	workflowGotoCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	customWorkflowCmd.AddCommand(workflowNewCmd, workflowGotoCmd, workflowStatusCmd, workflowStepsCmd)
}
