package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	knowledgeTriggerTemplatePath = "agents/knowledge-trigger.md"
	knowledgeTriggerHeading      = "## Knowledge-Worthy Discovery Recognition"
)

// installKnowledgeTriggerSection writes (or updates in place) the managed
// "Knowledge-Worthy Discovery Recognition" section in
// <projectPath>/AGENTS.md, rendering the embedded template against
// cfg.Command. Idempotent: re-running for the same projectPath leaves a
// single section and does not duplicate.
func installKnowledgeTriggerSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, knowledgeTriggerTemplatePath, knowledgeTriggerHeading, "Knowledge-Worthy Discovery Recognition section")
}
