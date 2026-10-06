package spec

import (
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

// The seeding and epic instructions live in the interview step's own
// template, so a resumed session that re-renders the interview from the
// workflow data gets them again. These tests render the interview step the
// way the workflow does, with the workflow data spec new stores, and check
// which branches come out.

const (
	seededMarker = "**This spec was started from existing material. Seed it before you ask anything.**"
	epicMarker   = "**This spec joins the epic `000050_rollout`. Read it first.**"
	seededSpec   = "000001_seeded"
	seededEpic   = "000050_rollout"
	// A query string with "&" proves the links reach the agent unescaped.
	seededURIA = "https://example.com/issues/45?tab=comments&page=2"
	seededURIB = "https://example.com/docs/design.md"
)

// seededSourceShapes are the two shapes the sources take in the workflow
// data: the stamped refs in the process that ran spec new, and plain maps
// once state.json has round-tripped them (the resumed-session case).
func seededSourceShapes() map[string]any {
	return map[string]any{
		"in process": []metadata.SourceRef{
			{URI: seededURIA, RetrievedDate: "2026-10-01"},
			{URI: seededURIB, RetrievedDate: "2026-10-01"},
		},
		"after state round trip": []any{
			map[string]any{"uri": seededURIA, "retrieved_date": "2026-10-01"},
			map[string]any{"uri": seededURIB, "retrieved_date": "2026-10-01"},
		},
	}
}

// Criterion: a seeded interview renders its branch once, listing each source
// link exactly once and unescaped, and carries the seed-then-ask-only-about-
// gaps instructions; in both data shapes.
func TestInterviewStep_SeededBranchRendersOnceWithEachSource(t *testing.T) {
	for name, sources := range seededSourceShapes() {
		t.Run(name, func(t *testing.T) {
			out := renderStepWithData(t, interview(), map[string]any{"name": seededSpec, "sources": sources})

			require.NotContains(t, out, "{{", "every mustache tag must render")
			require.Equal(t, 1, strings.Count(out, seededMarker), "the seeded branch must render exactly once, not once per source")
			for _, uri := range []string{seededURIA, seededURIB} {
				require.Equal(t, 1, strings.Count(out, "- `"+uri+"`"), "each source is listed once, unescaped: %s", uri)
			}
			require.NotContains(t, out, "&amp;", "a link must not be HTML-escaped")
			require.NotContains(t, out, epicMarker, "no epic was given")

			flat := strings.Join(strings.Fields(out), " ")
			for _, phrase := range []string{
				"Fetch each source again with your own tools",
				"If a source cannot be reached, say so and ask the user to paste its content",
				"never fall back to a blank interview",
				"under `.spektacular/work/" + seededSpec + "/`",
				"**List the gaps to the user:**",
				"**Ask only about the gaps.**",
				"each later section step presents its seeded draft for the user to confirm",
				"record what came from the source and what came from the user",
			} {
				require.Contains(t, flat, phrase)
			}
		})
	}
}

// Criterion: a spec joining an epic has the interview read the epic and each
// of its specs first, and offer to record dependencies on them.
func TestInterviewStep_EpicBranchReadsTheEpicAndOffersDependencies(t *testing.T) {
	out := renderStepWithData(t, interview(), map[string]any{"name": seededSpec, "epic": seededEpic})

	require.NotContains(t, out, "{{")
	require.Equal(t, 1, strings.Count(out, epicMarker))
	require.NotContains(t, out, seededMarker, "no sources were given")
	flat := strings.Join(strings.Fields(out), " ")
	for _, phrase := range []string{
		"Before asking anything, run `spektacular epic read " + seededEpic + "`",
		"read each spec it lists with `spektacular spec file read <name>`",
		"do not ask again about scope those specs settle",
		"offer to record that dependency in the epic",
		"`spektacular epic write " + seededEpic + "`",
		"record it only if the user agrees",
	} {
		require.Contains(t, flat, phrase)
	}
	epicAt := strings.Index(out, epicMarker)
	require.Less(t, epicAt, strings.Index(out, "**Ask adaptive, open questions"), "the epic is read before the questions start")
}

// Both branches render together, each once, for a seeded spec in an epic.
func TestInterviewStep_SeededSpecInAnEpicRendersBothBranches(t *testing.T) {
	sources := seededSourceShapes()["after state round trip"]
	out := renderStepWithData(t, interview(), map[string]any{"name": seededSpec, "sources": sources, "epic": seededEpic})
	require.Equal(t, 1, strings.Count(out, epicMarker))
	require.Equal(t, 1, strings.Count(out, seededMarker))
	require.NotContains(t, out, "{{")
}

// A plain spec, with neither sources nor an epic (or an empty source list),
// renders neither branch.
func TestInterviewStep_PlainSpecRendersNeitherBranch(t *testing.T) {
	for name, values := range map[string]map[string]any{
		"no keys":            {"name": seededSpec},
		"empty sources":      {"name": seededSpec, "sources": []any{}, "epic": ""},
		"nil sources":        {"name": seededSpec, "sources": nil},
		"empty typed source": {"name": seededSpec, "sources": []metadata.SourceRef{}},
	} {
		t.Run(name, func(t *testing.T) {
			out := renderStepWithData(t, interview(), values)
			require.NotContains(t, out, seededMarker)
			require.NotContains(t, out, "This spec joins the epic")
			require.NotContains(t, out, "{{")
			require.Contains(t, out, "**Ask adaptive, open questions", "the ordinary interview still renders")
		})
	}
}
