package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	historicalArtifactsTemplatePath = "agents/historical-artifacts.md"
	historicalArtifactsHeading      = "## Historical Artifacts: Specs and Plans as Archaeology"
)

// installHistoricalArtifactsSection writes (or updates in place) the managed
// "Historical Artifacts: Specs and Plans as Archaeology" section in
// <projectPath>/AGENTS.md, rendering the embedded template against
// cfg.Command. Idempotent: re-running for the same projectPath leaves a single
// section and does not duplicate.
func installHistoricalArtifactsSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installStandingRule(projectPath, cfg, out, "historical-artifacts")
}
