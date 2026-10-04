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
	if err := installRepoSourcesSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installStoreAccessSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installMemoryContextSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installKnowledgeTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installSpecTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installDesignTriggerSection(projectPath, cfg, out); err != nil {
		return err
	}
	if err := installDraftPresentationSection(projectPath, cfg, out); err != nil {
		return err
	}
	return installHistoricalArtifactsSection(projectPath, cfg, out)
}

func init() {
	register(copilotAgent{})
}
