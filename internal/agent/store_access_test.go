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

// fixtureStoreAccessFS returns an in-memory template fixture for the
// installation tests. Its body carries a {{command}} placeholder so a test can
// observe the render actually substituting it.
func fixtureStoreAccessFS() fs.FS {
	return fstest.MapFS{
		storeAccessTemplatePath: &fstest.MapFile{
			Data: []byte("# Spektacular's Files Are Reached Through Spektacular\n\nFull guidance: route through {{command}} spec file.\n"),
		},
		storeAccessShortTemplatePath: &fstest.MapFile{
			Data: []byte(storeAccessHeading + "\n\nReach the store with {{command}} spec file.\n"),
		},
	}
}

const fixtureStoreAccessRenderedDefault = "## Spektacular's Files Are Reached Through Spektacular\n\nReach the store with go run . spec file.\n"

// storeAccessShortTemplatePath is the embedded path of the rule's short
// AGENTS.md section, hand-written here rather than taken from the standing-rule table.
const storeAccessShortTemplatePath = "agents/short/store-access.md"

// fixtureStoreAccessGuidanceDefault is the fixture's full guidance rendered
// for cfg.Command = "go run .": it lands in .spektacular/guidance/store-access.md, not in AGENTS.md.
const fixtureStoreAccessGuidanceDefault = "# Spektacular's Files Are Reached Through Spektacular\n\nFull guidance: route through go run . spec file.\n"

func TestInstallStoreAccessSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureStoreAccessFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installStoreAccessSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureStoreAccessRenderedDefault, string(got))
	require.Equal(t, fixtureStoreAccessGuidanceDefault, readGuidanceFile(t, tmp, "store-access"),
		"the full guidance lands in its own guidance file, not in AGENTS.md")
}

// TestInstallStoreAccessSection_IsIdempotent asserts a second install leaves
// one section rather than two. Every managed section shares this property and
// each one is expected to prove it for itself.
func TestInstallStoreAccessSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureStoreAccessFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installStoreAccessSection(tmp, cfg, io.Discard))
	require.NoError(t, installStoreAccessSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	count := strings.Count(string(got), storeAccessHeading)
	require.Equal(t, 1, count, "exactly one store-access heading expected, got %d in:\n%s", count, string(got))
}

// renderStoreAccessSection installs the real embedded templates into a fresh
// temp directory and returns the rendered AGENTS.md, which holds only the
// rule's short section. Tests below assert against hand-maintained literals
// from it; the literals are never derived from the template, since deriving
// them would make the assertions tautological.
func renderStoreAccessSection(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, installStoreAccessSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	body, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	return string(body)
}

// renderStoreAccessGuidance installs the real embedded templates into a fresh
// temp directory and returns the rule's rendered guidance file, which holds
// the full text the short section links to.
func renderStoreAccessGuidance(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, installStoreAccessSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	return readGuidanceFile(t, tmp, "store-access")
}

// TestRenderedStoreAccessSectionForbidsDirectFileTools is the primary
// guardrail this section exists to carry. It was previously asserted three
// times over, once per workflow skill; those copies were removed so the rule
// has a single home, which makes this the only thing standing between an edit
// and a silent regression.
func TestRenderedStoreAccessSectionForbidsDirectFileTools(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	for _, needle := range []string{
		"Never use the `Write` or `Edit` tool on a file under a store directory",
		"never build a store path by hand",
		"Do not use `ls`, `find`, or the `Read` tool to discover what a store holds",
		".spektacular/specs/",
		".spektacular/plans/",
	} {
		require.Containsf(t, rendered, needle,
			"the store-access guidance must contain %q", needle)
	}
}

// TestRenderedStoreAccessSectionNamesEveryStoreCommand guards against the
// section drifting out of step with the CLI surface. A store whose command is
// not named here is a store an agent has no instruction for.
func TestRenderedStoreAccessSectionNamesEveryStoreCommand(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	for _, needle := range []string{
		"go run . spec file",
		"go run . plan file",
		"go run . changelog file",
		"go run . knowledge",
		"go run . design",
		"go run . epic",
		"--from <path>",
		// Removal is a CLI verb too. Without these two, the section names
		// read and write for every store and is silent on removal, which is
		// the silence that made reaching for `rm` look permissible.
		"go run . knowledge delete",
		"go run . design delete",
		"go run . epic delete",
	} {
		require.Containsf(t, rendered, needle,
			"the store-access guidance must name %q", needle)
	}
}

