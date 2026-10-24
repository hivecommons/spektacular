package templates

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSpekDesignLinksStagedDraftInsteadOfPrinting verifies that spek-design's
// author and revise branches stage the draft first and give the user its path,
// rather than printing the whole document in the terminal, while still
// writing to the source only after the user agrees.
func TestSpekDesignLinksStagedDraftInsteadOfPrinting(t *testing.T) {
	body := mustReadTemplate(t, "skills/workflows/spek-design/SKILL.md")

	require.NotContains(t, body, "Present the draft to the user as ordinary readable text, in full")
	require.NotContains(t, body, "Present the revised document to the user in full")

	for _, want := range []string{
		"1. Stage the draft under `.spektacular/tmp/<slug>.md` with your own `Write` tool.",
		"give the user the staged file's path with a short summary",
		"update the\n   staged file in place",
		"3. Stage the revised document under `.spektacular/tmp/<slug>.md`",
		"get their explicit agreement before writing it to the source",
	} {
		require.Contains(t, body, want)
	}
}
