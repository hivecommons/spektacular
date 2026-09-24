package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	specTriggerTemplatePath = "agents/spec-trigger.md"
	specTriggerHeading      = "## Spec-Worthy Discussion Recognition"
)

// installSpecTriggerSection writes (or updates in place) the managed
// "Spec-Worthy Discussion Recognition" section in <projectPath>/AGENTS.md,
// rendering the embedded template against cfg.Command. Idempotent:
// re-running for the same projectPath leaves a single section and does not
// duplicate.
func installSpecTriggerSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, specTriggerTemplatePath, specTriggerHeading, "Spec-Worthy Discussion Recognition section")
}
