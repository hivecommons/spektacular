package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/jumppad-labs/spektacular/internal/repo"
	"github.com/stretchr/testify/require"
)

func TestChangelogFileWriteRead_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "changelog:\n  config:\n    directory: ../docs/changelog\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("changelog body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "20260709000000-release-notes.md", "--from", srcPath})

	require.NoError(t, rootCmd.Execute())

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "read", "20260709000000-release-notes.md"})

	require.NoError(t, rootCmd.Execute())
	// The stored file now carries a frontmatter block; read returns the raw
	// bytes, so strip the block before asserting on body preservation.
	meta, body, err := metadata.Split(stdout.Bytes())
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
	require.Equal(t, "changelog body", string(body))
}

// A central (no --repo) changelog write lands directly under the configured
// changelog directory — no `<project>/` subfolder, because the project owns
// its own store. The project subfolder exists only for repo-routed writes
// where multiple projects may share a member repo. Reading back by the same
// flat name returns the content unchanged.
func TestChangelogFileWrite_LandsFlatInConfiguredChangelogDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	// counter id_method so the `<id>_<slug>.md` name clears validateIDPrefix.
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nchangelog:\n  config:\n    directory: ../docs/changelog\n")

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("implement run record"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	// On disk the record sits directly under the configured changelog
	// directory — no project-named subfolder (which is a repo-routed concern).
	require.FileExists(t, filepath.Join(dir, "docs", "changelog", "000001_feat.md"))
	require.NoFileExists(t, filepath.Join(dir, "docs", "changelog", "testproj", "000001_feat.md"),
		"the central record must not land under a <project>/ subfolder")

	// Reading back by the same flat, agent-facing name returns the content.
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "read", "000001_feat.md"})
	require.NoError(t, rootCmd.Execute())

	meta, body, err := metadata.Split(stdout.Bytes())
	require.NoError(t, err)
	require.NotNil(t, meta, "write must produce a frontmatter block")
	require.Equal(t, "implement run record", string(body))
}

// Criterion 1: the ID-prefix rule is unchanged by the namespace folder —
// validateIDPrefix still sees the ID as the leading segment of the
// user-supplied path, so a name without an ID prefix is rejected and no file
// is created anywhere in the store.
func TestChangelogFileWrite_StillRejectsNameWithoutIDPrefix(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\nchangelog:\n  config:\n    directory: ../docs/changelog\n")

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("body"), 0o644))

	stdout, _, code := runRootCmd(t, "changelog", "file", "write", "nope.md", "--from", srcPath)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "missing_id_prefix", er.Code)
	require.Equal(t, "nope.md", er.Resource)

	require.NoFileExists(t, filepath.Join(dir, "docs", "changelog", "nope.md"))
	require.NoFileExists(t, filepath.Join(dir, "docs", "changelog", "testproj", "nope.md"))
}

func TestChangelogFileList_ShowsAllWrittenRecords(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "changelog:\n  config:\n    directory: ../docs/changelog\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("entry body"), 0o644))

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "20260709000000-alpha.md", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "20260709000001-beta.md", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "list"})

	require.NoError(t, rootCmd.Execute())

	var result struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	names := make([]string, len(result.Files))
	for i, f := range result.Files {
		names[i] = f.Name
	}
	require.ElementsMatch(t, []string{"20260709000000-alpha.md", "20260709000001-beta.md"}, names)
}

// --- Phase 3.2: repo-routed changelog writes (`--repo`) ---

// footprintMemberRepo lays out a member repo in its own temp dir with a valid
// footprint rendered via the production install path (repo.EnsureFootprint),
// so its repo.yaml carries the default changelog directory.
func footprintMemberRepo(t *testing.T) string {
	t.Helper()
	member := t.TempDir()
	_, err := repo.EnsureFootprint(member, config.NewDefaultRepoConfig())
	require.NoError(t, err)
	return member
}

// writeNamedProjectConfig writes a project config carrying an explicit
// project name — unlike writeSpecCommandConfig, which hardcodes `testproj` —
// for tests that need two differently named projects.
func writeNamedProjectConfig(t *testing.T, dir, name, body string) {
	t.Helper()
	writeCurrentConfig(t, dir, "name: "+name+"\n"+body)
}

