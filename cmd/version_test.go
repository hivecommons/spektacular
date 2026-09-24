package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file covers the `version check` command through runRootCmd — the same
// wrapper Execute() uses in production. The check reports whether the
// project's settings files and installed skills are current, and every
// out-of-date status names `migrate` as the remedy.
//
// Every expected version string below is a hand-maintained oracle: the dev
// default the binary compiles with is the literal "0.20.0" (cmd/root.go's
// `version` var), asserted as that literal and never derived from the
// `version` var or versionString() at runtime.

// currentConfigYAML is a config.yaml already at the current settings format
// with the running build's skills recorded and no registered repos.
const currentConfigYAML = `schema: 3
written_by: 0.20.0
skills_version: 0.20.0
name: proj
command: spektacular
agent: claude
`

// writeSettingsFile writes dir/.spektacular/<name> with content and returns
// its path.
func writeSettingsFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	path := filepath.Join(dataDir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// runVersionCheckJSON runs `version check` and decodes stdout into a generic
// map so tests can assert both present and absent keys.
func runVersionCheckJSON(t *testing.T) (m map[string]any, code int) {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "version", "check")
	require.Empty(t, stderr)
	require.NoError(t, json.Unmarshal([]byte(stdout), &m))
	return m, code
}

// requireAction asserts m carries a string action containing every want.
func requireAction(t *testing.T, m map[string]any, want ...string) {
	t.Helper()
	action, ok := m["action"].(string)
	require.True(t, ok, "a non-match status must carry an action string, got %v", m)
	for _, w := range want {
		require.Contains(t, action, w)
	}
}

// A current project whose recorded skills version is the running build's
// reports "match" with no action key at all.
func TestVersionCheck_Match(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", currentConfigYAML)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "match", m["status"])
	require.Equal(t, "0.20.0", m["installed_version"])
	require.Equal(t, "0.20.0", m["current_version"])
	require.NotContains(t, m, "action", "matching versions must carry no action text")
}

// A stale skills_version reports "mismatch" with an action naming migrate,
// exit 0, and the check changes nothing on disk.
func TestVersionCheck_MismatchIsReadOnlyAndNamesMigrate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `schema: 3
written_by: 9.9.9
skills_version: 9.9.9
name: proj
command: spektacular
agent: claude
`)
	before := snapshotDir(t, dir)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "mismatch", m["status"])
	require.Equal(t, "9.9.9", m["installed_version"])
	require.Equal(t, "0.20.0", m["current_version"])
	requireAction(t, m, "`spektacular migrate`", "`spektacular migrate --dry-run`")

	require.Equal(t, before, snapshotDir(t, dir), "version check must never change any file")
}

// The action composes the configured command, not the default one.
func TestVersionCheck_ActionUsesConfiguredCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `schema: 3
skills_version: 9.9.9
name: proj
command: go run .
agent: claude
`)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "mismatch", m["status"])
	requireAction(t, m, "`go run . migrate`")
}

// A current project with no recorded skills version (and no legacy version
// file) reports "missing" and names migrate.
func TestVersionCheck_NoRecordedSkillsVersionIsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `schema: 3
name: proj
command: spektacular
agent: claude
`)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "missing", m["status"])
	require.NotContains(t, m, "installed_version")
	requireAction(t, m, "`spektacular migrate`")
}

// No config.yaml at all: "missing", no installed_version, and an action
// naming init rather than migrate.
func TestVersionCheck_NoProjectNamesInit(t *testing.T) {
	t.Chdir(t.TempDir())

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, false, m["error"])
	require.Equal(t, "missing", m["status"])
	require.NotContains(t, m, "installed_version", "missing state has nothing installed to report")
	require.Equal(t, "0.20.0", m["current_version"])
	requireAction(t, m, "`spektacular init <agent>`")
	require.NotContains(t, m["action"], "migrate")
}

// A current config.yaml with skills matching, but a registered repo whose
// repo.yaml is at an older format, reports "upgrade_needed" and names migrate.
func TestVersionCheck_RepoBehindIsUpgradeNeeded(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", currentConfigYAML+`repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", "description: A test project\n")
	before := snapshotDir(t, dir)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "upgrade_needed", m["status"])
	require.Equal(t, "0.20.0", m["installed_version"])
	requireAction(t, m, "older format", "`spektacular migrate`")
	require.Equal(t, before, snapshotDir(t, dir), "version check must never change any file")
}

