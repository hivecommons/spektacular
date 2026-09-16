package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// TestSpecFileWrite_ResolvesConfiguredDirectory asserts `spec file write` lands
// the file under the configured (non-default) spec directory rather than the
// .spektacular data directory.
func TestSpecFileWrite_ResolvesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("spec body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "write", "feature.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	content, err := os.ReadFile(filepath.Join(dir, "docs", "specs", "feature.md"))
	require.NoError(t, err)
	meta, body, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, "spec body", string(body))
}

// TestSpecFileWrite_PreservesProblematicCharacters asserts that the body
// portion written to the destination is byte-identical to the source bytes,
// even when the source contains shell-sensitive characters and embedded
// newlines. Since Phase 1.3 introduced a frontmatter layer, the on-disk bytes
// carry a metadata block before the body — this test strips it and asserts on
// the body portion only.
func TestSpecFileWrite_PreservesProblematicCharacters(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	body := []byte("line with `backticks` and $dollar and 'single' and \"double\" quotes\nsecond line\n")
	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, body, 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "write", "feature.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	dstPath := filepath.Join(dir, "docs", "specs", "feature.md")
	content, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	meta, gotBody, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, body, gotBody)
}

// TestSpecFileWrite_MissingSourceErrors asserts that pointing `--from` at a
// non-existent path returns an error referencing the offending path and does
// not create the destination file.
func TestSpecFileWrite_MissingSourceErrors(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	srcPath := filepath.Join(t.TempDir(), "missing.md")

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "write", "feature.md", "--from", srcPath})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.ErrorContains(t, err, srcPath)
	require.NoFileExists(t, filepath.Join(dir, "docs", "specs", "feature.md"))
}

// TestSpecFileWrite_PreservesSourceFile asserts that a successful write leaves
// the source file's bytes unchanged.
func TestSpecFileWrite_PreservesSourceFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	body := []byte("original source bytes")
	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, body, 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "write", "feature.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	after, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	require.Equal(t, body, after)
}

// TestSpecFileWrite_PipedStdinWithoutFromFails asserts that omitting `--from`
// fails the command even when stdin has data piped in, and that the
// destination file is not created.
func TestSpecFileWrite_PipedStdinWithoutFromFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	setupImplementCmd(t)
	rootCmd.SetIn(strings.NewReader("ignored"))
	t.Cleanup(func() { rootCmd.SetIn(nil) })
	rootCmd.SetArgs([]string{"spec", "file", "write", "feature.md"})

	err := rootCmd.Execute()
	require.Error(t, err)
	require.NoFileExists(t, filepath.Join(dir, "docs", "specs", "feature.md"))
}

// TestSpecFileRead_ResolvesConfiguredDirectory asserts `spec file read` reads
// from the configured (non-default) spec directory.
func TestSpecFileRead_ResolvesConfiguredDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: docs/specs\n")

	specPath := filepath.Join(dir, "docs", "specs", "feature.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(specPath), 0o755))
	require.NoError(t, os.WriteFile(specPath, []byte("stored body"), 0o644))

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"spec", "file", "read", "feature.md"})

	require.NoError(t, rootCmd.Execute())
	require.Equal(t, "stored body", stdout.String())
}

// TestSpecFileRead_MissingFileNamesResourceInError asserts that `spec file
// read` on a nonexistent file fails through the standard response envelope
// (exit code 1, "error": true, code "not_found") and that both the message
// and the resource field name the specific requested file — not just a
// generic "not found" — so an agent can tell which file it asked for.
func TestSpecFileRead_MissingFileNamesResourceInError(t *testing.T) {
	writeSpecFileFixture(t)

	stdout, stderr, code := runRootCmd(t, "spec", "file", "read", "missing.md")

	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "not_found", er.Code)
	require.Contains(t, er.Message, "missing.md")
	require.Equal(t, "missing.md", er.Resource)
}
