package repo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// writeFootprint creates a valid repo.yaml footprint in root.
func writeFootprint(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(root, 0o755))
	require.NoError(t, config.NewDefaultRepoConfig().ToYAMLFile(filepath.Join(root, config.RepoConfigFileName)))
}

// writeSourceFootprint creates a valid repo.yaml footprint in root whose
// source is the given value (a file path or a git location).
func writeSourceFootprint(t *testing.T, root, source string) {
	t.Helper()
	cfg := config.NewDefaultRepoConfig()
	src, err := config.SourceFromInput(source)
	require.NoError(t, err)
	cfg.Source = src
	writeRepoConfig(t, root, cfg)
}

// writeRepoYAML creates root/repo.yaml with the given raw content.
func writeRepoYAML(t *testing.T, root, content string) {
	t.Helper()
	dir := root
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.RepoConfigFileName), []byte(content), 0o644))
}

// writeRepoConfig creates a valid repo.yaml in root carrying
// the given RepoConfig, for tests that need to control the repo's
// descriptive metadata or source rather than just have a valid, empty
// footprint.
func writeRepoConfig(t *testing.T, root string, cfg config.RepoConfig) {
	t.Helper()
	dir := root
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, cfg.ToYAMLFile(filepath.Join(dir, config.RepoConfigFileName)))
}

// newSet builds a Set over projectRoot from the given entries, failing the
// test if construction errors.
func newSet(t *testing.T, projectRoot string, git GitRunner, entries ...config.RepoEntry) *Set {
	t.Helper()
	cfg := config.NewDefault()
	cfg.Repos = entries
	s, err := New(cfg, projectRoot, git)
	require.NoError(t, err)
	return s
}

// gitSourceURL is the git location the fake-git tests declare as a source.
const gitSourceURL = "https://example.com/member.git"

// newGitSourceLocation creates a registered location — a fresh temp dir
// holding a valid repo.yaml whose source is gitSourceURL — and returns it.
func newGitSourceLocation(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeSourceFootprint(t, root, gitSourceURL)
	return root
}

type cloneCall struct{ URL, Dir string }

// fakeGit is a recording GitRunner. Clone materializes the target directory
// so resolution can proceed without a real git binary; the repo's footprint
// lives at its registered location, never inside the clone, so the fake
// writes nothing else.
type fakeGit struct {
	t *testing.T

	CloneCalls      []cloneCall
	LocalHeadCalls  int
	RemoteHeadCalls int

	cloneErr      error
	localHead     string
	localHeadErr  error
	remoteHead    string
	remoteHeadErr error
}

// newFakeGit returns a fake whose local and remote heads agree, so tests that
// don't care about staleness see an empty StaleNote.
func newFakeGit(t *testing.T) *fakeGit {
	return &fakeGit{t: t, localHead: "abc123", remoteHead: "abc123"}
}

func (f *fakeGit) Clone(url, dir string) error {
	f.CloneCalls = append(f.CloneCalls, cloneCall{URL: url, Dir: dir})
	if f.cloneErr != nil {
		return f.cloneErr
	}
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	return nil
}

func (f *fakeGit) LocalHead(dir string) (string, error) {
	f.LocalHeadCalls++
	return f.localHead, f.localHeadErr
}

func (f *fakeGit) RemoteHead(url string) (string, error) {
	f.RemoteHeadCalls++
	return f.remoteHead, f.remoteHeadErr
}

// gitCalls is the total number of git invocations the fake has seen.
func (f *fakeGit) gitCalls() int {
	return len(f.CloneCalls) + f.LocalHeadCalls + f.RemoteHeadCalls
}

