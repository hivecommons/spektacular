// Package repo resolves a project's registered member repos to local
// directories: the registered location holding a repo's Spektacular files
// is its root, and the source its repo.yaml declares — a directory on disk,
// or a git location cloned into the project's working folder — is where its
// code lives, defaulting to the root. Resolution hands out paths, never file
// contents — agents and stores work directly against the resolved
// directories.
package repo

import (
	"fmt"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/gitexec"
)

// GitRunner is the narrow surface the git provider needs from git: a plain
// clone and two read-only head queries for the staleness check. It is
// deliberately small so tests can fake it and it must never grow into a git
// façade — the commit side has its own, equally narrow interface in the
// autocommit package. Both sit on the shared exec path in internal/gitexec,
// which is the only place in the codebase that shells out.
type GitRunner interface {
	// Clone performs a plain clone of url into dir (never a submodule).
	Clone(url, dir string) error
	// LocalHead returns the commit hash dir's HEAD points at. Read-only.
	LocalHead(dir string) (string, error)
	// RemoteHead returns the commit hash url's HEAD points at, without
	// fetching. Read-only and best-effort: callers treat errors as a notice,
	// never a failure.
	RemoteHead(url string) (string, error)
}

// NewGitRunner returns the GitRunner backed by the user's git binary,
// resolved from PATH at call time (git.exe included on Windows via
// LookPath). Authentication deliberately delegates to whatever credential
// machinery the platform's git provides.
func NewGitRunner() GitRunner {
	return execGitRunner{}
}

type execGitRunner struct{}

// run executes git with args in the current directory, returning trimmed
// stdout. The shared exec path in gitexec decides how the child is
// configured; this runner never needs a working directory or stdin, since
// the clone and head queries carry their own -C and URL arguments.
func (execGitRunner) run(args ...string) (string, error) {
	return gitexec.Run("", nil, args...)
}

func (g execGitRunner) Clone(url, dir string) error {
	_, err := g.run("clone", "--", url, dir)
	return err
}

func (g execGitRunner) LocalHead(dir string) (string, error) {
	return g.run("-C", dir, "rev-parse", "HEAD")
}

func (g execGitRunner) RemoteHead(url string) (string, error) {
	out, err := g.run("ls-remote", "--", url, "HEAD")
	if err != nil {
		return "", err
	}
	// Output shape: "<hash>\tHEAD" (possibly more refs on later lines).
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("git ls-remote returned no HEAD for %s", url)
	}
	return fields[0], nil
}