// TestRenderedStoreAccessSectionForbidsRawRemoval asserts the positive half of
// the corpus-wide removal sweep: the section does not merely offer `delete`
// verbs, it says in so many words that reaching for `rm` on a managed file is
// never correct, and that a refused removal is to be acted on rather than
// worked around. Naming the commands without banning the alternative is the
// silence that made `rm` look permissible in the first place.
//
// The literals are kept short on purpose — the template hard-wraps its prose,
// so a needle long enough to span a line break would fail on a harmless reflow.
func TestRenderedStoreAccessSectionForbidsRawRemoval(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	for _, needle := range []string{
		"Removal is a CLI verb too",
		"is never correct — not for a knowledge entry",
		"act on it rather than reaching past it",
	} {
		require.Containsf(t, rendered, needle,
			"the store-access guidance must contain %q", needle)
	}
}

// TestRenderedStoreAccessSectionNamesTheThreeExceptions asserts the scratch
// and hand-off surfaces survive an edit. Without them the rule reads as
// forbidding the working files every workflow depends on, which would be worse
// than not stating it at all.
func TestRenderedStoreAccessSectionNamesTheThreeExceptions(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	for _, needle := range []string{
		".spektacular/tmp/",
		".spektacular/work/<name>/",
		".spektacular/working-context.md",
	} {
		require.Containsf(t, rendered, needle,
			"the store-access guidance must name %q as a surface the agent writes itself", needle)
	}
}

// TestRenderedStoreAccessSectionBindsSubAgents asserts the clause that is the
// whole reason this rule cannot live in the skills. A sub-agent inherits
// AGENTS.md but not the skill that spawned it, so a rule stated only in a
// skill never reaches one.
func TestRenderedStoreAccessSectionBindsSubAgents(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	require.Contains(t, rendered, "binds every sub-agent you launch",
		"the store-access guidance must state that it binds sub-agents")
	// Kept short deliberately: the template hard-wraps its prose, so a literal
	// long enough to span a line break would fail on a harmless reflow.
	require.Contains(t, rendered, "inherits this file but not the",
		"the store-access guidance must explain why a sub-agent needs telling")
}

// TestRenderedStoreAccessSectionCoversInPlaceEdits asserts the small-edit case
// survives. Ticking a phase checkbox is the single most tempting in-place edit
// in the whole system, and the one that silently destroys a spec's recorded
// design references.
func TestRenderedStoreAccessSectionCoversInPlaceEdits(t *testing.T) {
	rendered := renderStoreAccessGuidance(t)

	require.Contains(t, rendered, "ticking a task checkbox",
		"the store-access guidance must name the in-place edit case explicitly")
	require.Contains(t, rendered, "A store write is not a file copy",
		"the store-access guidance must say why an in-place edit is not equivalent")
}

// TestRenderedStoreAccessSectionLeavesNoUnrenderedPlaceholder asserts neither
// installed surface hands an agent a literal {{command}} or {{guidance_path}}
// to type, and that each one did render the configured command.
func TestRenderedStoreAccessSectionLeavesNoUnrenderedPlaceholder(t *testing.T) {
	for surface, rendered := range map[string]string{
		"AGENTS.md section": renderStoreAccessSection(t),
		"guidance file":     renderStoreAccessGuidance(t),
	} {
		require.Containsf(t, rendered, "go run .",
			"the store-access %s must render the configured command", surface)
		require.NotContainsf(t, rendered, "{{",
			"the store-access %s must not leak a mustache placeholder", surface)
	}
}

// TestRenderedStoreAccessSectionStatesTheBindingRuleInline asserts the short
// AGENTS.md section carries the obligation itself, not just a pointer to it:
// an agent that never opens the guidance file must still know to go through
// the CLI, which paths are its own, and that the rule binds sub-agents.
func TestRenderedStoreAccessSectionStatesTheBindingRuleInline(t *testing.T) {
	rendered := renderStoreAccessSection(t)

	for _, needle := range []string{
		"never build a store path by hand",
		"use the `go run .` CLI",
		"--from <path>",
		".spektacular/tmp/",
		".spektacular/work/<name>/",
		".spektacular/working-context.md",
		"sub-agent",
		"say so when delegating",
		"Before touching `.spektacular/`, read the full guidance.",
		"[.spektacular/guidance/store-access.md](.spektacular/guidance/store-access.md)",
	} {
		require.Containsf(t, rendered, needle,
			"the short store-access section must contain %q", needle)
	}
}
