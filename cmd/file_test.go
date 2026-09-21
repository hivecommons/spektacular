package cmd

import (
	"encoding/json"
	"os"
	"path"
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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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
	writeSpecCommandConfig(t, dir, "spec:\n  config:\n    directory: ../docs/specs\n")

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

// ---------------------------------------------------------------------------
// `<kind> file delete` characterisation tests.
//
// The `delete` arm of newStoreFileCmd (cmd/storefile.go) is shared verbatim by
// `spec file delete`, `plan file delete` and `changelog file delete`, so the
// tests below drive all three from the shared kindFixtures table. They are
// deliberately descriptive rather than aspirational: they write down what the
// command does *today* so a later change to it shows up as a failing
// assertion instead of a silent behaviour drift. The underlying
// FileStore.Delete contract (absent file is a no-op, path escapes are
// rejected) is already covered in internal/store/store_test.go and is not
// repeated here.
// ---------------------------------------------------------------------------

// storeArtifactDir returns the store-relative directory holding fx's artifact,
// as an argument for `<kind> file list`, plus the artifact's own base name.
// The plan fixture nests its artifact one directory down
// (`<id>-feature/plan.md`), so listing its containing directory — rather than
// the store root — is what shows the artifact itself.
func storeArtifactDir(fx kindFixture) (listArg, baseName string) {
	slash := filepath.ToSlash(fx.artifactName)
	dir := path.Dir(slash)
	if dir == "." {
		dir = ""
	}
	return dir, path.Base(slash)
}

// storeListNames runs `<kind> file list` against the directory holding fx's
// artifact and returns the entry names it reports, so a test can assert what
// the store advertises before and after a delete.
func storeListNames(t *testing.T, fx kindFixture) []string {
	t.Helper()
	listArg, _ := storeArtifactDir(fx)
	args := []string{fx.kind, "file", "list"}
	if listArg != "" {
		args = append(args, listArg)
	}
	stdout, stderr, code := runRootCmd(t, args...)
	require.Equalf(t, 0, code, "list failed: %s", stdout)
	require.Empty(t, stderr)

	var resp struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &resp))
	names := make([]string, 0, len(resp.Files))
	for _, f := range resp.Files {
		names = append(names, f.Name)
	}
	return names
}

// writeStoreArtifact writes body into fx's store through `<kind> file write`,
// so the fixture is created by the same production path a real caller uses.
func writeStoreArtifact(t *testing.T, fx kindFixture, body string) {
	t.Helper()
	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(body), 0o644))
	stdout, _, code := runRootCmd(t, fx.kind, "file", "write", fx.artifactName, "--from", srcPath)
	require.Equalf(t, 0, code, "write failed: %s", stdout)
}

// TestStoreFileDelete_RemovesStoredDocumentByName records what `<kind> file
// delete <name>` does today, for each of the three store-backed document
// kinds: it removes the named file from disk, the file stops being reported
// by `<kind> file list`, the command exits 0, and it prints nothing at all —
// no response envelope, not even an empty one. That silence is the current
// contract, asserted explicitly so giving delete an envelope later is a
// visible diff rather than a silent change.
func TestStoreFileDelete_RemovesStoredDocumentByName(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			writeStoreArtifact(t, fx, "stored body")

			abs := filepath.Join(dir, fx.storeRelPath)
			require.FileExists(t, abs)

			_, baseName := storeArtifactDir(fx)
			require.Contains(t, storeListNames(t, fx), baseName,
				"the artifact must be listed before it is deleted")

			stdout, stderr, code := runRootCmd(t, fx.kind, "file", "delete", fx.artifactName)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)
			require.Equal(t, "", stdout,
				"delete emits no envelope today — nothing at all is written to stdout")

			require.NoFileExists(t, abs)
			require.NotContains(t, storeListNames(t, fx), baseName,
				"the deleted artifact must no longer be listed")
		})
	}
}

// TestStoreFileDelete_AbsentDocumentSucceedsAndChangesNothing records the
// other half of today's behaviour, for each of the three kinds: asking to
// remove a document that is not there is a success, not an error. Both routes
// to "not there" are covered — a name that was never written, and a name that
// a previous delete already removed — and in each case the command exits 0,
// prints nothing, and leaves the rest of the store exactly as it was.
func TestStoreFileDelete_AbsentDocumentSucceedsAndChangesNothing(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			t.Run("name that was never written", func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)

				// A second, real artifact stands in for "the rest of the
				// store", so an over-eager delete would be caught.
				writeStoreArtifact(t, fx, "stored body")
				abs := filepath.Join(dir, fx.storeRelPath)
				before, err := os.ReadFile(abs)
				require.NoError(t, err)
				namesBefore := storeListNames(t, fx)

				// `delete` performs no ID-prefix validation, so an arbitrary
				// name reaches the store layer even for the kinds whose
				// `write` would reject it.
				stdout, stderr, code := runRootCmd(t, fx.kind, "file", "delete", "not-here.md")
				require.Equal(t, 0, code, "deleting an absent document is a success")
				require.Empty(t, stderr)
				require.Equal(t, "", stdout)

				after, err := os.ReadFile(abs)
				require.NoError(t, err)
				require.Equal(t, before, after,
					"an absent-document delete must leave the rest of the store untouched")
				require.Equal(t, namesBefore, storeListNames(t, fx))
			})

			t.Run("name a previous delete already removed", func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)

				writeStoreArtifact(t, fx, "stored body")

				_, _, code := runRootCmd(t, fx.kind, "file", "delete", fx.artifactName)
				require.Equal(t, 0, code)
				namesAfterFirst := storeListNames(t, fx)

				stdout, stderr, code := runRootCmd(t, fx.kind, "file", "delete", fx.artifactName)
				require.Equal(t, 0, code, "a repeated, identical delete is a success")
				require.Empty(t, stderr)
				require.Equal(t, "", stdout)

				require.NoFileExists(t, filepath.Join(dir, fx.storeRelPath))
				require.Equal(t, namesAfterFirst, storeListNames(t, fx),
					"the second delete must change nothing")
			})
		})
	}
}

// TestChangelogFileDelete_DoesNotHonourRepoRouting records a known asymmetry
// in the shared `file` command group, so it is written down rather than
// rediscovered. `changelog file write`, `read` and `list` all accept
// `--repo <name>` and route through the named member repo's own changelog
// store; `delete` resolves through storeFileStore directly instead of the
// shared resolveStore closure (cmd/storefile.go), so it never gained the flag
// and can only ever operate on the central store. This test pins that as it
// is today — it is NOT an endorsement, and fixing it is out of scope for this
// spec.
func TestChangelogFileDelete_DoesNotHonourRepoRouting(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "changelog:\n  config:\n    directory: ../docs/changelog\n")

	// The sibling verbs accept --repo; delete does not have the flag at all,
	// so cobra rejects it during parsing.
	stdout, stderr, code := runRootCmd(t, "changelog", "file", "delete", "20260709000000-release-notes.md", "--repo", "testproj")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "unknown flag: --repo", er.Message)
}
