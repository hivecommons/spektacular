package cmd

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/agent"
	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/repo"
	"github.com/stretchr/testify/require"
)

// snapshotDir maps every file under root (as a slash-separated relative path)
// to a sha256 of its content, so two snapshots compare both the file list and
// every file's bytes.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snap[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	}))
	return snap
}

func TestInit_Claude(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	rootCmd.SetArgs([]string{"init", "claude"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	// .spektacular directory created
	_, err = os.Stat(filepath.Join(dir, ".spektacular"))
	require.NoError(t, err)

	// Three SKILL.md files exist, each rendered with the default `spektacular`
	// command. Each workflow embeds a distinctive `<command> <subcmd> new` line
	// that proves the {{command}} placeholder was expanded.
	skillAssertions := map[string]string{
		"spek-new":       "spektacular spec new",
		"spek-plan":      "spektacular plan new",
		"spek-implement": "spektacular implement new",
	}
	for skill, expected := range skillAssertions {
		skillPath := filepath.Join(dir, ".claude", "skills", skill, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err, "expected skill file %s to exist", skillPath)
		require.Contains(t, string(data), expected)
		require.NotContains(t, string(data), "{{command}}")
	}

	// Claude surfaces installed skills directly in its slash-command menu, so no
	// command wrappers are installed — the commands tree must not exist.
	require.NoDirExists(t, filepath.Join(dir, ".claude", "commands"))

	// The installing version is recorded in .spektacular/version. A fresh
	// temp dir has no pre-existing version file, so this also covers repos
	// initialised before the version file existed.
	versionData, err := os.ReadFile(filepath.Join(dir, ".spektacular", "version"))
	require.NoError(t, err)
	require.Equal(t, "0.1.0\n", string(versionData))
}

func TestInit_RewritesStaleVersionFile(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Simulate an installation recorded by a different binary version.
	versionPath := filepath.Join(dir, ".spektacular", "version")
	require.NoError(t, os.WriteFile(versionPath, []byte("9.9.9\n"), 0644))

	// Re-running init replaces the stale record with the current version.
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	versionData, err := os.ReadFile(versionPath)
	require.NoError(t, err)
	require.Equal(t, "0.1.0\n", string(versionData))
}

func TestInit_Bob(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	rootCmd.SetArgs([]string{"init", "bob"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	// .spektacular directory created
	_, err = os.Stat(filepath.Join(dir, ".spektacular"))
	require.NoError(t, err)

	// Three SKILL.md files under .bob/skills/spek-{new,plan,implement}/.
	skillAssertions := map[string]string{
		"spek-new":       "spektacular spec new",
		"spek-plan":      "spektacular plan new",
		"spek-implement": "spektacular implement new",
	}
	for skill, expected := range skillAssertions {
		skillPath := filepath.Join(dir, ".bob", "skills", skill, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err, "expected skill file %s to exist", skillPath)
		require.Contains(t, string(data), expected)
		require.NotContains(t, string(data), "{{command}}")
	}

	// Three command wrappers under .bob/commands/ — Bob keeps the `spek-`
	// prefix in the basename.
	commandAssertions := map[string]string{
		"spek-new.md":       "`spek-new` skill",
		"spek-plan.md":      "`spek-plan` skill",
		"spek-implement.md": "`spek-implement` skill",
	}
	for base, expected := range commandAssertions {
		cmdPath := filepath.Join(dir, ".bob", "commands", base)
		data, err := os.ReadFile(cmdPath)
		require.NoError(t, err, "expected command file %s to exist", cmdPath)
		require.Contains(t, string(data), expected)
		require.NotContains(t, string(data), "{{command}}")
		require.NotContains(t, string(data), "{{skill}}")
	}
}

func TestInit_Codex(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	rootCmd.SetArgs([]string{"init", "codex"})
	err := rootCmd.Execute()
	require.NoError(t, err)

	// .spektacular directory created
	_, err = os.Stat(filepath.Join(dir, ".spektacular"))
	require.NoError(t, err)

	// Three SKILL.md files under .agents/skills/spek-{new,plan,implement}/.
	skillAssertions := map[string]string{
		"spek-new":       "spektacular spec new",
		"spek-plan":      "spektacular plan new",
		"spek-implement": "spektacular implement new",
	}
	for skill, expected := range skillAssertions {
		skillPath := filepath.Join(dir, ".agents", "skills", skill, "SKILL.md")
		data, err := os.ReadFile(skillPath)
		require.NoError(t, err, "expected skill file %s to exist", skillPath)
		require.Contains(t, string(data), expected)
		require.NotContains(t, string(data), "{{command}}")
	}

	// Codex has no per-repo slash-command mechanism — no command wrappers or
	// other agent roots should be created.
	require.NoDirExists(t, filepath.Join(dir, ".agents", "commands"))
	require.NoDirExists(t, filepath.Join(dir, ".claude"))
	require.NoDirExists(t, filepath.Join(dir, ".bob"))
}

func TestInit_InvalidAgent(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	rootCmd.SetArgs([]string{"init", "unknown"})
	err := rootCmd.Execute()
	require.Error(t, err)
	require.True(t, errors.Is(err, agent.ErrUnknownAgent), "error should wrap agent.ErrUnknownAgent, got %v", err)
	require.Contains(t, err.Error(), "claude")
	require.Contains(t, err.Error(), "bob")
	require.Contains(t, err.Error(), "codex")
}

func TestInit_CustomCommand(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	// First init to create .spektacular with default config
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Override the command in config
	configPath := filepath.Join(dir, ".spektacular", "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("name: testproj\ncommand: \"go run .\"\n"), 0644))

	// Re-init — should use the custom command when rendering templates.
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	cfg, err := config.FromYAMLFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "go run .", cfg.Command)
	require.Equal(t, "timestamp", cfg.Spec.IDMethod)

	skillPath := filepath.Join(dir, ".claude", "skills", "spek-new", "SKILL.md")
	skillData, err := os.ReadFile(skillPath)
	require.NoError(t, err)
	require.Contains(t, string(skillData), "go run . spec new")
	require.Contains(t, string(skillData), "go run . version check",
		"version-check preamble must render with the custom command")
	require.NotContains(t, string(skillData), "{{command}}")
}

func TestInit_Idempotent(t *testing.T) {
	resetRootCmd(t)
	dir := t.TempDir()
	t.Chdir(dir)

	// Pre-create a sibling skill alongside the ones init manages. It must
	// survive re-init untouched.
	siblingSkillDir := filepath.Join(dir, ".claude", "skills", "other")
	require.NoError(t, os.MkdirAll(siblingSkillDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(siblingSkillDir, "SKILL.md"), []byte("keep-skill"), 0644))

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Sibling skill file still exists and is untouched.
	skillData, err := os.ReadFile(filepath.Join(siblingSkillDir, "SKILL.md"))
	require.NoError(t, err)
	require.Equal(t, "keep-skill", string(skillData))

	// Version file still exists with the expected content after the second init.
	versionData, err := os.ReadFile(filepath.Join(dir, ".spektacular", "version"))
	require.NoError(t, err)
	require.Equal(t, "0.1.0\n", string(versionData))
}

// Criterion 3: a second init run produces no changes — the full recursive
// directory state (file list and every file's content hash) is identical
// before and after the re-run.
func TestInit_SecondRunProducesNoChanges(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())
	before := snapshotDir(t, dir)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())
	after := snapshotDir(t, dir)

	require.Equal(t, before, after, "a second init run must not add, remove, or modify any file")
}

