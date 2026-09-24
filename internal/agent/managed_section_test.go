package agent

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// testSectionTemplatePath and testSectionHeading identify a synthetic managed
// section fixture used to exercise installManagedSection as a generic
// mechanism, independent of any real per-agent section.
const (
	testSectionTemplatePath = "agents/test-section.md"
	testSectionHeading      = "## Test Section"
	testSectionActionNoun   = "Test Section"
)

// fixtureTestSectionFS returns an in-memory template fixture for the generic
// managed-section tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureTestSectionFS() fs.FS {
	return fstest.MapFS{
		testSectionTemplatePath: &fstest.MapFile{
			Data: []byte("## Test Section\n\nRoute writes through {{command}} test.\n"),
		},
	}
}

const fixtureTestSectionRenderedDefault = "## Test Section\n\nRoute writes through go run . test.\n"

func installTestSection(projectPath string, cfg config.Config, out io.Writer) error {
	return installManagedSection(projectPath, cfg, out, testSectionTemplatePath, testSectionHeading, testSectionActionNoun)
}

func TestInstallManagedSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureTestSectionFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installTestSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureTestSectionRenderedDefault, string(got))
}

func TestInstallManagedSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureTestSectionFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installTestSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureTestSectionRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallManagedSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureTestSectionFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installTestSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installTestSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")
}

func TestInstallManagedSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureTestSectionFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Test Section\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installTestSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Test Section\n" +
		"\n" +
		"Route writes through go run . test.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallManagedSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureTestSectionFS())
	tmp := t.TempDir()

	require.NoError(t, installTestSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installTestSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Test Section\n\nRoute writes through spektacular test.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}
