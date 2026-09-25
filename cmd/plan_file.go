package cmd

import (
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/plantask"
)

// The `plan file` subcommand group reads and writes plan documents
// (plan.md, context.md, research.md) within the configured plan directory.
// See newStoreFileCmd for the shared implementation.
func init() {
	planCmd.AddCommand(newStoreFileCmd(
		"Read and write files in the plan store",
		func(c config.Config) string { return c.Plan.Config.Directory },
		true,
		false,
		validatePlanDocument,
	))
}

// validatePlanDocument refuses a plan.md whose tasks break the task format's
// structural rules. Only plan.md is checked, and only when it is written in
// the task format: a plan whose work is still "Phase N.M" headings saves
// unvalidated, so whole-plan implement keeps ticking old plans.
func validatePlanDocument(cfg config.Config, docPath string, body []byte) error {
	if filepath.Base(docPath) != "plan.md" {
		return nil
	}
	p := plantask.Parse(body)
	if p.Format != plantask.FormatTasks {
		return nil
	}
	repos := make([]string, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repos = append(repos, r.Name)
	}
	return plantask.Validate(p, repos)
}
