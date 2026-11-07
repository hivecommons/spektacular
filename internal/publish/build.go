package publish

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hivecommons/spektacular/internal/plantask"
)

// Source is everything Build needs, read by the caller: publish never reads
// spek files itself, so the model can be built from any store.
type Source struct {
	Kind       string
	Name       string
	ArtifactID string
	Target     string
	// Draft marks a publication of an artifact that is not final; the RFC
	// title says so.
	Draft bool
	// Body is the spec's body (frontmatter removed), or the epic's.
	Body []byte
	// Plan is the spec's task-format plan when it has one; its tasks become
	// the children. Ignored for an epic.
	Plan *plantask.Plan
	// PlanName is the plan's name, for the task bodies.
	PlanName string
	// Members are an epic's member specs in epic order; each becomes a child.
	Members []Member
	// DefaultRepo names the repo a child lands in when its source names none:
	// the project's single registered repo, or empty.
	DefaultRepo string
	Provenance  Provenance
}

// Member is one of an epic's specs.
type Member struct {
	Name      string
	Body      []byte
	DependsOn []string
	Repo      string
}

// Provenance is what the RFC footer records about where the publication came
// from.
type Provenance struct {
	Project string
	// Repo and Commit locate the spek file: the project's source address and
	// the commit of its checkout, each empty when unknown.
	Repo    string
	Commit  string
	Version string
}

// sectionOrder is the scaffold's section order (templates/scaffold/spec.md);
// the RFC renders a spec's sections in it, then any others in their own
// order.
var sectionOrder = []string{"Overview", "Requirements", "Constraints", "Acceptance Criteria", "Technical Approach", "Success Metrics", "Non-Goals"}

// skippedSections are spec sections the RFC never carries: the amendment log
// is Spek's own record, and an epic's specs list is replaced by the children.
var skippedSections = map[string]bool{"Amendments": true, "Specs": true}

// Build assembles the publication from src. It renders every body, so the
// result is ready for Validate and for a target.
func Build(src Source) (Publication, error) {
	if src.Kind != KindSpec && src.Kind != KindEpic {
		return Publication{}, fmt.Errorf("publish: unknown artifact kind %q", src.Kind)
	}
	body := stripComments(string(src.Body))
	title := documentTitle(body, src.Name)

	p := Publication{
		Kind:       src.Kind,
		Name:       src.Name,
		ArtifactID: src.ArtifactID,
		Target:     src.Target,
	}
	rfcTitle := "RFC: " + title
	if src.Draft {
		rfcTitle = "RFC: [DRAFT] " + title
	}
	marker := Marker{Name: src.Name, ArtifactID: src.ArtifactID, Target: src.Target}
	p.RFC = RFCDoc{Title: rfcTitle, Marker: marker.String()}
	p.rfcSections = renderSections(body)
	p.provenance = renderProvenance(src)

	secs := splitSections(body)
	overview := strings.TrimSpace(sectionBody(secs, "Overview"))
	specCriteria := listItems(sectionBody(secs, "Acceptance Criteria"))

	switch {
	case src.Kind == KindEpic:
		p.ChildrenFrom = ChildrenFromEpic
		for _, m := range src.Members {
			p.Children = append(p.Children, memberChild(src, m))
		}
	case src.Plan != nil && src.Plan.Format == plantask.FormatTasks:
		p.ChildrenFrom = ChildrenFromPlan
		for _, t := range src.Plan.Tasks {
			p.Children = append(p.Children, taskChild(src, t, overview, specCriteria))
		}
	default:
		p.ChildrenFrom = ChildrenFromRequirements
		reqs := listItems(sectionBody(secs, "Requirements"))
		for i, r := range reqs {
			p.Children = append(p.Children, requirementChild(src, i, r, reqs, specCriteria, overview))
		}
	}

	for i := range p.Children {
		c := &p.Children[i]
		if c.DependsOn == nil {
			c.DependsOn = []string{}
		}
		if c.AcceptanceCriteria == nil {
			c.AcceptanceCriteria = []string{}
		}
		if c.Repo == "" {
			c.Repo = src.DefaultRepo
		}
		m := marker
		m.Task = c.ID
		c.Marker = m.String()
	}
	// Bodies are laid out with no filed references yet; a target lays them
	// out again with the real ones.
	p.RFC.Body = p.RFCBody(nil)
	for i := range p.Children {
		p.Children[i].Body = p.Children[i].Render(p, "", nil)
	}
	return p, nil
}

