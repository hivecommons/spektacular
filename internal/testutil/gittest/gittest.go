// Package gittest holds the helpers shared by the tests that drive a real
// git binary. It lives outside a _test.go file because two packages' tests
// need it — internal/repo's clone-and-head integration tests and
// internal/autocommit's commit integration tests — and a test file cannot
// be imported across packages.
package gittest

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// RequireGit skips the test when git is unavailable or -short is set.
func RequireGit(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git integration test in -short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// RunGit executes git with args in dir, failing the test on error. Author
// and committer identity come from the environment so tests never depend on
// (or touch) the user's git configuration, which is masked out entirely.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=spektacular-test",
		"GIT_AUTHOR_EMAIL=test@spektacular.invalid",
		"GIT_COMMITTER_NAME=spektacular-test",
		"GIT_COMMITTER_EMAIL=test@spektacular.invalid",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return strings.TrimSpace(string(out))
}
