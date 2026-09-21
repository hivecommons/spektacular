package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file covers the upgrade gate (cmd/gate.go): every command except the
// way out is refused on a project whose settings are behind, whose skills are
// stale or unrecorded, or whose settings are newer than this build. Every
// fixture is literal YAML and every expected value a hand-maintained literal;
// the dev build version is "0.1.0".

// gateNextAction is the gate's next action for a project with no configured
// command.
const gateNextAction = "run `spektacular migrate` to upgrade (preview the changes with `spektacular migrate --dry-run`); nothing is changed until you do"

// currentRepoYAML is a current-format repo.yaml for the project's own
// footprint folder.
const currentRepoYAML = `schema: 2
description: A test project
knowledge:
    provider: file
    config:
        location: knowledge
`

// Criterion 1: on an unversioned project `spec new` is refused with an error
// naming migrate and creates nothing; after migrate the same command runs.
func TestGate_UnversionedProjectBlocksSpecNewUntilMigrated(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeStaleUnversionedProject(t, dir)
	before := snapshotDir(t, dir)

	er := runRootError(t, "spec", "new", "--data", `{"name":"x"}`)

	require.Equal(t, "upgrade_required", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Equal(t,
		"this project's settings are out of date for this Spektacular: "+
			cfgPath+" (format 1, needs 3), "+
			filepath.Join(dir, ".spektacular", "repo.yaml")+" (format 1, needs 2)",
		er.Message)
	require.Equal(t, gateNextAction, er.NextAction)
	require.Equal(t, before, snapshotDir(t, dir), "a refused command must change nothing")

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"x"}`)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
}

// Criterion 2: on a current-format project whose installed skills are stale,
// a spec command is refused naming migrate; after migrate it runs.
func TestGate_StaleSkillsBlockSpecNewUntilMigrated(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", `schema: 3
skills_version: 0.0.1
name: proj
command: spektacular
agent: claude
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", currentRepoYAML)
	before := snapshotDir(t, dir)

	er := runRootError(t, "spec", "new", "--data", `{"name":"x"}`)

	require.Equal(t, "upgrade_required", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Equal(t, "this project's installed agent skills are from Spektacular 0.0.1; this Spektacular is 0.1.0", er.Message)
	require.Equal(t, gateNextAction, er.NextAction)
	require.Equal(t, before, snapshotDir(t, dir))

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"x"}`)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
}

// A current-format project with no skills_version and no legacy version file
// has unrecorded skills, and is refused saying so.
func TestGate_UnrecordedSkillsAreUpgradeRequired(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", `schema: 3
name: proj
command: spektacular
agent: claude
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", currentRepoYAML)

	er := runRootError(t, "repo", "list")

	require.Equal(t, "upgrade_required", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Equal(t, "this project's installed agent skills are not recorded; this Spektacular is 0.1.0", er.Message)
	require.Equal(t, gateNextAction, er.NextAction)
}

// Criterion 3: the way out, and cobra's help and completion, all run on a
// project that is both format-behind and skills-stale.
func TestGate_ExemptCommandsRunOnOutdatedProject(t *testing.T) {
	for _, args := range [][]string{
		{"migrate", "--dry-run"},
		{"init", "claude"},
		{"version", "check"},
		{"help"},
		{"completion", "bash"},
	} {
		t.Run(args[0], func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeStaleUnversionedProject(t, dir)
			resetRootCmd(t)

			stdout, _, code := runRootCmd(t, args...)
			require.Equal(t, 0, code, stdout)
		})
	}
}

// Exemption is decided per command: `version check` is exempt through its own
// annotation, while its parent `version` carries none and is refused.
func TestGate_ExemptionIsPerCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeStaleUnversionedProject(t, dir)

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "upgrade_needed", m["status"])

	er := runRootError(t, "version")
	require.Equal(t, "upgrade_required", er.Code)
	require.Equal(t, gateNextAction, er.NextAction)
}

// A written_by that differs from the running build is not an upgrade matter:
// commands run.
func TestGate_DifferentWrittenByDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "written_by: 7.7.7\n")

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"x"}`)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
}

// A project config.yaml from a newer Spektacular is refused with
// config_newer_format by every gated command, and left byte for byte as it
// was.
func TestGate_NewerProjectIsRefused(t *testing.T) {
	for _, args := range [][]string{
		{"spec", "new", "--data", `{"name":"x"}`},
		{"repo", "list"},
	} {
		t.Run(args[0], func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			cfgPath := writeSettingsFile(t, dir, "config.yaml", "schema: 99\n"+unversionedProjectYAML)
			before := snapshotDir(t, dir)

			er := runRootError(t, args...)

			require.Equal(t, "config_newer_format", er.Code)
			require.Equal(t, cfgPath, er.Resource)
			require.Equal(t,
				"install a newer Spektacular release: this file needs settings format 99 and this build supports format 3. The file has not been changed",
				er.NextAction)
			require.Equal(t, before, snapshotDir(t, dir))
		})
	}
}

// A config.yaml that cannot be parsed is not an upgrade matter: the gate lets
// the command run, and the command's own loading reports the problem.
func TestGate_UnparseableConfigIsLeftToTheCommand(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", "{{{\n")

	er := runRootError(t, "repo", "list")

	require.Equal(t, "internal_error", er.Code)
	require.Contains(t, er.Message, "parsing config file "+cfgPath+": ")
}

// A registered repo whose repo.yaml cannot be parsed is not an upgrade
// matter either: the gate lets the command run to report it.
func TestGate_UnparseableMemberRepoIsLeftToTheCommand(t *testing.T) {
	member := t.TempDir()
	writeSettingsFile(t, member, "repo.yaml", "{{{\n")

	project := t.TempDir()
	t.Chdir(project)
	writeSpecCommandConfig(t, project, "repos:\n"+
		"  - name: testproj\n    location: .\n"+
		"  - name: member\n    location: "+filepath.Join(member, ".spektacular")+"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".spektacular", "knowledge"), 0o755))

	er := runRootError(t, "repo", "list")

	require.Equal(t, "repo_footprint_missing", er.Code)
	require.Equal(t, filepath.Join(member, ".spektacular"), er.Resource)
	require.Contains(t, er.Message, "parsing repo config file "+filepath.Join(member, ".spektacular", "repo.yaml")+": ")
}
