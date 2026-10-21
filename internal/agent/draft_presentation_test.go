package agent

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// draftPresentationRenderedDefault is the real
// templates/agents/short/draft-presentation.md content with {{command}} and
// {{guidance_path}} rendered by hand for cfg.Command = "go run .": the short
// section AGENTS.md carries. Kept byte-accurate with the source template rather
// than computed via mustache.Render.
const draftPresentationRenderedDefault = "## Presenting Drafts and Confirmations\n" +
	"\n" +
	"Always show a draft in full as plain chat text first; never put the draft itself inside a yes/no\n" +
	"or multiple-choice dialog. A short confirmation question may follow it.\n" +
	"Managed by `go run . init`; full guidance: [.spektacular/guidance/draft-presentation.md](.spektacular/guidance/draft-presentation.md).\n"

// draftPresentationGuidanceRenderedDefault is the real
// templates/agents/draft-presentation.md content with {{command}} rendered by
// hand for cfg.Command = "go run .": the full guidance written to
// .spektacular/guidance/draft-presentation.md. Kept byte-accurate with the
// source template rather than computed via mustache.Render.
const draftPresentationGuidanceRenderedDefault = "# Presenting Drafts and Confirmations\n" +
	"\n" +
	"> Managed by `go run . init` and refreshed by `go run . migrate` — edit\n" +
	"> `templates/agents/draft-presentation.md` in the Spektacular source, not this file in\n" +
	"> place. Hand edits will not survive the next init. Agents read this file\n" +
	"> when its rule's short section in `AGENTS.md` sends them here, and never edit it.\n" +
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
	require.Equal(t, draftPresentationGuidanceRenderedDefault, readGuidanceFile(t, tmp, "draft-presentation"))
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
	require.Contains(t, string(got), "Managed by `spektacular init`; full guidance:")
	require.NotContains(t, string(got), "go run . init")

	guidance := readGuidanceFile(t, tmp, "draft-presentation")
	require.Contains(t, guidance, "> Managed by `spektacular init`")
	require.NotContains(t, guidance, "go run . init")
}
