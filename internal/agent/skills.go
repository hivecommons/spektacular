package agent

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cbroglie/mustache"
	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/stepkit"
	"github.com/jumppad-labs/spektacular/templates"
)

// workflowSkill describes one of the top-level workflow skills every supported
// agent installs.
type workflowSkill struct {
	Name         string
	TemplatePath string
}

// workflowSkills is the single source of truth for which skills every agent
// installs. All three supported agents consume these byte-identical SKILL.md
// templates.
var workflowSkills = []workflowSkill{
	{Name: "spek-new", TemplatePath: "skills/workflows/spek-new/SKILL.md"},
	{Name: "spek-plan", TemplatePath: "skills/workflows/spek-plan/SKILL.md"},
	{Name: "spek-implement", TemplatePath: "skills/workflows/spek-implement/SKILL.md"},
	{Name: "spek-knowledge", TemplatePath: "skills/workflows/spek-knowledge/SKILL.md"},
	{Name: "spek-manage-repos", TemplatePath: "skills/workflows/spek-manage-repos/SKILL.md"},
	{Name: "spek-design", TemplatePath: "skills/workflows/spek-design/SKILL.md"},
}

// sourceFS is the filesystem the install helpers read templates from. It is
// templates.FS at runtime; tests substitute a fixture FS.
var sourceFS fs.FS = templates.FS

// installWorkflowSkills writes each workflow skill into
// <projectPath>/<targetSkillsDir>/<skill-name>/SKILL.md, rendering the
// {{command}} placeholder from cfg. Partial includes such as
// {{> partials/implement-plan-documents}} resolve from sourceFS, and a missing
// partial fails the install. One line per installed file is written to out.
func installWorkflowSkills(projectPath, targetSkillsDir string, cfg config.Config, out io.Writer) error {
	for _, s := range workflowSkills {
		tmplBytes, err := fs.ReadFile(sourceFS, s.TemplatePath)
		if err != nil {
			return fmt.Errorf("reading embedded skill template %s: %w", s.TemplatePath, err)
		}

		rendered, err := mustache.RenderPartials(string(tmplBytes), stepkit.FSPartials{FS: sourceFS}, map[string]string{"command": cfg.Command})
		if err != nil {
			return fmt.Errorf("rendering skill template %s: %w", s.TemplatePath, err)
		}

		destDir := filepath.Join(projectPath, targetSkillsDir, s.Name)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("creating skill directory %s: %w", destDir, err)
		}

		destPath := filepath.Join(destDir, "SKILL.md")
		if err := os.WriteFile(destPath, []byte(rendered), 0644); err != nil {
			return fmt.Errorf("writing skill %s: %w", destPath, err)
		}

		fmt.Fprintf(out, "  Skill:    %s\n", destPath)
	}
	return nil
}
