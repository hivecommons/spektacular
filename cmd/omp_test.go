package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitOMPAndMigrate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "init", "omp")
	require.Equal(t, 0, code, stdout)
	cfgPath := filepath.Join(dir, ".spektacular", "config.yaml")
	require.Equal(t, "omp", readSettingsMap(t, cfgPath)["agent"])
	before := snapshotDir(t, dir)
	resetRootCmd(t)
	stdout, _, code = runRootCmd(t, "init", "omp")
	require.Equal(t, 0, code, stdout)
	require.Equal(t, before, snapshotDir(t, dir))
	// Use an older config and stale artifacts to exercise the real migration
	// dispatcher, not just the agent's Install method.
	writeSettingsFile(t, dir, "config.yaml", "name: proj\ncommand: spektacular\nagent: omp\nrepos:\n    - name: proj\n      location: .\n")
	for _, path := range []string{".omp/skills/spek-new/SKILL.md", ".omp/commands/spek-new.md", ".omp/rules/spek-repo-sources.md", "AGENTS.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte("stale\n"), 0644))
	}
	rep := runMigrateReport(t)
	require.True(t, rep.Skills.Reinstall)
	require.Equal(t, "omp", rep.Skills.Agent)
	after := snapshotDir(t, dir)
	for path, body := range before {
		if filepath.Base(path) == "config.yaml" || filepath.Base(path) == "AGENTS.md" {
			continue
		}
		require.Equal(t, body, after[path], path)
	}
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(agents), "## Where the Code Lives")
	rules, err := os.ReadFile(filepath.Join(dir, ".omp/rules/spek-repo-sources.md"))
	require.NoError(t, err)
	require.Contains(t, string(agents), string(rules[len("---\nalwaysApply: true\n---\n\n"):]))
}

func TestInitHelpListsOMP(t *testing.T) {
	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "init", "--help")
	require.Equal(t, 0, code)
	require.Contains(t, stdout, "omp")
}
