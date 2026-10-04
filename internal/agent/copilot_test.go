package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

func copilotSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[rel] = string(body)
		return nil
	}))
	return files
}

func TestCopilotInstall(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewDefault()
	a, err := Lookup("copilot")
	require.NoError(t, err)
	require.Equal(t, "copilot", a.Name())
	require.NoError(t, a.Install(root, cfg, io.Discard))
	first := copilotSnapshot(t, root)
	require.Len(t, first, 7, "only six skills and AGENTS.md")
	require.NoError(t, a.Install(root, cfg, io.Discard))
	require.Equal(t, first, copilotSnapshot(t, root), "install is idempotent")
	for _, name := range []string{"claude", "bob", "codex"} {
		t.Run(name, func(t *testing.T) {
			other := t.TempDir()
			otherAgent, err := Lookup(name)
			require.NoError(t, err)
			require.NoError(t, otherAgent.Install(other, cfg, io.Discard))
			skillsRoot := map[string]string{"claude": ".claude/skills", "bob": ".bob/skills", "codex": ".agents/skills"}[name]
			require.Equal(t, copilotSnapshot(t, filepath.Join(root, ".github/skills")), copilotSnapshot(t, filepath.Join(other, skillsRoot)))
			require.Equal(t, first["AGENTS.md"], copilotSnapshot(t, other)["AGENTS.md"])
			before := copilotSnapshot(t, other)
			require.NoError(t, a.Install(other, cfg, io.Discard))
			after := copilotSnapshot(t, other)
			for path, body := range before {
				require.Equal(t, body, after[path], path)
			}
		})
	}
}

func TestCopilotPreservesProjectFilesAndReplacesOwnedSkills(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewDefault()
	own := map[string]string{
		"AGENTS.md":                                  "# Project rules\n\nKeep this text.\n",
		".github/skills/custom/SKILL.md":             "custom skill\n",
		".github/copilot-instructions.md":            "custom instructions\n",
		".github/instructions/style.instructions.md": "path instructions\n",
		".github/copilot/settings.json":              "{}\n",
		"CLAUDE.md":                                  "custom Claude instructions\n",
		"GEMINI.md":                                  "custom Gemini instructions\n",
	}
	for path, body := range own {
		full := filepath.Join(root, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0644))
	}
	a := copilotAgent{}
	for range 2 {
		skill := filepath.Join(root, ".github/skills/spek-new/SKILL.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(skill), 0755))
		require.NoError(t, os.WriteFile(skill, []byte("stale"), 0644))
		require.NoError(t, a.Install(root, cfg, io.Discard))
		after := copilotSnapshot(t, root)
		for path, body := range own {
			if path == "AGENTS.md" {
				require.Contains(t, after[path], body)
			} else {
				require.Equal(t, body, after[path], path)
			}
		}
		require.NotEqual(t, "stale", after[".github/skills/spek-new/SKILL.md"])
	}
}
