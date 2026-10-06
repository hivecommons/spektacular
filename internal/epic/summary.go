package epic

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hivecommons/spektacular/internal/metadata"
)

// The planning summary is the document planning an epic with one request
// leaves beside the epic: the decisions the user settled while planning, the order
// planning added for specs whose plans share files, and one section per
// planned spec. The CLI owns its skeleton; every section but the ordering log
// is written by the orchestrating agent, one section at a time.

// Section names a caller addresses besides a member spec's name.
const (
	SectionDecisions = "decisions"
	SectionOrdering  = "ordering"
)

const (
	decisionsHeading = "Decisions"
	// legacyDecisionsHeading is what summaries written before decisions were
	// settled during planning call the section; it still reads as decisions.
	legacyDecisionsHeading = "Decisions to settle"
	orderingHeading        = "Order added for shared files"
	noDecisions            = "None."
	noOrdering             = "None added."
)

// Summary is the in-memory mirror of an epic's planning summary. Empty
// Decisions and Ordering render their "none" lines.
type Summary struct {
	CreatedDate string
	Decisions   string
	Ordering    string
	Specs       []SummarySection
}

// SummarySection is one planned spec's section.
type SummarySection struct {
	Name string
	Body string
}

type summaryFrontmatter struct {
	CreatedDate string `yaml:"created_date"`
}

// ParseSummary reads a summary document, splitting its body on level-2
// headings into the decisions, the ordering log and one section per spec.
func ParseSummary(raw []byte) (Summary, error) {
	fm, body, ok, err := metadata.SplitRaw(raw)
	if err != nil {
		return Summary{}, err
	}
	var s Summary
	if ok {
		var in summaryFrontmatter
		if err := yaml.Unmarshal(fm, &in); err != nil {
			return Summary{}, fmt.Errorf("malformed summary frontmatter: %w", err)
		}
		s.CreatedDate = in.CreatedDate
	}
	heading := ""
	var lines []string
	flush := func() {
		text := strings.Trim(strings.Join(lines, "\n"), "\n")
		switch heading {
		case "":
		case decisionsHeading, legacyDecisionsHeading:
			if text != noDecisions {
				s.Decisions = text
			}
		case orderingHeading:
			if text != noOrdering {
				s.Ordering = text
			}
		default:
			s.SetSection(heading, text)
		}
		lines = nil
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		if name, ok := strings.CutPrefix(line, "## "); ok {
			flush()
			heading = strings.TrimSpace(name)
			continue
		}
		if heading != "" {
			lines = append(lines, line)
		}
	}
	flush()
	return s, nil
}

// SetSection replaces the body of the named spec's section, adding the
// section when it is new.
func (s *Summary) SetSection(name, body string) {
	for i := range s.Specs {
		if s.Specs[i].Name == name {
			s.Specs[i].Body = body
			return
		}
	}
	s.Specs = append(s.Specs, SummarySection{Name: name, Body: body})
}

// SectionNames lists the sections in the order Render writes them: decisions,
// ordering, then the spec sections.
func (s Summary) SectionNames(order []string) []string {
	names := []string{SectionDecisions, SectionOrdering}
	for _, sec := range s.orderedSpecs(order) {
		names = append(names, sec.Name)
	}
	return names
}

// orderedSpecs returns the spec sections in order's order, then any section
// for a spec order does not list, in stored order.
func (s Summary) orderedSpecs(order []string) []SummarySection {
	out := make([]SummarySection, 0, len(s.Specs))
	for _, name := range order {
		for _, sec := range s.Specs {
			if sec.Name == name {
				out = append(out, sec)
			}
		}
	}
	for _, sec := range s.Specs {
		if !slices.Contains(order, sec.Name) {
			out = append(out, sec)
		}
	}
	return out
}

// Render writes the summary in its fixed layout: the title, Decisions,
// Order added for shared files, then one section per spec in order
// (the epic's list order), whatever order the sections were written in.
func (s Summary) Render(epicName string, order []string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "---\ncreated_date: %q\n---\n\n", s.CreatedDate)
	fmt.Fprintf(&buf, "# Planning summary: %s\n", epicName)
	writeSection := func(heading, body, none string) {
		if body == "" {
			body = none
		}
		fmt.Fprintf(&buf, "\n## %s\n%s\n", heading, body)
	}
	writeSection(decisionsHeading, s.Decisions, noDecisions)
	writeSection(orderingHeading, s.Ordering, noOrdering)
	for _, sec := range s.orderedSpecs(order) {
		writeSection(sec.Name, sec.Body, "")
	}
	return buf.Bytes()
}

// ValidSectionBody refuses a section body carrying its own level-1 or level-2
// heading, which would break the summary's section structure.
func ValidSectionBody(body string) error {
	for i, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || line == "#" || line == "##" {
			return fmt.Errorf("line %d is a level-1 or level-2 heading (%q); use ### or deeper inside a section", i+1, line)
		}
	}
	return nil
}
