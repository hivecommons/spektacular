package workingcontext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPath(t *testing.T) {
	require.Equal(t, filepath.Join("/proj", ".spektacular", "working-context.md"), Path("/proj"))
}

func TestResetCreatesMissingFileEmpty(t *testing.T) {
	tmp := t.TempDir()
	path := Path(tmp)

	require.NoError(t, Reset(path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Empty(t, string(body), "a fresh workflow starts with no carried-over context")
}

func TestResetDiscardsPreviousSessionContent(t *testing.T) {
	tmp := t.TempDir()
	path := Path(tmp)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("## Decisions\n\nfrom the previous session\n"), 0o644))

	require.NoError(t, Reset(path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Empty(t, string(body))
}

// The roster is reported live by `repo list`, never cached into working-context.md:
// a stale roster in the file an agent reads first is exactly the failure the
// AGENTS.md "Where the Code Lives" section exists to prevent.
func TestResetWritesNoRepoRoster(t *testing.T) {
	tmp := t.TempDir()
	path := Path(tmp)

	require.NoError(t, Reset(path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(body), "## Repos")
	require.NotContains(t, string(body), "spektacular:repos")
}