func taskChild(src Source, t plantask.Task, overview string, specCriteria []listItem) Child {
	var what strings.Builder
	fmt.Fprintf(&what, "## What to build\n\n%s\n\n", strings.TrimSpace(t.Title))
	fmt.Fprintf(&what, "Task `%s` of milestone %d in plan `%s`.\n", t.ID, t.Milestone, src.PlanName)
	if overview != "" {
		fmt.Fprintf(&what, "\n## Context\n\n%s\n", clip(overview, 800))
	}
	criteria := make([]string, 0, len(t.Criteria.Items))
	for _, c := range t.Criteria.Items {
		criteria = append(criteria, c.Text)
	}
	if len(criteria) == 0 {
		for _, c := range specCriteria {
			criteria = append(criteria, c.Text)
		}
	}
	exec := t.Execution.Type
	if t.Execution.Reason != "" {
		exec += " — " + t.Execution.Reason
	}
	return Child{
		ID:                 t.ID,
		Title:              strings.TrimSpace(t.Title),
		Repo:               t.Repo,
		DependsOn:          append([]string{}, t.DependsOn...),
		Milestone:          t.Milestone,
		AcceptanceCriteria: criteria,
		Execution:          exec,
		what:               what.String(),
	}
}

func memberChild(src Source, m Member) Child {
	body := stripComments(string(m.Body))
	secs := splitSections(body)
	title := documentTitle(body, m.Name)
	var what strings.Builder
	what.WriteString("## What to build\n\n")
	if ov := strings.TrimSpace(sectionBody(secs, "Overview")); ov != "" {
		what.WriteString(ov)
		what.WriteString("\n\n")
	}
	fmt.Fprintf(&what, "Spec `%s` of epic `%s`.\n", m.Name, src.Name)
	if reqs := strings.TrimSpace(sectionBody(secs, "Requirements")); reqs != "" {
		fmt.Fprintf(&what, "\n## Requirements\n\n%s\n", reqs)
	}
	criteria := []string{}
	for _, c := range listItems(sectionBody(secs, "Acceptance Criteria")) {
		criteria = append(criteria, c.Text)
	}
	return Child{
		ID:                 m.Name,
		Title:              title,
		Repo:               m.Repo,
		DependsOn:          append([]string{}, m.DependsOn...),
		AcceptanceCriteria: criteria,
		what:               what.String(),
	}
}

func requirementChild(src Source, i int, r listItem, reqs, specCriteria []listItem, overview string) Child {
	id := fmt.Sprintf("R%d", i+1)
	var what strings.Builder
	fmt.Fprintf(&what, "## What to build\n\n%s\n\nRequirement %s of spec `%s`.\n", r.Text, id, src.Name)
	if overview != "" {
		fmt.Fprintf(&what, "\n## Context\n\n%s\n", clip(overview, 800))
	}
	return Child{
		ID:                 id,
		Title:              r.Title,
		AcceptanceCriteria: tracedCriteria(i, r, reqs, specCriteria),
		what:               what.String(),
	}
}

// tracedCriteria picks the acceptance criteria for requirement r: those
// whose bold title matches the requirement's; else, when the spec has as many
// criteria as requirements, the one in the same position; else every
// criterion, since an untraceable criterion still binds the whole feature.
func tracedCriteria(i int, r listItem, reqs, criteria []listItem) []string {
	var out []string
	if r.Bold != "" {
		for _, c := range criteria {
			if c.Bold != "" && normalise(c.Bold) == normalise(r.Bold) {
				out = append(out, c.Text)
			}
		}
	}
	if len(out) == 0 && len(criteria) == len(reqs) && i < len(criteria) {
		out = append(out, criteria[i].Text)
	}
	if len(out) == 0 {
		for _, c := range criteria {
			out = append(out, c.Text)
		}
	}
	return out
}

func normalise(s string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), ".:"))
}

// renderSections lays out the body's sections in scaffold order, dropping the
// title line, empty sections and the ones the RFC never carries.
func renderSections(body string) string {
	secs := splitSections(body)
	byHeading := map[string][]section{}
	var order []string
	for _, s := range secs {
		if s.Heading == "" || skippedSections[s.Heading] {
			continue
		}
		if _, seen := byHeading[s.Heading]; !seen {
			order = append(order, s.Heading)
		}
		byHeading[s.Heading] = append(byHeading[s.Heading], s)
	}
	var b strings.Builder
	write := func(h string) {
		for _, s := range byHeading[h] {
			content := strings.TrimSpace(s.Body)
			if content == "" {
				continue
			}
			fmt.Fprintf(&b, "## %s\n\n%s\n\n", h, content)
		}
		delete(byHeading, h)
	}
	for _, h := range sectionOrder {
		if _, ok := byHeading[h]; ok {
			write(h)
		}
	}
	for _, h := range order {
		if _, ok := byHeading[h]; ok {
			write(h)
		}
	}
	return b.String()
}