// An unversioned config.yaml is behind even when its skills match.
func TestVersionCheck_UnversionedConfigIsUpgradeNeeded(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `name: proj
command: spektacular
agent: claude
skills_version: 0.20.0
`)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "upgrade_needed", m["status"])
	requireAction(t, m, "`spektacular migrate`")
}

// Criterion 5 (check half): a project whose only record of its skills is the
// standalone .spektacular/version file is checked against that file.
func TestVersionCheck_ReadsLegacyVersionFile(t *testing.T) {
	t.Run("current config, legacy file matches", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSettingsFile(t, dir, "config.yaml", `schema: 3
name: proj
command: spektacular
agent: claude
`)
		writeSettingsFile(t, dir, "version", "0.20.0\n")

		m, code := runVersionCheckJSON(t)
		require.Equal(t, 0, code)
		require.Equal(t, "match", m["status"])
		require.Equal(t, "0.20.0", m["installed_version"])
		require.NotContains(t, m, "action")
	})
	t.Run("current config, legacy file stale", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSettingsFile(t, dir, "config.yaml", `schema: 3
name: proj
command: spektacular
agent: claude
`)
		writeSettingsFile(t, dir, "version", "9.9.9\n")

		m, code := runVersionCheckJSON(t)
		require.Equal(t, 0, code)
		require.Equal(t, "mismatch", m["status"])
		require.Equal(t, "9.9.9", m["installed_version"])
		requireAction(t, m, "`spektacular migrate`")
	})
	t.Run("unversioned config, legacy file stale", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSettingsFile(t, dir, "config.yaml", "name: proj\ncommand: spektacular\nagent: claude\n")
		writeSettingsFile(t, dir, "version", "9.9.9\n")

		m, code := runVersionCheckJSON(t)
		require.Equal(t, 0, code)
		require.Equal(t, "upgrade_needed", m["status"])
		require.Equal(t, "9.9.9", m["installed_version"])
		requireAction(t, m, "`spektacular migrate`")
	})
}

// Criterion 6 (check half): a written_by that differs from the running build
// is informational only and never makes the check report a problem.
func TestVersionCheck_DifferentWrittenByIsStillMatch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `schema: 3
written_by: 7.7.7
skills_version: 0.20.0
name: proj
command: spektacular
agent: claude
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", "schema: 2\nwritten_by: 8.8.8\ndescription: A test project\n")

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "match", m["status"])
	require.NotContains(t, m, "action")
}

// A config.yaml written in a newer settings format reports
// "unsupported_format" with an action telling the user to update
// Spektacular, and leaves the file untouched.
func TestVersionCheck_NewerFormatIsUnsupported(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := writeSettingsFile(t, dir, "config.yaml", "schema: 99\nname: proj\nagent: claude\n")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "unsupported_format", m["status"])
	requireAction(t, m, "newer Spektacular", "update Spektacular")
	require.NotContains(t, m["action"], "`spektacular migrate`", "migrate cannot fix a newer format")

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// A config.yaml that cannot be read is a genuine fault: exit 1 with the
// migration_check_failed error, never a silent status.
func TestVersionCheck_UnreadableConfigIsGenuineFault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".spektacular", "config.yaml"), 0o755))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "version", "check")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "migration_check_failed", er.Code)
}

// --schema short-circuits before any filesystem access and prints the output
// contract, including every status in the enum.
func TestVersionCheck_Schema(t *testing.T) {
	t.Chdir(t.TempDir())
	resetRootCmd(t)

	stdout, stderr, code := runRootCmd(t, "version", "check", "--schema")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Contains(t, stdout, `"status"`)
	for _, s := range []string{`"match"`, `"mismatch"`, `"missing"`, `"upgrade_needed"`, `"unsupported_format"`} {
		require.Contains(t, stdout, s)
	}
	require.NotContains(t, stdout, "migration_needed")
}