// memberRepoProject lays out a project (chdir'd into) named `testproj` with a
// `source:` value, counter id_method, and one registered member repo named
// "member" rooted at the returned member dir.
func memberRepoProject(t *testing.T) (projectDir, memberDir string) {
	t.Helper()
	memberDir = footprintMemberRepo(t)
	projectDir = t.TempDir()
	t.Chdir(projectDir)
	writeSpecCommandConfig(t, projectDir,
		"source: https://example.com/testproj\n"+
			"spec:\n  id_method: counter\n"+
			"changelog:\n  config:\n    directory: ../docs/changelog\n"+
			"repos:\n  - name: member\n    location: "+memberDir+"\n")
	return projectDir, memberDir
}

// Criterion 1: writing a changelog entry with --repo routes it into the
// member repo's own changelog store, under the project-named folder of that
// repo's configured changelog directory, with the full provenance front
// matter stamped by the CLI and the staged body — reference line included —
// preserved verbatim. Nothing lands in the central store.
func TestChangelogFileWriteRepo_RoutesToMemberStoreWithProvenance(t *testing.T) {
	projectDir, memberDir := memberRepoProject(t)

	body := "> Derived from project testproj (https://example.com/testproj), spec/plan 000001_feat.\n\nmember-scoped changes"
	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(body), 0o644))

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	// The entry sits in the member repo's own changelog directory (the
	// footprint's default, changelog), namespaced by the
	// PROJECT's name — not in the central store.
	entryPath := filepath.Join(memberDir, "changelog", "testproj", "000001_feat.md")
	require.FileExists(t, entryPath)
	require.NoFileExists(t, filepath.Join(projectDir, "docs", "changelog", "testproj", "000001_feat.md"),
		"a repo-routed write must not also land in the central store")

	content, err := os.ReadFile(entryPath)
	require.NoError(t, err)
	meta, gotBody, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta, "repo-routed write must produce a frontmatter block")
	require.Equal(t, "testproj", meta.Project)
	require.Equal(t, "https://example.com/testproj", meta.ProjectSource)
	require.Equal(t, "000001_feat", meta.Spec)
	require.Equal(t, "000001_feat", meta.Plan)
	require.Equal(t, body, string(gotBody), "the staged body, reference line included, must be preserved verbatim")
}

// Criterion 1: a repo-routed entry reads back through the same flat,
// agent-facing name with `read --repo`.
func TestChangelogFileReadRepo_ReturnsMemberEntry(t *testing.T) {
	memberRepoProject(t)

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("derived entry body"), 0o644))

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "read", "000001_feat.md", "--repo", "member"})
	require.NoError(t, rootCmd.Execute())

	meta, gotBody, err := metadata.Split(stdout.Bytes())
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "derived entry body", string(gotBody))
}

// Criterion 1: a repo-routed write against a member whose footprint has been
// deleted fails with the structured `repo_footprint` error whose next action
// offers the `repo add` repair, and writes nothing.
func TestChangelogFileWriteRepo_MissingFootprintErrorsWithRepairOffer(t *testing.T) {
	_, memberDir := memberRepoProject(t)
	require.NoError(t, os.Remove(filepath.Join(memberDir, "repo.yaml")))

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("body"), 0o644))

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, "changelog", "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath)
	require.Equal(t, 1, code)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "repo_footprint", er.Code)
	require.Contains(t, er.NextAction, "repo add")

	require.NoFileExists(t, filepath.Join(memberDir, "changelog", "testproj", "000001_feat.md"),
		"a footprint failure must not create the entry")
}

