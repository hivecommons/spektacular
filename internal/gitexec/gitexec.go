// Package gitexec is the single place the codebase locates and runs the
// user's git binary. Everything that shells out to git goes through Run:
// the repo package's clone and head queries, and the autocommit package's
// status and commit calls. Keeping one exec path means the credential and
// environment handling below is decided once, and each caller can keep its
// own narrow, fakeable interface on top rather than growing a shared git
// façade.
package gitexec

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Run executes git with args in dir, returning trimmed stdout. An empty dir
// runs git in the current directory; otherwise the call is prefixed with
// -C dir. When stdin is non-nil it is fed to the child, which is how a
// commit message reaches `commit -F -` without ever touching a temp file.
//
// The child environment disables interactive credential prompts so an auth
// failure surfaces as an error instead of hanging an agent-driven session.
// Nothing else about the user's git is overridden: hooks, identity and
// signing configuration all apply exactly as they would to a manual
// invocation.
//
// A failure carries git's own stderr where there is any, since that is the
// text a user needs to act on (a rejecting hook's output, for example).
func Run(dir string, stdin io.Reader, args ...string) (string, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("git is not installed or not on PATH: %w", err)
	}

	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}

	cmd := exec.Command(bin, full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		cmd.Env = append(cmd.Env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	if stdin != nil {
		cmd.Stdin = stdin
	}

	out, err := cmd.Output()
	if err != nil {
		// Stderr alone is enough to explain a failure, including a rejecting
		// hook's output: git redirects a hook's stdout to its own stderr, so
		// a hook that echoes without >&2 still lands here.
		var stderr string
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("git %s: %s", args[0], stderr)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	// TrimSpace (not just newline trimming) absorbs Windows CRLF endings.
	return strings.TrimSpace(string(out)), nil
}
