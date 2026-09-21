package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

func TestClaudeAgent_Name(t *testing.T) {
	require.Equal(t, "claude", claudeAgent{}.Name())
}

func TestClaudeAgent_Install(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.NewDefault()

	err := claudeAgent{}.Install(tmp, cfg, io.Discard)
	require.NoError(t, err)

	// Exactly six SKILL.md files under .claude/skills/spek-*/.
	skillAssertions := map[string]string{
		"spek-new":          "spektacular spec new",
		"spek-plan":         "spektacular plan new",
		"spek-implement":    "spektacular implement new",
		"spek-knowledge":    "knowledge",
		"spek-manage-repos": "repo add",
		"spek-design":       "spektacular design author",
	}
	for skill, expected := range skillAssertions {
		skillPath := filepath.Join(tmp, ".claude", "skills", skill, "SKILL.md")
		require.FileExists(t, skillPath)
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		require.Contains(t, string(data), expected)
		require.Contains(t, string(data), "spektacular version check",
			"every workflow skill must open with the version-check preamble")
		require.NotContains(t, string(data), "{{command}}")
	}

	// Claude surfaces installed skills directly in its slash-command menu, so no
	// command wrappers are installed — the commands tree must not exist.
	require.NoDirExists(t, filepath.Join(tmp, ".claude", "commands"))

	// Each installed SKILL.md must have a valid frontmatter block that
	// satisfies the agentskills.io naming rules.
	for skill := range skillAssertions {
		validateSkillFrontmatter(t, filepath.Join(tmp, ".claude", "skills", skill, "SKILL.md"))
	}

	// CLAUDE.md must exist and import AGENTS.md so the installed rules take effect.
	claudeData, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	require.NoError(t, err, "CLAUDE.md should be created")
	require.Contains(t, string(claudeData), "@AGENTS.md")
}

func TestClaudeAgent_Install_CreatesClaudeMdWhenAbsent(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, claudeAgent{}.Install(tmp, config.NewDefault(), io.Discard))

	data, err := os.ReadFile(filepath.Join(tmp, "CLAUDE.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "@AGENTS.md", "a fresh CLAUDE.md should import AGENTS.md")
}

func TestClaudeAgent_Install_AppendsImportToExistingClaudeMd(t *testing.T) {
	tmp := t.TempDir()
	claudePath := filepath.Join(tmp, "CLAUDE.md")
	existing := "# My project rules\n\nAlways use tabs.\n"
	require.NoError(t, os.WriteFile(claudePath, []byte(existing), 0644))

	require.NoError(t, claudeAgent{}.Install(tmp, config.NewDefault(), io.Discard))

	data, err := os.ReadFile(claudePath)
	require.NoError(t, err)
	// Existing content is preserved and the import is added.
	require.Contains(t, string(data), "Always use tabs.")
	require.Contains(t, string(data), "@AGENTS.md")
}

func TestClaudeAgent_Install_LeavesExistingImportUntouched(t *testing.T) {
	tmp := t.TempDir()
	claudePath := filepath.Join(tmp, "CLAUDE.md")
	existing := "# My rules\n\n@AGENTS.md\n\nSome more guidance.\n"
	require.NoError(t, os.WriteFile(claudePath, []byte(existing), 0644))

	require.NoError(t, claudeAgent{}.Install(tmp, config.NewDefault(), io.Discard))

	data, err := os.ReadFile(claudePath)
	require.NoError(t, err)
	// Idempotent: a CLAUDE.md that already imports AGENTS.md is unchanged, and the
	// import is not duplicated.
	require.Equal(t, existing, string(data))
	require.Equal(t, 1, strings.Count(string(data), "@AGENTS.md"))
}
