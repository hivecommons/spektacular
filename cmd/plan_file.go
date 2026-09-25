package cmd

import (
	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/plantask"
)

// The `plan file` subcommand group reads and writes plan documents (plan,
// context, research, test-plan), each addressed by the feature's bare name
// and a document name. See newStoreFileCmd for the shared implementation.
func init() {
	planCmd.AddCommand(newStoreFileCmd(storeFileKind{
		kind:      artifact.KindPlan,
		short:     "Read and write plan documents in the plan store",
		dir:       func(c config.Config) string { return c.Plan.Config.Directory },
		requireID: true,
		validate:  validatePlanDocument,
	}))
}

// validatePlanDocument refuses a plan document whose tasks break the task
// format's structural rules. Only the `plan` document is checked, and only when it is written in
// the task format: a plan whose work is still "Phase N.M" headings saves
// unvalidated, so whole-plan implement keeps ticking old plans.
func validatePlanDocument(cfg config.Config, addr artifact.Address, body []byte) error {
	if addr.Document != "plan" {
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
