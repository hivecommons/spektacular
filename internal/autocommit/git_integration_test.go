package autocommit

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// testIdentity is the author/committer the integration tests pin through the
// environment alone, never through git configuration.
const (
	testAuthorName  = "spektacular-test"
	testAuthorEmail = "test@spektacular.invalid"
)

// pinIdentity puts the test's git identity in the environment of the process
// under test. gitexec.Run builds its child environment from os.Environ(), so
// this is what the code under test commits as — and, with the global and
// system configuration masked out, git has no other identity to fall back on.
func pinIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_AUTHOR_NAME", testAuthorName)
	t.Setenv("GIT_AUTHOR_EMAIL", testAuthorEmail)
	t.Setenv("GIT_COMMITTER_NAME", testAuthorName)
	t.Setenv("GIT_COMMITTER_EMAIL", testAuthorEmail)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
}

// writeFile writes name (which may name a subdirectory) under dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// newWorkTree creates a git repo in a fresh temp dir with tracked.txt and
// doomed.txt committed, and returns its path with symlinks resolved, so it
// compares equal to the work-tree root git reports.
func newWorkTree(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	gittest.RunGit(t, dir, "init")
	writeFile(t, dir, "tracked.txt", "original\n")
	writeFile(t, dir, "doomed.txt", "delete me\n")
	gittest.RunGit(t, dir, "add", ".")
	gittest.RunGit(t, dir, "commit", "-m", "initial")
	return dir
}

// Criterion 1: a modified file, a deleted file and a never-added file all
// make the work tree dirty, and all three land in the resulting commit.
func TestIntegration_ModifiedDeletedAndUntrackedAllCommitted(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	writeFile(t, dir, "tracked.txt", "modified\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "doomed.txt")))
	writeFile(t, dir, "untracked.txt", "brand new\n")

	git := NewGit()
	dirty, err := git.Dirty(dir)
	require.NoError(t, err)
	require.True(t, dirty)

	require.NoError(t, git.CommitAll(dir, "commit everything"))

	names := gittest.RunGit(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	require.Equal(t, "doomed.txt\ntracked.txt\nuntracked.txt", names)
}

// Criterion 4: a work tree with nothing to commit reports itself clean, is
// skipped by CommitDirty without an error, and gains no commit.
func TestIntegration_CleanWorkTreeIsNotCommitted(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	git := NewGit()
	dirty, err := git.Dirty(dir)
	require.NoError(t, err)
	require.False(t, dirty)

	committed, err := CommitDirty([]Target{{Repos: []string{"clean"}, Dir: dir}}, "nothing to say", git)
	require.NoError(t, err)
	require.Empty(t, committed)

	require.Equal(t, "1", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
}

// Criterion 5: hooks are not bypassed — a pre-commit hook that refuses the
// commit fails it, and the failure names the registered repo and quotes what
// the hook said.
func TestIntegration_PreCommitHookRejectionIsReported(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook,
		[]byte("#!/bin/sh\necho 'lint-failed-in-hook' >&2\nexit 1\n"), 0o755))

	writeFile(t, dir, "tracked.txt", "modified\n")

	_, err := CommitDirty([]Target{{Repos: []string{"hooked"}, Dir: dir}}, "will be refused", NewGit())

	var commitErr *CommitError
	require.ErrorAs(t, err, &commitErr)
	require.Contains(t, commitErr.Error(), "hooked")
	require.Contains(t, commitErr.Error(), "lint-failed-in-hook")

	require.Equal(t, "1", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
}

// Criterion 5, the other channel: a hook is free to explain itself on stdout
// rather than stderr, and most linters do. Its output must still reach the
// user, or the failure reads as a bare "exit status 1" with nothing to act on.
// This works because git redirects a hook's stdout to its own stderr, which is
// what the exec path captures — so no stdout fallback is needed there, and
// adding one would be dead code. This test is what says so.
func TestIntegration_PreCommitHookOnStdoutIsReported(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook,
		[]byte("#!/bin/sh\necho 'lint-said-no-on-stdout'\nexit 1\n"), 0o755))

	writeFile(t, dir, "tracked.txt", "modified\n")

	_, err := CommitDirty([]Target{{Repos: []string{"hooked"}, Dir: dir}}, "will be refused", NewGit())

	var commitErr *CommitError
	require.ErrorAs(t, err, &commitErr)
	require.Contains(t, commitErr.Error(), "lint-said-no-on-stdout")

	require.Equal(t, "1", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
}

// Criterion 6: the commit is made as the user's own git identity — here
// pinned purely through the environment, with git's global and system
// configuration masked out — proving nothing in the commit path overrides it.
func TestIntegration_CommitUsesTheUsersOwnIdentity(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	writeFile(t, dir, "tracked.txt", "modified\n")
	require.NoError(t, NewGit().CommitAll(dir, "identity check"))

	require.Equal(t, "spektacular-test <test@spektacular.invalid>",
		gittest.RunGit(t, dir, "log", "-1", "--format=%an <%ae>"))
}

// Criterion 6: nothing is pushed — a clone's remote-tracking refs are
// byte-for-byte identical before and after a commit.
func TestIntegration_CommitPushesNothing(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	origin := newWorkTree(t)

	clone := filepath.Join(t.TempDir(), "clone")
	gittest.RunGit(t, t.TempDir(), "clone", origin, clone)

	before := gittest.RunGit(t, clone, "for-each-ref", "refs/remotes")
	require.NotEmpty(t, before, "the clone must have remote-tracking refs for this to prove anything")

	writeFile(t, clone, "tracked.txt", "modified\n")
	require.NoError(t, NewGit().CommitAll(clone, "local only"))

	require.Equal(t, before, gittest.RunGit(t, clone, "for-each-ref", "refs/remotes"))
}

// Criterion 2: a directory that is not inside a git work tree is reported as
// such — not on disk as an error — so Targets can skip it silently.
func TestIntegration_TopLevelOfNonGitDirectory(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)

	top, ok, err := NewGit().TopLevel(t.TempDir())
	require.NoError(t, err)
	require.False(t, ok)
	require.Empty(t, top)
}

