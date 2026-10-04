package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

func ompSnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			files[rel] = string(data)
		}
		return nil
	}))
	return files
}

func TestOMPInstall_PreservesProjectFilesAndRefreshesOwnedFiles(t *testing.T) {
	dir := t.TempDir()
	own := map[string]string{
		".omp/skills/custom/SKILL.md": "my skill\n",
		".omp/commands/custom.md":     "my command\n",
		".omp/rules/custom.md":        "my rule\n",
		".omp/AGENTS.md":              "my instructions\n",
		".omp/RULES.md":               "my sticky rules\n",
		".omp/config.yml":             "disabledProviders: [claude, codex, agents]\n",
	}
	for path, body := range own {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My project\n\n## My rules\nKeep me.\n"), 0644))
	a, err := Lookup("omp")
	require.NoError(t, err)
	cfg := config.Config{Command: "spektacular"}
	require.NoError(t, a.Install(dir, cfg, io.Discard))
	before := ompSnapshot(t, dir)
	require.NoError(t, a.Install(dir, cfg, io.Discard))
	require.Equal(t, before, ompSnapshot(t, dir))
	for path, body := range own {
		require.Equal(t, body, before[filepath.FromSlash(path)])
	}
	require.Contains(t, before["AGENTS.md"], "## My rules\nKeep me.")
	rules := 0
	for path, body := range before {
		if strings.HasPrefix(path, ".omp/rules/spek-") {
			rules++
			require.True(t, strings.HasPrefix(body, "---\nalwaysApply: true\n---\n\n"))
			section := strings.TrimPrefix(body, "---\nalwaysApply: true\n---\n\n")
			require.Contains(t, before["AGENTS.md"], strings.TrimSpace(section))
			require.NotContains(t, body, "Keep me.")
		}
	}
	require.Equal(t, 8, rules)
	// A previous release (or a hand-made workaround) owns the same names.
	for path := range before {
		if strings.HasPrefix(path, ".omp/") && own[filepath.ToSlash(path)] == "" {
			require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("stale"), 0644))
		}
	}
	require.NoError(t, a.Install(dir, cfg, io.Discard))
	require.Equal(t, before, ompSnapshot(t, dir))
	// Rendering with a changed command also refreshes every rule alongside AGENTS.
	cfg.Command = "spekx"
	require.NoError(t, a.Install(dir, cfg, io.Discard))
	for path, body := range ompSnapshot(t, dir) {
		if strings.HasPrefix(path, ".omp/rules/spek-") {
			require.Contains(t, body, "spekx")
			require.NotContains(t, body, "`spektacular")
		}
	}
}

func TestOMPInstall_SharedProjects(t *testing.T) {
	for _, name := range []string{"claude", "bob", "codex"} {
		for _, ompFirst := range []bool{false, true} {
			t.Run(name+map[bool]string{true: "/omp-first", false: "/omp-last"}[ompFirst], func(t *testing.T) {
				dir := t.TempDir()
				cfg := config.Config{Command: "spektacular"}
				other, err := Lookup(name)
				require.NoError(t, err)
				if ompFirst {
					require.NoError(t, (ompAgent{}).Install(dir, cfg, io.Discard))
				}
				require.NoError(t, other.Install(dir, cfg, io.Discard))
				before := ompSnapshot(t, dir)
				require.NoError(t, (ompAgent{}).Install(dir, cfg, io.Discard))
				after := ompSnapshot(t, dir)
				for path, body := range before {
					require.Equal(t, body, after[path], path)
				}
				skillDir := map[string]string{"claude": ".claude/skills", "bob": ".bob/skills", "codex": ".agents/skills"}[name]
				for _, skill := range workflowSkills {
					ompPath := filepath.Join(".omp/skills", skill.Name, "SKILL.md")
					require.Equal(t, after[filepath.Join(skillDir, skill.Name, "SKILL.md")], after[ompPath])
					validateSkillFrontmatter(t, filepath.Join(dir, ompPath))
					wrapper := after[filepath.Join(".omp/commands", skill.Name+".md")]
					require.Contains(t, wrapper, "Run the `"+skill.Name+"` skill.")
				}
			})
		}
	}
}
