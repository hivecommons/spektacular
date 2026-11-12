package voice

import (
	"regexp"
	"strings"
)

// Sections lists the spec scaffold headings in order.
var Sections = []string{"Overview", "Requirements", "Constraints", "Acceptance Criteria", "Technical Approach", "Success Metrics", "Non-Goals"}

// RequiredSections are what a note must carry to be published without an
// interview.
var RequiredSections = []string{"Requirements", "Acceptance Criteria"}

// sectionAliases maps lower-cased headings as a summary template (Plaud's,
// Wispr's, a human's) might phrase them onto scaffold sections.
var sectionAliases = map[string]string{
	"overview": "Overview", "summary": "Overview", "goal": "Overview", "problem": "Overview", "context": "Overview", "background": "Overview",
	"requirements": "Requirements", "requirement": "Requirements", "must": "Requirements", "features": "Requirements", "what to build": "Requirements", "scope": "Requirements",
	"constraints": "Constraints", "constraint": "Constraints", "limits": "Constraints", "boundaries": "Constraints",
	"acceptance criteria": "Acceptance Criteria", "acceptance": "Acceptance Criteria", "action items": "Acceptance Criteria", "done when": "Acceptance Criteria", "definition of done": "Acceptance Criteria", "todo": "Acceptance Criteria", "to-do": "Acceptance Criteria", "tasks": "Acceptance Criteria",
	"technical approach": "Technical Approach", "approach": "Technical Approach", "technical": "Technical Approach", "implementation": "Technical Approach", "how": "Technical Approach", "design": "Technical Approach",
	"success metrics": "Success Metrics", "metrics": "Success Metrics", "success": "Success Metrics", "measures": "Success Metrics",
	"non-goals": "Non-Goals", "non goals": "Non-Goals", "nongoals": "Non-Goals", "out of scope": "Non-Goals", "not doing": "Non-Goals", "exclusions": "Non-Goals",
	"open questions": "Open Questions", "questions": "Open Questions", "unknowns": "Open Questions",
}

// Draft maps a note onto the spec scaffold. Recognised summary headings land
// in their sections; unrecognised text goes to Overview; a note with no
// summary gets the transcript's opening as its Overview. Requirements and
// acceptance criteria are normalised to the scaffold's checklist form.
// missing names the required sections the note did not fill.
type Draft struct {
	Title   string
	Body    string
	Missing []string
	// Extra holds recognised sections the scaffold has no slot for (Open
	// Questions), appended after Non-Goals.
	Extra map[string]string
}

var headingLine = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$`)

// DraftNote builds a Draft from a note.
func DraftNote(n Note) Draft {
	sections := map[string]string{}
	var extra []string
	title := strings.TrimSpace(n.Title)

	if strings.TrimSpace(n.Summary) != "" {
		current := "Overview"
		var buf strings.Builder
		flush := func() {
			if t := strings.TrimSpace(buf.String()); t != "" {
				sections[current] = joinSection(sections[current], t)
			}
			buf.Reset()
		}
		for _, line := range strings.Split(n.Summary, "\n") {
			if m := headingLine.FindStringSubmatch(line); m != nil {
				heading := strings.TrimSpace(m[2])
				if len(m[1]) == 1 && title == "" {
					title = strings.TrimPrefix(heading, "Feature: ")
					continue
				}
				flush()
				if sec, ok := sectionAliases[normaliseHeading(heading)]; ok {
					current = sec
					if sec == "Open Questions" {
						extra = appendUnique(extra, sec)
					}
				} else {
					current = "Overview"
					buf.WriteString("**" + heading + "**\n")
				}
				continue
			}
			buf.WriteString(line + "\n")
		}
		flush()
	} else if t := strings.TrimSpace(n.Transcript); t != "" {
		sections["Overview"] = excerpt(t, 600)
	}

	for _, sec := range []string{"Requirements", "Acceptance Criteria"} {
		if sections[sec] != "" {
			sections[sec] = checklist(sections[sec])
		}
	}

	if title == "" {
		title = "Voice note"
	}
	var b strings.Builder
	b.WriteString("# Feature: " + title + "\n")
	for _, sec := range Sections {
		b.WriteString("\n## " + sec + "\n\n")
		if body := sections[sec]; body != "" {
			b.WriteString(body + "\n")
		}
	}
	d := Draft{Title: title, Body: b.String(), Extra: map[string]string{}}
	for _, sec := range extra {
		d.Extra[sec] = sections[sec]
		b.WriteString("\n## " + sec + "\n\n" + sections[sec] + "\n")
	}
	d.Body = b.String()
	for _, sec := range RequiredSections {
		if strings.TrimSpace(sections[sec]) == "" {
			d.Missing = append(d.Missing, sec)
		}
	}
	return d
}

func normaliseHeading(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimRight(h, ":")
	h = strings.TrimSpace(strings.TrimPrefix(h, "feature:"))
	return h
}

func joinSection(existing, more string) string {
	if existing == "" {
		return more
	}
	return existing + "\n\n" + more
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

var bulletPrefix = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+(?:\[[ xX]\]\s*)?`)

// checklist rewrites bullets as "- [ ] **Title**: detail" so publish's
// requirement parser and the scaffold agree. Already-formatted lines and
// indented detail lines pass through.
func checklist(text string) string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if !bulletPrefix.MatchString(line) {
			if len(out) > 0 && strings.HasPrefix(line, "  ") {
				out = append(out, line)
			} else {
				out = append(out, "- [ ] "+boldTitle(t))
			}
			continue
		}
		item := bulletPrefix.ReplaceAllString(line, "")
		if strings.HasPrefix(item, "**") {
			out = append(out, "- [ ] "+item)
			continue
		}
		out = append(out, "- [ ] "+boldTitle(item))
	}
	return strings.Join(out, "\n")
}

// boldTitle splits "Title: detail" or "Sentence. More." into a bold title
// and its detail; a short item becomes a bold title alone.
func boldTitle(item string) string {
	item = strings.TrimSpace(item)
	if i := strings.Index(item, ": "); i > 0 && i < 80 {
		return "**" + item[:i] + "**: " + strings.TrimSpace(item[i+2:])
	}
	if i := strings.Index(item, ". "); i > 0 && i < 80 {
		return "**" + item[:i] + "**: " + strings.TrimSpace(item[i+2:])
	}
	words := strings.Fields(item)
	if len(words) <= 10 {
		return "**" + strings.TrimRight(item, ".") + "**"
	}
	return "**" + strings.Join(words[:6], " ") + "**: " + item
}

// excerpt returns the first n runes of text at a word boundary.
func excerpt(text string, n int) string {
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	cut := string(r[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return cut + "…"
}

var slugStrip = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes a filesystem- and identifier-safe name from a title.
func Slug(title string) string {
	s := slugStrip.ReplaceAllString(strings.ToLower(title), "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		return "voice-note"
	}
	return s
}
