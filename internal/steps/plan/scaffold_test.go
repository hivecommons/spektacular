package plan

import (
	"strings"
	"testing"

	"github.com/cbroglie/mustache"
	"github.com/hivecommons/spektacular/templates"
	"github.com/stretchr/testify/require"
)

func TestPlanScaffoldShape(t *testing.T) {
	raw, err := templates.FS.ReadFile("scaffold/plan.md")
	require.NoError(t, err)

	rendered, err := mustache.Render(string(raw), map[string]any{"name": "test"})
	require.NoError(t, err)

	expectedHeadings := []string{
		"## Overview",
		"## Architecture & Design Decisions",
		"## Component Breakdown",
		"## Data Structures & Interfaces",
		"## Implementation Detail",
		"## Dependencies",
		"## Testing Approach",
		"## Milestones & Tasks",
		"## Open Questions",
		"## Out of Scope",
	}

	lastIdx := -1
	for _, h := range expectedHeadings {
		idx := strings.Index(rendered, h)
		require.NotEqual(t, -1, idx, "heading %q missing from rendered scaffold", h)
		require.Greater(t, idx, lastIdx, "heading %q appears out of order", h)
		lastIdx = idx
	}

	for i, h := range expectedHeadings {
		headingIdx := strings.Index(rendered, h)
		sectionStart := 0
		if i > 0 {
			prev := strings.Index(rendered, expectedHeadings[i-1])
			sectionStart = prev + len(expectedHeadings[i-1])
		}
		between := rendered[sectionStart:headingIdx]
		require.Contains(t, between, "<!--", "section %q is missing a preceding HTML comment", h)
	}

	require.Contains(t, rendered, "#### - [ ] Task:", "Milestones & Tasks must contain a checkbox task heading")
	for _, line := range []string{"**Id:**", "**Repo:**", "**Depends on:**", "**Execution:**"} {
		require.Contains(t, rendered, line, "the scaffold task must carry %s", line)
	}
	require.Contains(t, rendered, "*Technical detail:*", "Milestones & Tasks must contain a *Technical detail:* link")
}

// TestResearchScaffoldShape asserts the research scaffold's `##` headings are
// present and in order — including `## Drafting assumptions` between
// `## Open assumptions` and `## Rehydration cues` (Phase 2.2). The heading
// list is a hand-maintained oracle.
func TestResearchScaffoldShape(t *testing.T) {
	raw, err := templates.FS.ReadFile("scaffold/research.md")
	require.NoError(t, err)

	rendered, err := mustache.Render(string(raw), map[string]any{"name": "test"})
	require.NoError(t, err)

	expectedHeadings := []string{
		"## Alternatives considered and rejected",
		"## Chosen approach — evidence",
		"## Files examined",
		"## External references",
		"## Prior plans / specs consulted",
		"## Open assumptions",
		"## Drafting assumptions",
		"## Rehydration cues",
	}

	lastIdx := -1
	for _, h := range expectedHeadings {
		idx := strings.Index(rendered, h)
		require.NotEqual(t, -1, idx, "heading %q missing from rendered research scaffold", h)
		require.Greater(t, idx, lastIdx, "heading %q appears out of order", h)
		lastIdx = idx
	}
}
