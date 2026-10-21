package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

// copilotAgent uses Copilot CLI's native project skills. AGENTS.md is loaded
// directly by Copilot; no extra instruction file or command wrapper is needed.
type copilotAgent struct{}

func (copilotAgent) Name() string { return "copilot" }

func (copilotAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".github/skills", cfg, out); err != nil {
		return err
	}
	return installStandingRules(projectPath, cfg, out)
}

func init() {
	register(copilotAgent{})
}
