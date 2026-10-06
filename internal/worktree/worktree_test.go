package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/autocommit"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// These tests drive a real git binary against two real repositories laid out
// the way a Spektacular project with a sibling docs repo is on disk:
//
//	<base>/proj      the project repo; registers itself as "testproj" (location
//	                 ".") and the sibling as "docs" with a RELATIVE location
//	<base>/website   the sibling repo, registered as "docs"
//
// The sibling's folder name deliberately differs from its registry name, so
// that its location, resolved relative to a project worktree without the
// overlay, would land on a folder that does not exist rather than
// coincidentally on the spec's docs worktree.

const testSpec = "alpha"

// pinIdentity puts a test git identity in the environment of the code under
// test (gitexec builds its child environment from os.Environ) and masks out
// the user's global and system git configuration.
func pinIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", "spektacular-test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@spektacular.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "spektacular-test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@spektacular.invalid")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

// planBody names testproj, docs, an unregistered repo, and docs again.
const planBody = `# Plan: alpha

## Overview

fixture

## Milestones & Tasks

### Milestone 1: Both repos

#### - [ ] Task: Change the project
**Id:** 0b9f6d2e-5a41-4c7b-9e08-3d1f7a6c2b95
**Repo:** testproj
**Depends on:** none
**Execution:** agent

#### - [ ] Task: Change the docs
**Id:** 7c1e4b0a-9d3f-4e2a-8b61-0f5d2c9a7e34
**Repo:** docs
**Depends on:** none
**Execution:** agent

#### - [ ] Task: Something elsewhere
**Id:** e4a8c3f1-2b6d-4f90-a7c5-91d0b3e6f428
**Repo:** ghost
**Depends on:** none
**Execution:** agent

#### - [ ] Task: Change the docs again
**Id:** 1d2c3b4a-5e6f-4a7b-8c9d-0e1f2a3b4c5d
**Repo:** docs
**Depends on:** none
**Execution:** agent
`

type fixture struct {
	base, proj, site string
}

// projectConfig is the project's config.yaml: the project registers itself
// and the sibling, the sibling relative to the folder holding config.yaml.
func projectConfig() string {
	return fmt.Sprintf("schema: %d\nname: testproj\nrepos:\n  - name: testproj\n    location: .\n  - name: docs\n    location: ../../website/.spektacular\n", config.CurrentProjectSchema)
}

// writeRepoYAML scaffolds a repo.yaml whose code is its folder's parent.
func writeRepoYAML(t *testing.T, dir string) {
	t.Helper()
	rc := config.NewDefaultRepoConfig()
	rc.Source = config.DefaultRepoSource
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, rc.ToYAMLFile(filepath.Join(dir, config.RepoConfigFileName)))
}

