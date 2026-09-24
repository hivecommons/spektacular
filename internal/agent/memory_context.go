package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	memoryContextTemplatePath = "agents/memory-context.md"
	memoryContextHeading      = "## Memory & Context"
)

// installMemoryContextSection writes (or updates in place) the managed
// "Memory & Context" section in <projectPath>/AGENTS.md, rendering the
// embedded template against cfg.Command. Idempotent: re-running for the
// same projectPath leaves a single section and does not duplicate.
func installMemoryContextSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, memoryContextTemplatePath, memoryContextHeading, "Memory & Context section")
}
