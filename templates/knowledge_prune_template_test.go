package templates_test

import (
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/stretchr/testify/require"
)

// pruneEntryTemplate is the instruction `knowledge prune` hands the agent for
// each entry it checks.
const pruneEntryTemplate = "knowledge/prune-entry.md"

// pruneEntryBody carries characters mustache would HTML-escape, so a
// double-brace tag on the body would show up as &lt;, &amp; or &#34;.
const pruneEntryBody = "# Widgets\n\nEvery command publishes a schema. <b> & \"q\""

// renderPruneEntry renders the per-entry instruction the way the command does.
func renderPruneEntry(t *testing.T, dryRun bool) string {
	t.Helper()
	out, err := stepkit.RenderTemplate(pruneEntryTemplate, map[string]any{
		"command":   "spektacular",
		"tier":      "repo",
		"name":      "docs",
		"path":      "learnings/widgets.md",
		"body":      pruneEntryBody,
		"dry_run":   dryRun,
		"remaining": 3,
	})
	require.NoError(t, err)
	require.NotContains(t, out, "{{", "no mustache tag may survive rendering")
	return out
}

// Prune criterion 5: in a real run and a dry run alike, the instruction tells
// the agent that an entry stating a standard the code has not yet met is
// current, that only an entry whose subject no longer exists is deleted, that
// a change needs evidence, and not to read or search the entry; it carries the
// exact verdict command for the entry and inlines the entry's text verbatim.
func TestPruneEntryTemplate_StatesTheVerdictRulesAndInlinesTheEntry(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		out := renderPruneEntry(t, dryRun)
		text := flat(out)
		for _, phrase := range []string{
			"states a standard the code has not yet met is **current**",
			"is **current**",
			"only an entry whose subject no longer exists is deleted",
			"Evidence is required for every change.",
			"Do not run `spektacular knowledge read` or `spektacular knowledge search` for this entry",
			"3 entries are left to check",
		} {
			require.Contains(t, text, phrase, "dry_run=%v", dryRun)
		}
		require.Contains(t, out,
			`spektacular knowledge prune verdict --data '{"tier":"repo","name":"docs","path":"learnings/widgets.md","verdict":"<current|corrected|deleted>","evidence":"<what you checked and what you found>"}'`,
			"dry_run=%v", dryRun)
		require.Contains(t, out, "````markdown\n"+pruneEntryBody+"\n````", "the body is inlined verbatim, dry_run=%v", dryRun)
		for _, escaped := range []string{"&lt;", "&gt;", "&amp;", "&#34;", "&quot;"} {
			require.NotContains(t, out, escaped, "dry_run=%v", dryRun)
		}
	}
}

// Prune criteria 5 and 6: a real run tells the agent how to correct and
// delete the entry at its address; a dry run says to change nothing and
// carries neither instruction.
func TestPruneEntryTemplate_DryRunOmitsTheChangeInstructions(t *testing.T) {
	write := `spektacular knowledge write --data '{"tier":"repo","name":"docs","path":"learnings/widgets.md"}' --file .spektacular/tmp/<slug>.md`
	del := `spektacular knowledge delete --data '{"tier":"repo","name":"docs","path":"learnings/widgets.md"}'`

	real := renderPruneEntry(t, false)
	require.Contains(t, real, write)
	require.Contains(t, real, del)
	require.NotContains(t, real, "Change nothing")
	require.NotContains(t, real, "dry run")

	dry := renderPruneEntry(t, true)
	require.Contains(t, flat(dry), "**This is a dry run (a preview).** Change nothing")
	require.Contains(t, flat(dry), "in a dry run the entry must be untouched whatever the verdict")
	require.NotContains(t, dry, "knowledge write")
	require.NotContains(t, dry, "knowledge delete")
	require.False(t, strings.Contains(dry, "### Making the change"))
}
