package cmd

import (
	"fmt"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/status"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/workflow"
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