// A repo whose repo.yaml declares no source resolves with its code colocated
// with its Spektacular files — Source equals Root — without touching git.
func TestResolve_NoSourceResolvesToRootWithoutGit(t *testing.T) {
	root := t.TempDir()
	writeFootprint(t, root)
	git := newFakeGit(t)

	set := newSet(t, t.TempDir(), git, config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	r, err := set.Resolve("lib")
	require.NoError(t, err)
	require.Equal(t, root, r.Root)
	require.Equal(t, root, r.Source)
	require.False(t, r.Materialized)
	require.Empty(t, r.StaleNote)
	require.Zero(t, git.gitCalls(), "resolving a repo without a source must never invoke git")
}

// Phase 1.3 criterion 1: a repo whose repo.yaml declares a file source
// resolves with Root unchanged and Source set to that directory — for an
// absolute path, a relative path anchored at the folder holding repo.yaml, and a
// file:// path — without materializing anything or invoking git. One without
// a source resolves with Source equal to Root.
func TestResolve_FileSourceResolvesSourceDirWithoutGit(t *testing.T) {
	cases := []struct {
		name   string
		source func(base, code string) string
	}{
		{name: "absolute path", source: func(_, code string) string { return code }},
		{name: "relative path", source: func(_, _ string) string { return filepath.Join("..", "code") }},
		{name: "file scheme", source: func(_, code string) string { return "file://" + code }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "lib")
			code := filepath.Join(base, "code")
			require.NoError(t, os.MkdirAll(code, 0o755))
			writeSourceFootprint(t, root, tc.source(base, code))
			git := newFakeGit(t)

			set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

			r, err := set.Resolve("lib")
			require.NoError(t, err)
			require.Equal(t, root, r.Root)
			require.Equal(t, code, r.Source)
			require.False(t, r.Materialized)
			require.Empty(t, r.StaleNote)
			require.Zero(t, git.gitCalls(), "a file source must never invoke git")
		})
	}

	t.Run("no source", func(t *testing.T) {
		root := t.TempDir()
		writeFootprint(t, root)
		git := newFakeGit(t)

		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

		r, err := set.Resolve("lib")
		require.NoError(t, err)
		require.Equal(t, root, r.Root)
		require.Equal(t, root, r.Source)
		require.False(t, r.Materialized)
		require.Zero(t, git.gitCalls())
	})
}

// Phase 1.3 criterion 2: a git source declared in repo.yaml is cloned into
// <projectRoot>/.spektacular/repos/<name> on first resolution — exactly one
// Clone call with the URL from repo.yaml and that directory — and the clone
// is reused on the next resolution. The result is materialized, its Source
// is the clone, its Root stays the registered location, and a clone whose
// head differs from its remote's carries a stale note.
func TestResolve_GitSourceClonedOnceAndReused(t *testing.T) {
	projectRoot := t.TempDir()
	root := newGitSourceLocation(t)
	git := newFakeGit(t)
	git.localHead = "aaa111"
	git.remoteHead = "bbb222"

	set := newSet(t, projectRoot, git, config.RepoEntry{Name: "member", Location: root})

	wantClone := filepath.Join(projectRoot, ".spektacular", "repos", "member")

	r, err := set.Resolve("member")
	require.NoError(t, err)
	require.Equal(t, root, r.Root)
	require.Equal(t, wantClone, r.Source)
	require.True(t, r.Materialized)
	require.NotEmpty(t, r.StaleNote)
	require.Equal(t, []cloneCall{{URL: gitSourceURL, Dir: wantClone}}, git.CloneCalls)

	again, err := set.Resolve("member")
	require.NoError(t, err)
	require.Equal(t, root, again.Root)
	require.Equal(t, wantClone, again.Source)
	require.True(t, again.Materialized)
	require.NotEmpty(t, again.StaleNote)
	require.Len(t, git.CloneCalls, 1, "second resolve must not clone again")
}

// An unreachable remote degrades to a could-not-check notice while
// resolution still succeeds.
func TestResolve_RemoteHeadErrorDegradesToNotice(t *testing.T) {
	git := newFakeGit(t)
	git.remoteHeadErr = errors.New("network unreachable")

	set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "member", Location: newGitSourceLocation(t)})

	r, err := set.Resolve("member")
	require.NoError(t, err, "an unreachable remote must never fail resolution")
	require.Contains(t, r.StaleNote, "could not check")
}

