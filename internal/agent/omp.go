package agent

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/internal/config"
)

type ompAgent struct{}

func (ompAgent) Name() string { return "omp" }

func (ompAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	if err := installWorkflowSkills(projectPath, ".omp/skills", cfg, out); err != nil {
		return err
	}
	if err := installCommandWrappers(projectPath, ".omp/commands", bobCommandFilename, cfg, out); err != nil {
		return err
	}
	if err := installOMPCommandHandlers(projectPath, out); err != nil {
		return err
	}
	// Use the same section installers as Bob, without installing Bob's files.
	for _, install := range []func(string, config.Config, io.Writer) error{
		installRepoSourcesSection, installStoreAccessSection, installMemoryContextSection,
		installKnowledgeTriggerSection, installSpecTriggerSection, installDesignTriggerSection,
		installDraftPresentationSection, installHistoricalArtifactsSection,
	} {
		if err := install(projectPath, cfg, out); err != nil {
			return err
		}
	}
	return installOMPRules(projectPath, out)
}

// omp selects only one instruction file per directory. Always-applied rules
// keep our sections visible without taking that slot away from the project.
// Each rule carries exactly one installed section: omp's content deduplication
// can then suppress it when AGENTS.md already contains it, even if the project
// has its own sections between ours. Never copy the project's private text.
func installOMPRules(projectPath string, out io.Writer) error {
	body, err := os.ReadFile(filepath.Join(projectPath, "AGENTS.md"))
	if err != nil {
		return fmt.Errorf("reading omp standing rules: %w", err)
	}
	dir := filepath.Join(projectPath, ".omp", "rules")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating omp rules directory: %w", err)
	}
	for _, section := range []struct{ name, heading string }{
		{"repo-sources", repoSourcesHeading},
		{"store-access", storeAccessHeading},
		{"memory-context", memoryContextHeading},
		{"knowledge-trigger", knowledgeTriggerHeading},
		{"spec-trigger", specTriggerHeading},
		{"design-trigger", designTriggerHeading},
		{"draft-presentation", draftPresentationHeading},
		{"historical-artifacts", historicalArtifactsHeading},
	} {
		start, end, found := locateManagedSection(body, section.heading)
		if !found {
			return fmt.Errorf("missing omp standing-rule section %q", section.heading)
		}
		content := "---\nalwaysApply: true\n---\n\n" + strings.TrimSpace(string(body[start:end])) + "\n"
		path := filepath.Join(dir, "spek-"+section.name+".md")
		if err := writeFileAtomic(path, []byte(content)); err != nil {
			return err
		}
		fmt.Fprintf(out, "  Rule:     %s\n", path)
	}
	return nil
}

// Markdown wrappers alone normalize quotes and whitespace in omp. Its native
// custom-command API supplies rawArgs, preserving the user's complete input.
// Keeping the Markdown wrappers also refreshes hand-made copies of Bob's layout.
func installOMPCommandHandlers(projectPath string, out io.Writer) error {
	tmpl, err := fs.ReadFile(sourceFS, "commands/omp-wrapper.ts")
	if err != nil {
		return fmt.Errorf("reading omp command template: %w", err)
	}
	for _, skill := range workflowSkills {
		body, err := mustache.Render(string(tmpl), map[string]string{
			"nameJSON":        strconv.Quote(skill.Name),
			"descriptionJSON": strconv.Quote(workflowDescriptions[skill.Name]),
			"instructionJSON": strconv.Quote("Run the `" + skill.Name + "` skill."),
		})
		if err != nil {
			return fmt.Errorf("rendering omp command %s: %w", skill.Name, err)
		}
		dir := filepath.Join(projectPath, ".omp", "commands", skill.Name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating omp command directory: %w", err)
		}
		path := filepath.Join(dir, "index.ts")
		if err := writeFileAtomic(path, []byte(body)); err != nil {
			return err
		}
		fmt.Fprintf(out, "  Command:  %s\n", path)
	}
	return nil
}

func init() { register(ompAgent{}) }