// Criterion 2: two projects with different names writing the same entry name
// into the same member repo land in separate project-named folders — no
// collision, no interference — and each project's `read --repo` returns its
// own entry.
func TestChangelogFileWriteRepo_TwoProjectsUseSeparateNamespaceFolders(t *testing.T) {
	memberDir := footprintMemberRepo(t)

	projects := []struct{ name, body string }{
		{"proj-a", "entry derived by proj-a"},
		{"proj-b", "entry derived by proj-b"},
	}
	dirs := make(map[string]string, len(projects))

	for _, p := range projects {
		dir := t.TempDir()
		dirs[p.name] = dir
		t.Chdir(dir)
		writeNamedProjectConfig(t, dir, p.name,
			"spec:\n  id_method: counter\n"+
				"repos:\n  - name: member\n    location: "+memberDir+"\n")

		srcPath := filepath.Join(t.TempDir(), "staged.md")
		require.NoError(t, os.WriteFile(srcPath, []byte(p.body), 0o644))

		resetRootCmd(t)
		setupImplementCmd(t)
		rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath})
		require.NoError(t, rootCmd.Execute())
	}

	// Both entries coexist under their own project-named folders, each with
	// its own project's content and provenance.
	for _, p := range projects {
		content, err := os.ReadFile(filepath.Join(memberDir, "changelog", p.name, "000001_feat.md"))
		require.NoError(t, err)
		meta, gotBody, err := metadata.Split(content)
		require.NoError(t, err)
		require.NotNil(t, meta)
		require.Equal(t, p.name, meta.Project)
		require.Equal(t, p.body, string(gotBody))
	}

	// Each project's read --repo returns its own entry, not the other's.
	for _, p := range projects {
		t.Chdir(dirs[p.name])
		stdout, _ := setupImplementCmd(t)
		rootCmd.SetArgs([]string{"changelog", "file", "read", "000001_feat.md", "--repo", "member"})
		require.NoError(t, rootCmd.Execute())
		_, gotBody, err := metadata.Split(stdout.Bytes())
		require.NoError(t, err)
		require.Equal(t, p.body, string(gotBody))
	}
}

// Regression: a central write (no --repo) carries no provenance keys even
// when the project has a source configured — provenance stamping is strictly
// a repo-routed behavior.
func TestChangelogFileWrite_CentralWriteCarriesNoProvenance(t *testing.T) {
	projectDir, _ := memberRepoProject(t)

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("central record"), 0o644))

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	content, err := os.ReadFile(filepath.Join(projectDir, "docs", "changelog", "000001_feat.md"))
	require.NoError(t, err)
	meta, _, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Project, "central writes must not carry project provenance")
	require.Empty(t, meta.ProjectSource)
	require.Empty(t, meta.Spec)
	require.Empty(t, meta.Plan)
}

// Repo routing is changelog-only: the spec and plan `file write` commands do
// not grow a --repo flag, so passing one is rejected as unknown before any
// write happens.
func TestSpecAndPlanFileWrite_HaveNoRepoFlag(t *testing.T) {
	for _, kind := range []string{"spec", "plan"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, "spec:\n  id_method: counter\n")

			srcPath := filepath.Join(t.TempDir(), "staged.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("body"), 0o644))

			setupImplementCmd(t)
			rootCmd.SetArgs([]string{kind, "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath})

			err := rootCmd.Execute()
			require.Error(t, err)
			require.ErrorContains(t, err, "unknown flag: --repo")
		})
	}
}

// Re-writing an existing member-store entry with --repo stamps provenance
// onto it (the entry keeps/gains the keys) while preserving its created_date.
func TestChangelogFileWriteRepo_RewriteStampsProvenanceAndPreservesCreatedDate(t *testing.T) {
	_, memberDir := memberRepoProject(t)

	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	seeded, err := metadata.Render(metadata.Metadata{
		CreatedDate:    earlier,
		DocumentStatus: metadata.StatusDraft,
	}, []byte("original derived body"))
	require.NoError(t, err)
	entryPath := filepath.Join(memberDir, "changelog", "testproj", "000001_feat.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(entryPath), 0o755))
	require.NoError(t, os.WriteFile(entryPath, seeded, 0o644))

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("updated derived body"), 0o644))

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--repo", "member", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	content, err := os.ReadFile(entryPath)
	require.NoError(t, err)
	meta, gotBody, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "testproj", meta.Project, "a re-write must stamp provenance onto a pre-provenance entry")
	require.Equal(t, "https://example.com/testproj", meta.ProjectSource)
	require.Equal(t, "000001_feat", meta.Spec)
	require.Equal(t, "000001_feat", meta.Plan)
	require.True(t, meta.CreatedDate.Equal(earlier),
		"created_date must be preserved across the provenance-stamping re-write (%s), got %s", earlier, meta.CreatedDate)
	require.Equal(t, "updated derived body", string(gotBody))
}

