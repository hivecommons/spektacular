// Package publish turns a finished artifact (a spec, with or without a plan,
// or an epic) into a target-independent publication: one RFC document plus an
// ordered set of child work items, as data and as rendered markdown. Target
// adapters (the GitHub target in internal/publish/github today; Jira and
// Linear are designed to drop in beside it) consume this model and never touch
// spek files.
//
// Every filed item carries a hidden marker, identical in grammar on the RFC
// and its children, that makes a re-publish idempotent and tells a hive that
// the item was specced by Spek:
//
//	<!-- spek:publish name=<name> artifact_id=<id> target=<t> -->
//	<!-- spek:publish name=<name> artifact_id=<id> target=<t> task=<id> rfc=<url> -->
//
// The first form marks the RFC, the umbrella a hive must never claim; the
// second marks a child, which stays on the direct-fix path. See
// docs/publish-hive-handshake.md.
package publish

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hivecommons/spektacular/internal/depgraph"
)

// Artifact kinds a publication is built from.
const (
	KindSpec = "spec"
	KindEpic = "epic"
)

// Where a publication's children came from.
const (
	ChildrenFromPlan         = "plan"
	ChildrenFromEpic         = "epic"
	ChildrenFromRequirements = "requirements"
)

// MinBodyChars is the shortest body a hive's clarity gate accepts
// (hive `min_body_chars`, 80). Every rendered body must reach it.
const MinBodyChars = 80

// Publication is one artifact ready to file: the RFC and its children.
type Publication struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	ArtifactID string `json:"artifact_id"`
	Target     string `json:"target"`
	// ChildrenFrom says which source produced the children: plan, epic or
	// requirements.
	ChildrenFrom string  `json:"children_from"`
	RFC          RFCDoc  `json:"rfc"`
	Children     []Child `json:"children"`
	// RecordedURLs is Source.RecordedURLs, for the target's Hint.
	RecordedURLs []string `json:"-"`

	// rfcSections is the RFC body's rendered spek sections, and provenance
	// its footer, kept apart so RFCBody can lay the body out again once the
	// children have references.
	rfcSections string
	provenance  string
}

// RFCDoc is the parent item.
type RFCDoc struct {
	Title string `json:"title"`
	// Body is the rendered RFC body: the spek's sections, the children
	// checklist and the provenance footer, with the marker as its last line.
	Body   string `json:"body"`
	Marker string `json:"marker"`
}