// Equal local and remote heads produce no stale note.
func TestResolve_EqualHeadsProduceNoStaleNote(t *testing.T) {
	git := newFakeGit(t) // local and remote heads agree by default

	set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "member", Location: newGitSourceLocation(t)})

	r, err := set.Resolve("member")
	require.NoError(t, err)
	require.True(t, r.Materialized)
	require.Empty(t, r.StaleNote)
}

// A LocalHead failure silences the staleness check entirely — empty note,
// successful resolution.
func TestResolve_LocalHeadErrorSilencesStaleCheck(t *testing.T) {
	git := newFakeGit(t)
	git.localHeadErr = errors.New("not a git repository")

	set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "member", Location: newGitSourceLocation(t)})

	r, err := set.Resolve("member")
	require.NoError(t, err)
	require.Empty(t, r.StaleNote)
}

// A clone failure fails resolution, naming the repo and the source.
func TestResolve_CloneErrorFailsResolution(t *testing.T) {
	git := newFakeGit(t)
	git.cloneErr = errors.New("authentication failed")

	set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "member", Location: newGitSourceLocation(t)})

	_, err := set.Resolve("member")
	require.Error(t, err)
	require.Contains(t, err.Error(), "member")
	require.Contains(t, err.Error(), gitSourceURL)
	require.Contains(t, err.Error(), "authentication failed")
}

// Phase 1.3 criterion 3: footprint validation looks for
// <root>/.spektacular/repo.yaml at the registered location even when the
// source points elsewhere — a valid footprint whose file source has no
// .spektacular/ of its own resolves fine, and a location without a
// repo.yaml is a *FootprintError rooted at the location even though a
// source directory exists beside it.
func TestResolve_FootprintValidatedAtRootNotSource(t *testing.T) {
	t.Run("source without its own footprint resolves", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "lib")
		code := filepath.Join(base, "code")
		require.NoError(t, os.MkdirAll(code, 0o755))
		writeSourceFootprint(t, root, code)

		set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{Name: "lib", Location: root})

		r, err := set.Resolve("lib")
		require.NoError(t, err)
		require.Equal(t, root, r.Root)
		require.Equal(t, code, r.Source)
		require.NoDirExists(t, filepath.Join(code, ".spektacular"), "resolution must not scaffold anything under the source")
	})

	t.Run("location without repo.yaml errors at the location", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "lib")
		require.NoError(t, os.MkdirAll(root, 0o755)) // exists, but no footprint inside
		code := filepath.Join(base, "code")
		require.NoError(t, os.MkdirAll(code, 0o755))
		writeFootprint(t, code) // a footprint beside the location must not be picked up

		set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{Name: "lib", Location: root})

		r, err := set.Resolve("lib")
		var fe *FootprintError
		require.ErrorAs(t, err, &fe)
		require.Equal(t, "lib", fe.Repo)
		require.Equal(t, root, fe.Root)
		require.Equal(t, root, r.Root, "the resolved root must accompany the footprint error")
	})
}

// A registered location without a .spektacular/repo.yaml resolves to a
// *FootprintError carrying the repo name and root, while the returned
// ResolvedRepo still has its Root populated for the repair offer.
func TestResolve_MissingFootprintReturnsFootprintError(t *testing.T) {
	root := t.TempDir() // exists, but no footprint inside

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	r, err := set.Resolve("lib")
	require.Error(t, err)

	var fe *FootprintError
	require.ErrorAs(t, err, &fe)
	require.Equal(t, "lib", fe.Repo)
	require.Equal(t, root, fe.Root)
	require.Equal(t, root, r.Root, "the resolved root must accompany the footprint error")
}

