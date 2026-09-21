package agent

import (
	"io"

	"github.com/jumppad-labs/spektacular/internal/config"
)

const (
	designTriggerTemplatePath = "agents/design-trigger.md"
	designTriggerHeading      = "## Design-Worthy Detail Recognition"
)

// installDesignTriggerSection writes (or updates in place) the managed
// "Design-Worthy Detail Recognition" section in <projectPath>/AGENTS.md,
// rendering the embedded template against cfg.Command. Idempotent: re-running
// for the same projectPath leaves a single section and does not duplicate.
func installDesignTriggerSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, designTriggerTemplatePath, designTriggerHeading, "Design-Worthy Detail Recognition section")
}