// Child is one unit of work a hive picks up.
type Child struct {
	// ID is the plan task id, the member spec name, or R<n> for a
	// requirement. It is the marker's task= value.
	ID    string `json:"id"`
	Title string `json:"title"`
	// Body is the rendered child body with the marker as its last line. The
	// marker carries rfc= only once the RFC's URL is known; see Child.Render.
	Body               string   `json:"body"`
	Repo               string   `json:"repo"`
	DependsOn          []string `json:"depends_on"`
	Milestone          int      `json:"milestone,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Execution          string   `json:"execution,omitempty"`
	Marker             string   `json:"marker"`

	// what is the child's description without its links, dependencies or
	// marker, kept so Render can lay the body out again with the final
	// references.
	what string
}

// Marker is the parsed form of the hidden marker.
type Marker struct {
	Name       string
	ArtifactID string
	Target     string
	// Task is set on a child only.
	Task string
	// RFC is the RFC's URL, set on a filed child.
	RFC string
}

// IsRFC reports whether the marker is the RFC's (no task=).
func (m Marker) IsRFC() bool { return m.Task == "" }

// String renders the marker in the exact grammar a hive reads.
func (m Marker) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- spek:publish name=%s artifact_id=%s target=%s", m.Name, m.ArtifactID, m.Target)
	if m.Task != "" {
		fmt.Fprintf(&b, " task=%s", m.Task)
		if m.RFC != "" {
			fmt.Fprintf(&b, " rfc=%s", m.RFC)
		}
	}
	b.WriteString(" -->")
	return b.String()
}

var markerRE = regexp.MustCompile(`<!--\s*spek:publish((?:\s+[a-z_]+=\S+)+)\s*-->`)

// ParseMarker finds the first spek:publish marker in body. ok is false when
// there is none or the marker lacks name, artifact_id or target.
func ParseMarker(body string) (Marker, bool) {
	match := markerRE.FindStringSubmatch(body)
	if match == nil {
		return Marker{}, false
	}
	var m Marker
	for _, field := range strings.Fields(match[1]) {
		key, value, _ := strings.Cut(field, "=")
		switch key {
		case "name":
			m.Name = value
		case "artifact_id":
			m.ArtifactID = value
		case "target":
			m.Target = value
		case "task":
			m.Task = value
		case "rfc":
			m.RFC = value
		}
	}
	if m.Name == "" || m.ArtifactID == "" || m.Target == "" {
		return Marker{}, false
	}
	return m, true
}

// Ref is how a target spells a link to a filed child in another body: "#12"
// on GitHub. A renderer falls back to the child's ID when none is known yet.
type Ref struct {
	Ref string
	URL string
}

// RFCBody renders the RFC body with the children checklist pointing at the
// filed children in refs (keyed by child ID); a child with no ref is listed
// by its ID. It is what the target files and what a re-publish compares.
func (p Publication) RFCBody(refs map[string]Ref) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(p.rfcSections, "\n"))
	b.WriteString("\n\n## Children\n\n")
	for _, c := range p.Children {
		label := "`" + c.ID + "`"
		if r, ok := refs[c.ID]; ok && r.Ref != "" {
			label = r.Ref
		}
		fmt.Fprintf(&b, "- [ ] %s — %s\n", label, c.Title)
	}
	b.WriteString("\n")
	b.WriteString(p.provenance)
	b.WriteString("\n")
	b.WriteString(p.RFC.Marker)
	b.WriteString("\n")
	return b.String()
}

// Render lays the child's body out with its final references: the RFC's URL
// (empty until filed), and each dependency's ref. The marker is the last line.
func (c Child) Render(p Publication, rfcURL string, refs map[string]Ref) string {
	var b strings.Builder
	b.WriteString(c.what)
	b.WriteString("\n## Acceptance criteria\n\n")
	for _, ac := range c.AcceptanceCriteria {
		fmt.Fprintf(&b, "- [ ] %s\n", ac)
	}
	b.WriteString("\n## Where\n\n")
	if c.Repo != "" {
		fmt.Fprintf(&b, "- Repo: `%s`\n", c.Repo)
	} else {
		b.WriteString("- Repo: the project's repo\n")
	}
	if c.Milestone > 0 {
		fmt.Fprintf(&b, "- Milestone: %d\n", c.Milestone)
	}
	if c.Execution != "" {
		fmt.Fprintf(&b, "- Execution: %s\n", c.Execution)
	}
	if len(c.DependsOn) > 0 {
		b.WriteString("\n## Depends on\n\n")
		for _, dep := range c.DependsOn {
			label := "`" + dep + "`"
			if r, ok := refs[dep]; ok && r.Ref != "" {
				label = r.Ref
			}
			title := ""
			if t, ok := p.childByID(dep); ok {
				title = " — " + t.Title
			}
			fmt.Fprintf(&b, "- Blocked by %s%s\n", label, title)
		}
	}
	b.WriteString("\n## Links\n\n")
	if rfcURL != "" {
		fmt.Fprintf(&b, "- Part of RFC: %s\n", rfcURL)
	} else {
		fmt.Fprintf(&b, "- Part of RFC: `%s`\n", p.RFC.Title)
	}
	fmt.Fprintf(&b, "- Spek %s: `%s` (artifact_id `%s`)\n", p.Kind, p.Name, p.ArtifactID)
	b.WriteString("\n")
	m, _ := ParseMarker(c.Marker)
	m.RFC = rfcURL
	b.WriteString(m.String())
	b.WriteString("\n")
	return b.String()
}

func (p Publication) childByID(id string) (Child, bool) {
	for _, c := range p.Children {
		if c.ID == id {
			return c, true
		}
	}
	return Child{}, false
}

// Refusal codes publish reports. Each is a JSON refusal with next_action.
const (
	ErrNotFinal           = "publish_not_final"
	ErrNoChildren         = "publish_no_children"
	ErrDependencyCycle    = "publish_dependency_cycle"
	ErrBodyHygiene        = "publish_body_hygiene"
	ErrTargetUnconfigured = "publish_target_unconfigured"
	ErrRepoUnknown        = "publish_repo_unknown"
	ErrAuthMissing        = "publish_auth_missing"
	ErrConflict           = "publish_conflict"
	ErrTargetFailed       = "publish_target_failed"
)

// Error is a refusal raised while building, checking or filing a
// publication. The command layer turns it into the JSON envelope.
type Error struct {
	Code       string
	Message    string
	NextAction string
}

func (e *Error) Error() string { return e.Message }

// Validate applies the rules every publication must satisfy before filing:
// at least one child, acyclic dependencies that name known children, and
// bodies a hive's clarity gate accepts (≥ MinBodyChars, no unsubstituted
// <placeholders>, no unchosen high/medium/low lists), each child with at
// least one acceptance criterion.
func Validate(p Publication) error {
	if len(p.Children) == 0 {
		return &Error{Code: ErrNoChildren,
			Message:    fmt.Sprintf("%s %q yields no child work items: it has no plan tasks, member specs or requirements to file", p.Kind, p.Name),
			NextAction: "add requirements to the spec, or plan it first so its tasks become the children"}
	}
	order := make([]string, 0, len(p.Children))
	deps := map[string][]string{}
	known := map[string]bool{}
	for _, c := range p.Children {
		order = append(order, c.ID)
		deps[c.ID] = c.DependsOn
		known[c.ID] = true
	}
	for _, c := range p.Children {
		for _, d := range c.DependsOn {
			if !known[d] {
				return &Error{Code: ErrDependencyCycle,
					Message:    fmt.Sprintf("child %q depends on %q, which is not a child of this publication", c.ID, d),
					NextAction: "fix the dependency in the plan or epic so it names another child, then re-run publish"}
			}
		}
	}
	if cycle := depgraph.FindCycle(order, deps); cycle != nil {
		return &Error{Code: ErrDependencyCycle,
			Message:    fmt.Sprintf("the children's dependencies form a cycle: %s", strings.Join(cycle, " -> ")),
			NextAction: "break the cycle in the plan or epic, then re-run publish"}
	}
	if err := checkBody("the RFC", p.RFC.Title, p.RFC.Body); err != nil {
		return err
	}
	for _, c := range p.Children {
		if len(c.AcceptanceCriteria) == 0 {
			return &Error{Code: ErrBodyHygiene,
				Message:    fmt.Sprintf("child %q (%s) has no acceptance criterion", c.ID, c.Title),
				NextAction: "give the task, spec or requirement at least one acceptance criterion, then re-run publish"}
		}
		if err := checkBody("child "+c.ID, c.Title, c.Body); err != nil {
			return err
		}
	}
	return nil
}

var (
	// placeholderRE finds an unsubstituted <placeholder>: an angle-bracketed
	// word or phrase that is not an HTML tag or comment.
	placeholderRE = regexp.MustCompile(`<([A-Za-z][A-Za-z0-9 _.-]*)>`)
	htmlTags      = map[string]bool{"br": true, "p": true, "b": true, "i": true, "em": true, "strong": true, "code": true, "pre": true, "ul": true, "ol": true, "li": true, "details": true, "summary": true, "sub": true, "sup": true, "kbd": true, "a": true, "img": true, "table": true, "tr": true, "td": true, "th": true, "thead": true, "tbody": true, "hr": true, "div": true, "span": true}
	// optionListRE finds an unchosen "high/medium/low" style option list.
	optionListRE = regexp.MustCompile(`(?i)\b(high|medium|low)\s*(/|\|)\s*(high|medium|low)\s*(/|\|)\s*(high|medium|low)\b`)
)

func checkBody(what, title, body string) error {
	stripped := markerRE.ReplaceAllString(body, "")
	if len(strings.TrimSpace(stripped)) < MinBodyChars {
		return &Error{Code: ErrBodyHygiene,
			Message:    fmt.Sprintf("%s (%s) has a body of fewer than %d characters, which a hive's clarity gate parks", what, title, MinBodyChars),
			NextAction: "write more into the spec's sections or the task's description, then re-run publish"}
	}
	for _, m := range placeholderRE.FindAllStringSubmatch(stripped, -1) {
		if htmlTags[strings.ToLower(m[1])] {
			continue
		}
		return &Error{Code: ErrBodyHygiene,
			Message:    fmt.Sprintf("%s (%s) still carries the placeholder <%s>", what, title, m[1]),
			NextAction: "replace the placeholder in the spec with its real value, then re-run publish"}
	}
	if m := optionListRE.FindString(stripped); m != "" {
		return &Error{Code: ErrBodyHygiene,
			Message:    fmt.Sprintf("%s (%s) leaves the option list %q unchosen", what, title, m),
			NextAction: "choose one option in the spec, then re-run publish"}
	}
	return nil
}

// RenderMarkdown writes the whole publication as it would be filed: the RFC,
// then each child, separated by rules. It is what --dry-run prints.
func RenderMarkdown(p Publication) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", p.RFC.Title)
	b.WriteString(p.RFC.Body)
	for _, c := range p.Children {
		b.WriteString("\n---\n\n")
		fmt.Fprintf(&b, "# [%s] %s\n\n", c.ID, c.Title)
		b.WriteString(c.Body)
	}
	return b.String()
}