func renderProvenance(src Source) string {
	parts := []string{fmt.Sprintf("Spek %s `%s`", src.Kind, src.Name), fmt.Sprintf("artifact_id `%s`", src.ArtifactID)}
	if src.Provenance.Project != "" {
		parts = append(parts, fmt.Sprintf("project `%s`", src.Provenance.Project))
	}
	switch {
	case src.Provenance.Repo != "" && src.Provenance.Commit != "":
		parts = append(parts, fmt.Sprintf("source `%s@%s`", src.Provenance.Repo, src.Provenance.Commit))
	case src.Provenance.Repo != "":
		parts = append(parts, fmt.Sprintf("source `%s`", src.Provenance.Repo))
	case src.Provenance.Commit != "":
		parts = append(parts, fmt.Sprintf("commit `%s`", src.Provenance.Commit))
	}
	if src.Provenance.Version != "" {
		parts = append(parts, "spektacular "+src.Provenance.Version)
	}
	return "---\n\nPublished by Spek · " + strings.Join(parts, " · ") + "\n"
}

// section is one `## ` section of a body: its heading (empty for the text
// before the first heading) and the text under it, heading line excluded.
type section struct {
	Heading string
	Body    string
}

// splitSections splits body at its `## ` headings. A deeper heading stays in
// its section, and a heading inside a fenced code block is not a heading.
func splitSections(body string) []section {
	lines := strings.SplitAfter(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	secs := []section{{}}
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
			secs[len(secs)-1].Body = text.String()
			text.Reset()
			secs = append(secs, section{Heading: strings.TrimSpace(strings.TrimPrefix(line, "## "))})
			continue
		}
		text.WriteString(line)
	}
	secs[len(secs)-1].Body = text.String()
	return secs
}

func sectionBody(secs []section, heading string) string {
	for _, s := range secs {
		if s.Heading == heading {
			return s.Body
		}
	}
	return ""
}

var commentRE = regexp.MustCompile(`(?s)<!--.*?-->`)

// stripComments removes HTML comments: the scaffold's guidance blocks must
// never be filed, and they are full of the <placeholders> a hive parks.
func stripComments(body string) string {
	return commentRE.ReplaceAllString(body, "")
}

var titleRE = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

// documentTitle is the body's first H1, less a "Feature:" prefix, or fallback.
func documentTitle(body, fallback string) string {
	if m := titleRE.FindStringSubmatch(body); m != nil {
		t := strings.TrimSpace(m[1])
		for _, prefix := range []string{"Feature:", "Epic:", "Spec:"} {
			t = strings.TrimSpace(strings.TrimPrefix(t, prefix))
		}
		if t != "" {
			return t
		}
	}
	return fallback
}

// listItem is one top-level list item of a section with any indented
// continuation joined on, its bold title when it has one, and the plain text
// with the checkbox and bold markers removed.
type listItem struct {
	Title string
	Bold  string
	Text  string
}

var (
	itemStartRE = regexp.MustCompile(`^[-*]\s+(?:\[[ xX]\]\s+)?(.*)$`)
	boldRE      = regexp.MustCompile(`^\*\*(.+?)\*\*:?\s*(.*)$`)
)

// listItems reads a section's top-level bullets. Indented lines continue the
// item above; blank lines and prose outside a bullet are ignored.
func listItems(text string) []listItem {
	var items []listItem
	var cur *strings.Builder
	flush := func() {
		if cur == nil {
			return
		}
		raw := strings.TrimSpace(cur.String())
		cur = nil
		if raw == "" {
			return
		}
		it := listItem{Text: raw}
		if m := boldRE.FindStringSubmatch(raw); m != nil {
			it.Bold = strings.TrimSpace(m[1])
			it.Title = it.Bold
			rest := strings.TrimSpace(m[2])
			if rest == "" {
				it.Text = it.Bold
			} else {
				it.Text = it.Bold + ": " + rest
			}
		} else {
			it.Title = clip(firstSentence(raw), 80)
		}
		items = append(items, it)
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		switch {
		case itemStartRE.MatchString(line) && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t"):
			flush()
			cur = &strings.Builder{}
			cur.WriteString(itemStartRE.FindStringSubmatch(line)[1])
		case cur != nil && strings.TrimSpace(line) == "":
			flush()
		case cur != nil && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")):
			cur.WriteString(" ")
			cur.WriteString(strings.TrimSpace(line))
		default:
			flush()
		}
	}
	flush()
	return items
}

func firstSentence(s string) string {
	if i := strings.IndexAny(s, ".;:"); i > 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// clip shortens s to at most n runes on a word boundary, marking the cut.
func clip(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	runes := []rune(s)[:n]
	cut := strings.LastIndex(string(runes), " ")
	if cut <= 0 {
		cut = len(string(runes))
	}
	return strings.TrimRight(string(runes)[:cut], " ,.;:") + "…"
}
