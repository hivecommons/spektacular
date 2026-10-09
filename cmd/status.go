package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

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
named in "requested". With no name, reports every workflow in progress, the
shared one and each spec's own run, with the first one's epic or spec in full,
or that nothing is in progress.

--watch keeps the readable tree on screen and refreshes it every --interval
(2s by default) until interrupted with Ctrl+C.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

var statusWorkflowSchema = &schemaProp{Type: "object", Properties: map[string]*schemaProp{
	"kind":            {Type: "string", Enum: []string{"spec", "plan", "implement"}},
	"name":            {Type: "string"},
	"current_step":    {Type: "string"},
	"completed_steps": {Type: "array", Items: &schemaProp{Type: "string"}},
	"updated_at":      {Type: "string"},
	"orchestrated":    {Type: "boolean", Description: "an epic orchestrator started it"},
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
	"order":       {Type: "array", Items: &schemaProp{Type: "string"}, Description: "dependency order; ties follow the epic's list order"},
	"plan":        statusRunCountsSchema,
	"implement":   statusRunCountsSchema,
	"dirty":       {Type: "boolean", Description: "a repo touched by this epic's plans has uncommitted changes"},
	"dirty_repos": {Type: "array", Items: &schemaProp{Type: "string"}, Description: "the repos touched by this epic's plans that have uncommitted changes; empty when none"},
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
		"workflows": {Type: "array", Items: statusWorkflowSchema, Description: "no name only: every workflow in progress, the shared one first, then each spec's lane, most recently updated first; workflow is its first entry"},
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
				"format":   {Type: "string", Enum: []string{statusFormatPretty, statusFormatJSON}},
				"watch":    {Type: "boolean", Description: "redraw the readable tree every interval until interrupted; pretty only"},
				"interval": {Type: "string", Description: "how often --watch refreshes, as a Go duration such as 2s; at least 200ms"},
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

	watch, _ := cmd.Flags().GetBool("watch")
	interval, _ := cmd.Flags().GetDuration("interval")
	if watch && format != statusFormatPretty {
		return output.NewError("status_watch_format_unsupported",
			fmt.Sprintf("--watch redraws the readable tree and cannot be used with --format %s", format)).
			WithResource(format).
			WithNextAction("re-run with --watch alone, or poll status --format json yourself")
	}
	if watch && interval < minWatchInterval {
		return output.NewError("status_interval_invalid",
			fmt.Sprintf("--interval %s is too short; the shortest is %s", interval, minWatchInterval)).
			WithResource(interval.String()).
			WithNextAction(fmt.Sprintf("re-run with --interval %s or longer", minWatchInterval))
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

	if watch {
		// The frames go straight to the terminal: the debug session log
		// would otherwise keep every frame of a watch that runs for hours.
		term := outputTerminal(cmd)
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		title := "spektacular status"
		if len(args) == 1 {
			title += " " + args[0]
		}
		return watchStatus(ctx, term, interval, func() string {
			var b bytes.Buffer
			b.WriteString(status.WatchHeader(term, title, interval, time.Now()) + "\n\n")
			r, err := buildStatusReport(cfg, root, dir, args)
			if err != nil {
				b.WriteString(status.WatchError(term, toErrorResponse(err).Message) + "\n")
				return b.String()
			}
			if err := status.RenderPrettyStyled(&b, r, term); err != nil {
				b.WriteString(status.WatchError(term, err.Error()) + "\n")
			}
			return b.String()
		})
	}

	r, err := buildStatusReport(cfg, root, dir, args)
	if err != nil {
		return err
	}

	if format == statusFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(r)
	}
	return status.RenderPrettyStyled(cmd.OutOrStdout(), r, outputTerminal(cmd))
}

// buildStatusReport reads the project afresh and reports on the named epic,
// spec or plan, or on the workflows in progress when no name is given.
func buildStatusReport(cfg config.Config, root, dir string, args []string) (status.Report, error) {
	opts := status.Options{
		Config: cfg,
		Store:  store.NewSourceStore(root, "project"),
		State:  status.ReadState(stateFilePath(dir)),
		Locate: status.RepoLocations(cfg, root, repoGit),
		Lane: func(kind, name string) *workflow.State {
			s, _ := workflow.ReadLane(dir, kind, name)
			return s
		},
		LaneNames: func(kind string) []string {
			return workflow.LaneNames(dir, kind)
		},
		Unmerged: unmergedFn(root),
	}
	if len(args) == 1 {
		opts.Run = statusRunSource(cfg, root, opts.Store)
		return status.Build(opts, args[0])
	}
	return status.BuildCurrent(opts)
}

// terminalOut is the command's own output writer while the debug session log
// tees it into a buffer. The tee hides whether output goes to a terminal, so
// colour is decided from this writer instead.
var terminalOut io.Writer

// outputTerminal is the writer whose terminal decides whether readable
// output is coloured.
func outputTerminal(cmd *cobra.Command) io.Writer {
	if terminalOut != nil {
		return terminalOut
	}
	return cmd.OutOrStdout()
}

func init() {
	statusCmd.Flags().Bool("schema", false, "Print the input/output schema and exit")
	statusCmd.Flags().String("format", statusFormatPretty, "Output format: pretty or json")
	statusCmd.Flags().Bool("watch", false, "Keep the readable tree on screen, refreshed every --interval, until interrupted")
	statusCmd.Flags().Duration("interval", defaultWatchInterval, "How often --watch refreshes")
}

// statusRunSource is what the run view reads beyond the project store: the
// spec worktrees, the repos each plan touches,
// and which of the touched repos have uncommitted changes. status reports
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
		Dirty: func(names []string) []string {
			if len(names) == 0 {
				return nil
			}
			touched := map[string]bool{}
			for _, name := range names {
				touched[name] = true
			}
			// Only the touched repos are resolved, so a dirty checkout that
			// holds none of them is never looked at, and repos sharing a
			// checkout with a touched one are not reported.
			filtered := cfg
			filtered.Repos = nil
			for _, entry := range cfg.Repos {
				if touched[entry.Name] {
					filtered.Repos = append(filtered.Repos, entry)
				}
			}
			targets, err := autocommit.Targets(filtered, root, autoCommitGit)
			if err != nil {
				return nil
			}
			// Only code counts: a worktree branches from the last commit, so
			// uncommitted code would be missing from it, while specs, plans,
			// progress and the orchestrator's notes are read from the project
			// and written there throughout the run.
			dirty, err := autocommit.CodeDirtyTargets(targets, autoCommitGit)
			if err != nil {
				return nil
			}
			var result []string
			for _, target := range dirty {
				result = append(result, target.Repos...)
			}
			return result
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
