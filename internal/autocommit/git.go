// Package autocommit owns every decision about committing a project's work
// to git on the user's behalf: which registered repos are commit targets,
// which of them have anything to commit, which workflow transitions are
// commit points, which plan milestones have just completed, and whether a
// commit message says what it must.
//
// The package deliberately knows nothing of cobra, the output envelope or
// the workflow engine. It takes a mode, a workflow kind, step names, plan
// text and a Git, and returns plain values, so its rules are unit-testable
// without git or a project on disk. The command layer enforces what it
// decides.
package autocommit

import (
	"strings"

	"github.com/hivecommons/spektacular/internal/gitexec"
)

// Git is the narrow commit-side surface the engine needs. It is the
// counterpart to repo.GitRunner's clone-and-head surface, kept separate and
// equally small so neither grows into a git façade. Tests fake it;
// production wraps the shared exec path in gitexec.
type Git interface {
	// TopLevel reports the work-tree root containing dir. ok is false, with
	// a nil error, when dir is not inside a git work tree at all — that is
	// an ordinary "skip this repo", not a failure.
	TopLevel(dir string) (top string, ok bool, err error)
	// Dirty reports whether the work tree at top has uncommitted changes,
	// untracked files included.
	Dirty(top string) (bool, error)
	// CommitAll stages everything in the work tree at top and commits it
	// with message. Hooks, identity and signing are the user's own: nothing
	// is overridden and nothing is bypassed.
	CommitAll(top, message string) error
}

// NewGit returns the Git backed by the user's git binary.
func NewGit() Git {
	return execGit{}
}

type execGit struct{}

// notARepo is the phrase git uses when a command needing a work tree is run
// outside one. Matching on it is how a registered directory that simply is
// not a git repo is told apart from a real git failure.
const notARepo = "not a git repository"

func (execGit) TopLevel(dir string) (string, bool, error) {
	out, err := gitexec.Run(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), notARepo) {
			return "", false, nil
		}
		return "", false, err
	}
	return out, true, nil
}

func (execGit) Dirty(top string) (bool, error) {
	out, err := gitexec.Run(top, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func (execGit) CommitAll(top, message string) error {
	if _, err := gitexec.Run(top, nil, "add", "-A"); err != nil {
		return err
	}
	// -F - reads the message from stdin, so no temp file is written and the
	// message survives newlines and quoting untouched. No --no-verify and no
	// -c user.*: a rejecting hook must reject, and the commit must be the
	// user's own.
	_, err := gitexec.Run(top, strings.NewReader(message), "commit", "-F", "-")
	return err
}
