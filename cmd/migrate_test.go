package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/migrate"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file covers the `migrate` command through runRootCmd. Every fixture is
// literal YAML and every expected value a hand-maintained literal; the dev
// build version is "0.1.0".

// staleUnversionedConfigYAML is a config.yaml that predates format
// versioning: no schema key and no skills_version. Paired with an unversioned
// repo.yaml and a stale legacy version file it is both format-behind and
// skills-stale.
const staleUnversionedConfigYAML = `name: proj
command: spektacular
agent: claude
repos:
    - name: proj
      location: .
`

const unversionedRepoYAML = `description: A test project
knowledge:
    provider: file
    config:
        location: knowledge
`

// writeStaleUnversionedProject lays out a project in dir that is both behind
// on settings format and stale on skills, and returns the config.yaml path.
func writeStaleUnversionedProject(t *testing.T, dir string) string {
	t.Helper()
	cfg := writeSettingsFile(t, dir, "config.yaml", staleUnversionedConfigYAML)
	writeSettingsFile(t, dir, "repo.yaml", unversionedRepoYAML)
	writeSettingsFile(t, dir, "version", "9.9.9\n")
	return cfg
}

// runMigrateCmd runs `migrate` with args and returns the raw stdout and exit code.
func runMigrateCmd(t *testing.T, args ...string) (string, int) {
	t.Helper()
	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, append([]string{"migrate"}, args...)...)
	require.Empty(t, stderr)
	return stdout, code
}

// runMigrateReport runs `migrate` expecting success and decodes its report.
func runMigrateReport(t *testing.T, args ...string) migrate.Report {
	t.Helper()
	stdout, code := runMigrateCmd(t, args...)
	require.Equal(t, 0, code, stdout)
	var rep migrate.Report
	require.NoError(t, json.Unmarshal([]byte(stdout), &rep))
	return rep
}

// runMigrateError runs `migrate` expecting failure and decodes its error.
func runMigrateError(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	stdout, code := runMigrateCmd(t, args...)
	require.Equal(t, 1, code, stdout)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// hasAction reports whether acts contains an action matching op, path and
// (when non-empty) key and to.
func hasAction(acts []migrate.Action, op, path, key, to string) bool {
	for _, a := range acts {
		if a.Op == op && a.Path == path && (key == "" || a.Key == key) && (to == "" || a.To == to) {
			return true
		}
	}
	return false
}

// Criterion 1 (with success metric 4): one migrate run on a project that is
// both format-behind and skills-stale upgrades both, reinstalls skills for
// the configured agent, and leaves version check reporting match.
func TestMigrate_FormatBehindAndSkillsStaleReachesMatch(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeStaleUnversionedProject(t, dir)

	m, _ := runVersionCheckJSON(t)
	require.Equal(t, "upgrade_needed", m["status"])

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)
	require.False(t, rep.DryRun)
	require.Len(t, rep.Files, 2)
	require.True(t, rep.Skills.Reinstall)
	require.Equal(t, "claude", rep.Skills.Agent)
	require.Equal(t, "9.9.9", rep.Skills.Installed)

	require.FileExists(t, filepath.Join(dir, ".claude", "skills", "spek-new", "SKILL.md"))
	cfg := readSettingsMap(t, cfgPath)
	require.Equal(t, 3, cfg["schema"])
	require.Equal(t, "0.1.0", cfg["skills_version"])
	require.Equal(t, 2, readSettingsMap(t, filepath.Join(dir, ".spektacular", "repo.yaml"))["schema"])
	require.NoFileExists(t, filepath.Join(dir, ".spektacular", "version"))

	m, code := runVersionCheckJSON(t)
	require.Equal(t, 0, code)
	require.Equal(t, "match", m["status"])
	require.NotContains(t, m, "action")
}