// newFixture builds and commits both repos, each on branch main.
func newFixture(t *testing.T) fixture {
	t.Helper()
	gittest.RequireGit(t)
	pinIdentity(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	f := fixture{base: base, proj: filepath.Join(base, "proj"), site: filepath.Join(base, "website")}

	require.NoError(t, os.MkdirAll(f.site, 0o755))
	gittest.RunGit(t, f.site, "init", "-q", "-b", "main")
	writeFile(t, f.site, "lib.txt", "lib v1\n")
	writeRepoYAML(t, filepath.Join(f.site, ".spektacular"))
	gittest.RunGit(t, f.site, "add", "-A")
	gittest.RunGit(t, f.site, "commit", "-q", "-m", "initial site")

	require.NoError(t, os.MkdirAll(f.proj, 0o755))
	gittest.RunGit(t, f.proj, "init", "-q", "-b", "main")
	writeFile(t, f.proj, "main.txt", "main v1\n")
	writeFile(t, f.proj, ".spektacular/config.yaml", projectConfig())
	writeRepoYAML(t, filepath.Join(f.proj, ".spektacular"))
	writeFile(t, f.proj, ".spektacular/specs/alpha.md", "---\ncreated_date: 2026-07-01\n---\n\n# Alpha\n")
	writeFile(t, f.proj, ".spektacular/plans/alpha/plan.md", planBody)
	gittest.RunGit(t, f.proj, "add", "-A")
	gittest.RunGit(t, f.proj, "commit", "-q", "-m", "initial project")
	return f
}

func loadConfig(t *testing.T, projectRoot string) config.Config {
	t.Helper()
	cfg, err := config.FromYAMLFile(filepath.Join(projectRoot, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	return cfg
}

func (f fixture) manager(t *testing.T) Manager {
	t.Helper()
	cfg := loadConfig(t, f.proj)
	set, err := repo.New(cfg, f.proj, nil)
	require.NoError(t, err)
	return Manager{ProjectRoot: f.proj, Config: cfg, Repos: set, Git: NewRunner()}
}

// wt is where the fixture's worktree for repo dir lands for spec.
func (f fixture) wt(spec, dir string) string {
	return filepath.Join(f.proj, ".spektacular", "worktrees", spec, dir)
}

// requireRefusal asserts err is an output refusal with the given code.
func requireRefusal(t *testing.T, err error, code string) *output.ErrorResponse {
	t.Helper()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.True(t, errors.As(err, &er), "want an output refusal, got %T: %v", err, err)
	require.Equal(t, code, er.Code, er.Message)
	require.NotEmpty(t, er.NextAction)
	return er
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gittest.RunGit(t, dir, "add", "-A")
	gittest.RunGit(t, dir, "commit", "-q", "-m", msg)
}

func head(t *testing.T, dir string) string {
	t.Helper()
	return gittest.RunGit(t, dir, "rev-parse", "HEAD")
}

// --- TouchedRepos -----------------------------------------------------------

func TestTouchedRepos_RegisteredInFirstMentionOrderDeduplicated(t *testing.T) {
	f := newFixture(t)
	got, err := TouchedRepos(loadConfig(t, f.proj), store.NewSourceStore(f.proj, "project"), testSpec)
	require.NoError(t, err)
	require.Equal(t, []string{"testproj", "docs"}, got)
}

func TestTouchedRepos_NoPlanIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := TouchedRepos(loadConfig(t, f.proj), store.NewSourceStore(f.proj, "project"), "nonesuch")
	er := requireRefusal(t, err, "plan_not_found")
	require.Equal(t, "nonesuch", er.Resource)
}

// --- Ensure -----------------------------------------------------------------

// Criterion 1: a spec touching two repos gets a worktree in each, on its own
// branch, and asking again returns the very same worktrees.
func TestEnsure_WorktreeInEveryTouchedRepoAndIdempotent(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)

	sw, created, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	require.True(t, created)

	want := SpecWorktrees{
		Spec:    "alpha",
		Project: f.wt("alpha", "testproj"),
		Repos: []RepoWorktree{
			{Repo: "testproj", Path: f.wt("alpha", "testproj"), Branch: "spek/alpha", Top: f.proj},
			{Repo: "docs", Path: f.wt("alpha", "docs"), Branch: "spek/alpha", Top: f.site},
		},
	}
	require.Equal(t, want, sw)

	for _, p := range []string{f.wt("alpha", "testproj"), f.wt("alpha", "docs")} {
		require.Equal(t, "spek/alpha", gittest.RunGit(t, p, "rev-parse", "--abbrev-ref", "HEAD"))
	}
	require.FileExists(t, filepath.Join(f.wt("alpha", "testproj"), "main.txt"))
	require.FileExists(t, filepath.Join(f.wt("alpha", "docs"), "lib.txt"))
	// Each worktree starts from its repo's HEAD.
	require.Equal(t, head(t, f.proj), head(t, f.wt("alpha", "testproj")))
	require.Equal(t, head(t, f.site), head(t, f.wt("alpha", "docs")))

	again, created, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, want, again)

	// The worktrees are rediscovered from git alone.
	found, ok, err := m.Find(testSpec)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, found)
	all, err := m.List()
	require.NoError(t, err)
	require.Equal(t, []SpecWorktrees{want}, all)
}

// Touching only a sibling still gives the project its own worktree first,
// because the implement lane runs inside it.
func TestEnsure_ProjectWorktreeAlwaysComesFirst(t *testing.T) {
	f := newFixture(t)
	sw, _, err := f.manager(t).Ensure(testSpec, []string{"docs"})
	require.NoError(t, err)
	require.Len(t, sw.Repos, 2)
	require.Equal(t, "testproj", sw.Repos[0].Repo)
	require.Equal(t, "docs", sw.Repos[1].Repo)
	require.Equal(t, f.wt("alpha", "testproj"), sw.Project)
}

