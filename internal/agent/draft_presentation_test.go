package agent

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// draftPresentationRenderedDefault is the real templates/agents/draft-presentation.md
// content with {{command}} rendered by hand for cfg.Command = "go run .". Kept
// byte-accurate with the source template rather than computed via mustache.Render.
const draftPresentationRenderedDefault = "## Presenting Drafts and Confirmations\n" +
	"\n" +
	"> Managed by `go run . init` — edit `templates/agents/draft-presentation.md`\n" +
	"> in the Spektacular source, not this section in place. Hand edits will not\n" +
	"> survive the next init.\n" +
	"\n" +
	"When you have drafted substantial content for the user to review — an architecture write-up, a set of options, a written section, a summary, a plan or spec excerpt — always present that draft as normal, readable chat text first, in full. Never embed the draft itself inside a structured yes/no or multiple-choice dialog element: that kind of UI truncates or compresses long text, making it hard for the user to actually read what you're proposing.\n" +
	"\n" +
	"Once the draft has been shown in full as plain text, you may then ask a short, direct confirmation question — e.g. \"does this look right, or should anything change?\" — using a structured yes/no or multiple-choice element if one is available. That confirmation step comes strictly after the content has already been presented as text, and is never a substitute for showing it.\n" +
	"\n" +
	"This rule applies across every workflow step that follows a draft-then-confirm pattern — spec, plan, and implement workflows alike — not just one. If a specific step's own instructions don't repeat this explicitly, this standing rule still applies.\n"

func TestInstallDraftPresentationSection_CreatesFromMissing(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installDraftPresentationSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, draftPresentationRenderedDefault, string(got))
}

func TestInstallDraftPresentationSection_AppendsAfterTesslBlock(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDraftPresentationSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		draftPresentationRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallDraftPresentationSection_IsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDraftPresentationSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installDraftPresentationSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallDraftPresentationSection_PreservesSurroundingContent(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Presenting Drafts and Confirmations\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDraftPresentationSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		draftPresentationRenderedDefault +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallDraftPresentationSection_PicksUpTemplateChange(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, installDraftPresentationSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installDraftPresentationSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "> Managed by `spektacular init`")
	require.NotContains(t, string(got), "go run . init")
}
