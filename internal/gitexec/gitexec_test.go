package gitexec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/testutil/gittest"
	"github.com/stretchr/testify/require"
)

// RunCode reports exit 0 with trimmed stdout, exit 1 as an answer rather than
// an error, and anything above 1 as an error.
func TestRunCode_ExitCodes(t *testing.T) {
	gittest.RequireGit(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	gittest.RunGit(t, dir, "init", "-q")

	out, code, err := RunCode(dir, "rev-parse", "--show-toplevel")
	require.NoError(t, err)
	require.Equal(t, 0, code)
	require.Equal(t, dir, out)

	// --verify --quiet on a missing ref exits 1 with no output.
	out, code, err = RunCode(dir, "rev-parse", "--verify", "--quiet", "refs/heads/nope")
	require.NoError(t, err)
	require.Equal(t, 1, code)
	require.Empty(t, out)

	// A real failure (exit 128) is an error carrying git's stderr.
	notRepo, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(notRepo))
	_, code, err = RunCode(notRepo, "rev-parse", "--show-toplevel")
	require.Error(t, err)
	require.Equal(t, 128, code)
	require.Contains(t, err.Error(), "not a git repository")
}
