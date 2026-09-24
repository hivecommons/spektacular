package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	reposteps "github.com/hivecommons/spektacular/internal/steps/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage the project's registered member repos",
	RunE:  runUnknownSubcommand,
}

var repoAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Register a repo in the project config and create its minimal footprint",
	RunE:  runRepoAdd,
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the project's registered repos with metadata and the resolved root of each repo's code",
	RunE:  runRepoList,
}

var repoNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Start a guided add that registers a repo by proposing its details",
	RunE:  runRepoNew,
}

var repoGotoCmd = &cobra.Command{
	Use:   "goto",
	Short: "Advance a guided add to a named step",
	RunE:  runRepoGoto,
}

// repoGit is the GitRunner the repo commands resolve with; a variable so
// tests can substitute a fake.
var repoGit repo.GitRunner = repo.NewGitRunner()

var repoAddInputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"name":         {Type: "string"},
		"location":     {Type: "string"},
		"source":       {Type: "string"},
		"description":  {Type: "string"},
		"role":         {Type: "string"},
		"tags":         {Type: "array", Items: &schemaProp{Type: "string"}},
		"dependencies": {Type: "array", Items: &schemaProp{Type: "string"}},
	},
	Required: []string{"name", "location"},
}

var repoAddOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"registered":    {Type: "boolean"},
		"footprint":     {Type: "string", Enum: []string{repo.FootprintCreated, repo.FootprintRepaired, repo.FootprintUnchanged}},
		"metadata_note": {Type: "string"},
	},
}

// repoWorkflowOutputSchema is the shape both guided add subcommands return:
// the step just rendered, the repo being added, and the instruction to carry
// out. It mirrors the spec and plan workflows' result schema.
var repoWorkflowOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"step":        {Type: "string"},
		"repo_path":   {Type: "string"},
		"repo_name":   {Type: "string"},
		"instruction": {Type: "string"},
	},
}

var repoListOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"repos": {
			Type: "array",
			Items: &schemaProp{
				Type: "object",
				Properties: map[string]*schemaProp{
					"name":          {Type: "string"},
					"location":      {Type: "string"},
					"root":          {Type: "string"},
					"provider":      {Type: "string"},
					"description":   {Type: "string"},
					"role":          {Type: "string"},
					"tags":          {Type: "array", Items: &schemaProp{Type: "string"}},
					"dependencies":  {Type: "array", Items: &schemaProp{Type: "string"}},
					"materialized":  {Type: "boolean"},
					"stale_note":    {Type: "string"},
					"metadata_note": {Type: "string"},
				},
			},
		},
	},
}

// repoAddInput is the --data payload for repo add: the registry entry's
// identity and location, plus the fields written into the repo's own
// config — its descriptive metadata and, when given, the source its code
// lives at.
type repoAddInput struct {
	Name         string   `json:"name"`
	Location     string   `json:"location"`
	Source       string   `json:"source"`
	Description  string   `json:"description"`
	Role         string   `json:"role"`
	Tags         []string `json:"tags"`
	Dependencies []string `json:"dependencies"`
}

