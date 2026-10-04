package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeLaneFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func TestLanePaths_KeyedByKindAndName(t *testing.T) {
	dataDir := filepath.Join("proj", ".spektacular")
	require.Equal(t, filepath.Join(dataDir, "workflows", "plan-alpha.json"), LaneStatePath(dataDir, "plan", "alpha"))
	require.Equal(t, filepath.Join(dataDir, "workflows", "implement-alpha.md"), LaneNotesPath(dataDir, "implement", "alpha"))
	require.Equal(t, ".spektacular/workflows/plan-alpha.json", LaneStateRel("plan", "alpha"))
	require.Equal(t, ".spektacular/workflows/implement-alpha.md", LaneNotesRel("implement", "alpha"))
}

func TestLaneNames_MissingFolderIsNoLanes(t *testing.T) {
	require.Empty(t, LaneNames(t.TempDir(), "plan"))
}

func TestLaneNames_SortedAndFilteredByKind(t *testing.T) {
	dataDir := t.TempDir()
	for _, f := range []string{
		"plan-zeta.json", "plan-alpha.json", "plan-mid_1.json",
		"implement-alpha.json", // another kind
		"plan-alpha.md",        // notes, not state
		"plan-beta.json.dryrun-tmp",
	} {
		writeLaneFile(t, filepath.Join(dataDir, LaneDir, f), "{}")
	}
	require.Equal(t, []string{"alpha", "mid_1", "zeta"}, LaneNames(dataDir, "plan"))
	require.Equal(t, []string{"alpha"}, LaneNames(dataDir, "implement"))
}

func TestReadLane_AbsentIsNilNil(t *testing.T) {
	s, err := ReadLane(t.TempDir(), "plan", "nosuch")
	require.NoError(t, err)
	require.Nil(t, s)
}

func TestReadLane_LoadsState(t *testing.T) {
	dataDir := t.TempDir()
	writeLaneFile(t, LaneStatePath(dataDir, "plan", "alpha"),
		`{"kind":"plan","current_step":"discovery","completed_steps":["new","overview"],"data":{"name":"alpha"}}`)

	s, err := ReadLane(dataDir, "plan", "alpha")
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, "plan", s.Kind)
	require.Equal(t, "discovery", s.CurrentStep)
	require.Equal(t, []string{"new", "overview"}, s.CompletedSteps)
	require.Equal(t, "alpha", s.Data["name"])
	require.True(t, s.InProgress())

	// The other kind's lane for the same name is a separate file.
	other, err := ReadLane(dataDir, "implement", "alpha")
	require.NoError(t, err)
	require.Nil(t, other)
}

func TestReadLane_CorruptFileIsAnError(t *testing.T) {
	dataDir := t.TempDir()
	writeLaneFile(t, LaneStatePath(dataDir, "plan", "alpha"), "{not json")
	s, err := ReadLane(dataDir, "plan", "alpha")
	require.Error(t, err)
	require.Nil(t, s)
}
