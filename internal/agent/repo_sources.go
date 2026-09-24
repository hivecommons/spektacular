package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

const (
	repoSourcesTemplatePath = "agents/repo-sources.md"
	repoSourcesHeading      = "## Where the Code Lives — Read This First"
)

// installRepoSourcesSection writes (or updates in place) the managed "Where
// the Code Lives" section in <projectPath>/AGENTS.md, rendering the embedded
// template against cfg.Command. It is placed at the top of the file, ahead of
// every other section: a project's code often does not live in the directory
// the agent starts in, and an agent that reads this rule after it has already
// opened the wrong tree has read it too late. Idempotent: re-running for the
// same projectPath leaves a single section, and hoists one an older init left
// further down the file.
func installRepoSourcesSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSectionAt(projectPath, cfg, out, repoSourcesTemplatePath, repoSourcesHeading, "Where the Code Lives section", placeAtTop)
}
