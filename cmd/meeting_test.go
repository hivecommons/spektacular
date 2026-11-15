package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// meeting's stdout is a line protocol read by hive: every line must be a
// contract intent, even when the command refuses its flags, and a normal end
// exits 0. It runs outside a project, as a hive runs it.
func TestMeeting_StdoutIsContractLinesOnly(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("SPEK_MEETING_LLM", "none")
	stdout, stderr := setupImplementCmd(t)
	rootCmd.SetIn(strings.NewReader(strings.Join([]string{
		`{"type":"segment","speaker":{"name":"Ana"},"text":"hive, research quorum reconnects","final":true}`,
		`garbage`,
		`{"type":"end"}`,
	}, "\n") + "\n"))
	t.Cleanup(func() { rootCmd.SetIn(nil) })
	rootCmd.SetArgs([]string{"meeting", "--from", "stdin", "--format", "jsonl", "--title", "Sync"})
	require.Equal(t, 0, runRoot())

	var types []string
	for _, l := range strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "stdout line %q", l)
		types = append(types, m["type"].(string))
	}
	require.Equal(t, []string{"heartbeat", "research", "notes.append"}, types)
	require.Contains(t, stderr.String(), "skipping malformed input line")
	require.Contains(t, stderr.String(), "model backend none")
}

func TestMeeting_RefusalKeepsStdoutEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"meeting", "--format", "pretty"},
		{"meeting", "--from", "plaud"},
		{"meeting", "--context", "does-not-exist"},
	} {
		stdout, stderr, code := runRootCmd(t, args...)
		require.Equal(t, 1, code, args)
		require.Empty(t, stdout, args)
		require.Contains(t, stderr, "spektacular meeting:", args)
	}
}
