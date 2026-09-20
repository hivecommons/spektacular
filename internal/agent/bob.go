package agent

import (
	"io"

	"github.com/jumppad-labs/spektacular/internal/config"
)

type bobAgent struct{}

func (bobAgent) Name() string { return "bob" }

func (bobAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".bob/skills", cfg, out); err != nil {
		return err
	}
	if err := installCommandWrappers(projectPath, ".bob/commands", bobCommandFilename, cfg, out); err != nil {
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

func bobCommandFilename(skillName string) string {
	return skillName + ".md"
}

func init() {
	register(bobAgent{})
}