// Criterion 2: --dry-run reports every settings change, every file that
// would be created or removed, and whether skills would be reinstalled, and
// changes nothing on disk.
func TestMigrate_DryRunReportsEverythingAndChangesNothing(t *testing.T) {
	t.Run("legacy single-file project", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cfgPath := writeSettingsFile(t, dir, "config.yaml", "name: proj\ncommand: spektacular\nagent: claude\n")
		versionPath := writeSettingsFile(t, dir, "version", "9.9.9\n")
		repoPath := filepath.Join(dir, ".spektacular", "repo.yaml")
		before := snapshotDir(t, dir)

		stdout, code := runMigrateCmd(t, "--dry-run")
		require.Equal(t, 0, code, stdout)
		require.NotContains(t, stdout, `"backup"`, "a preview writes no backup")
		var rep migrate.Report
		require.NoError(t, json.Unmarshal([]byte(stdout), &rep))

		require.Equal(t, "upgrade_needed", rep.Status)
		require.True(t, rep.DryRun)
		require.Len(t, rep.Files, 1)
		f := rep.Files[0]
		require.Equal(t, cfgPath, f.Path)
		require.Equal(t, migrate.Kind("project"), f.Kind)
		require.Equal(t, 1, f.From)
		require.Equal(t, 3, f.To)
		require.Equal(t, []string{
			"split legacy single-file settings and record installed skills version",
			"resolve spec, plan and changelog folders from the settings file",
		}, f.Steps)
		for key, to := range map[string]string{
			"spec.config.directory":      "specs",
			"plan.config.directory":      "plans",
			"changelog.config.directory": "changelog",
		} {
			require.True(t, hasAction(f.Actions, "set", cfgPath, key, to), "must report %s set to %s: %+v", key, to, f.Actions)
		}
		require.True(t, hasAction(f.Actions, "create", repoPath, "", ""), "must report repo.yaml would be created: %+v", f.Actions)
		require.True(t, hasAction(f.Actions, "set", cfgPath, "repos", ""), "must report the repos registry change: %+v", f.Actions)
		require.True(t, hasAction(f.Actions, "set", cfgPath, "skills_version", "9.9.9"), "must report skills_version carried over: %+v", f.Actions)
		require.True(t, hasAction(f.Actions, "remove", versionPath, "", ""), "must report the version file would be removed: %+v", f.Actions)

		require.Equal(t, "mismatch", rep.Skills.Status)
		require.Equal(t, "claude", rep.Skills.Agent)
		require.True(t, rep.Skills.Reinstall, "must report skills would be reinstalled")

		require.Equal(t, before, snapshotDir(t, dir), "a dry run must not change any file")
	})

	t.Run("split unversioned project", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cfgPath := writeStaleUnversionedProject(t, dir)
		repoPath := filepath.Join(dir, ".spektacular", "repo.yaml")
		before := snapshotDir(t, dir)

		rep := runMigrateReport(t, "--dry-run")
		require.Equal(t, "upgrade_needed", rep.Status)
		require.Len(t, rep.Files, 2)
		require.Equal(t, cfgPath, rep.Files[0].Path)
		require.Equal(t, repoPath, rep.Files[1].Path)
		require.Equal(t, migrate.Kind("repo"), rep.Files[1].Kind)
		require.Equal(t, 1, rep.Files[1].From)
		require.Equal(t, 2, rep.Files[1].To)
		require.True(t, rep.Skills.Reinstall)

		require.Equal(t, before, snapshotDir(t, dir), "a dry run must not change any file")
	})
}

// Criterion 3: a second migrate reports up_to_date and changes no file.
func TestMigrate_SecondRunIsUpToDateAndChangesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeStaleUnversionedProject(t, dir)

	require.Equal(t, "upgraded", runMigrateReport(t).Status)
	before := snapshotDir(t, dir)

	rep := runMigrateReport(t)
	require.Equal(t, "up_to_date", rep.Status)
	require.Empty(t, rep.Files)
	require.False(t, rep.Skills.Reinstall)
	require.Equal(t, "match", rep.Skills.Status)
	require.Equal(t, before, snapshotDir(t, dir), "a second migrate must not change any file")
}

