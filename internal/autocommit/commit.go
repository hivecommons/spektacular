package autocommit

import (
	"fmt"
	"strings"
)

// CommitError names the work tree whose commit failed and git's own reason,
// which is the text the user needs to act on: a rejecting hook's output, a
// signing failure, or whatever else git refused with.
type CommitError struct {
	Target Target
	Cause  error
}

func (e *CommitError) Error() string {
	return fmt.Sprintf("git commit failed in %s (%s): %v",
		strings.Join(e.Target.Repos, ", "), e.Target.Dir, e.Cause)
}

func (e *CommitError) Unwrap() error { return e.Cause }

// DirtyTargets returns the subset of targets with uncommitted changes,
// untracked files included, in the order given.
func DirtyTargets(targets []Target, git Git) ([]Target, error) {
	var dirty []Target
	for _, t := range targets {
		d, err := git.Dirty(t.Dir)
		if err != nil {
			return nil, err
		}
		if d {
			dirty = append(dirty, t)
		}
	}
	return dirty, nil
}

// CommitDirty commits every target that has uncommitted changes, in order,
// with the same message. A clean target is skipped without error, since
// having nothing to commit is an ordinary outcome at a commit point.
//
// It stops at the first failure and returns a *CommitError, along with the
// targets committed before it. Those commits stand: the caller reports the
// failure and halts, and a retry finds the committed targets clean and
// commits only what is left.
func CommitDirty(targets []Target, message string, git Git) ([]Target, error) {
	var committed []Target
	for _, t := range targets {
		dirty, err := git.Dirty(t.Dir)
		if err != nil {
			return committed, &CommitError{Target: t, Cause: err}
		}
		if !dirty {
			continue
		}
		if err := git.CommitAll(t.Dir, message); err != nil {
			return committed, &CommitError{Target: t, Cause: err}
		}
		committed = append(committed, t)
	}
	return committed, nil
}
