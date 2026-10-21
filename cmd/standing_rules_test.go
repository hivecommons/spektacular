package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file covers the compact standing rules end to end through `init` and
// `migrate`: AGENTS.md carries a short section per rule linking to a guidance
// file under .spektacular/guidance/, a project written by the pre-compact
// binary converts in place, and repeating either command changes nothing.
// Every expected value is a hand-maintained literal.

// compactRuleSlugs is the hand-maintained list of every standing rule, by the
// slug naming its guidance file, in install order.
var compactRuleSlugs = []string{
	"repo-sources",
	"store-access",
	"memory-context",
	"knowledge-trigger",
	"spec-trigger",
	"design-trigger",
	"draft-presentation",
	"historical-artifacts",
}

// compactRuleHeadings is the hand-maintained AGENTS.md heading of every
// standing rule, in install order.
var compactRuleHeadings = []string{
	"## Where the Code Lives — Read This First",
	"## Spektacular's Files Are Reached Through Spektacular",
	"## Memory & Context",
	"## Knowledge-Worthy Discovery Recognition",
	"## Spec-Worthy Discussion Recognition",
	"## Design-Worthy Detail Recognition",
	"## Presenting Drafts and Confirmations",
	"## Historical Artifacts: Specs and Plans as Archaeology",
}

// legacyAgentsMDPath is a frozen copy of the AGENTS.md the pre-compact binary
// wrote, with the project's own blocks before, between and after its full
// sections.
const legacyAgentsMDPath = "testdata/agents-md-legacy.md"

// The project's own blocks in the legacy fixture, written out by hand.
const (
	legacyBlockBefore  = "# Team rules\n\nKeep A.\n"
	legacyBlockBetween = "## Our section\n\nKeep B.\n"
	legacyBlockAfter   = "## Trailing\n\nKeep C.\n"
)

// compactGuidanceLink is the markdown link a rule's short section carries to
// its guidance file.
func compactGuidanceLink(slug string) string {
	return "[.spektacular/guidance/" + slug + ".md](.spektacular/guidance/" + slug + ".md)"
}

// readCompactRuleFiles returns AGENTS.md and every guidance file under dir,
// keyed by project-relative path.
func readCompactRuleFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	paths := []string{"AGENTS.md"}
	for _, slug := range compactRuleSlugs {
		paths = append(paths, ".spektacular/guidance/"+slug+".md")
	}
	for _, rel := range paths {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		require.NoError(t, err, rel)
		files[rel] = string(body)
	}
	return files
}

// requireCompactAGENTS asserts AGENTS.md holds every rule's heading once, in
// order, each with its guidance link, and none of the pre-compact full text.
func requireCompactAGENTS(t *testing.T, agents string) {
	t.Helper()
	prev := -1
	for i, h := range compactRuleHeadings {
		require.Equal(t, 1, strings.Count(agents, h+"\n"), "heading %q", h)
		idx := strings.Index(agents, h+"\n")
		require.Greater(t, idx, prev, "heading %q out of order", h)
		prev = idx
		require.Contains(t, agents, compactGuidanceLink(compactRuleSlugs[i]))
	}
	require.NotContains(t, agents, "> Managed by `spektacular init` — edit `templates/agents/",
		"no pre-compact full section may remain")
}

// requireLegacyBlocksInOrder asserts the project's own blocks from the legacy
// fixture survive byte for byte, in their places around the rules.
func requireLegacyBlocksInOrder(t *testing.T, agents string) {
	t.Helper()
	order := []string{
		legacyBlockBefore,
		"## Where the Code Lives — Read This First\n",
		"## Spektacular's Files Are Reached Through Spektacular\n",
		legacyBlockBetween,
		"## Memory & Context\n",
		"## Historical Artifacts: Specs and Plans as Archaeology\n",
		legacyBlockAfter,
	}
	prev := -1
	for _, piece := range order {
		require.Equal(t, 1, strings.Count(agents, piece), "%q must appear once", piece)
		idx := strings.Index(agents, piece)
		require.Greater(t, idx, prev, "%q must follow the piece before it", piece)
		prev = idx
	}
}

