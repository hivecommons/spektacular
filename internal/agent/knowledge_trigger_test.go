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

// fixtureKnowledgeTriggerFS returns an in-memory template fixture for the
// knowledge-trigger tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureKnowledgeTriggerFS() fs.FS {
	return fstest.MapFS{
		knowledgeTriggerTemplatePath: &fstest.MapFile{
			Data: []byte("## Knowledge-Worthy Discovery Recognition\n\nRoute writes through {{command}} knowledge write.\n"),
		},
	}
}

const fixtureKnowledgeTriggerRenderedDefault = "## Knowledge-Worthy Discovery Recognition\n\nRoute writes through go run . knowledge write.\n"

func TestInstallKnowledgeTriggerSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureKnowledgeTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installKnowledgeTriggerSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureKnowledgeTriggerRenderedDefault, string(got))
}

func TestInstallKnowledgeTriggerSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureKnowledgeTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installKnowledgeTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureKnowledgeTriggerRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallKnowledgeTriggerSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureKnowledgeTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installKnowledgeTriggerSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installKnowledgeTriggerSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallKnowledgeTriggerSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureKnowledgeTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Knowledge-Worthy Discovery Recognition\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installKnowledgeTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Knowledge-Worthy Discovery Recognition\n" +
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

func TestInstallKnowledgeTriggerSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureKnowledgeTriggerFS())
	tmp := t.TempDir()

	require.NoError(t, installKnowledgeTriggerSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installKnowledgeTriggerSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Knowledge-Worthy Discovery Recognition\n\nRoute writes through spektacular knowledge write.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallKnowledgeTriggerSection_CrossAgentIdempotency(t *testing.T) {
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

	count := strings.Count(string(body), "## Knowledge-Worthy Discovery Recognition")
	require.Equal(t, 1, count, "exactly one Knowledge-Worthy Discovery Recognition heading expected, got %d in:\n%s", count, body)

	// Each agent's skill directory should still be present, confirming the
	// shared AGENTS.md write did not displace any earlier per-agent install.
	for _, dir := range []string{".claude/skills/spek-new", ".agents/skills/spek-new", ".bob/skills/spek-new"} {
		require.DirExists(t, filepath.Join(tmp, dir), "skills dir %s should exist after cross-agent install", dir)
	}
}