// migrate reports where it kept the original settings, and the backup is
// byte-identical to the file before the upgrade.
func TestMigrate_ReportsByteIdenticalBackup(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeStaleUnversionedProject(t, dir)

	rep := runMigrateReport(t)
	require.Equal(t, cfgPath, rep.Files[0].Path)
	require.Equal(t, cfgPath+".v1.old", rep.Files[0].Backup)
	backup, err := os.ReadFile(rep.Files[0].Backup)
	require.NoError(t, err)
	require.Equal(t, staleUnversionedConfigYAML, string(backup))
}

// Success metric 4: on a current-format project whose only staleness is its
// recorded skills version, migrate reinstalls skills for the configured
// agent, records skills_version 0.1.0, and changes no other setting.
func TestMigrate_StaleSkillsOnCurrentProjectOnlyTouchesSkillsVersion(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", `schema: 3
written_by: 9.9.9
skills_version: 9.9.9
name: proj
command: spektacular
agent: claude
spec_trigger_threshold: strict
debug:
    enabled: false
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
`)
	writeSettingsFile(t, dir, "repo.yaml", "schema: 2\nwritten_by: 9.9.9\ndescription: A test project\n")
	before := readSettingsMap(t, cfgPath)

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)
	require.Empty(t, rep.Files, "no settings file is behind")
	require.True(t, rep.Skills.Reinstall)

	require.FileExists(t, filepath.Join(dir, ".claude", "skills", "spek-new", "SKILL.md"))
	// An upgrade installs skills this project never had, spek-design included,
	// and the settings comparison below is what pins "without any settings
	// change" alongside it.
	require.FileExists(t, filepath.Join(dir, ".claude", "skills", "spek-design", "SKILL.md"))
	after := readSettingsMap(t, cfgPath)
	require.Equal(t, "0.1.0", after["skills_version"])
	for _, k := range []string{"skills_version", "written_by"} {
		delete(before, k)
		delete(after, k)
	}
	require.Equal(t, before, after, "migrate must change no setting other than skills_version and written_by")
}

// A registered repo whose location is not on disk is named in skipped_repos
// rather than failing the upgrade.
func TestMigrate_AbsentRepoIsSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", currentConfigYAML+`repos:
    - name: ghost
      location: ../ghost
`)

	rep := runMigrateReport(t)
	require.Equal(t, "up_to_date", rep.Status)
	require.Len(t, rep.Skipped, 1)
	require.Equal(t, "ghost", rep.Skipped[0].Name)
	require.Equal(t, filepath.Join(dir, "ghost"), rep.Skipped[0].Location)
	require.Equal(t, "not on disk", rep.Skipped[0].Reason)
	require.NoDirExists(t, filepath.Join(dir, "ghost"))
}

// A .spektacular directory migrate cannot write to fails the upgrade with
// migrate_failed and a next_action naming migrate.
func TestMigrate_ReadOnlyProjectFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeStaleUnversionedProject(t, dir)
	before, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	dataDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.Chmod(dataDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dataDir, 0o755) })

	er := runMigrateError(t)
	require.Equal(t, "migrate_failed", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Contains(t, er.NextAction, "`spektacular migrate`")

	after, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Equal(t, before, after, "a failed upgrade must leave config.yaml untouched")
}

// A config.yaml in a newer settings format is refused with
// config_newer_format, pointing at a newer Spektacular, and left untouched.
func TestMigrate_NewerFormatIsRefused(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", "schema: 99\nname: proj\nagent: claude\nskills_version: 9.9.9\n")
	before := snapshotDir(t, dir)

	er := runMigrateError(t)
	require.Equal(t, "config_newer_format", er.Code)
	require.Equal(t, cfgPath, er.Resource)
	require.Contains(t, er.NextAction, "newer Spektacular")

	require.Equal(t, before, snapshotDir(t, dir), "a refused upgrade must not change any file")
}

// migrate outside a project fails with no_project.
func TestMigrate_NoProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	er := runMigrateError(t)
	require.Equal(t, "no_project", er.Code)
	require.Contains(t, er.NextAction, "init")
	require.NoDirExists(t, filepath.Join(dir, ".spektacular"))
}

// Stale skills on a project that records no agent cannot be reinstalled:
// migrate fails with migrate_failed and tells the user to run init.
func TestMigrate_NoAgentWithStaleSkillsFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", "schema: 3\nskills_version: 9.9.9\nname: proj\ncommand: spektacular\n")

	er := runMigrateError(t)
	require.Equal(t, "migrate_failed", er.Code)
	require.Contains(t, er.NextAction, "`spektacular init <agent>`")
	require.Contains(t, er.NextAction, "claude")
}

// Criterion 6 (migrate half): a written_by that differs from the running
// build is not something to upgrade — migrate reports up_to_date and changes
// nothing.
func TestMigrate_DifferentWrittenByIsUpToDate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSettingsFile(t, dir, "config.yaml", `schema: 3
written_by: 7.7.7
skills_version: 0.1.0
name: proj
command: spektacular
agent: claude
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", "schema: 2\nwritten_by: 8.8.8\ndescription: A test project\n")
	before := snapshotDir(t, dir)

	rep := runMigrateReport(t)
	require.Equal(t, "up_to_date", rep.Status)
	require.Empty(t, rep.Files)
	require.False(t, rep.Skills.Reinstall)
	require.Equal(t, before, snapshotDir(t, dir))
}

