package repo

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// footprintCategories is the hand-maintained list of knowledge categories a
// footprint must scaffold, kept independent of the registry the production
// code reads.
var footprintCategories = []string{
	"conventions", "glossary", "architecture", "gotchas", "learnings", "decisions",
}

// EnsureFootprint on a directory with no repo.yaml creates the full minimal
// footprint: a valid repo config plus a directory and README for every
// knowledge category.
func TestEnsureFootprint_FreshDirCreates(t *testing.T) {
	root := t.TempDir()

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintCreated, status)

	// The written repo.yaml parses back as a valid repo config.
	_, err = config.RepoConfigFromYAMLFile(filepath.Join(root, config.RepoConfigFileName))
	require.NoError(t, err)

	for _, cat := range footprintCategories {
		dir := filepath.Join(root, "knowledge", cat)
		require.DirExists(t, dir)
		readme := filepath.Join(dir, "README.md")
		require.FileExists(t, readme)
		content, err := os.ReadFile(readme)
		require.NoError(t, err)
		require.NotEmpty(t, content)
	}
}

// snapshotKnowledge maps every file under the footprint's knowledge tree, as a
// slash-separated path relative to root, to its exact bytes, so two snapshots
// compare both the file list and every file's content.
func snapshotKnowledge(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	require.NoError(t, filepath.WalkDir(filepath.Join(root, "knowledge"), func(path string, d fs.DirEntry, err error) error {
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
		snap[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return snap
}

// A deleted README is recreated on the next run, and a knowledge entry that
// was not missing is left exactly as it was — the only file repair rewrites is
// a category's own generated description, never content someone wrote.
func TestEnsureFootprint_MissingREADMERepairedWithoutRewritingOthers(t *testing.T) {
	root := t.TempDir()
	_, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)

	// A hand-written knowledge entry is the sentinel: repair must not touch
	// it. It is deliberately not a category description — that one file is
	// generated output and is brought back into line by design.
	sentinel := filepath.Join(root, "knowledge", "conventions", "hand-written-entry.md")
	require.NoError(t, os.WriteFile(sentinel, []byte("written by hand\n"), 0o644))

	missing := filepath.Join(root, "knowledge", "gotchas", "README.md")
	require.NoError(t, os.Remove(missing))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status)

	require.FileExists(t, missing)
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "written by hand\n", string(content), "an existing knowledge entry must never be overwritten")
}

// Criterion 3 and 5: a category description whose bytes have drifted from the
// registry's rendering is brought back into line by the next run, the run
// reports a repair, and no other file in the knowledge store is rewritten.
func TestEnsureFootprint_DriftedCategoryDescriptionRepairedWithoutRewritingOthers(t *testing.T) {
	root := t.TempDir()
	_, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)

	// A hand-written entry alongside the description must survive untouched.
	entry := filepath.Join(root, "knowledge", "conventions", "hand-written-entry.md")
	require.NoError(t, os.WriteFile(entry, []byte("written by hand\n"), 0o644))

	// The healthy tree is the oracle for what repair must restore, captured
	// without calling the renderer.
	healthy := snapshotKnowledge(t, root)

	drifted := filepath.Join(root, "knowledge", "conventions", "README.md")
	require.NoError(t, os.WriteFile(drifted, []byte("# Conventions\n\nout of step with the registry\n"), 0o644))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status, "a drifted category description is a repair")

	repaired, err := os.ReadFile(drifted)
	require.NoError(t, err)
	require.NotContains(t, string(repaired), "out of step with the registry")
	require.Contains(t, string(repaired), "**Tier:**")
	require.Contains(t, string(repaired), "**Purpose:**")

	require.Equal(t, healthy, snapshotKnowledge(t, root),
		"repair must restore the description and leave every other file in the store alone")
}

// Criterion 4: a category description that already matches the registry is
// left byte-identical — its modification time is preserved, so the file was
// not rewritten at all. (That such a run reports FootprintUnchanged is
// asserted by TestEnsureFootprint_HealthyFootprintUnchanged.)
func TestEnsureFootprint_MatchingCategoryDescriptionIsNotRewritten(t *testing.T) {
	root := t.TempDir()
	_, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)

	readme := filepath.Join(root, "knowledge", "conventions", "README.md")
	past := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(readme, past, past))

	_, err = EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)

	info, err := os.Stat(readme)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(past),
		"a matching category description must not be rewritten: modification time moved from %s to %s", past, info.ModTime())
}

