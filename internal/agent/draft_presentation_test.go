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
	"When you have drafted content for the user to review — an architecture write-up, a set of options, a written section, a summary, a design, a spec, a plan or test plan, a research write-up — how you show it depends on its size.\n" +
	"\n" +
	"**A large draft goes in a file, not the terminal.** A draft is large when it has more than one section, contains a table or a code block, or runs longer than about twenty lines. A terminal is a poor place to read one: tables, code blocks and long sections scroll past and are hard to navigate. Instead of printing it:\n" +
	"\n" +
	"1. Put the draft where the user can open it. If the step you are in already saves the draft before asking — a workflow working file under `.spektacular/work/<name>/`, or a document already written to its store through the CLI — use that file. Otherwise stage the draft under `.spektacular/tmp/<slug>.md` with your own `Write` tool. Do not write a draft into a store before the user has agreed to it just to have something to link to; the store write happens at the point the step's own instructions put it, so a document the user has not accepted never reads as accepted.\n" +
	"2. Give the user the file's path, relative to the project root, with a short summary of what the draft contains and the decisions it records — a few lines, not the draft again.\n" +
	"3. Then ask whether it is right or what should change. When the user asks for changes, update the same file in place and say what changed; do not print the revised draft in full. If they decline, remove a file you staged under `.spektacular/tmp/` rather than leaving a half-finished proposal on disk.\n" +
	"\n" +
	"**Short content is shown inline.** A sentence, a few bullet points or a one-paragraph section reads fine in the terminal: present it as normal, readable chat text, in full, before asking for confirmation.\n" +
	"\n" +
	"**Never embed a draft inside a structured yes/no or multiple-choice dialog element**, whatever its size: that kind of UI truncates or compresses long text, making it hard for the user to actually read what you're proposing. Once the draft has been shown inline or linked, you may then ask a short, direct confirmation question — e.g. \"does this look right, or should anything change?\" — using a structured yes/no or multiple-choice element if one is available. That confirmation step comes strictly after the content has been shown or linked, and is never a substitute for it.\n" +
	"\n" +
	"This rule applies across every workflow step that follows a draft-then-confirm pattern — spec, plan, and implement workflows alike, and skills such as `spek-design` — not just one. If a specific step's own instructions don't repeat this explicitly, this standing rule still applies, and a step that says to present a draft \"in full\" means in full in the file when the draft is large.\n"

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

// TestDraftPresentation_LinksLargeDraftsInsteadOfPrinting pins the behaviour
// the section asks for: a large draft is put in a file and linked, short
// content is still shown inline, a draft never goes inside a dialog element,
// and nothing reaches a store before the user agrees just to be linked.
func TestDraftPresentation_LinksLargeDraftsInsteadOfPrinting(t *testing.T) {
	for _, want := range []string{
		"**A large draft goes in a file, not the terminal.**",
		"more than one section, contains a table or a code block, or runs longer than about twenty lines",
		"stage the draft under `.spektacular/tmp/<slug>.md`",
		"Give the user the file's path",
		"update the same file in place",
		"Do not write a draft into a store before the user has agreed to it",
		"**Short content is shown inline.**",
		"**Never embed a draft inside a structured yes/no or multiple-choice dialog element**",
	} {
		require.Contains(t, draftPresentationRenderedDefault, want)
	}
	require.NotContains(t, draftPresentationRenderedDefault, "always present that draft as normal, readable chat text first, in full")
}