// Criterion 5 (upgrade half): a project whose only skills record is the
// standalone version file ends with skills_version in config.yaml and the
// file gone.
func TestMigrate_CarriesLegacyVersionFileIntoConfig(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", "name: proj\ncommand: spektacular\nagent: claude\n")
	versionPath := writeSettingsFile(t, dir, "version", "9.9.9\n")

	m, _ := runVersionCheckJSON(t)
	require.Equal(t, "9.9.9", m["installed_version"], "version check must read the legacy file")

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)
	require.Equal(t, "0.1.0", readSettingsMap(t, cfgPath)["skills_version"])
	require.NoFileExists(t, versionPath)

	m, _ = runVersionCheckJSON(t)
	require.Equal(t, "match", m["status"])
}

// legacyStoreConfigYAML is an unversioned config.yaml that names every store
// folder the way format 1 and 2 did: relative to the project root.
const legacyStoreConfigYAML = `name: proj
command: spektacular
agent: claude
spec:
    provider: file
    config:
        directory: .spektacular/specs
plan:
    provider: file
    config:
        directory: .spektacular/plans
changelog:
    provider: file
    config:
        directory: .spektacular/changelog
repos:
    - name: proj
      location: .
`

// writeProjectFile writes content at the project-root-relative rel under dir.
func writeProjectFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// artifactKindPaths returns the sorted "kind path" pairs of every artifact.
func artifactKindPaths(resp artifactsResponse) []string {
	out := make([]string, 0, len(resp.Artifacts))
	for _, a := range resp.Artifacts {
		out = append(out, fmt.Sprintf("%v %v", a["kind"], a["path"]))
	}
	sort.Strings(out)
	return out
}