// Criterion 1: breaking a member repo's footprint and re-running init repairs
// it — the member's repo.yaml is restored valid — and once everything is
// healthy, a further re-init is a no-change operation over both the project
// and the member repo.
func TestInit_RepairsBrokenMemberFootprint(t *testing.T) {
	project := t.TempDir()
	member := t.TempDir()
	t.Chdir(project)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Register the sibling member repo; repo add creates its footprint.
	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "member",
		"location": member,
	}))
	require.NoError(t, err)
	memberRepoConfig := filepath.Join(member, ".spektacular", config.RepoConfigFileName)
	require.FileExists(t, memberRepoConfig)

	// Break the member's footprint.
	require.NoError(t, os.Remove(memberRepoConfig))

	// Re-running init in the project cascades over the registry and repairs it.
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())
	_, err = config.RepoConfigFromYAMLFile(memberRepoConfig)
	require.NoError(t, err, "the member's repo.yaml must be restored valid by re-init")

	// A healthy project re-init changes nothing, in the project or the member.
	beforeProject := snapshotDir(t, project)
	beforeMember := snapshotDir(t, member)
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())
	require.Equal(t, beforeProject, snapshotDir(t, project), "a healthy re-init must not change the project")
	require.Equal(t, beforeMember, snapshotDir(t, member), "a healthy re-init must not change the member repo")
}

