package autocommit

import (
	"os"
	"path/filepath"
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
