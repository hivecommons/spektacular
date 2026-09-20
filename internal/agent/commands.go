package agent

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cbroglie/mustache"
	"github.com/jumppad-labs/spektacular/internal/config"
)

const wrapperTemplatePath = "commands/wrapper.md"

// workflowDescriptions supplies the human-readable description rendered into
// each command wrapper so the agent's slash-command menu shows meaningful text.
var workflowDescriptions = map[string]string{
	"spek-new":          "Create a new Specification for a feature.",
	"spek-plan":         "Create a new Plan from an approved Specification.",
	"spek-implement":    "Execute an approved Plan to implement the feature.",
	"spek-knowledge":    "Search, contribute to, or update the project's knowledge base.",
	"spek-manage-repos": "Add a repo to this project through a guided conversation, inspect the registry, and repair a repo's footprint.",
	"spek-design":       "Author, bring in, revise or reference a design document.",
}

// installCommandWrappers renders the shared command wrapper once per workflow
// skill and writes each file into <projectPath>/<targetCommandsDir>. filename
// maps a skill name (e.g. "spek-plan") to the on-disk basename the agent
// expects. One line per installed file is written to out.
func installCommandWrappers(projectPath, targetCommandsDir string, filename func(skillName string) string, cfg config.Config, out io.Writer) error {
	tmplBytes, err := fs.ReadFile(sourceFS, wrapperTemplatePath)
	if err != nil {
		return fmt.Errorf("reading embedded command wrapper template %s: %w", wrapperTemplatePath, err)
	}
	tmpl := string(tmplBytes)

	destDir := filepath.Join(projectPath, targetCommandsDir)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating command directory %s: %w", destDir, err)
	}

	for _, s := range workflowSkills {
		rendered, err := mustache.Render(tmpl, map[string]string{
			"command":     cfg.Command,
			"skill":       s.Name,
			"description": workflowDescriptions[s.Name],
		})
		if err != nil {
			return fmt.Errorf("rendering command wrapper for %s: %w", s.Name, err)
		}

		destPath := filepath.Join(destDir, filename(s.Name))
		if err := os.WriteFile(destPath, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("writing command wrapper %s: %w", destPath, err)
		}

		fmt.Fprintf(out, "  Command:  %s\n", destPath)
	}
	return nil
}