// For every agent, `init` writes the compact AGENTS.md and all eight guidance
// files, and running it again changes neither.
func TestInit_InstallsCompactRulesForEveryAgent(t *testing.T) {
	for _, agentName := range []string{"claude", "codex", "bob", "copilot", "omp"} {
		t.Run(agentName, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			resetRootCmd(t)
			stdout, _, code := runRootCmd(t, "init", agentName)
			require.Equal(t, 0, code, stdout)

			for _, slug := range compactRuleSlugs {
				require.FileExists(t, filepath.Join(dir, ".spektacular", "guidance", slug+".md"))
			}
			agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			require.NoError(t, err)
			require.Less(t, len(agents), 5120, "AGENTS.md is %d bytes", len(agents))
			requireCompactAGENTS(t, string(agents))
			if agentName == "claude" {
				claude, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
				require.NoError(t, err)
				require.Contains(t, string(claude), "@AGENTS.md")
			}

			once := readCompactRuleFiles(t, dir)
			resetRootCmd(t)
			stdout, _, code = runRootCmd(t, "init", agentName)
			require.Equal(t, 0, code, stdout)
			require.Equal(t, once, readCompactRuleFiles(t, dir))
		})
	}
}

// staleSkillsConfigYAML is a current-format config.yaml whose recorded
// skills predate the compact standing rules.
const staleSkillsConfigYAML = `schema: 4
written_by: 0.0.1
skills_version: 0.0.1
name: proj
command: spektacular
agent: claude
spec:
    provider: file
    id_method: timestamp
    config:
        directory: specs
plan:
    provider: file
    config:
        directory: plans
changelog:
    provider: file
    config:
        directory: changelog
repos:
    - name: proj
      location: .
`

// markSkillsStale rewrites the recorded skills_version in config.yaml back
// to 0.0.1, so the next migrate reinstalls.
func markSkillsStale(t *testing.T, dir string) {
	t.Helper()
	cfgPath := filepath.Join(dir, ".spektacular", "config.yaml")
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "skills_version: 0.20.0\n")
	require.NoError(t, os.WriteFile(cfgPath,
		[]byte(strings.Replace(string(raw), "skills_version: 0.20.0\n", "skills_version: 0.0.1\n", 1)), 0o644))
}

// A project the pre-compact binary initialised, with its own content around
// the full sections and no guidance folder, upgrades to the compact form with
// that content unchanged and in place; a second upgrade changes nothing.
func TestMigrate_ConvertsPreChangeProject(t *testing.T) {
	legacy, err := os.ReadFile(legacyAgentsMDPath)
	require.NoError(t, err)

	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", staleSkillsConfigYAML)
	writeSettingsFile(t, dir, "repo.yaml", "schema: 2\nwritten_by: 0.0.1\ndescription: A test project\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), legacy, 0o644))
	require.NoDirExists(t, filepath.Join(dir, ".spektacular", "guidance"))

	rep := runMigrateReport(t)
	require.True(t, rep.Skills.Reinstall)
	require.Equal(t, "claude", rep.Skills.Agent)

	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	requireCompactAGENTS(t, string(agents))
	requireLegacyBlocksInOrder(t, string(agents))
	for _, slug := range compactRuleSlugs {
		require.FileExists(t, filepath.Join(dir, ".spektacular", "guidance", slug+".md"))
	}

	once := readCompactRuleFiles(t, dir)
	markSkillsStale(t, dir)
	rep = runMigrateReport(t)
	require.True(t, rep.Skills.Reinstall)
	require.Equal(t, once, readCompactRuleFiles(t, dir))
}

// An upgrade from an older version restores every outdated or missing
// guidance file to the shipped content.
func TestMigrate_RefreshesOutdatedGuidance(t *testing.T) {
	// The shipped guidance is what a fresh init writes.
	shipped := t.TempDir()
	t.Chdir(shipped)
	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "init", "claude")
	require.Equal(t, 0, code, stdout)
	want := readCompactRuleFiles(t, shipped)

	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)
	stdout, _, code = runRootCmd(t, "init", "claude")
	require.Equal(t, 0, code, stdout)

	guidance := filepath.Join(dir, ".spektacular", "guidance")
	require.NoError(t, os.WriteFile(filepath.Join(guidance, "repo-sources.md"), []byte("stale\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(guidance, "historical-artifacts.md"), []byte("stale\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(guidance, "memory-context.md")))
	markSkillsStale(t, dir)

	rep := runMigrateReport(t)
	require.True(t, rep.Skills.Reinstall)

	got := readCompactRuleFiles(t, dir)
	for _, slug := range compactRuleSlugs {
		rel := ".spektacular/guidance/" + slug + ".md"
		require.Equal(t, want[rel], got[rel], rel)
	}
}