// An invalid repo.yaml (unsupported changelog provider) also yields a
// *FootprintError rather than a silent continuation.
func TestResolve_InvalidFootprintReturnsFootprintError(t *testing.T) {
	root := t.TempDir()
	writeRepoYAML(t, root, "changelog:\n  provider: unknown\n")

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	_, err := set.Resolve("lib")
	var fe *FootprintError
	require.ErrorAs(t, err, &fe)
	require.Equal(t, "lib", fe.Repo)
	require.Equal(t, root, fe.Root)
}

// A repo.yaml whose source declares a provider that is neither file nor git
// fails resolution naming the repo, without invoking git.
func TestResolve_UnsupportedSourceProviderErrors(t *testing.T) {
	root := t.TempDir()
	cfg := config.NewDefaultRepoConfig()
	cfg.Source = config.RepoSourceConfig{Provider: "s3", Config: config.RepoSourceLocation{Location: "bucket/lib"}}
	writeRepoConfig(t, root, cfg)
	git := newFakeGit(t)

	set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

	_, err := set.Resolve("lib")
	require.Error(t, err)
	require.Contains(t, err.Error(), "lib")
	require.Contains(t, err.Error(), "s3")
	require.Zero(t, git.gitCalls())
}

// New fails fast on an unsupported provider, naming the repo and the provider.
func TestNew_UnknownProviderFailsFast(t *testing.T) {
	cfg := config.NewDefault()
	cfg.Repos = []config.RepoEntry{{Name: "lib", Provider: "svn", Location: t.TempDir()}}

	set, err := New(cfg, t.TempDir(), newFakeGit(t))
	require.Error(t, err)
	require.Nil(t, set)
	require.Contains(t, err.Error(), "lib")
	require.Contains(t, err.Error(), "svn")
}

// Resolving a name that is not registered errors.
func TestResolve_UnregisteredNameErrors(t *testing.T) {
	set := newSet(t, t.TempDir(), newFakeGit(t))

	_, err := set.Resolve("ghost")
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}

// An empty provider defaults to git and resolves fine.
func TestNew_EmptyProviderDefaultsToGit(t *testing.T) {
	root := t.TempDir()
	writeFootprint(t, root)

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	entries := set.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, config.ProviderGit, entries[0].Provider)

	r, err := set.Resolve("lib")
	require.NoError(t, err)
	require.Equal(t, root, r.Root)
}

// The deprecated local key is folded into Location by WithDefaults, so an
// entry built with Local alone resolves to that directory.
func TestNew_LocalAliasFoldedIntoLocation(t *testing.T) {
	root := t.TempDir()
	writeFootprint(t, root)

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:  "lib",
		Local: root,
	})

	entries := set.Entries()
	require.Len(t, entries, 1)
	require.Equal(t, root, entries[0].Location)
	require.Empty(t, entries[0].Local)

	r, err := set.Resolve("lib")
	require.NoError(t, err)
	require.Equal(t, root, r.Root)
}

// A registered location that does not exist on disk errors with an
// *output.ErrorResponse coded repo_location_missing whose message names the
// missing path; git is never invoked.
func TestResolve_LocationMissingErrorsNamingPath(t *testing.T) {
	projectRoot := t.TempDir()
	git := newFakeGit(t)

	set := newSet(t, projectRoot, git, config.RepoEntry{
		Name:     "lib",
		Location: "gone",
	})

	_, err := set.Resolve("lib")
	require.Error(t, err)

	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "repo_location_missing", er.Code)
	require.Contains(t, er.Message, filepath.Join(projectRoot, ".spektacular", "gone"))
	require.Zero(t, git.gitCalls())
}

