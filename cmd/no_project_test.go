package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file tests the project gate in cmd/root.go's loadConfig: running a
// project-operating command in a directory without .spektacular/config.yaml
// must fail explicitly with a no_project error pointing at `init`, instead of
// silently falling back to a default config. The bootstrap commands (init,
// version, skill) deliberately bypass the gate via loadConfigLenient or by
// never loading config at all, and there is no parent-directory search — the
// gate judges the current working directory only.

// Criterion 1: any project-operating command run in a directory with no
// project config returns an explicit error naming the init command as the
// next action, rather than proceeding on silent defaults.
func TestNoProject_ProjectOperatingCommandFailsExplicitly(t *testing.T) {
	assertNoProjectEnvelope := func(t *testing.T, dir, stdout string, code int) {
		t.Helper()
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
		require.Equal(t, "no_project", er.Code)
		require.Contains(t, er.Message, dir, "message must name the directory that has no project")
		require.Equal(t, filepath.Join(dir, ".spektacular", "config.yaml"), er.Resource)
		require.Contains(t, er.NextAction, "init", "next_action must point the agent at the init command")

		// The refused command must not have scaffolded a project as a side
		// effect of failing.
		require.NoDirExists(t, filepath.Join(dir, ".spektacular"))
	}

	t.Run("knowledge sources", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "knowledge", "sources")
		assertNoProjectEnvelope(t, dir, stdout, code)
	})

	t.Run("spec new", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "spec", "new", "--data", `{"name":"billing"}`)
		assertNoProjectEnvelope(t, dir, stdout, code)
	})

	t.Run("design sources", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "design", "sources")
		assertNoProjectEnvelope(t, dir, stdout, code)
	})

	t.Run("design read", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		resetRootCmd(t)

		stdout, _, code := runRootCmd(t, "design", "read", "--data", `{"source":"api","path":"x.md"}`)
		assertNoProjectEnvelope(t, dir, stdout, code)
	})
}

// Criterion 2: `init` still works in an uninitialized directory — it is the
// command the no_project error points at, so it must run before a project
// exists (and creating the project's files in the process is its job).
func TestNoProject_InitStillWorksInUninitializedDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, stderr, code := runRootCmd(t, "init", "claude")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.FileExists(t, filepath.Join(dir, ".spektacular", "config.yaml"))
}

// Criterion 2: `version check` still works in an uninitialized directory — a
// missing project is a successful "no version recorded" report, not a
// no_project failure.
func TestNoProject_VersionCheckStillWorksInUninitializedDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	resetRootCmd(t)

	stdout, _, code := runRootCmd(t, "version", "check")
	require.Equal(t, 0, code)

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(stdout), &m))
	require.Equal(t, false, m["error"])
}

// Criterion 2: `skill` still works in an uninitialized directory — it never
// loads project config at all.
func TestNoProject_SkillStillWorksInUninitializedDir(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	stdout, _, code := runRootCmd(t, "skill", "determine-feature-slug")
	require.Equal(t, 0, code)
	require.NotEmpty(t, stdout)
}

// Criterion 3: running from a subdirectory of a project does not silently
// create or assume a new project there — there is deliberately no
// parent-directory search, so the subdirectory itself is judged (and fails
// the gate) even though an initialised project sits one level up.
func TestNoProject_SubdirectoryOfProjectIsNotTheProject(t *testing.T) {
	dir := t.TempDir()
	writeSpecCommandConfig(t, dir, "")

	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	t.Chdir(sub)
	resetRootCmd(t)

	stdout, _, code := runRootCmd(t, "knowledge", "sources")
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "no_project", er.Code)
	require.Equal(t, filepath.Join(sub, ".spektacular", "config.yaml"), er.Resource, "the gate must judge the cwd, not a parent directory")
	require.NoDirExists(t, filepath.Join(sub, ".spektacular"), "no project may be scaffolded in the subdirectory")
}
