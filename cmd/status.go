package cmd

import (
	"fmt"

	"github.com/hivecommons/spektacular/internal/autocommit"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/status"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
	"github.com/hivecommons/spektacular/internal/worktree"
	"github.com/spf13/cobra"
)

// Status formats. pretty is the default, for people; json is the document
// orchestrators decode. Both carry the same report.
const (
	statusFormatPretty = "pretty"
	statusFormatJSON   = "json"
)

var statusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Report where a piece of work stands, from the epic down to each task",
	Long: `Report where a piece of work stands. <name> may be an epic, a spec or a plan:
a spec in an epic, or its plan, reports the whole epic, with the spec asked for
named in "requested". With no name, reports the workflow in progress, or that
nothing is in progress.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

var statusWorkflowSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"kind":            {Type: "string", Enum: []string{"spec", "plan", "implement"}},
	"name":            {Type: "string"},
	"current_step":    {Type: "string"},
	"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
	"updated_at":      {Type: "string"},
}}

var statusSourcesSchema = &schemaProp{Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"uri":            {Type: "string"},
	"retrieved_date": {Type: "string"},
}}}

var statusTaskSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"id":         {Type: "string"},
	"title":      {Type: "string"},
	"milestone":  {Type: "integer"},
	"repo":       {Type: "object", Properties: map[string]*schemaProp{"name": {Type: "string"}, "location": {Type: "string"}}},
	"depends_on": {Type: "array", Items: &schemaProp{Type: "string"}},
	"execution":  {Type: "object", Properties: map[string]*schemaProp{"type": {Type: "string", Enum: []string{"agent", "human"}}, "reason": {Type: "string"}}},
	"completed":  {Type: "boolean"},
	"acceptance_criteria": {Type: "object", Properties: map[string]*schemaProp{
		"met":   {Type: "integer"},
		"total": {Type: "integer"},
	}},
}}

// statusRunPartSchema is one part, planning or implementing, of a spec's run.
var statusRunPartSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"state":        {Type: "string", Enum: []string{"done", "in_progress", "awaiting_merge", "ready", "blocked"}, Description: "awaiting_merge is for implementing only"},
	"waiting_on":   {Type: "array", Items: &schemaProp{Type: "string"}, Description: "blocked only: the specs it waits on"},
	"current_step": {Type: "string", Description: "in_progress only: the live workflow's step"},
	"root":         {Type: "string", Description: "where the work runs: the project, or the spec's worktree for its code"},
	"repos":        {Type: "array", Items: &schemaProp{Type: "string"}, Description: "implementing only: the registered repos the plan touches"},
}}

// statusRunCountsSchema totals one part across an epic.
var statusRunCountsSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"done":           {Type: "integer"},
	"in_progress":    {Type: "integer"},
	"awaiting_merge": {Type: "integer"},
	"ready":          {Type: "integer"},
	"blocked":        {Type: "integer"},
	"remaining":      {Type: "integer"},
}}

// statusEpicRunSchema is where an epic stands for planning and implementing.
var statusEpicRunSchema = &schemaProp{Type: "object", Description: "present when an epic is named", Properties: map[string]*schemaProp{
	"order":     {Type: "array", Items: &schemaProp{Type: "string"}, Description: "dependency order; ties follow the epic's list order"},
	"plan":      statusRunCountsSchema,
	"implement": statusRunCountsSchema,
	"dirty":     {Type: "boolean", Description: "a registered repo has uncommitted changes"},
	"problems": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
		"code":    {Type: "string", Enum: []string{"epic_unplanned", "epic_dependency_cycle", "epic_dependency_outside"}},
		"specs":   {Type: "array", Items: &schemaProp{Type: "string"}},
		"message": {Type: "string"},
		"blocks":  {Type: "array", Items: &schemaProp{Type: "string"}, Description: "the parts it stops; never planning"},
	}}},
}}

var statusOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"workflow":  statusWorkflowSchema,
		"requested": {Type: "string"},
		"epic": {Type: "object", Properties: map[string]*schemaProp{
			"name":            {Type: "string"},
			"document_status": {Type: "string"},
			"created_at":      {Type: "string"},
			"done":            {Type: "boolean"},
			"progress": {Type: "object", Properties: map[string]*schemaProp{
				"specs_implemented": {Type: "integer"},
				"specs_total":       {Type: "integer"},
				"tasks_completed":   {Type: "integer"},
				"tasks_total":       {Type: "integer"},
			}},
			"sources": statusSourcesSchema,
			"run":     statusEpicRunSchema,
		}},
		"specs": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"name":            {Type: "string"},
			"state":           {Type: "string", Enum: []string{"missing", "stale", "specified", "planned", "in_progress", "implemented"}},
			"document_status": {Type: "string"},
			"current_step":    {Type: "string"},
			"depends_on":      {Type: "array", Items: &schemaProp{Type: "string"}},
			"ready":           {Type: "boolean"},
			"blocked_by":      {Type: "array", Items: &schemaProp{Type: "string"}},
			"sources":         statusSourcesSchema,
			"plan": {Type: "object", Properties: map[string]*schemaProp{
				"name":            {Type: "string"},
				"document_status": {Type: "string"},
				"current_step":    {Type: "string"},
				"progress": {Type: "object", Properties: map[string]*schemaProp{
					"tasks_completed": {Type: "integer"},
					"tasks_total":     {Type: "integer"},
				}},
				"tasks": {Type: "array", Items: statusTaskSchema},
			}},
			"run": {Type: "object", Description: "what the spec still needs; present when a name is given", Properties: map[string]*schemaProp{
				"plan":      statusRunPartSchema,
				"implement": statusRunPartSchema,
			}},
		}}},
	},
}

// runStatus reports on the named epic, spec or plan, or on the workflow in
// progress when no name is given. Failures are always the JSON error
// envelope, whatever format was asked for.
func runStatus(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: statusOutputSchema,
			Flags: map[string]*schemaProp{
				"format": {Type: "string", Enum: []string{statusFormatPretty, statusFormatJSON}},
			},
		}, "")
	}

	format, _ := cmd.Flags().GetString("format")
	if format != statusFormatPretty && format != statusFormatJSON {
		return output.NewError("status_format_unsupported",
			fmt.Sprintf("status format %q is not supported; supported formats: %s, %s", format, statusFormatPretty, statusFormatJSON)).
			WithResource(format).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", statusFormatPretty, statusFormatJSON))
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	dir, err := dataDir()
	if err != nil {
		return err
	}

	opts := status.Options{
		Config: cfg,
		Store:  store.NewSourceStore(root, "project"),
		State:  status.ReadState(stateFilePath(dir)),
		Locate: status.RepoLocations(cfg, root, repoGit),
		Lane: func(kind, name string) *workflow.State {
			s, _ := workflow.ReadLane(dir, kind, name)
			return s
		},
	}
	if len(args) == 1 {
		opts.Run = statusRunSource(cfg, root, opts.Store)
	}

	var r status.Report
	if len(args) == 1 {
		r, err = status.Build(opts, args[0])
	} else {
		r, err = status.BuildCurrent(opts)
	}
	if err != nil {
		return err
	}

	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(r)
	}
	return status.RenderPretty(cmd.OutOrStdout(), r)
}

func init() {
	statusCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	statusCmd.Flags().String("format", statusFormatPretty, "Output format: pretty or json")
}

// statusRunSource is what the run view reads beyond the project store: the
// spec worktrees, the repos each plan touches,
// and whether any registered repo has uncommitted changes. status reports
// and never refuses, so anything it cannot read — no git, an unregistered
// repo — is simply absent from the view.
func statusRunSource(cfg config.Config, root string, st store.Reader) *status.RunSource {
	src := &status.RunSource{
		ProjectRoot: root,
		Touched: func(spec string) []string {
			names, err := worktree.TouchedRepos(cfg, st, spec)
			if err != nil {
				return nil
			}
			return names
		},
		Dirty: func() bool {
			targets, err := autocommit.Targets(cfg, root, autoCommitGit)
			if err != nil {
				return false
			}
			dirty, err := autocommit.DirtyTargets(targets, autoCommitGit)
			return err == nil && len(dirty) > 0
		},
	}
	if set, err := repo.New(cfg, root, repoGit); err == nil {
		m := worktree.Manager{ProjectRoot: root, Config: cfg, Repos: set, Git: worktreeGit}
		src.Worktrees = func() ([]worktree.SpecWorktrees, error) {
			all, err := m.List()
			if err != nil {
				return nil, nil
			}
			return all, nil
		}
	}
	return src
}
