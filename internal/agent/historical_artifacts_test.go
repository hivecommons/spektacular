package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// fixtureHistoricalArtifactsFS returns an in-memory template fixture for the
// historical-artifacts tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureHistoricalArtifactsFS() fs.FS {
	return fstest.MapFS{
		historicalArtifactsTemplatePath: &fstest.MapFile{
			Data: []byte("## Historical Artifacts: Specs and Plans as Archaeology\n\nRoute reads through {{command}} plan file read.\n"),
		},
	}
}

const fixtureHistoricalArtifactsRenderedDefault = "## Historical Artifacts: Specs and Plans as Archaeology\n\nRoute reads through go run . plan file read.\n"

func TestInstallHistoricalArtifactsSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureHistoricalArtifactsFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installHistoricalArtifactsSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureHistoricalArtifactsRenderedDefault, string(got))
}

func TestInstallHistoricalArtifactsSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureHistoricalArtifactsFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installHistoricalArtifactsSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureHistoricalArtifactsRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallHistoricalArtifactsSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureHistoricalArtifactsFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installHistoricalArtifactsSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installHistoricalArtifactsSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallHistoricalArtifactsSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureHistoricalArtifactsFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Historical Artifacts: Specs and Plans as Archaeology\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installHistoricalArtifactsSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Historical Artifacts: Specs and Plans as Archaeology\n" +
		"\n" +
		"Route reads through go run . plan file read.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallHistoricalArtifactsSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureHistoricalArtifactsFS())
	tmp := t.TempDir()

	require.NoError(t, installHistoricalArtifactsSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installHistoricalArtifactsSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Historical Artifacts: Specs and Plans as Archaeology\n\nRoute reads through spektacular plan file read.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallHistoricalArtifactsSection_CrossAgentIdempotency(t *testing.T) {
	// Use the real templates.FS so the agents' real Install paths exercise
	// end to end — skills, command wrappers, and the AGENTS.md write.
	tmp := t.TempDir()
	cfg := config.NewDefault()

	for _, name := range []string{"claude", "codex", "bob"} {
		a, err := Lookup(name)
		require.NoError(t, err, "agent %s should be registered", name)
		require.NoError(t, a.Install(tmp, cfg, io.Discard), "Install for %s", name)
	}

	body, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	historicalArtifactsCount := strings.Count(string(body), historicalArtifactsHeading)
	require.Equal(t, 1, historicalArtifactsCount, "exactly one Historical Artifacts heading expected, got %d in:\n%s", historicalArtifactsCount, body)

	memoryContextCount := strings.Count(string(body), "## Memory & Context")
	require.Equal(t, 1, memoryContextCount, "exactly one Memory & Context heading expected, got %d in:\n%s", memoryContextCount, body)

	specTriggerCount := strings.Count(string(body), "## Spec-Worthy Discussion Recognition")
	require.Equal(t, 1, specTriggerCount, "exactly one Spec-Worthy Discussion Recognition heading expected, got %d in:\n%s", specTriggerCount, body)

	// Each agent's skill directory should still be present, confirming the
	// shared AGENTS.md write did not displace any earlier per-agent install.
	for _, dir := range []string{".claude/skills/spek-new", ".agents/skills/spek-new", ".bob/skills/spek-new"} {
		require.DirExists(t, filepath.Join(tmp, dir), "skills dir %s should exist after cross-agent install", dir)
	}
}