// A relative location is resolved from the folder holding config.yaml, so
// a folder beside .spektacular/ is reached with a leading "..".
func TestResolve_RelativeLocationResolvedFromConfigDir(t *testing.T) {
	projectRoot := t.TempDir()
	root := filepath.Join(projectRoot, "vendor", "lib")
	require.NoError(t, os.MkdirAll(root, 0o755))
	writeFootprint(t, root)

	set := newSet(t, projectRoot, newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: filepath.Join("..", "vendor", "lib"),
	})

	r, err := set.Resolve("lib")
	require.NoError(t, err)
	require.Equal(t, root, r.Root)
	require.Equal(t, root, r.Source)
}

// Phase 1.3 criterion 4: LocalSource reports where the repo's code is
// without ever invoking git — the file-source directory, the clone of a git
// source when it exists, nothing for a git source that has not been cloned,
// and the root when no source is declared.
func TestLocalSource_ReportsCodeDirWithoutGit(t *testing.T) {
	t.Run("file source", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "lib")
		code := filepath.Join(base, "code")
		require.NoError(t, os.MkdirAll(code, 0o755))
		writeSourceFootprint(t, root, code)
		git := newFakeGit(t)

		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

		src, ok := set.LocalSource("lib")
		require.True(t, ok)
		require.Equal(t, code, src)
		require.Zero(t, git.gitCalls())
	})

	t.Run("cloned git source", func(t *testing.T) {
		projectRoot := t.TempDir()
		root := newGitSourceLocation(t)
		clone := filepath.Join(projectRoot, ".spektacular", "repos", "member")
		require.NoError(t, os.MkdirAll(clone, 0o755)) // a clone left by an earlier resolution
		git := newFakeGit(t)

		set := newSet(t, projectRoot, git, config.RepoEntry{Name: "member", Location: root})

		src, ok := set.LocalSource("member")
		require.True(t, ok)
		require.Equal(t, clone, src)
		require.Zero(t, git.gitCalls())
	})

	t.Run("uncloned git source", func(t *testing.T) {
		root := newGitSourceLocation(t)
		git := newFakeGit(t)

		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "member", Location: root})

		src, ok := set.LocalSource("member")
		require.False(t, ok)
		require.Empty(t, src)
		require.Zero(t, git.gitCalls(), "LocalSource must never clone")
		require.True(t, set.Present("member"), "the location itself is on disk")
	})

	t.Run("no source", func(t *testing.T) {
		root := t.TempDir()
		writeFootprint(t, root)
		git := newFakeGit(t)

		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

		src, ok := set.LocalSource("lib")
		require.True(t, ok)
		require.Equal(t, root, src)
		require.Zero(t, git.gitCalls())
	})

	t.Run("missing repo.yaml falls back to root", func(t *testing.T) {
		root := t.TempDir() // exists, but no footprint inside
		git := newFakeGit(t)

		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: root})

		src, ok := set.LocalSource("lib")
		require.True(t, ok)
		require.Equal(t, root, src)
		require.Zero(t, git.gitCalls())
	})

	t.Run("location not on disk", func(t *testing.T) {
		git := newFakeGit(t)
		set := newSet(t, t.TempDir(), git, config.RepoEntry{Name: "lib", Location: "gone"})

		src, ok := set.LocalSource("lib")
		require.False(t, ok)
		require.Empty(t, src)
		require.False(t, set.Present("lib"))
		require.Zero(t, git.gitCalls())
	})
}

// DescriptiveMetadata returns the named repo's own descriptive metadata when
// its registered location is on disk and its repo.yaml is readable.
func TestDescriptiveMetadata_OnDiskAndReadableReturnsMetadata(t *testing.T) {
	root := t.TempDir()
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Description = "the documentation repo"
	repoCfg.Role = "documentation"
	repoCfg.Tags = []string{"docs", "markdown"}
	writeRepoConfig(t, root, repoCfg)

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "docs",
		Location: root,
	})

	meta, ok := set.DescriptiveMetadata("docs")
	require.True(t, ok)
	require.Equal(t, "the documentation repo", meta.Description)
	require.Equal(t, "documentation", meta.Role)
	require.Equal(t, []string{"docs", "markdown"}, meta.Tags)
}

