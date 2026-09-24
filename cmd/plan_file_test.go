package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

// TestPlanFileWrite_ResolvesConfiguredDirectory asserts `plan file write` lands
// the document under the configured (non-default) plan directory.
func TestPlanFileWrite_ResolvesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("plan body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	content, err := os.ReadFile(filepath.Join(dir, "docs", "plans", "20260709000000-feature", "plan.md"))
	require.NoError(t, err)
	meta, body, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, "plan body", string(body))
}

// TestPlanFileWrite_PreservesProblematicCharacters asserts that the body
// portion written to the destination is byte-identical to the source bytes,
// even when the source contains shell-sensitive characters and embedded
// newlines. Since Phase 1.3 introduced a frontmatter layer, the on-disk bytes
// carry a metadata block before the body — this test strips it and asserts on
// the body portion only.
func TestPlanFileWrite_PreservesProblematicCharacters(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	body := []byte("line with `backticks` and $dollar and 'single' and \"double\" quotes\nsecond line\n")
	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, body, 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	dstPath := filepath.Join(dir, "docs", "plans", "20260709000000-feature", "plan.md")
	content, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	meta, gotBody, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, body, gotBody)
}

// TestPlanFileWrite_MissingSourceErrors asserts that pointing `--from` at a
// non-existent path returns an error referencing the offending path and does
// not create the destination file.
func TestPlanFileWrite_MissingSourceErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	srcPath := filepath.Join(t.TempDir(), "missing.md")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, srcPath)
	require.NoFileExists(t, filepath.Join(dir, "docs", "plans", "20260709000000-feature", "plan.md"))
}

// TestPlanFileWrite_PreservesSourceFile asserts that a successful write leaves
// the source file's bytes unchanged.
func TestPlanFileWrite_PreservesSourceFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	body := []byte("original source bytes")
	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, body, 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	after, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	require.Equal(t, body, after)
}

// TestPlanFileWrite_PipedStdinWithoutFromFails asserts that omitting `--from`
// fails the command even when stdin has data piped in, and that the
// destination file is not created.
func TestPlanFileWrite_PipedStdinWithoutFromFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	setupImplementCmd(t)
	rootCmd.SetIn(strings.NewReader("ignored"))
	t.Cleanup(func() { rootCmd.SetIn(nil) })
	rootCmd.SetArgs([]string{"plan", "file", "write", "feature/plan.md"})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(dir, "docs", "plans", "feature", "plan.md"))
}

// TestPlanFileRead_ResolvesConfiguredDirectory asserts `plan file read` reads
// from the configured (non-default) plan directory.
func TestPlanFileRead_ResolvesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	planPath := filepath.Join(dir, "docs", "plans", "feature", "context.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(planPath), 0o755))
	require.NoError(t, os.WriteFile(planPath, []byte("stored context"), 0o644))

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "read", "feature/context.md"})

	require.NoError(t, rootCmd.Execute())
	require.Equal(t, "stored context", stdout.String())
}

// TestPlanFileWrite_RejectsNameWithoutIDPrefix asserts `plan file write`
// rejects a plan name that has no ID prefix matching the configured
// spec.id_method, with an error that points the agent at reusing the
// originating spec's ID rather than inventing a new one. It does not create
// the destination file.
func TestPlanFileWrite_RejectsNameWithoutIDPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("plan body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "feature/plan.md", "--from", srcPath})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, "feature/plan.md")
	require.ErrorContains(t, err, "spec.id_method")
	require.NoFileExists(t, filepath.Join(dir, "docs", "plans", "feature", "plan.md"))
}

// TestPlanFileWrite_AcceptsCounterIDPrefix asserts `plan file write` accepts
// a plan name carrying a counter-style ID prefix (six digits + underscore)
// when spec.id_method is "counter".
func TestPlanFileWrite_AcceptsCounterIDPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nplan:\n  config:\n    directory: ../docs/plans\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("plan body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "000034_feature/plan.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	content, err := os.ReadFile(filepath.Join(dir, "docs", "plans", "000034_feature", "plan.md"))
	require.NoError(t, err)
	meta, body, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, "plan body", string(body))
}

// TestPlanFileWrite_RejectsTimestampIDWhenCounterConfigured asserts that a
// name matching the wrong id_method scheme (timestamp-shaped, but counter is
// configured) is still rejected.
func TestPlanFileWrite_RejectsTimestampIDWhenCounterConfigured(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nplan:\n  config:\n    directory: ../docs/plans\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("plan body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(dir, "docs", "plans", "20260709000000-feature", "plan.md"))
}

// Criterion 3 (plan): plan.config.directory is relative to the folder
// holding config.yaml, so `directory: x` stores plans in .spektacular/x.
func TestPlanFileWrite_CustomDirectoryResolvesFromSettingsFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: x\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("plan body"), 0o644))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "plan", "file", "write", "20260709000000-feature/plan.md", "--from", srcPath)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	require.FileExists(t, filepath.Join(dir, ".spektacular", "x", "20260709000000-feature", "plan.md"))
	require.NoDirExists(t, filepath.Join(dir, "x"))
}
