package agent

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// repoSourcesRenderedDefault is the real templates/agents/repo-sources.md
// content with {{command}} rendered by hand for cfg.Command = "go run .".
// Kept byte-accurate with the source template rather than computed via
// mustache.Render, so a template change that loses the instruction fails
// here instead of silently agreeing with itself.
const repoSourcesRenderedDefault = "## Where the Code Lives — Read This First\n" +
	"\n" +
	"> Managed by `go run . init` — edit `templates/agents/repo-sources.md`\n" +
	"> in the Spektacular source, not this section in place. Hand edits will not\n" +
	"> survive the next init.\n" +
	"\n" +
	"**STOP. The directory you are running in is not necessarily the code.**\n" +
	"\n" +
	"This is a Spektacular project. The code it describes may live anywhere on\n" +
	"disk — a sibling directory, somewhere else entirely on the filesystem, or a\n" +
	"clone this project manages. A project directory holding only\n" +
	"`.spektacular/`, `knowledge/`, and `repos/` is normal, and is **not** an\n" +
	"empty or broken repository: the code is elsewhere, and the project knows\n" +
	"where.\n" +
	"\n" +
	"**Before any code-touching work — searching, reading, grepping, editing,\n" +
	"running tests, verifying, or answering a question about how something\n" +
	"works — run:**\n" +
	"\n" +
	"```\n" +
	"go run . repo list\n" +
	"```\n" +
	"\n" +
	"Every registered repo comes back with a `root`: the absolute path where\n" +
	"that repo's code actually lives. Work in that `root`. Cite files by paths\n" +
	"under it. Run tests from it.\n" +
	"\n" +
	"These rules are not negotiable:\n" +
	"\n" +
	"- **Never assume the directory you started in holds a repo's code.** Check\n" +
	"  with `go run . repo list` first, in every session, before you open the\n" +
	"  first file.\n" +
	"- **Never guess a path** from the project's name, a repo's name, or a\n" +
	"  directory that looks plausible. The registry is the only authority on\n" +
	"  where code lives.\n" +
	"- **Pass the relevant repo's `root` explicitly to every sub-agent you\n" +
	"  launch.** A sub-agent inherits your working directory, not your knowledge\n" +
	"  of where the code is.\n" +
	"- **If a repo's `root` is not present on disk, stop and tell the user.**\n" +
	"  Report the path the registry gave and that it is missing. Do not\n" +
	"  substitute a directory that looks close, and do not silently carry on in\n" +
	"  the project directory instead.\n" +
	"\n" +
	"This rule outranks your default assumptions about working directories, and\n" +
	"it binds everywhere in this project — inside spec, plan and implement\n" +
	"workflows, and equally in ad-hoc questions, unrelated skills, and general\n" +
	"exploration. It is not limited to workflow steps.\n"

// tesslPreamble is the leading title-and-import block a Spektacular project's
// AGENTS.md carries above its first `## ` section.
const tesslPreamble = "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"

func TestInstallRepoSourcesSection_CreatesFromMissing(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "go run ."}, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, repoSourcesRenderedDefault, string(got))
}

// The section's whole purpose is to be read before the agent opens a file, so
// it must land above every other managed section — below only the preamble.
func TestInstallRepoSourcesSection_LandsAboveExistingSections(t *testing.T) {
	tmp := t.TempDir()

	existing := tesslPreamble +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"existing memory rule\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "go run ."}, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		repoSourcesRenderedDefault +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"existing memory rule\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

// An AGENTS.md with no title or imports gets the section at byte zero.
func TestInstallRepoSourcesSection_LandsAtByteZeroWithoutPreamble(t *testing.T) {
	tmp := t.TempDir()

	existing := "## Memory & Context\n\nexisting memory rule\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "go run ."}, io.Discard))

	want := repoSourcesRenderedDefault +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"existing memory rule\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

// A project initialised before this section was top-placed carries it lower
// down the file; init must hoist it rather than update it where it sits.
func TestInstallRepoSourcesSection_HoistsSectionFoundLowerInFile(t *testing.T) {
	tmp := t.TempDir()

	existing := tesslPreamble +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"existing memory rule\n" +
		"\n" +
		"## Where the Code Lives — Read This First\n" +
		"\n" +
		"STALE BODY IN THE WRONG PLACE.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "go run ."}, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		repoSourcesRenderedDefault +
		"\n" +
		"## Memory & Context\n" +
		"\n" +
		"existing memory rule\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallRepoSourcesSection_IsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := tesslPreamble + "\n## Memory & Context\n\nexisting memory rule\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installRepoSourcesSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installRepoSourcesSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallRepoSourcesSection_PicksUpTemplateChange(t *testing.T) {
	tmp := t.TempDir()

	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installRepoSourcesSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Contains(t, string(got), "> Managed by `spektacular init`")
	require.Contains(t, string(got), "spektacular repo list")
	require.NotContains(t, string(got), "go run . repo list")
}

// Every registered agent installs the section, and installs it at the top:
// an agent that skipped it would leave its users with the failure this
// section exists to prevent.
func TestEveryAgentInstallsRepoSourcesSectionAtTheTop(t *testing.T) {
	for _, name := range Supported() {
		t.Run(name, func(t *testing.T) {
			tmp := t.TempDir()
			a, err := Lookup(name)
			require.NoError(t, err)

			require.NoError(t, a.Install(tmp, config.Config{Command: "go run ."}, io.Discard))

			got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
			require.NoError(t, err)
			require.Truef(t, len(string(got)) >= len(repoSourcesRenderedDefault) &&
				string(got)[:len(repoSourcesRenderedDefault)] == repoSourcesRenderedDefault,
				"agent %q must install the Where the Code Lives section at the very top of AGENTS.md", name)
		})
	}
}