// An existing spec branch is attached rather than recreated.
func TestEnsure_AttachesAnExistingBranch(t *testing.T) {
	f := newFixture(t)
	gittest.RunGit(t, f.site, "checkout", "-q", "-b", "spek/beta")
	writeFile(t, f.site, "lib.txt", "on the branch\n")
	commitAll(t, f.site, "branch work")
	branchHead := head(t, f.site)
	gittest.RunGit(t, f.site, "checkout", "-q", "main")

	_, created, err := f.manager(t).Ensure("beta", []string{"docs"})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, branchHead, head(t, f.wt("beta", "docs")))
	require.Equal(t, "on the branch\n", readFile(t, filepath.Join(f.wt("beta", "docs"), "lib.txt")))
}

func TestEnsure_TouchedRepoNotOnDiskIsRefused(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	m.Config.Repos = append(m.Config.Repos, config.RepoEntry{Name: "gone", Location: filepath.Join(f.base, "nowhere")})
	set, err := repo.New(m.Config, f.proj, nil)
	require.NoError(t, err)
	m.Repos = set

	_, _, err = m.Ensure(testSpec, []string{"gone"})
	er := requireRefusal(t, err, "worktree_failed")
	require.Contains(t, er.Message, `"gone"`)
}

// Criterion 2: the overlay maps every touched repo into the spec's worktrees,
// and every reader built on the registry follows it from inside the project
// worktree — code roots, knowledge and changelog roots, and commit targets.
func TestEnsure_ReposResolveIntoTheSpecWorktrees(t *testing.T) {
	f := newFixture(t)
	sw, _, err := f.manager(t).Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT, docsWT := f.wt("alpha", "testproj"), f.wt("alpha", "docs")

	var overlay repo.Overlay
	require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(projWT, ".spektacular", repo.OverlayFile))), &overlay))
	require.Equal(t, repo.Overlay{Spec: "alpha", Repos: map[string]string{
		"testproj": filepath.Join(projWT, ".spektacular"),
		"docs":     filepath.Join(docsWT, ".spektacular"),
	}}, overlay)

	cfg := loadConfig(t, sw.Project)
	set, err := repo.New(cfg, sw.Project, nil)
	require.NoError(t, err)

	root, ok := set.LocalRoot("testproj")
	require.True(t, ok)
	require.Equal(t, filepath.Join(projWT, ".spektacular"), root)
	src, ok := set.LocalSource("testproj")
	require.True(t, ok)
	require.Equal(t, projWT, src)

	root, ok = set.LocalRoot("docs")
	require.True(t, ok)
	require.Equal(t, filepath.Join(docsWT, ".spektacular"), root)
	src, ok = set.LocalSource("docs")
	require.True(t, ok)
	require.Equal(t, docsWT, src)

	// Resolve is what the changelog and knowledge stores are rooted at.
	r, err := set.Resolve("docs")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(docsWT, ".spektacular"), r.Root)
	require.Equal(t, docsWT, r.Source)

	targets, err := autocommit.Targets(cfg, sw.Project, autocommit.NewGit())
	require.NoError(t, err)
	require.Equal(t, []autocommit.Target{
		{Repos: []string{"testproj"}, Dir: projWT},
		{Repos: []string{"docs"}, Dir: docsWT},
	}, targets)

	// From the main checkout, nothing changed.
	mainSet, err := repo.New(loadConfig(t, f.proj), f.proj, nil)
	require.NoError(t, err)
	src, ok = mainSet.LocalSource("docs")
	require.True(t, ok)
	require.Equal(t, f.site, src)
}

