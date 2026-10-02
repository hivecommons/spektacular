// Package epic owns the epic document: a parent that groups several specs
// delivering one oversized request. An epic holds only an overview and its
// specs with the dependencies between them; everything else lives in the
// specs. It has its own frontmatter type rather than reusing the shared spec
// metadata, because an epic's specs list is a list of {name, depends_on}
// entries while the shared metadata's specs is a design's list of bare
// back-link names: a spec-style rewrite would silently drop the graph.
package epic

import (
	"bytes"
	"fmt"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hivecommons/spektacular/internal/metadata"
)

// EpicSpec is one entry of an epic's specs list. DependsOn names the specs in
// the same epic it depends on. A nil DependsOn means the field was absent,
// which Validate refuses; a non-nil empty slice means no dependencies, and is
// what every caller building an entry by hand must pass.
type EpicSpec struct {
	Name      string   `yaml:"name" json:"name"`
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
}

// Epic is the in-memory mirror of an epic document.
type Epic struct {
	CreatedDate    time.Time
	DocumentStatus metadata.DocumentStatus
	ClosedDate     time.Time
	// Spec is provenance: the spec whose split produced this epic, which is
	// always its first spec. Empty when the epic was written from a source.
	Spec string
	// Specs is the epic's members in display order, which also breaks ties
	// between specs that are ready at the same time.
	Specs []EpicSpec
	// Sources are the external materials that directly seeded the epic.
	Sources []metadata.SourceRef
	// Body is everything after the frontmatter: "## Overview" and "## Specs".
	Body []byte
}

// SpecNames returns the members' names in list order.
func (e Epic) SpecNames() []string {
	names := make([]string, len(e.Specs))
	for i, s := range e.Specs {
		names[i] = s.Name
	}
	return names
}

// Member returns the entry for name, and whether the epic lists it.
func (e Epic) Member(name string) (EpicSpec, bool) {
	for _, s := range e.Specs {
		if s.Name == name {
			return s, true
		}
	}
	return EpicSpec{}, false
}

// yamlOut is the on-disk frontmatter shape. depends_on carries no omitempty:
// it is always written, as [] when a spec has no dependencies, so a reader
// never has to guess whether the field was forgotten. specs is always written
// too, as [] for an epic created before its first spec.
type yamlOut struct {
	CreatedDate    string                  `yaml:"created_date"`
	DocumentStatus metadata.DocumentStatus `yaml:"document_status"`
	ClosedDate     string                  `yaml:"closed_date,omitempty"`
	Spec           string                  `yaml:"spec,omitempty"`
	Specs          []EpicSpec              `yaml:"specs"`
	Sources        []metadata.SourceRef    `yaml:"sources,omitempty"`
}

// yamlIn is the decode-side twin of yamlOut. document_status is a raw node so
// a non-string value reads as blank instead of failing, as for every other
// document. The specs graph is decoded strictly: a malformed graph is an
// error, never silently read as an empty epic.
type yamlIn struct {
	CreatedDate    string               `yaml:"created_date"`
	DocumentStatus yaml.Node            `yaml:"document_status"`
	ClosedDate     string               `yaml:"closed_date"`
	Spec           string               `yaml:"spec"`
	Specs          []EpicSpec           `yaml:"specs"`
	Sources        []metadata.SourceRef `yaml:"sources"`
}

// Parse reads an epic document. A document with no frontmatter reads as an
// epic with only a body; malformed frontmatter is an error.
func Parse(raw []byte) (Epic, error) {
	fm, body, ok, err := metadata.SplitRaw(raw)
	if err != nil {
		return Epic{}, err
	}
	if !ok {
		return Epic{Body: raw}, nil
	}
	var in yamlIn
	if err := yaml.Unmarshal(fm, &in); err != nil {
		return Epic{}, fmt.Errorf("malformed epic frontmatter: %w", err)
	}
	e := Epic{Spec: in.Spec, Specs: in.Specs, Sources: in.Sources, Body: body}
	if in.CreatedDate != "" {
		if e.CreatedDate, err = time.Parse(metadata.DateFormat, in.CreatedDate); err != nil {
			return Epic{}, fmt.Errorf("parsing created_date %q: %w", in.CreatedDate, err)
		}
	}
	if in.DocumentStatus.Kind == yaml.ScalarNode {
		if s, ok := metadata.ParseDocumentStatus(in.DocumentStatus.Value); ok {
			e.DocumentStatus = s
		}
	}
	if in.ClosedDate != "" {
		if e.ClosedDate, err = time.Parse(metadata.DateFormat, in.ClosedDate); err != nil {
			return Epic{}, fmt.Errorf("parsing closed_date %q: %w", in.ClosedDate, err)
		}
	}
	return e, nil
}

// Render writes e as a fenced YAML frontmatter block, a blank line, and the
// body, in the same layout as every other Spektacular document.
func (e Epic) Render() ([]byte, error) {
	out := yamlOut{
		CreatedDate:    e.CreatedDate.Format(metadata.DateFormat),
		DocumentStatus: e.DocumentStatus,
		Spec:           e.Spec,
		Specs:          make([]EpicSpec, len(e.Specs)),
		Sources:        e.Sources,
	}
	for i, s := range e.Specs {
		deps := s.DependsOn
		if deps == nil {
			deps = []string{}
		}
		out.Specs[i] = EpicSpec{Name: s.Name, DependsOn: deps}
	}
	if !e.ClosedDate.IsZero() {
		out.ClosedDate = e.ClosedDate.Format(metadata.DateFormat)
	}
	yamlBytes, err := yaml.Marshal(out)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(yamlBytes)
	buf.WriteString("---\n\n")
	buf.Write(e.Body)
	return buf.Bytes(), nil
}

// Stamp sets next's lifecycle fields for a write, with the same rules as
// metadata.Merge: on a first write (existing nil) the created date is today
// and the status defaults to draft; afterwards the created date, status and
// closed date carry forward. A status update is validated; the closed date is
// stamped once on the first transition to a closed status and cleared on a
// return to draft. today is caller-injected for determinism; zero means now.
func Stamp(existing *Epic, next Epic, status *metadata.DocumentStatus, today time.Time) (Epic, error) {
	if today.IsZero() {
		today = time.Now().UTC()
	}
	today = today.UTC().Truncate(24 * time.Hour)

	if existing == nil || existing.CreatedDate.IsZero() {
		next.CreatedDate = today
		next.DocumentStatus = metadata.StatusDraft
		next.ClosedDate = time.Time{}
	} else {
		next.CreatedDate = existing.CreatedDate
		next.DocumentStatus = existing.DocumentStatus
		next.ClosedDate = existing.ClosedDate
	}
	if status != nil {
		if err := metadata.ValidateDocumentStatus(*status); err != nil {
			return Epic{}, err
		}
		next.DocumentStatus = *status
		if metadata.IsClosed(next.DocumentStatus) {
			if next.ClosedDate.IsZero() {
				next.ClosedDate = today
			}
		} else {
			next.ClosedDate = time.Time{}
		}
	}
	return next, nil
}
