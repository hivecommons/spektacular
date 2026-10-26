package agent

import (
	"io"

	"github.com/hivecommons/spektacular/internal/config"
)

type cursorAgent struct{}

func (cursorAgent) Name() string { return "cursor" }

// Install writes the workflow skills into .cursor/skills, where Cursor
// discovers them and lists each in its `/` menu, so no command wrappers are
// needed. Cursor reads the project's root AGENTS.md natively, in the editor
// and in the CLI alike, so the standing-rule sections go there and no
// .cursor/rules file is written.
func (cursorAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".cursor/skills", cfg, out); err != nil {
		return err
	}
	for _, install := range []func(string, config.Config, io.Writer) error{
		installRepoSourcesSection, installStoreAccessSection, installMemoryContextSection,
		installKnowledgeTriggerSection, installSpecTriggerSection, installDesignTriggerSection,
		installDraftPresentationSection, installHistoricalArtifactsSection,
	} {
		if err := install(projectPath, cfg, out); err != nil {
			return err
		}
	}
	return nil
}

func init() { register(cursorAgent{}) }