// Criterion 3: the worktrees and the overlay never reach the main copy's
// commits, and the exclude lines are written once.
func TestEnsure_WorktreesStayOutOfTheMainCopy(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	_, _, err = m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	_, _, err = m.Ensure("beta", []string{"docs"})
	require.NoError(t, err)

	// The record is on disk in the main project, yet git does not list it.
	require.FileExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json"))
	require.FileExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "beta", "record.json"))
	require.Empty(t, gittest.RunGit(t, f.proj, "status", "--porcelain", "--untracked-files=all"))
	// The overlay inside the project worktree is ignored there too.
	require.Empty(t, gittest.RunGit(t, f.wt("alpha", "testproj"), "status", "--porcelain", "--untracked-files=all"))

	gittest.RunGit(t, f.proj, "add", "-A")
	staged := gittest.RunGit(t, f.proj, "ls-files", "-s")
	require.NotContains(t, staged, "160000", "a worktree was staged as an embedded repository")
	require.NotContains(t, staged, ".spektacular/worktrees")
	require.NotContains(t, staged, repo.OverlayFile)
	require.NotContains(t, staged, "record.json")

	exclude := readFile(t, filepath.Join(f.proj, ".git", "info", "exclude"))
	require.Equal(t, 1, strings.Count(exclude, "/.spektacular/worktrees/\n"), exclude)
	require.Equal(t, 1, strings.Count(exclude, "/**/.spektacular/worktree-repos.json\n"), exclude)
	require.Equal(t, 1, strings.Count(exclude, "# Spektacular epic worktrees\n"), exclude)
}

// --- Record -----------------------------------------------------------------

// After Ensure, the main project holds a record naming every touched repo's
// code root inside the spec's worktrees. Both repos' repo.yaml put the code at
// the .spektacular folder's parent, so each code root is the repo's worktree.
func TestEnsure_WritesRecordOfCodeRootsInTheMainProject(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)

	want := Record{Spec: "alpha", Repos: map[string]string{
		"testproj": filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "testproj"),
		"docs":     filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "docs"),
	}}

	// The file sits in the main project, beside the worktree folders.
	var onDisk Record
	require.NoError(t, json.Unmarshal([]byte(readFile(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json"))), &onDisk))
	require.Equal(t, want, onDisk)

	got, ok, err := ReadRecord(f.proj, testSpec)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)

	got, ok, err = m.Record(testSpec)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, want, got)

	// Nothing about the record is written inside a worktree.
	require.NoFileExists(t, filepath.Join(f.wt("alpha", "testproj"), ".spektacular", "worktrees", "alpha", "record.json"))
}

// A spec with no worktrees has no record.
func TestReadRecord_UnknownSpecReportsNone(t *testing.T) {
	f := newFixture(t)
	_, _, err := f.manager(t).Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)

	got, ok, err := ReadRecord(f.proj, "nonesuch")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, Record{}, got)
}

// ReadRecord needs no git at all: it reads a hand-written record from a plain
// directory, keeping absolute roots and dropping relative ones.
func TestReadRecord_WorksWithoutGitAndDropsRelativeRoots(t *testing.T) {
	dir := t.TempDir()
	// No git anywhere: PATH holds nothing, so any git call would fail.
	t.Setenv("PATH", "")
	writeFile(t, dir, ".spektacular/worktrees/alpha/record.json",
		`{"spec":"alpha","repos":{"testproj":"/abs/proj/./code","docs":"relative/docs"}}`)

	got, ok, err := ReadRecord(dir, "alpha")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, Record{Spec: "alpha", Repos: map[string]string{"testproj": "/abs/proj/code"}}, got)

	_, ok, err = ReadRecord(dir, "beta")
	require.NoError(t, err)
	require.False(t, ok)
}

// A malformed record is an error, not silently absent.
func TestReadRecord_MalformedIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".spektacular/worktrees/alpha/record.json", "not json")
	_, _, err := ReadRecord(dir, "alpha")
	require.Error(t, err)
}

// --- Merge ------------------------------------------------------------------

