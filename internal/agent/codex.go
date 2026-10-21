package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

type codexAgent struct{}

func (codexAgent) Name() string { return "codex" }

func (codexAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".agents/skills", cfg, out); err != nil {
		return err
	}
	return installStandingRules(projectPath, cfg, out)
}

func init() {
	register(codexAgent{})
}