// A repo.yaml that does not parse is rewritten from the given defaults, so
// the footprint ends the call valid.
func TestEnsureFootprint_BrokenRepoYAMLRepairedWithDefaults(t *testing.T) {
	root := t.TempDir()
	dir := root
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, config.RepoConfigFileName)
	require.NoError(t, os.WriteFile(path, []byte("{{ this is not yaml"), 0o644))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status)

	_, err = config.RepoConfigFromYAMLFile(path)
	require.NoError(t, err, "a repaired repo.yaml must parse as a valid repo config")
	require.FileExists(t, filepath.Join(root, "knowledge", "conventions", "README.md"))
}

// A healthy, complete footprint is reported unchanged.
func TestEnsureFootprint_HealthyFootprintUnchanged(t *testing.T) {
	root := t.TempDir()
	_, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintUnchanged, status)
}

// A healthy existing repo.yaml is the authority for its own footprint: a
// custom knowledge location in it drives where scaffolding lands, and the
// passed-in defaults' location is ignored.
func TestEnsureFootprint_ExistingConfigCustomLocationDrivesScaffolding(t *testing.T) {
	root := t.TempDir()
	dir := root
	require.NoError(t, os.MkdirAll(dir, 0o755))

	custom := config.NewDefaultRepoConfig()
	custom.Knowledge.Config.Location = "kb"
	require.NoError(t, custom.ToYAMLFile(filepath.Join(dir, config.RepoConfigFileName)))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status, "topping up the missing knowledge tree is a repair")

	for _, cat := range footprintCategories {
		require.FileExists(t, filepath.Join(root, "kb", cat, "README.md"))
	}
	require.NoDirExists(t, filepath.Join(root, "knowledge"),
		"the defaults' location must not be scaffolded when the existing config names another")
}

// Phase 1.3 criterion 3: EnsureFootprint on a root whose repo.yaml declares
// a source scaffolds the knowledge tree under the root — the repo's own
// Spektacular files stay with repo.yaml — and creates nothing under the
// source directory.
func TestEnsureFootprint_SourceDeclaredScaffoldsUnderRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "lib")
	code := filepath.Join(base, "code")
	require.NoError(t, os.MkdirAll(code, 0o755))
	writeSourceFootprint(t, root, code)

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status, "topping up the missing knowledge tree is a repair")

	require.FileExists(t, filepath.Join(root, "knowledge", "conventions", "README.md"))
	require.NoDirExists(t, filepath.Join(code, ".spektacular"), "nothing may be scaffolded under the source")

	loaded, err := config.RepoConfigFromYAMLFile(filepath.Join(root, config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, config.FileSource(code), loaded.Source, "the declared source must survive the repair")
}

// Phase 2.1: a repo.yaml written by a newer Spektacular is not broken and is
// never overwritten — EnsureFootprint refuses it with a FormatError reporting
// a newer format, leaves the file byte-identical, and scaffolds nothing.
func TestEnsureFootprint_NewerFormatRepoYAMLIsRefusedAndUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, config.RepoConfigFileName)
	const body = "schema: 99\n" +
		"description: from the future\n" +
		"knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: knowledge\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.Error(t, err)
	require.Empty(t, status)
	fe, ok := config.IsFormatError(err)
	require.True(t, ok, "expected a *config.FormatError, got %T: %v", err, err)
	require.True(t, fe.Newer())
	require.Equal(t, 99, fe.Found)
	require.Equal(t, 2, fe.Want)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, string(after), "a newer-format repo.yaml must never be rewritten")
	require.NoDirExists(t, filepath.Join(root, "knowledge"))
	require.NoFileExists(t, path+".v99.old")
}

// Phase 2.1: an unversioned but otherwise valid repo.yaml is upgraded in
// place, not replaced from defaults — its own settings survive, a byte-
// identical backup is kept beside it, and its knowledge location (not the
// defaults') drives the scaffolding.
func TestEnsureFootprint_OutdatedRepoYAMLIsUpgradedNotOverwritten(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, config.RepoConfigFileName)
	const body = "description: keep me\n" +
		"knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: kb\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))

	status, err := EnsureFootprint(root, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, status)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "schema: 2\n")
	require.Contains(t, string(raw), "description: keep me\n")

	backup, err := os.ReadFile(path + ".v1.old")
	require.NoError(t, err)
	require.Equal(t, body, string(backup), "the backup must hold the original bytes")

	loaded, err := config.RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "keep me", loaded.Description)
	require.Equal(t, "kb", loaded.Knowledge.Config.Location)

	for _, cat := range footprintCategories {
		require.FileExists(t, filepath.Join(root, "kb", cat, "README.md"))
	}
	require.NoDirExists(t, filepath.Join(root, "knowledge"),
		"the upgraded file, not the defaults, drives the scaffolding")
}
