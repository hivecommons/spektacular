package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyState is an in-progress state as the previous version wrote it: no
// run_id, plus a key this build does not know and a large integer.
const legacyState = `{
  "kind": "spec",
  "current_step": "overview",
  "completed_steps": [
    "new",
    "interview"
  ],
  "created_at": "2026-10-24T09:03:34.911373161Z",
  "updated_at": "2026-10-24T09:05:00.123456789Z",
  "data": {
    "name": "000001_billing",
    "count": 12345678901234567890
  },
  "future_field": {
    "nested": [1, 2.5, "x"]
  }
}`

func writeState(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	return p
}

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	require.NoError(t, dec.Decode(&m))
	return m
}

func TestNewRunIDFormat(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		id := NewRunID()
		require.Regexp(t, `^[0-9a-f]{32}$`, id)
		require.NotEqual(t, strings.Repeat("0", 32), id)
		require.False(t, seen[id])
		seen[id] = true
	}
}

func TestEnsureRunIDBackfillsLegacyStateKeepingEveryField(t *testing.T) {
	p := writeState(t, legacyState)

	id, err := EnsureRunID(p)
	require.NoError(t, err)
	require.Regexp(t, `^[0-9a-f]{32}$`, id)

	after, err := os.ReadFile(p)
	require.NoError(t, err)
	got := decode(t, after)
	want := decode(t, []byte(legacyState))
	require.Equal(t, id, got["run_id"])
	delete(got, "run_id")
	require.Equal(t, want, got, "every other field, unknown keys included, must be kept")

	// The backfilled state loads, keeps its id, and stays in place.
	s, err := loadState(p)
	require.NoError(t, err)
	require.Equal(t, id, s.RunID)
	require.Equal(t, "overview", s.CurrentStep)

	// A second call returns the same id without rewriting the file.
	again, err := EnsureRunID(p)
	require.NoError(t, err)
	require.Equal(t, id, again)
	unchanged, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, after, unchanged)

	// No temporary file is left behind.
	entries, err := os.ReadDir(filepath.Dir(p))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestEnsureRunIDLeavesOtherStatesUntouched(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		wantID string
	}{
		{
			name:   "state that already has an id",
			body:   `{"kind":"spec","run_id":"4bf92f3577b34da6a3ce929d0e0e4736","current_step":"overview","completed_steps":[],"data":{}}`,
			wantID: "4bf92f3577b34da6a3ce929d0e0e4736",
		},
		{
			name: "finished state",
			body: `{"kind":"spec","current_step":"finished","completed_steps":["new"],"data":{}}`,
		},
		{
			name: "state with no current step",
			body: `{"kind":"spec","current_step":"","completed_steps":[],"data":{}}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeState(t, tc.body)
			id, err := EnsureRunID(p)
			require.NoError(t, err)
			require.Equal(t, tc.wantID, id)
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(after))
		})
	}
}

func TestEnsureRunIDMissingStateIsUntouched(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	id, err := EnsureRunID(p)
	require.NoError(t, err)
	require.Empty(t, id)
	_, err = os.Stat(p)
	require.True(t, os.IsNotExist(err))
}

func TestEnsureRunIDUnparseableStateErrors(t *testing.T) {
	p := writeState(t, "{not json")
	_, err := EnsureRunID(p)
	require.Error(t, err)
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "{not json", string(after))
}