// Success metric 1: migrating a legacy project whose store folders are
// written project-root-relative loses nothing — every stored spec, plan and
// changelog entry is still listed afterwards, and a new spec lands in the
// same folder as the old ones.
func TestMigrate_LegacyStoreFoldersKeepEveryArtifact(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", legacyStoreConfigYAML)
	writeSettingsFile(t, dir, "repo.yaml", unversionedRepoYAML)
	writeSettingsFile(t, dir, "version", "9.9.9\n")
	writeProjectFile(t, dir, ".spektacular/specs/20260101000000-alpha.md", "alpha spec")
	writeProjectFile(t, dir, ".spektacular/specs/20260102000000-beta.md", "beta spec")
	writeProjectFile(t, dir, ".spektacular/plans/000001_p/plan.md", "p plan")
	writeProjectFile(t, dir, ".spektacular/changelog/proj/20260101000000-alpha.md", "alpha changelog")

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)
	require.Equal(t, cfgPath, rep.Files[0].Path)
	require.Equal(t, 1, rep.Files[0].From)
	require.Equal(t, 3, rep.Files[0].To)
	require.Equal(t, []string{
		"split legacy single-file settings and record installed skills version",
		"resolve spec, plan and changelog folders from the settings file",
	}, rep.Files[0].Steps)
	for key, to := range map[string]string{
		"spec.config.directory":      "specs",
		"plan.config.directory":      "plans",
		"changelog.config.directory": "changelog",
	} {
		require.True(t, hasAction(rep.Files[0].Actions, "set", cfgPath, key, to), "must report %s rewritten to %s: %+v", key, to, rep.Files[0].Actions)
	}
	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	for _, want := range []string{"directory: specs\n", "directory: plans\n", "directory: changelog\n"} {
		require.Contains(t, string(raw), want)
	}
	require.NotContains(t, string(raw), ".spektacular/")

	resetRootCmd(t)
	require.Equal(t, []string{"20260101000000-alpha.md", "20260102000000-beta.md"}, fileNames(runListJSON(t, "spec").Files))
	resetRootCmd(t)
	require.Equal(t, []string{"000001_p"}, fileNames(runListJSON(t, "plan").Files))
	resetRootCmd(t)
	require.Equal(t, []string{"proj"}, fileNames(runListJSON(t, "changelog").Files))
	resetRootCmd(t)
	require.Equal(t, []string{"20260101000000-alpha.md"}, fileNames(runListJSON(t, "changelog", "proj").Files))
	resetRootCmd(t)
	require.Equal(t, []string{
		"changelog .spektacular/changelog/proj/20260101000000-alpha.md",
		"plan.plan .spektacular/plans/000001_p/plan.md",
		"spec .spektacular/specs/20260101000000-alpha.md",
		"spec .spektacular/specs/20260102000000-beta.md",
	}, artifactKindPaths(runArtifactsListJSON(t)))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "spec", "new", "--data", `{"name":"n"}`)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)
	var result specCommandResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	require.Equal(t, filepath.Join(dir, ".spektacular", "specs", result.SpecName+".md"), result.SpecPath)
	require.FileExists(t, result.SpecPath)
	require.NoDirExists(t, filepath.Join(dir, ".spektacular", ".spektacular"))
}

// Two formats behind with a custom store folder: migrate moves nothing. A
// spec stored at <root>/docs/specs is still listed afterwards, and
// config.yaml names that folder relative to itself.
func TestMigrate_LegacyCustomSpecFolderMovesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cfgPath := writeSettingsFile(t, dir, "config.yaml", `name: proj
command: spektacular
agent: claude
spec:
    provider: file
    config:
        directory: docs/specs
repos:
    - name: proj
      location: .
`)
	writeSettingsFile(t, dir, "repo.yaml", unversionedRepoYAML)
	writeSettingsFile(t, dir, "version", "9.9.9\n")
	writeProjectFile(t, dir, "docs/specs/a.md", "a spec")

	rep := runMigrateReport(t)
	require.Equal(t, "upgraded", rep.Status)
	require.True(t, hasAction(rep.Files[0].Actions, "set", cfgPath, "spec.config.directory", "../docs/specs"), "%+v", rep.Files[0].Actions)

	raw, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Contains(t, string(raw), "directory: ../docs/specs\n")
	require.Contains(t, string(raw), "directory: plans\n")
	require.Contains(t, string(raw), "directory: changelog\n")

	require.FileExists(t, filepath.Join(dir, "docs", "specs", "a.md"))
	require.NoDirExists(t, filepath.Join(dir, ".spektacular", "docs"))
	resetRootCmd(t)
	require.Equal(t, []string{"a.md"}, fileNames(runListJSON(t, "spec").Files))
}
