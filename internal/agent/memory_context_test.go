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

// fixtureMemoryFS returns an in-memory template fixture for the
// memory-context tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureMemoryFS() fs.FS {
	return fstest.MapFS{
		memoryContextTemplatePath: &fstest.MapFile{
			Data: []byte("## Memory & Context\n\nRoute writes through {{command}} knowledge write.\n"),
		},
	}
}

const fixtureRenderedDefault = "## Memory & Context\n\nRoute writes through go run . knowledge write.\n"

func TestInstallMemoryContextSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureMemoryFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installMemoryContextSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureRenderedDefault, string(got))
}

func TestInstallMemoryContextSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureMemoryFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installMemoryContextSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallMemoryContextSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureMemoryFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installMemoryContextSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installMemoryContextSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallMemoryContextSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureMemoryFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installMemoryContextSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"Route writes through go run . knowledge write.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallMemoryContextSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureMemoryFS())
	tmp := t.TempDir()

	require.NoError(t, installMemoryContextSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installMemoryContextSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Memory & Context\n\nRoute writes through spektacular knowledge write.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallMemoryContextSection_CrossAgentIdempotency(t *testing.T) {
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

	count := strings.Count(string(body), "## Memory & Context")
	require.Equal(t, 1, count, "exactly one Memory & Context heading expected, got %d in:\n%s", count, body)

	// Each agent's skill directory should still be present, confirming the
	// shared AGENTS.md write did not displace any earlier per-agent install.
	for _, dir := range []string{".claude/skills/spek-new", ".agents/skills/spek-new", ".bob/skills/spek-new"} {
		require.DirExists(t, filepath.Join(tmp, dir), "skills dir %s should exist after cross-agent install", dir)
	}
}
