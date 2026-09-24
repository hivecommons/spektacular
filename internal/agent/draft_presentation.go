package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	draftPresentationTemplatePath = "agents/draft-presentation.md"
	draftPresentationHeading      = "## Presenting Drafts and Confirmations"
)

// installDraftPresentationSection writes (or updates in place) the managed
// "Presenting Drafts and Confirmations" section in <projectPath>/AGENTS.md,
// rendering the embedded template against cfg.Command. Idempotent:
// re-running for the same projectPath leaves a single section and does not
// duplicate.
func installDraftPresentationSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, draftPresentationTemplatePath, draftPresentationHeading, "Presenting Drafts and Confirmations section")
}
