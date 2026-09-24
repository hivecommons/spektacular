package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file covers how commands refuse settings files whose format is not the
// one this build reads: an older file is stopped by the upgrade gate
// (cmd/gate.go) with upgrade_required pointing at `migrate`, a newer file
// points at a newer release and is never touched. It also covers `repo add`
// upgrading an older-format repo.yaml in place. Every fixture is literal YAML
// and every expected value a hand-maintained literal.

// unversionedProjectYAML is a config.yaml that predates format versioning: it
// has no schema key, so it reads as format 1.
const unversionedProjectYAML = `name: proj
repos:
    - name: proj
      location: .
`

// runRootError runs args through the production wrapper expecting failure,
// and decodes the error envelope.
func runRootError(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equal(t, 1, code, stdout)
	require.Empty(t, stderr)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// A command run on an unversioned project is stopped by the gate with
// upgrade_required: the message names config.yaml and its format, and the
// next action names migrate and its dry run.
func TestFormatRefusal_UnversionedProjectIsGatedUpgradeRequired(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", unversionedProjectYAML)

	er := runRootError(t, "repo", "list")

	require.Equal(t, "upgrade_required", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Equal(t,
		"this project's settings are out of date for this Spektacular: "+cfgPath+" (format 1, needs 3)",
		er.Message)
	require.Equal(t,
		"run `spektacular migrate` to upgrade (preview the changes with `spektacular migrate --dry-run`); nothing is changed until you do",
		er.NextAction)
}

// The migrate command in the gate's next action is the project's own
// configured command, not a hardcoded binary name.
func TestFormatRefusal_UpgradeRequiredNextActionUsesConfiguredCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", "command: go run .\n"+unversionedProjectYAML)

	er := runRootError(t, "repo", "list")

	require.Equal(t, "upgrade_required", er.Code)
	require.Contains(t, er.NextAction, "`go run . migrate`")
	require.Contains(t, er.NextAction, "`go run . migrate --dry-run`")
}

// A config.yaml from a newer Spektacular is refused with config_newer_format
// and left byte for byte as it was.
func TestFormatRefusal_NewerProjectIsRefusedAndUntouched(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", "schema: 99\n"+unversionedProjectYAML)
	before, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	er := runRootError(t, "repo", "list")

	require.Equal(t, "config_newer_format", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Contains(t, er.NextAction, "newer Spektacular")
	require.Contains(t, er.NextAction, "The file has not been changed")

	after, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// A member repo.yaml from a newer Spektacular, registered in a current
// project, is refused with config_newer_format when a command loads it, and
// is left byte for byte as it was.
func TestFormatRefusal_NewerMemberRepoIsRefusedAndUntouched(t *testing.T) {
	member := t.TempDir()
	writeSettingsFile(t, member, "repo.yaml", `schema: 99
description: from the future
knowledge:
    provider: file
    config:
        location: knowledge
`)
	repoPath := filepath.Join(member, ".spektacular", "repo.yaml")
	before, err := os.ReadFile(repoPath)
	require.NoError(t, err)

	project := t.TempDir()
	t.Chdir(project)
	writeSpecCommandConfig(t, project, "repos:\n"+
		"  - name: testproj\n    location: .\n"+
		"  - name: member\n    location: "+filepath.Join(member, ".spektacular")+"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".spektacular", "knowledge"), 0o755))

	er := runRootError(t, "knowledge", "sources")

	after, err := os.ReadFile(repoPath)
	require.NoError(t, err)
	require.Equal(t, before, after)

	require.Equal(t, "config_newer_format", er.Code)
	require.Equal(t, repoPath, er.Resource)
	require.Contains(t, er.NextAction, "newer Spektacular")
}

// A registered member repo.yaml that predates format versioning, in an
// otherwise current project, stops every command that would reach it through
// a footprint load (knowledge sources, repo list, and repo-routed changelog
// listing) at the gate with upgrade_required naming migrate, rather than
// being reported as a broken footprint to repair. The file is not touched.
func TestFormatRefusal_OutdatedMemberRepoIsGatedUpgradeRequired(t *testing.T) {
	for _, args := range [][]string{
		{"knowledge", "sources"},
		{"repo", "list"},
		{"changelog", "file", "list", "--repo", "member"},
	} {
		t.Run(args[0], func(t *testing.T) {
			resetRootCmd(t)
			member := t.TempDir()
			writeSettingsFile(t, member, "repo.yaml", `description: from the past
knowledge:
    provider: file
    config:
        location: knowledge
`)
			repoPath := filepath.Join(member, ".spektacular", "repo.yaml")
			before, err := os.ReadFile(repoPath)
			require.NoError(t, err)

			project := t.TempDir()
			t.Chdir(project)
			writeSpecCommandConfig(t, project, "repos:\n"+
				"  - name: testproj\n    location: .\n"+
				"  - name: member\n    location: "+filepath.Join(member, ".spektacular")+"\n")
			require.NoError(t, os.MkdirAll(filepath.Join(project, ".spektacular", "knowledge"), 0o755))

			er := runRootError(t, args...)

			after, err := os.ReadFile(repoPath)
			require.NoError(t, err)
			require.Equal(t, before, after)

			require.Equal(t, "upgrade_required", er.Code)
			require.Equal(t, repoPath, er.Resource)
			require.Equal(t,
				"this project's settings are out of date for this Spektacular: "+repoPath+" (format 1, needs 2)",
				er.Message)
			require.Contains(t, er.NextAction, "`spektacular migrate`")
			require.Contains(t, er.NextAction, "`spektacular migrate --dry-run`")
		})
	}
}

// `repo add` on a current project, for a target whose repo.yaml predates
// format versioning, upgrades that repo.yaml in place rather than
// overwriting it: its own description survives, it gains the current schema,
// and the original is kept as a format-1 backup.
func TestRepoAdd_UpgradesUnversionedTargetRepoConfigInPlace(t *testing.T) {
	repoProject(t)
	target := t.TempDir()
	const original = `description: the legacy target
knowledge:
    provider: file
    config:
        location: knowledge
`
	repoPath := writeSettingsFile(t, target, "repo.yaml", original)

	_, _, err := runRepo(t, "add", "--data", repoAddJSON(t, map[string]any{
		"name":     "legacy",
		"location": target,
	}))
	require.NoError(t, err)

	upgraded := readSettingsMap(t, repoPath)
	require.Equal(t, 2, upgraded["schema"])
	require.Equal(t, "the legacy target", upgraded["description"])

	backup, err := os.ReadFile(repoPath + ".v1.old")
	require.NoError(t, err)
	require.Equal(t, original, string(backup))
}
