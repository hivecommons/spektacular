package epic

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The fully populated summary every round-trip test starts from, rendered by
// hand in the fixed layout.
const summaryFull = "---\n" +
	"created_date: \"2026-09-01\"\n" +
	"---\n" +
	"\n" +
	"# Planning summary: 000050_rollout\n" +
	"\n" +
	"## Decisions to settle\n" +
	"- Pick a storage backend.\n" +
	"\n" +
	"## Order added for shared files\n" +
	"- auth before billing: both change cmd/root.go\n" +
	"\n" +
	"## auth\n" +
	"Adds login.\n" +
	"\n" +
	"### Risks\n" +
	"None known.\n" +
	"\n" +
	"## billing\n" +
	"Adds invoices.\n"

func TestSummary_RenderParseRenderIsByteIdentical(t *testing.T) {
	s, err := ParseSummary([]byte(summaryFull))
	require.NoError(t, err)
	require.Equal(t, Summary{
		CreatedDate: "2026-09-01",
		Decisions:   "- Pick a storage backend.",
		Ordering:    "- auth before billing: both change cmd/root.go",
		Specs: []SummarySection{
			{Name: "auth", Body: "Adds login.\n\n### Risks\nNone known."},
			{Name: "billing", Body: "Adds invoices."},
		},
	}, s)
	require.Equal(t, summaryFull, string(s.Render("000050_rollout", []string{"auth", "billing"})))
}

func TestSummary_RendersInCanonicalOrderWhateverTheWriteOrder(t *testing.T) {
	var s Summary
	s.CreatedDate = "2026-09-01"
	s.SetSection("billing", "Adds invoices.")
	s.SetSection("auth", "Adds login.\n\n### Risks\nNone known.")
	s.Ordering = "- auth before billing: both change cmd/root.go"
	s.Decisions = "- Pick a storage backend."

	require.Equal(t, summaryFull, string(s.Render("000050_rollout", []string{"auth", "billing"})))
	require.Equal(t, []string{"decisions", "ordering", "auth", "billing"}, s.SectionNames([]string{"auth", "billing"}))
}

func TestSummary_SectionForASpecNoLongerInTheEpicComesLast(t *testing.T) {
	var s Summary
	s.CreatedDate = "2026-09-01"
	s.SetSection("gone", "Was removed.")
	s.SetSection("auth", "Adds login.")

	require.Equal(t, []string{"decisions", "ordering", "auth", "gone"}, s.SectionNames([]string{"auth"}))
	require.Equal(t, "---\ncreated_date: \"2026-09-01\"\n---\n\n# Planning summary: e\n\n"+
		"## Decisions to settle\nNone.\n\n## Order added for shared files\nNone added.\n\n"+
		"## auth\nAdds login.\n\n## gone\nWas removed.\n",
		string(s.Render("e", []string{"auth"})))
}

func TestSummary_ReplacingOneSectionLeavesTheOthersByteIdentical(t *testing.T) {
	s, err := ParseSummary([]byte(summaryFull))
	require.NoError(t, err)
	s.SetSection("auth", "Rewritten.")

	want := "---\n" +
		"created_date: \"2026-09-01\"\n" +
		"---\n" +
		"\n" +
		"# Planning summary: 000050_rollout\n" +
		"\n" +
		"## Decisions to settle\n" +
		"- Pick a storage backend.\n" +
		"\n" +
		"## Order added for shared files\n" +
		"- auth before billing: both change cmd/root.go\n" +
		"\n" +
		"## auth\n" +
		"Rewritten.\n" +
		"\n" +
		"## billing\n" +
		"Adds invoices.\n"
	require.Equal(t, want, string(s.Render("000050_rollout", []string{"auth", "billing"})))
}

func TestSummary_EmptyDecisionsAndOrderingRenderTheirNoneLines(t *testing.T) {
	s := Summary{CreatedDate: "2026-09-01"}
	rendered := s.Render("000050_rollout", nil)
	require.Equal(t, "---\ncreated_date: \"2026-09-01\"\n---\n\n# Planning summary: 000050_rollout\n\n"+
		"## Decisions to settle\nNone.\n\n## Order added for shared files\nNone added.\n",
		string(rendered))

	// The "none" lines read back as empty, not as content.
	back, err := ParseSummary(rendered)
	require.NoError(t, err)
	require.Equal(t, Summary{CreatedDate: "2026-09-01"}, back)
	require.Equal(t, []string{"decisions", "ordering"}, back.SectionNames(nil))
}

func TestValidSectionBody(t *testing.T) {
	for _, body := range []string{
		"# Title",
		"intro\n## Sub",
		"#",
		"##",
	} {
		require.Errorf(t, ValidSectionBody(body), "%q must be refused", body)
	}
	for _, body := range []string{
		"",
		"plain text",
		"### Deeper\nfine",
		"#### Deeper still",
		"#hashtag is not a heading",
		"text with ## in the middle",
	} {
		require.NoErrorf(t, ValidSectionBody(body), "%q must be accepted", body)
	}

	err := ValidSectionBody("ok\n## Bad")
	require.ErrorContains(t, err, "line 2")
}