// Criterion 3: a subdirectory of a work tree resolves to the work tree's
// root, which is what makes two repos registered inside one git repository
// collapse to a single commit target.
func TestIntegration_TopLevelOfSubdirectoryIsWorkTreeRoot(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)

	sub := filepath.Join(dir, "packages", "api")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	top, ok, err := NewGit().TopLevel(sub)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, dir, top)
}

// commitPathsTree extends newWorkTree with two committed lanes' files, so a
// path-scoped commit has a whole tracked directory to delete and another
// lane's files to leave alone.
func commitPathsTree(t *testing.T) string {
	t.Helper()
	dir := newWorkTree(t)
	writeFile(t, dir, "lanes/a/state.json", "{}\n")
	writeFile(t, dir, "lanes/a/notes.md", "a notes\n")
	writeFile(t, dir, "lanes/b/state.json", "{}\n")
	gittest.RunGit(t, dir, "add", ".")
	gittest.RunGit(t, dir, "commit", "-m", "lanes")
	return dir
}

// CommitPaths commits additions and the deletion of a whole tracked directory
// under the given paths, and nothing else: a modified file, an untracked file
// and an already-staged deletion outside the paths are all left exactly as
// they were, the staged one still staged. A path that neither exists nor is
// tracked is skipped rather than failing the pathspec.
func TestIntegration_CommitPathsCommitsOnlyTheGivenPaths(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := commitPathsTree(t)

	writeFile(t, dir, "mine/plan.md", "my plan\n")
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "lanes", "a")))
	writeFile(t, dir, "lanes/b/state.json", "{\"step\":\"tasks\"}\n")
	writeFile(t, dir, "tracked.txt", "modified\n")
	writeFile(t, dir, "theirs/plan.md", "their plan\n")
	gittest.RunGit(t, dir, "rm", "-q", "doomed.txt")

	paths := []string{
		filepath.Join(dir, "mine"),
		filepath.Join(dir, "lanes", "a"),
		filepath.Join(dir, "never-created"),
	}
	require.NoError(t, NewGit().CommitPaths(dir, paths, "commit mine"))

	require.Equal(t, "3", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
	require.Equal(t, "commit mine", gittest.RunGit(t, dir, "log", "-1", "--format=%B"))
	require.Equal(t,
		"D\tlanes/a/notes.md\nD\tlanes/a/state.json\nA\tmine/plan.md",
		gittest.RunGit(t, dir, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"))
	require.Equal(t,
		"D  doomed.txt\n M lanes/b/state.json\n M tracked.txt\n?? theirs/",
		gittest.RunGit(t, dir, "status", "--porcelain"))
}

// With every path either unchanged or absent, CommitPaths makes no commit and
// reports no error, though the rest of the tree is dirty.
func TestIntegration_CommitPathsWithNothingToCommitMakesNoCommit(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := commitPathsTree(t)
	writeFile(t, dir, "tracked.txt", "modified\n")

	git := NewGit()
	require.NoError(t, git.CommitPaths(dir, []string{filepath.Join(dir, "lanes", "a")}, "unchanged"))
	require.NoError(t, git.CommitPaths(dir, []string{filepath.Join(dir, "never-created")}, "absent"))

	require.Equal(t, "2", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
	// RunGit trims the output, so the leading space of " M" is gone.
	require.Equal(t, "M tracked.txt", gittest.RunGit(t, dir, "status", "--porcelain"))
}

// Two path-scoped commits started at the same moment against one work tree,
// each under the commit lock, both succeed one after the other, and each
// commit holds only its own files.
func TestIntegration_ConcurrentCommitPathsUnderTheLockBothSucceed(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := newWorkTree(t)
	dataDir := filepath.Join(dir, ".spektacular")
	writeFile(t, dir, "alpha/plan.md", "alpha\n")
	writeFile(t, dir, "beta/plan.md", "beta\n")

	git := NewGit()
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, name := range []string{"alpha", "beta"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := AcquireLock(dataDir)
			if err != nil {
				errs[i] = err
				return
			}
			defer release()
			errs[i] = git.CommitPaths(dir, []string{filepath.Join(dir, name)}, "commit "+name)
		}()
	}
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])

	require.Equal(t, "3", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
	for _, name := range []string{"alpha", "beta"} {
		rev := gittest.RunGit(t, dir, "log", "-1", "--format=%H", "--grep", "commit "+name)
		require.Equal(t, name+"/plan.md",
			gittest.RunGit(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", rev))
	}
	require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain"), "the lock is released after both")
}

// A deletion already staged under a path — what a commit attempt a hook
// refused leaves behind, since the path was added before git commit ran — is
// still a change under that path, and a retry commits it rather than skipping
// the path as unknown and leaving the deletion staged.
func TestIntegration_CommitPathsCommitsADeletionAlreadyStaged(t *testing.T) {
	gittest.RequireGit(t)
	pinIdentity(t)
	dir := commitPathsTree(t)

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "lanes", "a")))
	gittest.RunGit(t, dir, "add", "-A", "--", "lanes/a")

	require.NoError(t, NewGit().CommitPaths(dir, []string{filepath.Join(dir, "lanes", "a")}, "retry"))

	require.Equal(t, "3", gittest.RunGit(t, dir, "rev-list", "--count", "HEAD"))
	require.Equal(t, "D\tlanes/a/notes.md\nD\tlanes/a/state.json",
		gittest.RunGit(t, dir, "diff-tree", "--no-commit-id", "--name-status", "-r", "HEAD"))
	require.Empty(t, gittest.RunGit(t, dir, "status", "--porcelain"))
}
