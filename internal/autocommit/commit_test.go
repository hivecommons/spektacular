package autocommit

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirtyTargets_ReturnsOnlyDirtyTargetsInOrder(t *testing.T) {
	targets := []Target{
		{Repos: []string{"one"}, Dir: "/work/one"},
		{Repos: []string{"two"}, Dir: "/work/two"},
		{Repos: []string{"three"}, Dir: "/work/three"},
	}
	git := &fakeGit{dirty: map[string]bool{
		"/work/one":   true,
		"/work/two":   false,
		"/work/three": true,
	}}

	dirty, err := DirtyTargets(targets, git)

	require.NoError(t, err)
	require.Equal(t, []Target{
		{Repos: []string{"one"}, Dir: "/work/one"},
		{Repos: []string{"three"}, Dir: "/work/three"},
	}, dirty)
}

// Criterion 4: a target with nothing to commit is skipped — no commit is
// attempted for it and no error is returned.
func TestCommitDirty_SkipsCleanTargets(t *testing.T) {
	targets := []Target{
		{Repos: []string{"clean"}, Dir: "/work/clean"},
		{Repos: []string{"dirty"}, Dir: "/work/dirty"},
	}
	git := &fakeGit{dirty: map[string]bool{"/work/dirty": true}}

	committed, err := CommitDirty(targets, "a message", git)

	require.NoError(t, err)
	require.Equal(t, []Target{{Repos: []string{"dirty"}, Dir: "/work/dirty"}}, committed)
	require.Equal(t, []commitCall{{Dir: "/work/dirty", Message: "a message"}}, git.commitCalls)
}

// Criterion 5: a rejected commit stops the run at that target, reports a
// *CommitError naming the repos and the work tree and quoting git's reason,
// and hands back the targets already committed so the caller knows what
// stands.
func TestCommitDirty_StopsAtFirstFailureAndReportsIt(t *testing.T) {
	targets := []Target{
		{Repos: []string{"first"}, Dir: "/work/first"},
		{Repos: []string{"second", "second-sub"}, Dir: "/work/second"},
		{Repos: []string{"third"}, Dir: "/work/third"},
	}
	hook := errors.New("git commit: pre-commit hook rejected this change")
	git := &fakeGit{
		dirty: map[string]bool{
			"/work/first":  true,
			"/work/second": true,
			"/work/third":  true,
		},
		commitErr: map[string]error{"/work/second": hook},
	}

	committed, err := CommitDirty(targets, "a message", git)

	require.Equal(t, []Target{{Repos: []string{"first"}, Dir: "/work/first"}}, committed)
	require.Equal(t, []commitCall{
		{Dir: "/work/first", Message: "a message"},
		{Dir: "/work/second", Message: "a message"},
	}, git.commitCalls, "the run must stop at the failure, leaving /work/third untouched")

	var commitErr *CommitError
	require.ErrorAs(t, err, &commitErr)
	require.Equal(t,
		"git commit failed in second, second-sub (/work/second): git commit: pre-commit hook rejected this change",
		commitErr.Error())
	require.ErrorIs(t, err, hook)
}
