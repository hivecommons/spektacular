package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestInitCopilotAndMigrate(t *testing.T) {
	for _, previous := range []string{"copilot", "claude", "bob", "codex"} {
		t.Run(previous, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			resetRootCmd(t)
			rootCmd.SetArgs([]string{"init", previous})
			require.NoError(t, rootCmd.Execute())
			resetRootCmd(t)
			rootCmd.SetArgs([]string{"init", "copilot"})
			require.NoError(t, rootCmd.Execute())
			cfgPath := filepath.Join(dir, ".spektacular/config.yaml")
			cfg := readSettingsMap(t, cfgPath)
			require.Equal(t, "copilot", cfg["agent"])
			skill := filepath.Join(dir, ".github/skills/spek-new/SKILL.md")
			fresh, err := os.ReadFile(skill)
			require.NoError(t, err)
			agentsPath := filepath.Join(dir, "AGENTS.md")
			freshRules, err := os.ReadFile(agentsPath)
			require.NoError(t, err)
			resetRootCmd(t)
			rootCmd.SetArgs([]string{"init", "copilot"})
			require.NoError(t, rootCmd.Execute())
			require.Equal(t, cfg, readSettingsMap(t, cfgPath))
			require.NoError(t, os.WriteFile(skill, []byte("old skill"), 0644))
			require.NoError(t, os.WriteFile(agentsPath, []byte(""), 0644))
			// A version mismatch, not changed file content, triggers reinstallation.
			cfg["skills_version"] = "9.9.9"
			body, err := yaml.Marshal(cfg)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(cfgPath, body, 0644))
			rep := runMigrateReport(t)
			require.True(t, rep.Skills.Reinstall)
			got, err := os.ReadFile(skill)
			require.NoError(t, err)
			require.Equal(t, fresh, got)
			got, err = os.ReadFile(agentsPath)
			require.NoError(t, err)
			require.Equal(t, freshRules, got)
		})
	}
}

func TestInitHelpListsCopilot(t *testing.T) {
	stdout, _, code := runRootCmd(t, "init", "--help")
	require.Zero(t, code)
	require.Contains(t, stdout, "copilot")
}
