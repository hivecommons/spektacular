package cmd

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hivecommons/spektacular/internal/metadata"
)

// amendmentsSection is the spec section spec amend appends its record to. It is
// written only by the CLI, so an amended body must carry it unchanged.
const amendmentsSection = "Amendments"

// preambleSection names the text before a spec's first `## ` heading in a
// refusal, since that text has no heading of its own.
const preambleSection = "(text before the first section)"

// amendableSections are the spec sections an implement run may amend.
var amendableSections = []string{"Requirements", "Acceptance Criteria", "Constraints", "Success Metrics"}

// specSection is one `## ` section of a spec body: its heading text (empty for
// the preamble before the first heading) and its full text, heading line
// included.
type specSection struct {
	Heading string
	Text    string
}

// splitSections splits a spec body at its `## ` headings. A `###` or deeper
// heading stays inside its enclosing section, and a `## ` line inside a fenced
// code block is not a heading. The text before the first heading is returned
// as a section with an empty heading, even when it is empty, so every body
// yields at least one section.
func splitSections(body []byte) []specSection {
	lines := strings.SplitAfter(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	sections := []specSection{{}}
	var text strings.Builder
	fence := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
		case strings.HasPrefix(line, "## "):
			sections[len(sections)-1].Text = text.String()
			text.Reset()
			sections = append(sections, specSection{Heading: strings.TrimSpace(strings.TrimPrefix(line, "## "))})
		}
		text.WriteString(line)
	}
	sections[len(sections)-1].Text = text.String()
	return sections
}

// changedSections reports the headings of the sections that differ between
// two spec bodies, in the order they appear in next, followed by any removed
// from prev, and then any that moved relative to the others. Ticked and unticked checkboxes, line endings and surrounding
// blank lines are not differences. A heading that appears more than once is
// compared occurrence by occurrence. The preamble is reported by its empty
// heading.
func changedSections(prev, next []byte) []string {
	keyed := func(body []byte) ([]string, map[string]string) {
		var order []string
		texts := map[string]string{}
		seen := map[string]int{}
		for _, s := range splitSections(body) {
			key := fmt.Sprintf("%s\x00%d", s.Heading, seen[s.Heading])
			seen[s.Heading]++
			order = append(order, key)
			texts[key] = string(bytes.TrimSpace(metadata.NormaliseCheckboxes([]byte(s.Text))))
		}
		return order, texts
	}
	prevOrder, prevTexts := keyed(prev)
	nextOrder, nextTexts := keyed(next)

	var changed []string
	reported := map[string]bool{}
	report := func(key string) {
		heading := key[:strings.IndexByte(key, 0)]
		if !reported[heading] {
			reported[heading] = true
			changed = append(changed, heading)
		}
	}
	for _, key := range nextOrder {
		if old, ok := prevTexts[key]; !ok || old != nextTexts[key] {
			report(key)
		}
	}
	for _, key := range prevOrder {
		if _, ok := nextTexts[key]; !ok {
			report(key)
		}
	}
	// A section both bodies hold but in a different position has moved, and
	// a move is a change to that section even when its text is the same.
	prevCommon := commonKeys(prevOrder, nextTexts)
	nextCommon := commonKeys(nextOrder, prevTexts)
	for i := range nextCommon {
		if nextCommon[i] != prevCommon[i] {
			report(nextCommon[i])
		}
	}
	return changed
}

// commonKeys returns the keys of order that are also in other, keeping order.
func commonKeys(order []string, other map[string]string) []string {
	var keys []string
	for _, k := range order {
		if _, ok := other[k]; ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// isAmendable reports whether heading names a section an implement run may
// amend.
func isAmendable(heading string) bool {
	for _, s := range amendableSections {
		if heading == s {
			return true
		}
	}
	return false
}

// appendAmendmentEntry appends entry to the body's `## Amendments` section,
// creating the section at the end of the body when it is absent. The entry is
// placed after the section's last line, ahead of any section that follows it.
func appendAmendmentEntry(body []byte, entry string) []byte {
	sections := splitSections(body)
	for i, s := range sections {
		if s.Heading != amendmentsSection {
			continue
		}
		text := strings.TrimRight(s.Text, "\n") + "\n" + entry + "\n"
		if i < len(sections)-1 {
			text += "\n"
		}
		sections[i].Text = text
		var b strings.Builder
		for _, s := range sections {
			b.WriteString(s.Text)
		}
		return []byte(b.String())
	}
	trimmed := strings.TrimRight(string(body), "\n")
	if trimmed != "" {
		trimmed += "\n\n"
	}
	return []byte(trimmed + "## " + amendmentsSection + "\n\n" + entry + "\n")
}