// initProjectWithAbsentRepo initialises a claude project in a temp dir,
// hand-registers a repo named "ghost" whose location ./ghost is not on disk,
// and returns the project root, leaving the working directory inside it.
func initProjectWithAbsentRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Register directly in the config: going through `repo add` would fail
	// on the missing location, and these tests need an entry that is
	// registered but not on disk.
	cfgPath := filepath.Join(dir, ".spektacular", "config.yaml")
	cfg, err := config.FromYAMLFile(cfgPath)
	require.NoError(t, err)
	cfg.Repos = append(cfg.Repos, config.RepoEntry{
		Name:     "ghost",
		Location: "../ghost",
	})
	require.NoError(t, cfg.ToYAMLFile(cfgPath))

	return dir
}

// Init notices: re-running init over a registry containing a repo whose
// location is not on disk prints a Notice naming that repo instead of
// failing.
func TestInit_NoticesUnmaterializedRepo(t *testing.T) {
	initProjectWithAbsentRepo(t)

	out, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	require.Contains(t, out.String(), "Notice:")
	require.Contains(t, out.String(), `"ghost"`, "the notice must name the skipped repo")
}

// Cascade never creates: init with a registry entry whose location is not
// on disk neither creates that location nor materializes a clone for it.
func TestInit_CascadeNeverClonesUnmaterializedRepo(t *testing.T) {
	resetRootCmd(t)
	dir := initProjectWithAbsentRepo(t)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	require.NoDirExists(t, filepath.Join(dir, "ghost"), "init must not create an absent repo's location")
	require.NoDirExists(t, filepath.Join(dir, ".spektacular", repo.MaterializeDirName, "ghost"))
}

// TestInit_NameFlagSetsProjectName asserts that `init --name` records the
// given name in config.yaml instead of deriving it from the directory.
func TestInit_NameFlagSetsProjectName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude", "--name", "custom-name"})
	require.NoError(t, rootCmd.Execute())

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "custom-name", cfg.Name)
}

// TestInit_NameFlagOverridesStoredName asserts that an explicit --name on a
// re-init replaces the name already recorded in config.yaml.
func TestInit_NameFlagOverridesStoredName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	rootCmd.SetArgs([]string{"init", "claude", "--name", "first-name"})
	require.NoError(t, rootCmd.Execute())

	rootCmd.SetArgs([]string{"init", "claude", "--name", "second-name"})
	require.NoError(t, rootCmd.Execute())

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "second-name", cfg.Name)
}

// Phase 1.4 criterion 5: re-running init over a colocated project whose own
// repo.yaml declares a file source at <elsewhere> keeps that source block,
// leaves the source directory byte-identical, and footprints nothing there
// — init cascades over registered locations only, never over sources.
func TestInit_RerunLeavesColocatedFileSourceUntouched(t *testing.T) {
	project := t.TempDir()
	elsewhere := t.TempDir()
	t.Chdir(project)
	resetRootCmd(t)
	require.NoError(t, os.WriteFile(filepath.Join(elsewhere, "main.go"), []byte("package main\n"), 0o644))

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	// Declare the source in the colocated repo's own config after the first
	// init, as a user would.
	rcPath := filepath.Join(project, ".spektacular", config.RepoConfigFileName)
	rc, err := config.RepoConfigFromYAMLFile(rcPath)
	require.NoError(t, err)
	rc.Source = config.FileSource(elsewhere)
	require.NoError(t, rc.ToYAMLFile(rcPath))

	beforeElsewhere := snapshotDir(t, elsewhere)
	beforeProject := snapshotDir(t, project)

	rootCmd.SetArgs([]string{"init", "claude"})
	require.NoError(t, rootCmd.Execute())

	require.Equal(t, beforeElsewhere, snapshotDir(t, elsewhere), "re-init must not touch the source dir")
	require.NoDirExists(t, filepath.Join(elsewhere, ".spektacular"), "re-init must not footprint the source dir")
	require.Equal(t, beforeProject, snapshotDir(t, project), "a healthy re-init must not change the project")

	raw, err := os.ReadFile(rcPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "source:\n    provider: file\n    config:\n        location: "+elsewhere+"\n", "re-init must keep the declared source")
}