// Criterion 4: a clean merge lands in every repo's main line, then the
// worktrees and branches are gone.
func TestMerge_CleanMergeLandsEverywhereAndCleansUp(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT, docsWT := f.wt("alpha", "testproj"), f.wt("alpha", "docs")

	writeFile(t, projWT, "main.txt", "main v2\n")
	commitAll(t, projWT, "project work")
	writeFile(t, docsWT, "lib.txt", "lib v2\n")
	commitAll(t, docsWT, "docs work")

	res, err := m.Merge(testSpec)
	require.NoError(t, err)
	require.Equal(t, MergeResult{Spec: "alpha", Merged: true, Removed: true}, res)

	require.Equal(t, "main v2\n", readFile(t, filepath.Join(f.proj, "main.txt")))
	require.Equal(t, "lib v2\n", readFile(t, filepath.Join(f.site, "lib.txt")))
	for _, top := range []string{f.proj, f.site} {
		require.Equal(t, "main", gittest.RunGit(t, top, "rev-parse", "--abbrev-ref", "HEAD"))
		parents := strings.Fields(gittest.RunGit(t, top, "rev-list", "--parents", "-n", "1", "HEAD"))
		require.Len(t, parents, 3, "a --no-ff merge commit has two parents")
		require.Empty(t, gittest.RunGit(t, top, "branch", "--list", "spek/alpha"))
		require.Empty(t, gittest.RunGit(t, top, "status", "--porcelain"))
		require.NotContains(t, gittest.RunGit(t, top, "worktree", "list", "--porcelain"), "spek/alpha")
	}
	require.NoDirExists(t, projWT)
	require.NoDirExists(t, docsWT)
	require.NoDirExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha"))
	require.NoFileExists(t, filepath.Join(f.proj, ".spektacular", "worktrees", "alpha", "record.json"))

	_, ok, err := m.Find(testSpec)
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = m.Record(testSpec)
	require.NoError(t, err)
	require.False(t, ok, "the record outlived the merge")
}

// Criterion 5: a conflict in one repo merges nothing anywhere and reports the
// conflicting paths per repo, leaving the worktrees for the user.
func TestMerge_ConflictInOneRepoMergesNothing(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT, docsWT := f.wt("alpha", "testproj"), f.wt("alpha", "docs")

	writeFile(t, projWT, "main.txt", "main v2\n")
	commitAll(t, projWT, "clean project work")
	writeFile(t, docsWT, "lib.txt", "from the spec\n")
	commitAll(t, docsWT, "docs work")
	writeFile(t, f.site, "lib.txt", "from main\n")
	commitAll(t, f.site, "competing docs work")

	projHead, siteHead := head(t, f.proj), head(t, f.site)

	res, err := m.Merge(testSpec)
	require.NoError(t, err)
	require.Equal(t, MergeResult{Spec: "alpha", Conflicts: map[string][]string{"docs": {"lib.txt"}}}, res)

	require.Equal(t, projHead, head(t, f.proj))
	require.Equal(t, siteHead, head(t, f.site))
	require.Equal(t, "main v1\n", readFile(t, filepath.Join(f.proj, "main.txt")))
	for _, top := range []string{f.proj, f.site} {
		require.Empty(t, gittest.RunGit(t, top, "status", "--porcelain"))
		require.NoFileExists(t, filepath.Join(top, ".git", "MERGE_HEAD"))
	}
	_, ok, err := m.Find(testSpec)
	require.NoError(t, err)
	require.True(t, ok, "the worktrees stay for the user")
}

func TestMerge_DirtySpecWorktreeIsRefused(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	writeFile(t, f.wt("alpha", "docs"), "uncommitted.txt", "wip\n")
	siteHead := head(t, f.site)

	_, err = m.Merge(testSpec)
	er := requireRefusal(t, err, "worktree_failed")
	require.Contains(t, er.Message, "uncommitted work")
	require.Equal(t, siteHead, head(t, f.site))
}

func TestMerge_OverlappingMainCopyChangesAreRefused(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	writeFile(t, f.wt("alpha", "testproj"), "main.txt", "main v2\n")
	commitAll(t, f.wt("alpha", "testproj"), "project work")
	writeFile(t, f.proj, "main.txt", "local edit\n")
	projHead := head(t, f.proj)

	_, err = m.Merge(testSpec)
	er := requireRefusal(t, err, "worktree_failed")
	require.Contains(t, er.Message, "main.txt")
	require.Equal(t, projHead, head(t, f.proj))
	require.Equal(t, "local edit\n", readFile(t, filepath.Join(f.proj, "main.txt")))
}