// repoInfo is the list projection agents consume for cross-repo attribution.
type repoInfo struct {
	Name         string   `json:"name"`
	Location     string   `json:"location,omitempty"`
	Root         string   `json:"root"`
	Provider     string   `json:"provider,omitempty"`
	Description  string   `json:"description,omitempty"`
	Role         string   `json:"role,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Materialized bool     `json:"materialized"`
	StaleNote    string   `json:"stale_note,omitempty"`
	MetadataNote string   `json:"metadata_note,omitempty"`
}

func runRepoAdd(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: repoAddInputSchema, Output: repoAddOutputSchema}, "")
	}

	input, err := repoAddData(cmd)
	if err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	root, err := projectRoot()
	if err != nil {
		return err
	}

	res, err := repo.Register(&cfg, root, repoGit, repo.Registration{
		Name:         input.Name,
		Location:     input.Location,
		Source:       input.Source,
		Description:  input.Description,
		Role:         input.Role,
		Tags:         input.Tags,
		Dependencies: input.Dependencies,
	})
	if err != nil {
		return err
	}

	result := map[string]any{"registered": res.Registered, "footprint": res.Footprint}
	if res.MetadataNote != "" {
		result["metadata_note"] = res.MetadataNote
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(result)
}

// repoStateFilePath is the guided add's own workflow state, a sibling of the
// state.json that spec, plan and implement share. Keeping it separate is what
// lets an add be started while one of those is in progress without either
// disturbing the other.
func repoStateFilePath(dataDir string) string {
	return filepath.Join(dataDir, "repo-state.json")
}

func runRepoNew(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"location": {Type: "string"},
				},
			},
			Output: repoWorkflowOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	dir, err := dataDir()
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

	// Check for an add already in progress before doing anything else, so the
	// driving agent can offer to resume it rather than silently starting over.
	statePath := repoStateFilePath(dir)
	if dryRun {
		statePath += ".dryrun-tmp"
	} else {
		handled, err := resumeOrClear(statePath, cfg.Command, "repo", force)
		if err != nil {
			return err
		}
		if handled {
			return err
		}
	}

	// Unlike a spec, an add needs no name up front: the only thing it can be
	// given is the repo to add, and even that is optional.
	input := map[string]any{}
	if dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
			return fmt.Errorf("parsing --data: %w", err)
		}
	}

	st := store.NewSourceStore(root, "project")
	wfCfg := workflow.Config{Command: cfg.Command, Kind: "repo", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory}
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(reposteps.Steps(), statePath, wfCfg, st, out)

	for k, v := range input {
		wf.SetData(k, v)
	}
	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	return wf.Next()
}

func runRepoGoto(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		s := commandSchema{
			Input: &schemaObj{
				Type: "object",
				Properties: map[string]*schemaProp{
					"step": {Type: "string", Enum: workflow.New(reposteps.Steps(), "", workflow.Config{}, nil, nil).StepNames()},
				},
				Required: []string{"step"},
			},
			Output: repoWorkflowOutputSchema,
		}
		return output.Write(cmd.OutOrStdout(), s, "")
	}

	dataStr, _ := cmd.Flags().GetString("data")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if dataStr == "" {
		return output.NewError("step_required", "no step was provided").
			WithNextAction(`specify the step with --data '{"step":"<step_name>"}'`)
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return fmt.Errorf("parsing --data: %w", err)
	}
	stepVal, _ := input["step"].(string)
	if stepVal == "" {
		return output.NewError("step_required", `"step" is missing or empty in --data`).
			WithNextAction(`include a non-empty "step" in --data, e.g. --data '{"step":"description"}'`)
	}

	dir, err := dataDir()
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

	if handled, err := guardKind(repoStateFilePath(dir), cfg.Command, "repo"); err != nil {
		return err
	} else if handled {
		return err
	}

	wfCfg := workflow.Config{Command: cfg.Command, Kind: "repo", DryRun: dryRun, SpecDir: cfg.Spec.Config.Directory, PlanDir: cfg.Plan.Config.Directory}
	out := output.New(cmd.OutOrStdout(), globalFields)
	wf := workflow.New(reposteps.Steps(), repoStateFilePath(dir), wfCfg, store.NewSourceStore(root, "project"), out)

	// Every field but the step is an answer the user just agreed to.
	for k, v := range input {
		if k != "step" {
			wf.SetData(k, v)
		}
	}
	if err := readInputIntoWorkflow(cmd, wf); err != nil {
		return err
	}

	return wf.Goto(stepVal)
}

func runRepoList(cmd *cobra.Command, _ []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{Input: nil, Output: repoListOutputSchema}, "")
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	set, err := repo.New(cfg, root, repoGit)
	if err != nil {
		return err
	}

	// Listing is inspection and must stay side-effect-free: entries whose
	// location is on disk resolve to their root (no cloning happens for
	// them), while repos whose location is missing are reported unresolved
	// rather than triggering any side effect.
	infos := make([]repoInfo, 0, len(set.Entries()))
	for _, e := range set.Entries() {
		info := repoInfo{
			Name:         e.Name,
			Location:     e.ResolvedLocation(root),
			Dependencies: e.Dependencies,
		}
		// A registered repo whose location is not on disk, or whose footprint
		// is missing or unreadable, is a misregistration: report it as an
		// error naming the path that was looked at, rather than listing the
		// repo with an empty root and no metadata as if that were fine.
		meta, err := set.Footprint(e.Name)
		if err != nil {
			if refusal := formatRefusal(err); refusal != nil {
				return refusal
			}
			var fpErr *repo.FootprintError
			if errors.As(err, &fpErr) {
				return output.NewError("repo_footprint_missing", fpErr.Error()).
					WithResource(fpErr.Root).
					WithNextAction(fmt.Sprintf("expected %s; run 'repo add' with name %q and location %q to scaffold it, or correct repos[].location for %q", filepath.Join(fpErr.Root, config.RepoConfigFileName), e.Name, e.Location, e.Name))
			}
			return err
		}
		// Descriptive metadata comes from the repo's own config at its
		// location, the single source now that the registry carries
		// membership only.
		// The reported provider is the one the repo declares for its own
		// source, so it says how the code at `root` was reached: `file` for a
		// directory, `git` for a clone. A repo declaring no source has none.
		info.Provider = meta.Source.Provider
		info.Description = meta.Description
		info.Role = meta.Role
		info.Tags = meta.Tags
		if repo.DescriptiveFieldsEmpty(meta) {
			info.MetadataNote = fmt.Sprintf("repo %q has no descriptive metadata set; run 'repo add' with description/role/tags to describe it", e.Name)
		}
		// The reported root is the repo's code: its source when one is
		// declared, otherwise the location itself. A git source that has
		// not been cloned yet is reported unresolved rather than cloned —
		// resolution only runs once the code is already on disk.
		if _, ok := set.LocalSource(e.Name); ok {
			resolved, err := set.Resolve(e.Name)
			if err != nil {
				return err
			}
			info.Root = resolved.Source
			info.Materialized = resolved.Materialized
			info.StaleNote = resolved.StaleNote
		}
		infos = append(infos, info)
	}

	out := output.New(cmd.OutOrStdout(), globalFields)
	return out.WriteResult(map[string]any{"repos": infos})
}

// repoAddData parses and validates the --data flag for repo add.
func repoAddData(cmd *cobra.Command) (repoAddInput, error) {
	const example = `{"name":"docs","location":"../repos/docs","source":"git@example.com:org/docs.git","description":"the documentation repo"}`
	dataStr, _ := cmd.Flags().GetString("data")
	if dataStr == "" {
		return repoAddInput{}, fmt.Errorf(`--data is required (e.g. --data '%s')`, example)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(dataStr), &raw); err != nil {
		return repoAddInput{}, fmt.Errorf("parsing --data: %w", err)
	}
	if addr, ok := raw["address"]; ok {
		return repoAddInput{}, output.NewError("invalid_data", "--data no longer accepts \"address\"; a repo's git origin is its \"source\", written into the repo's own repo.yaml").
			WithNextAction(fmt.Sprintf(`re-run with --data '{"name":%q,"location":"<folder holding the repo's .spektacular/>","source":"%v"}'`, raw["name"], addr))
	}
	var input repoAddInput
	if err := json.Unmarshal([]byte(dataStr), &input); err != nil {
		return repoAddInput{}, fmt.Errorf("parsing --data: %w", err)
	}
	if input.Name == "" {
		return repoAddInput{}, fmt.Errorf(`--data must include a non-empty "name"`)
	}
	if input.Location == "" {
		return repoAddInput{}, output.NewError("invalid_data", fmt.Sprintf(`--data must include a non-empty "location": the folder holding %q's .spektacular/ directory (created if missing)`, input.Name)).
			WithNextAction(fmt.Sprintf(`re-run with --data '%s'`, example))
	}
	return input, nil
}

func init() {
	repoCmd.PersistentFlags().Bool("schema", false, "Print the input/output schema for this subcommand and exit")
	repoCmd.PersistentFlags().BoolP("dry-run", "n", false, "Validate and preview without writing any files or persisting state")

	repoAddCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"name":"docs","location":"/path/to/docs","description":"the documentation repo"}')`)

	repoNewCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"location":"/path/to/docs"}')`)
	repoNewCmd.Flags().Bool("force", false, "Discard any in-progress guided add and start fresh")
	repoNewCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	repoNewCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	repoGotoCmd.Flags().StringP("data", "d", "", `JSON input (e.g. '{"step":"description"}')`)
	repoGotoCmd.Flags().String("stdin", "", "Read stdin and store it in workflow data under this key")
	repoGotoCmd.Flags().String("file", "", "Read a file at <path> (relative to cwd) and store its contents under the filename's basename (without extension)")

	repoCmd.AddCommand(repoAddCmd, repoListCmd, repoNewCmd, repoGotoCmd)
}
