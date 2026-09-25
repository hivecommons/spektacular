package cmd

import (
	"errors"
	"fmt"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/plantask"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/steps/plan"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/spf13/cobra"
)

// Export formats. pretty is the default, for people; json is the document
// orchestrators decode.
const (
	exportFormatPretty = "pretty"
	exportFormatJSON   = "json"
)

var planExportCmd = &cobra.Command{
	Use:   "export <name>",
	Short: "Export a plan's task graph",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlanExport,
}

var planExportOutputSchema = &schemaObj{
	Type: "object",
	Properties: map[string]*schemaProp{
		"kind":            {Type: "string"},
		"name":            {Type: "string"},
		"document_status": {Type: "string"},
		"tasks": {Type: "array", Items: &schemaProp{Type: "object", Properties: map[string]*schemaProp{
			"id":         {Type: "string"},
			"title":      {Type: "string"},
			"milestone":  {Type: "integer"},
			"repo":       {Type: "object", Properties: map[string]*schemaProp{"name": {Type: "string"}, "location": {Type: "string"}}},
			"depends_on": {Type: "array", Items: &schemaProp{Type: "string"}},
			"execution":  {Type: "object", Properties: map[string]*schemaProp{"type": {Type: "string", Enum: []string{"agent", "human"}}, "reason": {Type: "string"}}},
			"completed":  {Type: "boolean"},
		}}},
	},
}

// runPlanExport parses the named plan at call time — nothing is stored
// alongside it — and prints its tasks in the requested format. Failures are
// always the JSON error envelope, whatever format was asked for.
func runPlanExport(cmd *cobra.Command, args []string) error {
	if schema, _ := cmd.Flags().GetBool("schema"); schema {
		return output.Write(cmd.OutOrStdout(), commandSchema{
			Output: planExportOutputSchema,
			Flags: map[string]*schemaProp{
				"format": {Type: "string", Enum: []string{exportFormatPretty, exportFormatJSON}},
			},
		}, "")
	}

	format, _ := cmd.Flags().GetString("format")
	if format != exportFormatPretty && format != exportFormatJSON {
		return output.NewError("export_format_unsupported",
			fmt.Sprintf("export format %q is not supported; supported formats: %s, %s", format, exportFormatPretty, exportFormatJSON)).
			WithResource(format).
			WithNextAction(fmt.Sprintf("re-run with --format %s or --format %s", exportFormatPretty, exportFormatJSON))
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	root, err := projectRoot()
	if err != nil {
		return err
	}
	name := args[0]
	st := store.NewSourceStore(root, "project")
	raw, err := st.Read(plan.PlanFilePath(cfg.Plan.Config.Directory, name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return output.NewError("artifact_not_found", fmt.Sprintf("plan artifact %q was not found", name)).
				WithResource(name).
				WithNextAction(fmt.Sprintf("run `%s plan file list` to see available plans", cfg.Command))
		}
		return err
	}
	fm, body, err := metadata.Split(raw)
	if err != nil {
		return output.NewError("metadata_read_failed", fmt.Sprintf("could not read metadata for plan artifact %q: %v", name, err)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("repair the plan's frontmatter, then re-run `%s plan export %s`", cfg.Command, name))
	}

	parsed := plantask.Parse(body)
	if err := parsed.RequireTasks(); err != nil {
		return err
	}

	status := resolveDocumentStatus(fm, strictPlanStatusHook(cfg, st, name))
	export := plantask.NewExport(name, string(status), parsed, repoLocations(cfg, root))

	if format == exportFormatJSON {
		return output.New(cmd.OutOrStdout(), globalFields).WriteResult(export)
	}
	return plantask.RenderPretty(cmd.OutOrStdout(), export)
}

// repoLocations maps a registered repo name to its declared git source: the
// location in its repo.yaml when that source's provider is git, and "" for a
// file source, no source, or a repo that is not on disk. It never clones and
// never asks git, so a checkout's remotes are never mistaken for a declared
// location.
func repoLocations(cfg config.Config, root string) func(string) string {
	set, err := repo.New(cfg, root, repoGit)
	if err != nil {
		return func(string) string { return "" }
	}
	return func(name string) string {
		meta, ok := set.DescriptiveMetadata(name)
		if !ok {
			return ""
		}
		kind, location, err := meta.ParseSource("")
		if err != nil || kind != config.SourceGit {
			return ""
		}
		return location
	}
}

func init() {
	planExportCmd.Flags().String("format", exportFormatPretty, "Output format: pretty or json")
	planCmd.AddCommand(planExportCmd)
}
