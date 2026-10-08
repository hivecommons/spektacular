package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSplitSections checks how a spec body is cut at its `## ` headings: the
// preamble is always the first section, deeper headings stay inside their
// section, headings inside fenced code are not headings, and CRLF line endings
// are normalised.
func TestSplitSections(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []specSection
	}{
		{"empty body", "", []specSection{{}}},
		{"preamble", "# T\n\nintro\n## A\na\n", []specSection{
			{Heading: "", Text: "# T\n\nintro\n"},
			{Heading: "A", Text: "## A\na\n"},
		}},
		{"deeper heading stays inside", "## A\n### Sub\nx\n## B\n", []specSection{
			{},
			{Heading: "A", Text: "## A\n### Sub\nx\n"},
			{Heading: "B", Text: "## B\n"},
		}},
		{"backtick fence", "## A\n```\n## not\n```\n## B\nb", []specSection{
			{},
			{Heading: "A", Text: "## A\n```\n## not\n```\n"},
			{Heading: "B", Text: "## B\nb"},
		}},
		{"tilde fence", "## A\n~~~md\n## not\n~~~\n", []specSection{
			{},
			{Heading: "A", Text: "## A\n~~~md\n## not\n~~~\n"},
		}},
		{"a tilde line does not close a backtick fence", "## A\n```\n~~~\n## not\n```\n## B\n", []specSection{
			{},
			{Heading: "A", Text: "## A\n```\n~~~\n## not\n```\n"},
			{Heading: "B", Text: "## B\n"},
		}},
		{"CRLF", "## A\r\na\r\n## B\r\n", []specSection{
			{},
			{Heading: "A", Text: "## A\na\n"},
			{Heading: "B", Text: "## B\n"},
		}},
		{"heading text trimmed, no-space hashes not a heading", "## A  \n##B\n", []specSection{
			{},
			{Heading: "A", Text: "## A  \n##B\n"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, splitSections([]byte(tc.body)))
		})
	}
}

// TestChangedSections checks which headings changedSections reports between
// two bodies: checkbox marks, line endings and surrounding blank lines are not
// changes; edited, added and removed sections are; duplicate headings compare
// occurrence by occurrence; and the preamble is reported as "".
func TestChangedSections(t *testing.T) {
	const base = "# T\n\nintro\n\n## Requirements\n\n- [ ] one\n\n## Overview\n\ntext\n"
	cases := []struct {
		name       string
		prev, next string
		want       []string
	}{
		{"no change", base, base, nil},
		{"checkbox only", base, "# T\n\nintro\n\n## Requirements\n\n- [x] one\n\n## Overview\n\ntext\n", nil},
		{"whitespace only", base, "# T\r\n\r\nintro\r\n\r\n\r\n## Requirements\r\n\r\n- [ ] one\r\n\r\n\r\n## Overview\r\n\r\ntext\r\n\r\n", nil},
		{"text change", base, "# T\n\nintro\n\n## Requirements\n\n- [ ] two\n\n## Overview\n\ntext\n", []string{"Requirements"}},
		{"two changes in next's order", base, "# T\n\nintro\n\n## Requirements\n\n- [ ] two\n\n## Overview\n\nother\n", []string{"Requirements", "Overview"}},
		{"added section", base, base + "\n## Constraints\n\n- c\n", []string{"Constraints"}},
		{"removed section", base, "# T\n\nintro\n\n## Requirements\n\n- [ ] one\n", []string{"Overview"}},
		{"duplicate headings", "## Notes\na\n## Notes\nb\n", "## Notes\na\n## Notes\nc\n", []string{"Notes"}},
		{"preamble change", base, "# T\n\nnew intro\n\n## Requirements\n\n- [ ] one\n\n## Overview\n\ntext\n", []string{""}},
		{"moved sections", base, "# T\n\nintro\n\n## Overview\n\ntext\n\n## Requirements\n\n- [ ] one\n", []string{"Overview", "Requirements"}},
		{"move with a text change", base, "# T\n\nintro\n\n## Overview\n\ntext\n\n## Requirements\n\n- [ ] two\n", []string{"Requirements", "Overview"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, changedSections([]byte(tc.prev), []byte(tc.next)))
		})
	}
}

// TestAppendAmendmentEntry checks that an entry creates `## Amendments` at the
// end of a body that lacks it, and otherwise lands after the section's last
// line, ahead of any section that follows it.
func TestAppendAmendmentEntry(t *testing.T) {
	const entry = "- **2026-10-08: Requirements** (run 1)\n  reason"
	cases := []struct {
		name string
		body string
		want string
	}{
		{"empty body", "", "## Amendments\n\n" + entry + "\n"},
		{"creates the section at the end", "# T\n\n## A\n\na\n\n\n", "# T\n\n## A\n\na\n\n## Amendments\n\n" + entry + "\n"},
		{"appends to an existing last section", "## A\n\na\n\n## Amendments\n\n- old\n  why\n",
			"## A\n\na\n\n## Amendments\n\n- old\n  why\n" + entry + "\n"},
		{"appends ahead of a following section", "## Amendments\n\n- old\n\n## B\n\nb\n",
			"## Amendments\n\n- old\n" + entry + "\n\n## B\n\nb\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, string(appendAmendmentEntry([]byte(tc.body), entry)))
		})
	}
}