// `list --repo` enumerates the member store's entries for this project.
func TestChangelogFileListRepo_ListsMemberEntries(t *testing.T) {
	memberRepoProject(t)

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("entry body"), 0o644))

	for _, name := range []string{"000001_feat.md", "000002_more.md"} {
		resetRootCmd(t)
		setupImplementCmd(t)
		rootCmd.SetArgs([]string{"changelog", "file", "write", name, "--repo", "member", "--from", srcPath})
		require.NoError(t, rootCmd.Execute())
	}

	resetRootCmd(t)
	stdout, _ := setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "list", "--repo", "member"})
	require.NoError(t, rootCmd.Execute())

	var result struct {
		Files []struct {
			Name string `json:"name"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &result))
	names := make([]string, len(result.Files))
	for i, f := range result.Files {
		names[i] = f.Name
	}
	require.ElementsMatch(t, []string{"000001_feat.md", "000002_more.md"}, names)
}

// Phase 1.4 criterion 4: a repo-routed changelog write for a member whose
// repo.yaml declares `source: file://<code>` lands in the member's
// changelog store at its LOCATION — <root>/repos/api/.spektacular/changelog/
// <project>/ — and the code directory the source points at stays
// byte-identical.
func TestChangelogFileWriteRepo_MemberWithFileSourceWritesAtLocationNotSource(t *testing.T) {
	projectDir, location, code := sourcedMemberProject(t)
	git := &stubGit{}
	swapRepoGit(t, git)
	// Reuse the standard project shape the routing tests rely on: a source
	// for provenance and the counter id method.
	writeSpecCommandConfig(t, projectDir,
		"source: https://example.com/testproj\n"+
			"spec:\n  id_method: counter\n"+
			"repos:\n"+
			"  - name: testproj\n"+
			"    location: \".\"\n"+
			"  - name: api\n"+
			"    location: ../repos/api/.spektacular\n")
	codeBefore := snapshotDir(t, code)

	srcPath := filepath.Join(t.TempDir(), "staged.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("api-scoped changes"), 0o644))

	resetRootCmd(t)
	setupImplementCmd(t)
	rootCmd.SetArgs([]string{"changelog", "file", "write", "000001_feat.md", "--repo", "api", "--from", srcPath})
	require.NoError(t, rootCmd.Execute())

	entryPath := filepath.Join(projectDir, "repos", "api", ".spektacular", "changelog", "testproj", "000001_feat.md")
	require.FileExists(t, entryPath)
	require.Equal(t, filepath.Join(location, ".spektacular", "changelog", "testproj", "000001_feat.md"), entryPath)
	content, err := os.ReadFile(entryPath)
	require.NoError(t, err)
	_, gotBody, err := metadata.Split(content)
	require.NoError(t, err)
	require.Equal(t, "api-scoped changes", string(gotBody))

	require.NoFileExists(t, filepath.Join(projectDir, ".spektacular", "changelog", "000001_feat.md"),
		"a repo-routed write must not land in the central store")
	require.Equal(t, codeBefore, snapshotDir(t, code), "the code dir the source points at must stay byte-identical")
	require.NoDirExists(t, filepath.Join(code, ".spektacular", "changelog"))
	require.Zero(t, git.calls, "a file source never invokes git")
}

// Criterion 3 (changelog): changelog.config.directory is relative to the
// folder holding config.yaml, so `directory: x` stores entries in
// .spektacular/x.
func TestChangelogFileWrite_CustomDirectoryResolvesFromSettingsFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "changelog:\n  config:\n    directory: x\n")

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("changelog body"), 0o644))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "changelog", "file", "write", "20260709000000-release-notes.md", "--from", srcPath)
	require.Equal(t, 0, code, stdout)
	require.Empty(t, stderr)

	require.FileExists(t, filepath.Join(dir, ".spektacular", "x", "20260709000000-release-notes.md"))
	require.NoDirExists(t, filepath.Join(dir, "x"))
}
