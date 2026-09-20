package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// fixtureSpecTriggerFS returns an in-memory template fixture for the
// spec-trigger tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureSpecTriggerFS() fs.FS {
	return fstest.MapFS{
		specTriggerTemplatePath: &fstest.MapFile{
			Data: []byte("## Spec-Worthy Discussion Recognition\n\nRoute writes through {{command}} spec new.\n"),
		},
	}
}

const fixtureSpecTriggerRenderedDefault = "## Spec-Worthy Discussion Recognition\n\nRoute writes through go run . spec new.\n"

func TestInstallSpecTriggerSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureSpecTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installSpecTriggerSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureSpecTriggerRenderedDefault, string(got))
}

func TestInstallSpecTriggerSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureSpecTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installSpecTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureSpecTriggerRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallSpecTriggerSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureSpecTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installSpecTriggerSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installSpecTriggerSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallSpecTriggerSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureSpecTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Spec-Worthy Discussion Recognition\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installSpecTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Spec-Worthy Discussion Recognition\n" +
		"\n" +
		"Route writes through go run . spec new.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallSpecTriggerSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureSpecTriggerFS())
	tmp := t.TempDir()

	require.NoError(t, installSpecTriggerSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installSpecTriggerSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Spec-Worthy Discussion Recognition\n\nRoute writes through spektacular spec new.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallSpecTriggerSection_CrossAgentIdempotency(t *testing.T) {
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

	specTriggerCount := strings.Count(string(body), "## Spec-Worthy Discussion Recognition")
	require.Equal(t, 1, specTriggerCount, "exactly one Spec-Worthy Discussion Recognition heading expected, got %d in:\n%s", specTriggerCount, body)

	memoryContextCount := strings.Count(string(body), "## Memory & Context")
	require.Equal(t, 1, memoryContextCount, "exactly one Memory & Context heading expected, got %d in:\n%s", memoryContextCount, body)

	draftPresentationCount := strings.Count(string(body), "## Presenting Drafts and Confirmations")
	require.Equal(t, 1, draftPresentationCount, "exactly one Presenting Drafts and Confirmations heading expected, got %d in:\n%s", draftPresentationCount, body)

	knowledgeTriggerCount := strings.Count(string(body), "## Knowledge-Worthy Discovery Recognition")
	require.Equal(t, 1, knowledgeTriggerCount, "exactly one Knowledge-Worthy Discovery Recognition heading expected, got %d in:\n%s", knowledgeTriggerCount, body)

	designTriggerCount := strings.Count(string(body), "## Design-Worthy Detail Recognition")
	require.Equal(t, 1, designTriggerCount, "exactly one Design-Worthy Detail Recognition heading expected, got %d in:\n%s", designTriggerCount, body)

	// Headings must appear in the order Memory & Context -> Knowledge-Worthy
	// Discovery Recognition -> Spec-Worthy Discussion Recognition ->
	// Design-Worthy Detail Recognition -> Presenting Drafts and Confirmations.
	memoryContextIdx := strings.Index(string(body), "## Memory & Context")
	knowledgeTriggerIdx := strings.Index(string(body), "## Knowledge-Worthy Discovery Recognition")
	specTriggerIdx := strings.Index(string(body), "## Spec-Worthy Discussion Recognition")
	designTriggerIdx := strings.Index(string(body), "## Design-Worthy Detail Recognition")
	draftPresentationIdx := strings.Index(string(body), "## Presenting Drafts and Confirmations")
	require.True(t, memoryContextIdx < knowledgeTriggerIdx && knowledgeTriggerIdx < specTriggerIdx && specTriggerIdx < designTriggerIdx && designTriggerIdx < draftPresentationIdx,
		"expected heading order Memory & Context (%d) < Knowledge-Worthy Discovery Recognition (%d) < Spec-Worthy Discussion Recognition (%d) < Design-Worthy Detail Recognition (%d) < Presenting Drafts and Confirmations (%d) in:\n%s",
		memoryContextIdx, knowledgeTriggerIdx, specTriggerIdx, designTriggerIdx, draftPresentationIdx, body)

	// Each agent's skill directory should still be present, confirming the
	// shared AGENTS.md write did not displace any earlier per-agent install.
	for _, dir := range []string{".claude/skills/spek-new", ".agents/skills/spek-new", ".bob/skills/spek-new"} {
		require.DirExists(t, filepath.Join(tmp, dir), "skills dir %s should exist after cross-agent install", dir)
	}
}
