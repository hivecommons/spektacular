package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/identifier"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/steps/spec"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

var nameRegexp = regexp.MustCompile(`^[a-z0-9_-]+$`)

const identifierInputPattern = `^[^\s/\\\x00-\x1F\x7F](?:[^/\\\x00-\x1F\x7F]*[^\s/\\\x00-\x1F\x7F])?$`

var (
	specIdentifierNow      = time.Now
	specIdentifierRandomID func() (string, error)
)

// Schema types for --schema output.
type schemaProp struct {
	Type       string                 `json:"type"`
	Enum       []string               `json:"enum,omitempty"`
	Pattern    string                 `json:"pattern,omitempty"`
	MaxLen     int                    `json:"maxLength,omitempty"`
	Items      *schemaProp            `json:"items,omitempty"`
	Properties map[string]*schemaProp `json:"properties,omitempty"`
}

type schemaObj struct {
	Type       string                 `json:"type"`
	Properties map[string]*schemaProp `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

type commandSchema struct {
	Input  *schemaObj `json:"input"`
	Output *schemaObj `json:"output"`
	// Flags describes the options a command accepts on the command line rather
	// than as JSON input, so an interface that is not wholly expressible in
	// --data is still discoverable. Omitted when empty, so a command family
	// that takes no such options publishes exactly what it published before.
	Flags map[string]*schemaProp `json:"flags,omitempty"`
}

var resultOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"step":        {Type: "string"},
		"spec_path":   {Type: "string"},
		"spec_name":   {Type: "string"},
		"instruction": {Type: "string"},
	},
}

var statusOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"spec_name":       {Type: "string"},
		"spec_path":       {Type: "string"},
		"current_step":    {Type: "string"},
		"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
		"total_steps":     {Type: "integer"},
		"progress":        {Type: "string"},
		"steps":           {Type: "array"},
	},
}

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Manage spec workflow",
	RunE:  runUnknownSubcommand,
}

var specNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new spec workflow",
	RunE:  runSpecNew,
}

var specGotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Jump to a named step",
	RunE:  runSpecGoto,
}

var specStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show current workflow progress",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSpecStatus,
}

var specStepsCmd = &cobra.Command{
	Use:   "steps",
	Short: "List available workflow step names",
	RunE:  runSpecSteps,
}

func stateFilePath(dataDir string) string {
	return filepath.Join(dataDir, "state.json")
}

type workflowDataBuffer map[string]any

func (b workflowDataBuffer) SetData(key string, value any) {
	b[key] = value
}

// readInputIntoWorkflow reads content from either --stdin or --file and stores
// it in the workflow data. --stdin <key> reads from standard input and stores
// under <key>. --file <path> reads the file at <path> (relative paths resolve
// against the process cwd) and stores under the filename's basename without
// extension. Only one of the two flags may be set at a time.
func readInputIntoWorkflow(cmd *cobra.Command, wf interface{ SetData(string, any) }) error {
	stdinKey, _ := cmd.Flags().GetString("stdin")
	filePath, _ := cmd.Flags().GetString("file")

	if stdinKey != "" && filePath != "" {
		return fmt.Errorf("--stdin and --file are mutually exclusive")
	}

	if stdinKey != "" {
		content, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}
		wf.SetData(stdinKey, string(content))
		return nil
	}

	if filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading file %s: %w", filePath, err)
		}
		base := filepath.Base(filePath)
		key := strings.TrimSuffix(base, filepath.Ext(base))
		if key == "" {
			return fmt.Errorf("--file path %q has no filename", filePath)
		}
		wf.SetData(key, string(content))
		return nil
	}

	return nil
}

func runSpecNew(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name": {Type: "string", Pattern: identifierInputPattern, MaxLen: identifier.MaxPartLength},
					"id":   {Type: "string", Pattern: identifierInputPattern, MaxLen: identifier.MaxPartLength},
				},
				Required: []string{"name"},
			},
			Output: resultOutputSchema,
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

	// Check for an in-progress workflow BEFORE requiring a name. The resume
	// check reads the single state.json and takes the in-progress name from it,
	// so it needs no name argument — running it first lets the driving agent
	// offer resume without first prompting the user for a spec name.
	statePath := stateFilePath(dataDir)
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		handled, err := probeResume(statePath, cfg.Command, "spec", force)
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
			WithNextAction(`specify the spec name with --data '{"name":"<spec_name>"}'; to see existing specs, run "spec file list"`)
	}
	var input struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	}
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}

	st := store.NewSourceStore(root, "project")
	extraData := workflowDataBuffer{}
	if err := readInputIntoWorkflow(cmd, extraData); err != nil {
		return err
	}

	resolved, err := spec.ResolveIdentifier(spec.IdentifierRequest{
		Name:     input.Name,
		ID:       input.ID,
		Method:   cfg.Spec.IDMethod,
		SpecDir:  cfg.Spec.Config.Directory,
		Store:    st,
		Now:      specIdentifierNow,
		RandomID: specIdentifierRandomID,
	})
	if err != nil {
		var notAllowed *identifier.IDNotAllowedError
		if errors.As(err, &notAllowed) {
			return output.NewError("id_not_allowed",
				fmt.Sprintf("an explicit id was supplied, but spec.id_method is %q; ids are only accepted when spec.id_method is %q", notAllowed.Method, identifier.MethodExternal)).
				WithNextAction(fmt.Sprintf(`re-run without "id" so the CLI mints a %s ID: --data '{"name":%q}'`, notAllowed.Method, input.Name))
		}
		return err
	}

	// The uncommitted-changes gate runs once the spec name is settled, so its
	// pre-workflow commit message can name the real spec, and before
	// clearState — the first thing this command writes.
	if err := startGate(cfg, root, "spec", resolved.Name, dataStr, dryRun); err != nil {
		return err
	}
	if !dryRun {
		clearState(statePath)
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "spec", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, AutoCommit: cfg.AutoCommitMode()}
	steps := spec.Steps()
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(steps, statePath, wfCfg, st, out)
	for k, v := range extraData {
		if k != "name" {
			wf.SetData(k, v)
		}
	}
	wf.SetData("name", resolved.Name)

	if err := wf.Next(); err != nil {
		return err
	}
	return nil
}

func runSpecGoto(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"step": {Type: "string", Enum: workflow.New(spec.Steps(), "", workflow.Config{}, nil, nil).StepNames()},
				},
				Required: []string{"step"},
			},
			Output: resultOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dataStr == "" {
		return output.NewError("step_required", "no step was provided").
			WithNextAction(`specify the step with --data '{"step":"<step_name>"}'; run "spec steps" to see valid step names`)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	stepVal, _ := input["step"].(string)
	if stepVal == "" {
		return output.NewError("step_required", `"step" is missing or empty in --data`).
			WithNextAction(`include a non-empty "step" in --data, e.g. --data '{"step":"<step_name>"}'; run "spec steps" to see valid step names`)
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
	// plan); resuming it from here would apply spec steps to a plan's state.
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "spec"); err != nil {
		return err
	} else if handled {
		return err
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "spec", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory, AutoCommit: cfg.AutoCommitMode()}
	return gotoWithAutoCommit(cmd, cfg, root, stateFilePath(dataDir), "spec",
		spec.Steps(), wfCfg, input, stepVal, "")
}

func runSpecStatus(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		if len(args) == 1 {
			s := commandSchema{Input: nil, Output: artifactStatusOutputSchema}
			return output.Write(cmd.OutOrStdout(), s, "")
		}
		s := commandSchema{Input: nil, Output: statusOutputSchema}
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
	steps := spec.Steps()

	if len(args) == 1 {
		st := store.NewSourceStore(root, "project")
		return runArtifactStatus(cmd, "spec", args[0], spec.SpecFilePath(cfg.Spec.Config.Directory, args[0]), stateFilePath(dataDir), cfg.Command, steps, st)
	}

	// Refuse to report on an in-progress workflow of a different kind — its
	// steps and counts would be meaningless under the spec step list.
	if handled, err := guardKind(stateFilePath(dataDir), cfg.Command, "spec"); err != nil {
		return err
	} else if handled {
		return err
	}

	wf := workflow.New(steps, stateFilePath(dataDir), workflow.Config{}, nil, nil)
	st := wf.State()

	specName, _ := wf.GetData("name")
	specPath := filepath.Join(root, spec.SpecFilePath(cfg.Spec.Config.Directory, fmt.Sprintf("%v", specName)))

	stepInfos := wf.StepStatus()
	entries := make([]spec.StepEntry, len(stepInfos))
	for i, info := range stepInfos {
		entries[i] = spec.StepEntry{Name: info.Name, Status: info.Status}
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(spec.StatusResult{
		SpecName:       fmt.Sprintf("%v", specName),
		SpecPath:       specPath,
		CurrentStep:    wf.Current(),
		CompletedSteps: st.CompletedSteps,
		TotalSteps:     len(steps),
		Progress:       fmt.Sprintf("%d/%d", len(st.CompletedSteps), len(steps)),
		Steps:          entries,
	})
}

func runSpecSteps(cmd *cobra.Command, _ []string) error {
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

	wf := workflow.New(spec.Steps(), "", workflow.Config{}, nil, nil)
	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(spec.StepsResult{Steps: wf.StepNames()})
}

func init() {
	specCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	specCmd.PersistentFlags().BoolP("dry-run", "n", false, "Validate and preview without writing any files or persisting state")

	specNewCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"my-feature"}')`)
	specNewCmd.Flags().Bool("force", false, "Overwrite any in-progress workflow and start fresh")
	specNewCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	specNewCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")
	specGotoCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"step":"requirements"}')`)
	specGotoCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	specGotoCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	specCmd.AddCommand(specNewCmd, specGotoCmd, specStatusCmd, specStepsCmd)
}
