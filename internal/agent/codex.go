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
	register(codexAgent{})
}
