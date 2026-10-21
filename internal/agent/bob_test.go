package agent

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBobAgent_Name(t *testing.T) {
	require.Equal(t, "bob", bobAgent{}.Name())
}

func TestBobAgent_Install(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.NewDefault()

	err := bobAgent{}.Install(tmp, cfg, io.Discard)
	require.NoError(t, err)

	// Exactly eight SKILL.md files under .bob/skills/spek-*/.
	skillAssertions := map[string]string{
		"spek-new":            "spektacular spec new",
		"spek-plan":           "spektacular plan new",
		"spek-implement":      "spektacular implement new",
		"spek-knowledge":      "knowledge",
		"spek-manage-repos":   "repo add",
		"spek-design":         "spektacular design author",
		"spek-plan-epic":      "plan this epic",
		"spek-implement-epic": "implement this epic",
	}
	for skill, expected := range skillAssertions {
		skillPath := filepath.Join(tmp, ".bob", "skills", skill, "SKILL.md")
		require.FileExists(t, skillPath)
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		require.Contains(t, string(data), expected)
		require.Contains(t, string(data), "spektacular version check",
			"every workflow skill must open with the version-check preamble")
		require.NotContains(t, string(data), "{{command}}")
	}

	// Exactly eight command wrappers under .bob/commands/, basenames keep the
	// `spek-` prefix.
	commandAssertions := map[string]string{
		"spek-new.md":            "`spek-new` skill",
		"spek-plan.md":           "`spek-plan` skill",
		"spek-implement.md":      "`spek-implement` skill",
		"spek-knowledge.md":      "`spek-knowledge` skill",
		"spek-manage-repos.md":   "`spek-manage-repos` skill",
		"spek-design.md":         "`spek-design` skill",
		"spek-plan-epic.md":      "`spek-plan-epic` skill",
		"spek-implement-epic.md": "`spek-implement-epic` skill",
	}
	for base, expected := range commandAssertions {
		cmdPath := filepath.Join(tmp, ".bob", "commands", base)
		require.FileExists(t, cmdPath)
		data, err := os.ReadFile(cmdPath)
		require.NoError(t, err)
		require.Contains(t, string(data), expected)
		require.NotContains(t, string(data), "{{command}}")
		require.NotContains(t, string(data), "{{skill}}")
	}

	// A wrapper's frontmatter carries the skill's description, which is what an
	// agent with no native skill mechanism shows in its slash-command menu. An
	// empty description there is silent, so the text is pinned as a literal.
	designWrapper, err := os.ReadFile(filepath.Join(tmp, ".bob", "commands", "spek-design.md"))
	require.NoError(t, err)
	require.Contains(t, string(designWrapper), "description: Author, bring in, revise or reference a design document.",
		"the spek-design wrapper must carry a meaningful description in its frontmatter")

	planEpicWrapper, err := os.ReadFile(filepath.Join(tmp, ".bob", "commands", "spek-plan-epic.md"))
	require.NoError(t, err)
	require.Contains(t, string(planEpicWrapper), "description: Plan every outstanding spec of an epic, side by side in dependency order.",
		"the spek-plan-epic wrapper must carry a meaningful description in its frontmatter")
	implementEpicWrapper, err := os.ReadFile(filepath.Join(tmp, ".bob", "commands", "spek-implement-epic.md"))
	require.NoError(t, err)
	require.Contains(t, string(implementEpicWrapper), "description: Implement every planned spec of an epic, each in its own worktrees, in dependency order.",
		"the spek-implement-epic wrapper must carry a meaningful description in its frontmatter")

	// Bob command filenames keep the `spek-` prefix — make sure the stripped
	// variants do NOT exist on disk.
	for _, stripped := range []string{"new.md", "plan.md", "implement.md", "knowledge.md"} {
		require.NoFileExists(t, filepath.Join(tmp, ".bob", "commands", stripped))
	}

	// Each installed SKILL.md must have a valid frontmatter block that
	// satisfies the agentskills.io naming rules.
	for skill := range skillAssertions {
		validateSkillFrontmatter(t, filepath.Join(tmp, ".bob", "skills", skill, "SKILL.md"))
	}
}

func writeBobSettings(t *testing.T, home, body string) string {
	t.Helper()
	path := BobSettingsPath(home)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestBobMaxTurns(t *testing.T) {
	t.Run("no settings file is Bob's default", func(t *testing.T) {
		n, err := BobMaxTurns(t.TempDir())
		require.NoError(t, err)
		require.Equal(t, BobDefaultMaxTurns, n)
	})
	t.Run("settings without the key is Bob's default", func(t *testing.T) {
		home := t.TempDir()
		writeBobSettings(t, home, `{"bobShell":{"lastRunVersion":"2.0.6"}}`)
		n, err := BobMaxTurns(home)
		require.NoError(t, err)
		require.Equal(t, BobDefaultMaxTurns, n)
	})
	t.Run("a configured value", func(t *testing.T) {
		home := t.TempDir()
		writeBobSettings(t, home, `{"session":{"maxTurns":250}}`)
		n, err := BobMaxTurns(home)
		require.NoError(t, err)
		require.Equal(t, 250, n)
	})
	t.Run("zero is no limit", func(t *testing.T) {
		home := t.TempDir()
		writeBobSettings(t, home, `{"session":{"maxTurns":0}}`)
		n, err := BobMaxTurns(home)
		require.NoError(t, err)
		require.Zero(t, n)
	})
	t.Run("a non-number is an error", func(t *testing.T) {
		home := t.TempDir()
		writeBobSettings(t, home, `{"session":{"maxTurns":"lots"}}`)
		_, err := BobMaxTurns(home)
		require.Error(t, err)
	})
	t.Run("a file that is not JSON is an error", func(t *testing.T) {
		home := t.TempDir()
		writeBobSettings(t, home, `not json`)
		_, err := BobMaxTurns(home)
		require.Error(t, err)
	})
}

func TestRaiseBobTurnLimit(t *testing.T) {
	t.Run("creates the settings file when there is none", func(t *testing.T) {
		home := t.TempDir()
		require.NoError(t, RaiseBobTurnLimit(home))
		n, err := BobMaxTurns(home)
		require.NoError(t, err)
		require.Zero(t, n)
	})
	t.Run("keeps every other setting and the file mode", func(t *testing.T) {
		home := t.TempDir()
		path := writeBobSettings(t, home, `{"bobShell":{"lastRunVersion":"2.0.6"},"session":{"maxTurns":100,"defaultMode":"agent"},"locale":"en"}`)
		require.NoError(t, RaiseBobTurnLimit(home))

		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(data, &got))
		require.Equal(t, map[string]any{
			"bobShell": map[string]any{"lastRunVersion": "2.0.6"},
			"session":  map[string]any{"maxTurns": float64(0), "defaultMode": "agent"},
			"locale":   "en",
		}, got)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})
	t.Run("refuses a file that is not JSON and leaves it alone", func(t *testing.T) {
		home := t.TempDir()
		path := writeBobSettings(t, home, `not json`)
		require.Error(t, RaiseBobTurnLimit(home))
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, "not json", string(data))
	})
}
