package agent

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/config"
)

type claudeAgent struct{}

func (claudeAgent) Name() string { return "claude" }

func (claudeAgent) Install(projectPath string, cfg config.Config, out io.Writer) error {
	// Claude Code surfaces installed skills directly in its slash-command menu
	// (e.g. `/spek-new`), so a separate command wrapper would be redundant. Other
	// agents that lack a skill mechanism still get wrappers via installCommandWrappers.
	if err := installWorkflowSkills(projectPath, ".claude/skills", cfg, out); err != nil {
		return err
	}
	if err := installRepoSourcesSection(projectPath, cfg, out); err != nil {
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
	if err := installHistoricalArtifactsSection(projectPath, cfg, out); err != nil {
		return err
	}
	return ensureClaudeImportsAGENTS(projectPath, out)
}

// claudeAGENTSImport is the line Claude Code uses to import AGENTS.md into its
// own instruction file. Claude reads CLAUDE.md, while Spektacular installs its
// rules and the Memory & Context section into AGENTS.md, so CLAUDE.md must
// import AGENTS.md with this `@` reference for that content to take effect.
const claudeAGENTSImport = "@AGENTS.md"

// ensureClaudeImportsAGENTS makes sure CLAUDE.md imports AGENTS.md. If CLAUDE.md
// does not exist it is created with the import; if it exists but does not
// already import AGENTS.md, the import is appended without disturbing the
// existing content. Idempotent: a CLAUDE.md that already imports AGENTS.md is
// left untouched.
func ensureClaudeImportsAGENTS(projectPath string, out io.Writer) error {
	claudePath := filepath.Join(projectPath, "CLAUDE.md")

	existing, err := os.ReadFile(claudePath)
	switch {
	case os.IsNotExist(err):
		content := "# Claude Code Instructions\n\n" + claudeAGENTSImport + "\n"
		if err := writeFileAtomic(claudePath, []byte(content)); err != nil {
			return err
		}
		fmt.Fprintf(out, "  CLAUDE.md: created %s importing AGENTS.md\n", claudePath)
		return nil
	case err != nil:
		return fmt.Errorf("reading %s: %w", claudePath, err)
	}

	if strings.Contains(string(existing), claudeAGENTSImport) {
		return nil // already imports AGENTS.md — nothing to do
	}

	trimmed := bytes.TrimRight(existing, "\n\r \t")
	var buf bytes.Buffer
	buf.Write(trimmed)
	if len(trimmed) > 0 {
		buf.WriteString("\n\n")
	}
	buf.WriteString(claudeAGENTSImport)
	buf.WriteString("\n")
	if err := writeFileAtomic(claudePath, buf.Bytes()); err != nil {
		return err
	}
	fmt.Fprintf(out, "  CLAUDE.md: added AGENTS.md import to %s\n", claudePath)
	return nil
}

func init() {
	register(claudeAgent{})
}