// requireSpektacularRefusal merges alpha, asserts the epic_merge_touches_spektacular
// refusal, and that neither main line moved and the worktrees remain.
func requireSpektacularRefusal(t *testing.T, f fixture, m Manager) *output.ErrorResponse {
	t.Helper()
	projHead, siteHead := head(t, f.proj), head(t, f.site)

	_, err := m.Merge(testSpec)
	er := requireRefusal(t, err, "epic_merge_touches_spektacular")
	require.Equal(t, "alpha", er.Resource)
	require.Contains(t, er.NextAction, "spek/alpha")
	require.Contains(t, er.NextAction, "revert <commit>")
	require.Contains(t, er.NextAction, `epic merge --data '{"spec":"alpha"}'`)

	require.Equal(t, projHead, head(t, f.proj))
	require.Equal(t, siteHead, head(t, f.site))
	for _, top := range []string{f.proj, f.site} {
		require.Empty(t, gittest.RunGit(t, top, "status", "--porcelain"))
		require.NoFileExists(t, filepath.Join(top, ".git", "MERGE_HEAD"))
	}
	_, ok, err := m.Find(testSpec)
	require.NoError(t, err)
	require.True(t, ok, "the worktrees stay for the user")
	return er
}

// A spec branch that changes the project's own .spektacular directory is
// refused before anything merges, even alongside a clean code change.
func TestMerge_ProjectSpektacularChangeIsRefused(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT := f.wt("alpha", "testproj")
	writeFile(t, projWT, "main.txt", "main v2\n")
	writeFile(t, projWT, ".spektacular/plans/alpha/plan.md", planBody+"\nticked in the worktree\n")
	commitAll(t, projWT, "project work and a plan edit")

	er := requireSpektacularRefusal(t, f, m)
	require.Contains(t, er.Message, "testproj: .spektacular/plans/alpha/plan.md")
	require.NotContains(t, er.Message, "main.txt")
	require.NotContains(t, er.Message, "docs:")
	require.Contains(t, er.NextAction, "git -C "+projWT+" revert")
	require.Equal(t, "main v1\n", readFile(t, filepath.Join(f.proj, "main.txt")))
}

// A spec branch that adds a file under a sibling repo's .spektacular
// directory is refused the same way.
func TestMerge_SiblingSpektacularChangeIsRefused(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT, docsWT := f.wt("alpha", "testproj"), f.wt("alpha", "docs")
	writeFile(t, projWT, "main.txt", "main v2\n")
	commitAll(t, projWT, "project work")
	writeFile(t, docsWT, ".spektacular/knowledge/x.md", "a stray entry\n")
	commitAll(t, docsWT, "docs knowledge")

	er := requireSpektacularRefusal(t, f, m)
	require.Contains(t, er.Message, "docs: .spektacular/knowledge/x.md")
	require.NotContains(t, er.Message, "testproj:")
	require.Contains(t, er.NextAction, "git -C "+docsWT+" revert")
	require.NoFileExists(t, filepath.Join(f.site, ".spektacular", "knowledge", "x.md"))
}

// When both repos offend, the refusal lists each repo's paths, in the order
// the worktrees are held, and points the example at the first.
func TestMerge_SpektacularChangesInBothReposAreAllListed(t *testing.T) {
	f := newFixture(t)
	m := f.manager(t)
	_, _, err := m.Ensure(testSpec, []string{"testproj", "docs"})
	require.NoError(t, err)
	projWT, docsWT := f.wt("alpha", "testproj"), f.wt("alpha", "docs")
	writeFile(t, projWT, ".spektacular/specs/alpha.md", "---\ncreated_date: 2026-07-01\n---\n\n# Alpha edited\n")
	commitAll(t, projWT, "spec edit")
	writeFile(t, docsWT, ".spektacular/knowledge/x.md", "a stray entry\n")
	commitAll(t, docsWT, "docs knowledge")

	er := requireSpektacularRefusal(t, f, m)
	require.Equal(t,
		"alpha's branch changes Spektacular's files, which are only ever written in the project, so nothing was merged in any repo: testproj: .spektacular/specs/alpha.md; docs: .spektacular/knowledge/x.md",
		er.Message)
	require.Contains(t, er.NextAction, "git -C "+projWT+" revert")
}

func TestMerge_NoWorktreesIsRefused(t *testing.T) {
	f := newFixture(t)
	_, err := f.manager(t).Merge(testSpec)
	er := requireRefusal(t, err, "worktree_not_found")
	require.Equal(t, "alpha", er.Resource)
}
