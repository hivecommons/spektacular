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

// fixtureDesignTriggerFS returns an in-memory template fixture for the
// design-trigger tests. Its body contains a {{command}} placeholder so the
// template-change-picked-up test can observe a difference in rendered output
// when cfg.Command changes.
func fixtureDesignTriggerFS() fs.FS {
	return fstest.MapFS{
		designTriggerTemplatePath: &fstest.MapFile{
			Data: []byte("## Design-Worthy Detail Recognition\n\nRoute writes through {{command}} design write.\n"),
		},
	}
}

const fixtureDesignTriggerRenderedDefault = "## Design-Worthy Detail Recognition\n\nRoute writes through go run . design write.\n"

func TestInstallDesignTriggerSection_CreatesFromMissing(t *testing.T) {
	withSourceFS(t, fixtureDesignTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	require.NoError(t, installDesignTriggerSection(tmp, cfg, io.Discard))

	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, fixtureDesignTriggerRenderedDefault, string(got))
}

func TestInstallDesignTriggerSection_AppendsAfterTesslBlock(t *testing.T) {
	withSourceFS(t, fixtureDesignTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDesignTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		fixtureDesignTriggerRenderedDefault
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallDesignTriggerSection_IsIdempotent(t *testing.T) {
	withSourceFS(t, fixtureDesignTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n\n@.tessl/RULES.md\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDesignTriggerSection(tmp, cfg, io.Discard))
	first, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.NoError(t, installDesignTriggerSection(tmp, cfg, io.Discard))
	second, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)

	require.Equal(t, string(first), string(second), "second run must produce byte-identical output")

	count := strings.Count(string(second), designTriggerHeading)
	require.Equal(t, 1, count, "exactly one Design-Worthy Detail Recognition heading expected, got %d in:\n%s", count, second)
}

func TestInstallDesignTriggerSection_PreservesSurroundingContent(t *testing.T) {
	withSourceFS(t, fixtureDesignTriggerFS())
	tmp := t.TempDir()
	cfg := config.Config{Command: "go run ."}

	existing := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Design-Worthy Detail Recognition\n" +
		"\n" +
		"STALE BODY THAT SHOULD BE REPLACED.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(existing), 0644))

	require.NoError(t, installDesignTriggerSection(tmp, cfg, io.Discard))

	want := "# Agent Rules <!-- tessl-managed -->\n" +
		"\n" +
		"@.tessl/RULES.md\n" +
		"\n" +
		"## Design-Worthy Detail Recognition\n" +
		"\n" +
		"Route writes through go run . design write.\n" +
		"\n" +
		"## Other Section\n" +
		"\n" +
		"unrelated content preserved verbatim\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallDesignTriggerSection_PicksUpTemplateChange(t *testing.T) {
	withSourceFS(t, fixtureDesignTriggerFS())
	tmp := t.TempDir()

	require.NoError(t, installDesignTriggerSection(tmp, config.Config{Command: "go run ."}, io.Discard))
	require.NoError(t, installDesignTriggerSection(tmp, config.Config{Command: "spektacular"}, io.Discard))

	want := "## Design-Worthy Detail Recognition\n\nRoute writes through spektacular design write.\n"
	got, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	require.Equal(t, want, string(got))
}

func TestInstallDesignTriggerSection_CrossAgentIdempotency(t *testing.T) {
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

	count := strings.Count(string(body), "## Design-Worthy Detail Recognition")
	require.Equal(t, 1, count, "exactly one Design-Worthy Detail Recognition heading expected, got %d in:\n%s", count, body)
}

// renderDesignTriggerSection installs the real, embedded design-trigger
// template through the production install path into a directory the test owns
// and returns the rendered AGENTS.md. Rendering rather than reading the
// committed AGENTS.md is deliberate: a guard that read the committed copy would
// still pass when someone edited the template and never re-ran init.
//
// The command is a hand-chosen literal rather than config.NewDefault()'s so the
// placeholder-rendering assertions cannot be satisfied by an unrendered
// template that happened to spell the default command.
func renderDesignTriggerSection(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()
	require.NoError(t, installDesignTriggerSection(tmp, config.Config{Command: "go run ."}, io.Discard))

	body, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	require.NoError(t, err)
	return string(body)
}

// designTriggerAcceptBranch returns the body of the rendered section's
// `**Accept**` bullet — from its marker up to the `**Defer**` bullet that
// follows it. Scoping the command assertions to that branch is what makes them
// meaningful: naming `design ref add` anywhere else in the section would not
// tell an accepting agent to record the reference.
func designTriggerAcceptBranch(t *testing.T, rendered string) string {
	t.Helper()

	const acceptMarker = "- **Accept**"
	const deferMarker = "- **Defer**"

	start := strings.Index(rendered, acceptMarker)
	require.NotEqual(t, -1, start, "the rendered design-trigger section has no %q bullet", acceptMarker)

	branch := rendered[start:]
	end := strings.Index(branch, deferMarker)
	require.NotEqual(t, -1, end, "the rendered design-trigger section has no %q bullet after %q", deferMarker, acceptMarker)
	return branch[:end]
}

// TestRenderedDesignTriggerSectionOffersRatherThanWrites asserts the rendered
// section tells the agent to offer rather than write, and that a non-answer is
// not treated as agreement. Both phrases are load-bearing: without them the
// standing rule reads as licence to create design documents unprompted, which
// is the exact behaviour the section exists to prevent. Expected substrings are
// hand-maintained literals, never derived from the template.
func TestRenderedDesignTriggerSectionOffersRatherThanWrites(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	anchors := []string{
		"offer — never write a design document",
		"Silence or deflection is not acceptance.",
	}
	for _, needle := range anchors {
		count := strings.Count(rendered, needle)
		require.Equalf(t, 1, count,
			"the rendered Design-Worthy Detail Recognition section must contain %q exactly once (found %d)", needle, count)
	}
}

// TestRenderedDesignTriggerSectionNamesThreeOutcomes asserts all three response
// outcomes survive an edit, and that a decline is stated to be final for that
// detail rather than a "not now" the agent may re-raise.
func TestRenderedDesignTriggerSectionNamesThreeOutcomes(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	anchors := []string{
		"one of three outcomes",
		"- **Accept**",
		"- **Defer**",
		"- **Decline**",
		"decline is final for that detail",
	}
	for _, needle := range anchors {
		count := strings.Count(rendered, needle)
		require.Equalf(t, 1, count,
			"the rendered Design-Worthy Detail Recognition section must contain %q exactly once (found %d)", needle, count)
	}
}

// TestRenderedDesignTriggerAcceptBranchNamesTheWriteCommands asserts the accept
// branch names every command the capture can end in. `design author` is for a
// design Spektacular works out with the user, `design write` for one the user
// handed over and that is stored byte for byte; `design ref add` records the
// reference. Dropping any of them is a broken outcome: an interviewed design
// with nowhere to land, a supplied document silently rewritten, or a document
// nothing references and so invisible to the plan workflow.
func TestRenderedDesignTriggerAcceptBranchNamesTheWriteCommands(t *testing.T) {
	branch := designTriggerAcceptBranch(t, renderDesignTriggerSection(t))

	for _, needle := range []string{"go run . design author", "go run . design write", "go run . design ref add"} {
		require.Contains(t, branch, needle,
			"the accept branch of the Design-Worthy Detail Recognition section must name %q", needle)
	}
}

// TestRenderedDesignTriggerSectionNamesCommandsDirectly guards against the
// section telling the agent to fetch a skill first. Plan 000041 recorded that
// `<command> skill <name>` does not resolve for skills nested under
// templates/skills/workflows/, so an instruction to load one would send the
// agent down a path that errors instead of capturing the design.
func TestRenderedDesignTriggerSectionNamesCommandsDirectly(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	require.NotContains(t, rendered, "skill spek-",
		"the Design-Worthy Detail Recognition section must name the design commands directly, not fetch a skill")
}

// TestRenderedDesignTriggerSectionLeavesNoUnrenderedPlaceholder asserts the
// section's mustache placeholders are substituted, so the installed AGENTS.md
// never hands an agent a literal `{{command}}` to type.
func TestRenderedDesignTriggerSectionLeavesNoUnrenderedPlaceholder(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	require.Contains(t, rendered, "go run . design sources",
		"the {{command}} placeholder must render to the configured command")
	require.NotContains(t, rendered, "{{command}}",
		"the rendered section must not leak the {{command}} placeholder")
}

// TestRenderedDesignTriggerSectionSeparatesAlertnessFromOffering asserts the
// section opens by telling the agent to stay alert throughout a design
// conversation, and states that being alert is a different act from offering.
// Both anchors are needed: an instruction that only mentions alertness, without
// separating it from the offer, collapses back into the old behaviour where the
// agent looked only once a detail had already settled and the moment had passed.
// Expected substrings are hand-maintained literals, never derived from the
// template.
func TestRenderedDesignTriggerSectionSeparatesAlertnessFromOffering(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	anchors := []string{
		"Stay alert whenever a conversation is working out",
		"Being alert is not the same as offering.",
	}
	for _, needle := range anchors {
		count := strings.Count(rendered, needle)
		require.Equalf(t, 1, count,
			"the rendered Design-Worthy Detail Recognition section must contain %q exactly once (found %d)", needle, count)
	}
}

// TestRenderedDesignTriggerSectionNamesThreeEntryCases asserts the section still
// covers all three ways a design enters a project that are easy to walk past: a
// document the user already has, a design already held that the conversation is
// changing, and design talk with no spec yet in existence. Losing any one of
// them narrows the instruction back to a design emerging fresh in conversation.
func TestRenderedDesignTriggerSectionNamesThreeEntryCases(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	anchors := []string{
		"- **The user already has the document.**",
		"- **A design that already exists is being changed.**",
		"- **There is no spec in sight.**",
	}
	for _, needle := range anchors {
		count := strings.Count(rendered, needle)
		require.Equalf(t, 1, count,
			"the rendered Design-Worthy Detail Recognition section must contain %q exactly once (found %d)", needle, count)
	}
}

// TestRenderedDesignTriggerAcceptBranchConditionsTheReferenceOnASpec asserts the
// accept branch records the reference only when a spec exists. The wording this
// replaced made the reference unconditional, which stalls an agent that worked a
// design out before any spec was written, so this is the phrase most likely to
// be lost silently in a future reword.
func TestRenderedDesignTriggerAcceptBranchConditionsTheReferenceOnASpec(t *testing.T) {
	branch := designTriggerAcceptBranch(t, renderDesignTriggerSection(t))

	require.Contains(t, branch, "if a spec exists**, record the reference",
		"the accept branch must make recording the design reference conditional on a spec existing")
}

// TestRenderedDesignTriggerAcceptBranchHandsOffToTheSkillByName asserts the
// accept branch hands the conversation to the design skill by name. Scoping this
// to the accept branch is deliberate: naming the skill anywhere else in the
// section would not tell an accepting agent which skill owns the capture.
// TestRenderedDesignTriggerSectionNamesCommandsDirectly is the matching guard
// that the hand-off stays prose and never becomes a command-line skill fetch.
func TestRenderedDesignTriggerAcceptBranchHandsOffToTheSkillByName(t *testing.T) {
	branch := designTriggerAcceptBranch(t, renderDesignTriggerSection(t))

	require.Contains(t, branch, "invoke the `spek-design` skill",
		"the accept branch must hand off to the design skill by name")
}

// TestRenderedDesignTriggerRoutesTheDesignPointerToConstraints asserts the
// standing guidance says a spec records a referenced design among its
// constraints rather than as technical direction.
//
// The section already lists three homes for design detail — a constraint, a
// one-line steer, or a document of its own — and that list reads as three
// degrees of how binding something is, which it is not: it sorts by how much
// room the detail needs, not by whether the planner may renegotiate it. A
// design document is settled by definition, so the pointer to one is binding
// wherever it is recorded. Without this clarification the neighbouring
// sentence actively suggests the wrong section.
//
// The expected strings are hand-copied from templates/agents/design-trigger.md.
func TestRenderedDesignTriggerRoutesTheDesignPointerToConstraints(t *testing.T) {
	rendered := renderDesignTriggerSection(t)

	require.Contains(t, rendered, "three homes for the *content*, not three degrees of how binding",
		"the guidance must say the three homes sort content by size, not by how binding it is")
	require.Contains(t, rendered, "records that pointer among its **constraints**, never as technical",
		"the guidance must route a referenced design's pointer to constraints")
	require.Contains(t, rendered, "raises a disagreement with the user rather than",
		"the guidance must say the plan escalates a disagreement rather than designing around a referenced design")
}