// DescriptiveMetadata reads repo.yaml from the registered location, not
// from the clone of a git source: after a git-source repo has been cloned
// (the clone carrying no repo.yaml of its own), the root's metadata is
// still what is reported.
func TestDescriptiveMetadata_GitSourceReadsRootNotClone(t *testing.T) {
	projectRoot := t.TempDir()
	root := t.TempDir()
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Description = "the member repo"
	repoCfg.Source = config.GitSource(gitSourceURL)
	writeRepoConfig(t, root, repoCfg)
	git := newFakeGit(t)

	set := newSet(t, projectRoot, git, config.RepoEntry{Name: "member", Location: root})

	_, err := set.Resolve("member")
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(projectRoot, ".spektacular", "repos", "member", config.RepoConfigFileName))

	meta, ok := set.DescriptiveMetadata("member")
	require.True(t, ok)
	require.Equal(t, "the member repo", meta.Description)
	require.Equal(t, config.GitSource(gitSourceURL), meta.Source)
}

// DescriptiveMetadata reports absent (zero value, false) for a repo whose
// registered location is not on disk, and never invokes git to find out.
func TestDescriptiveMetadata_LocationMissingReturnsAbsentWithoutGit(t *testing.T) {
	git := newFakeGit(t)
	set := newSet(t, t.TempDir(), git, config.RepoEntry{
		Name:     "member",
		Location: filepath.Join(t.TempDir(), "gone"),
	})

	meta, ok := set.DescriptiveMetadata("member")
	require.False(t, ok)
	require.Equal(t, config.RepoConfig{}, meta)
	require.Zero(t, git.gitCalls(), "DescriptiveMetadata must never clone or fetch")
}

// DescriptiveMetadata reports absent, not an error, when the location is on
// disk but its repo.yaml is missing.
func TestDescriptiveMetadata_MissingRepoYAMLReturnsAbsent(t *testing.T) {
	root := t.TempDir() // exists, but no .spektacular/repo.yaml inside

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	meta, ok := set.DescriptiveMetadata("lib")
	require.False(t, ok)
	require.Equal(t, config.RepoConfig{}, meta)
}

// DescriptiveMetadata reports absent, not an error, when the location is on
// disk but its repo.yaml is present and corrupt/invalid.
func TestDescriptiveMetadata_CorruptRepoYAMLReturnsAbsent(t *testing.T) {
	root := t.TempDir()
	writeRepoYAML(t, root, "changelog:\n  provider: unknown\n")

	set := newSet(t, t.TempDir(), newFakeGit(t), config.RepoEntry{
		Name:     "lib",
		Location: root,
	})

	meta, ok := set.DescriptiveMetadata("lib")
	require.False(t, ok)
	require.Equal(t, config.RepoConfig{}, meta)
}

// DescriptiveMetadata reports absent for a name that isn't registered at
// all, without error.
func TestDescriptiveMetadata_UnregisteredNameReturnsAbsent(t *testing.T) {
	set := newSet(t, t.TempDir(), newFakeGit(t))

	meta, ok := set.DescriptiveMetadata("ghost")
	require.False(t, ok)
	require.Equal(t, config.RepoConfig{}, meta)
}

// ResolveAll returns every repo in registry (configuration) order.
func TestResolveAll_ReturnsRegistryOrder(t *testing.T) {
	dirB := t.TempDir()
	writeFootprint(t, dirB)
	dirA := t.TempDir()
	writeFootprint(t, dirA)

	set := newSet(t, t.TempDir(), newFakeGit(t),
		config.RepoEntry{Name: "bravo", Location: dirB},
		config.RepoEntry{Name: "alpha", Location: dirA},
	)

	resolved, err := set.ResolveAll()
	require.NoError(t, err)

	names := make([]string, len(resolved))
	for i, r := range resolved {
		names[i] = r.Name
	}
	require.Equal(t, []string{"bravo", "alpha"}, names)
}
