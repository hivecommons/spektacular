package agent

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

func installFor(t *testing.T, name string, cfg config.Config) string {
	t.Helper()
	dir := t.TempDir()
	a, err := Lookup(name)
	require.NoError(t, err)
	require.NoError(t, a.Install(dir, cfg, io.Discard))
	return dir
}

func readReviewer(t *testing.T, dir, agent string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(goalReviewers[agent].path)))
	require.NoError(t, err)
	return string(b)
}

// Every agent that can name a helper's model installs spek-goal-reviewer in
// its own format, embedding the shared brief unchanged, on a cheaper default
// model and with read-only tools where the format allows.
func TestGoalReviewer_InstalledForEveryAgentButBob(t *testing.T) {
	brief, err := templates.FS.ReadFile("partials/goal-reviewer-brief.md")
	require.NoError(t, err)
	cfg := config.NewDefault()

	cases := map[string]struct {
		path  string
		model string
		tools string
	}{
		"claude":  {".claude/agents/spek-goal-reviewer.md", "model: haiku\n", "tools: Read, Grep, Glob\n"},
		"codex":   {".codex/agents/spek-goal-reviewer.toml", "model = \"gpt-6-luna\"\n", "Never edit, create or delete files: you are read-only."},
		"copilot": {".github/agents/spek-goal-reviewer.agent.md", "model: Claude Haiku 4.5\n", "tools: ['read', 'search']\n"},
		"omp":     {".omp/agents/spek-goal-reviewer.md", "model: \"@smol\"\n", "tools: read, grep, glob\n"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := installFor(t, name, cfg)
			body := readReviewer(t, dir, name)
			require.Equal(t, tc.path, goalReviewers[name].path)
			require.Contains(t, body, GoalReviewerName)
			require.Contains(t, body, tc.model, "a cheaper default model, with no project configuration")
			require.Contains(t, body, tc.tools)
			require.Contains(t, body, string(brief), "the shared brief is embedded unchanged")
			require.NotContains(t, body, "{{")
			for _, tool := range []string{"Write", "Edit", "Bash", "execute", "edit"} {
				require.NotContains(t, strings.SplitN(body, "\n---\n", 2)[0], "tools: "+tool, "no write tools")
			}
		})
	}

	bob := installFor(t, "bob", cfg)
	for _, dir := range []string{".bob/agents", ".claude/agents", ".codex/agents"} {
		require.NoDirExists(t, filepath.Join(bob, dir), "bob installs no reviewer helper")
	}
	_, ok := goalReviewers["bob"]
	require.False(t, ok)
	require.Empty(t, GoalReviewerModel(cfg, "bob"))
}

// The Codex reviewer is valid TOML with the brief in a literal multi-line
// string: nothing in the brief can close it early.
func TestGoalReviewer_CodexDefinitionIsWellFormed(t *testing.T) {
	body := readReviewer(t, installFor(t, "codex", config.NewDefault()), "codex")
	require.True(t, strings.HasPrefix(body, "name = \"spek-goal-reviewer\"\n"))
	require.Equal(t, 2, strings.Count(body, "'''"), "the instructions are one literal multi-line string")
	require.NotContains(t, body, "sandbox_mode", "Codex ignores sandbox_mode in an agent file, so it is not claimed")
	require.True(t, strings.HasSuffix(body, "'''\n"))
}

// A project's model override for one agent changes that agent's helper only.
func TestGoalReviewer_ModelOverrideIsPerAgent(t *testing.T) {
	cfg := config.NewDefault()
	cfg.Implement.GoalCheck.Models = map[string]string{"claude": "sonnet"}

	require.Contains(t, readReviewer(t, installFor(t, "claude", cfg), "claude"), "model: sonnet\n")
	require.Contains(t, readReviewer(t, installFor(t, "codex", cfg), "codex"), "model = \"gpt-6-luna\"\n")
	require.Contains(t, readReviewer(t, installFor(t, "copilot", cfg), "copilot"), "model: Claude Haiku 4.5\n")
	require.Contains(t, readReviewer(t, installFor(t, "omp", cfg), "omp"), "model: \"@smol\"\n")
	require.Equal(t, "sonnet", GoalReviewerModel(cfg, "claude"))
	require.Equal(t, "gpt-6-luna", GoalReviewerModel(cfg, "codex"))
}

// Re-installing rewrites the helper byte for byte, and picks up a changed
// override.
func TestGoalReviewer_ReinstallIsByteIdenticalAndRefreshes(t *testing.T) {
	cfg := config.NewDefault()
	for name := range goalReviewers {
		t.Run(name, func(t *testing.T) {
			dir := installFor(t, name, cfg)
			first := readReviewer(t, dir, name)
			a, err := Lookup(name)
			require.NoError(t, err)
			require.NoError(t, a.Install(dir, cfg, io.Discard))
			require.Equal(t, first, readReviewer(t, dir, name))

			changed := config.NewDefault()
			changed.Implement.GoalCheck.Models = map[string]string{name: "override-model"}
			require.NoError(t, a.Install(dir, changed, io.Discard))
			require.Contains(t, readReviewer(t, dir, name), "override-model")
		})
	}
}

func TestGoalReviewer_CodexWritesNothingUnderClaude(t *testing.T) {
	dir := installFor(t, "codex", config.NewDefault())
	require.NoDirExists(t, filepath.Join(dir, ".claude"))
}
