// Package metadata owns the artifact-frontmatter schema Spektacular writes at
// the top of every workflow-produced document. It is the single Go module that
// parses, renders, and merges the `created_date` / `document_status` / `closed_date`
// block; every write site in the codebase — the CLI `<kind> file write`
// handler and each workflow's own `st.Write` callbacks — routes through this
// package so the schema stays consistent across sites.
package metadata

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// DocumentStatus is where a workflow-produced document stands in its
// lifecycle. A named status is one of the four values declared below; the
// empty value is the blank status a stored artifact reads as when its
// document_status is missing or unrecognised.
type DocumentStatus string

const (
	// StatusDraft marks a document that is still being written or revised.
	StatusDraft DocumentStatus = "draft"
	// StatusFinal marks a document its owning workflow signed off.
	StatusFinal DocumentStatus = "final"
	// StatusStale marks a plan invalidated by a later spec change under
	// strict plan invalidation.
	StatusStale DocumentStatus = "stale"
	// StatusSuperseded marks a document replaced by a later document.
	StatusSuperseded DocumentStatus = "superseded"
	// StatusArchived marks a document removed from active view.
	StatusArchived DocumentStatus = "archived"
)

// DocumentStatuses returns the four named document statuses in lifecycle
// order. It is the single list of allowed values; flag help text and error
// messages derive from it.
func DocumentStatuses() []DocumentStatus {
	return []DocumentStatus{StatusDraft, StatusFinal, StatusStale, StatusSuperseded, StatusArchived}
}

// ParseDocumentStatus reports whether raw is one of the four named document
// statuses, returning it typed when it is. The blank value is not a named
// status and reports false. Callers validating user input treat false as an
// error; the frontmatter parser treats it as blank.
func ParseDocumentStatus(raw string) (DocumentStatus, bool) {
	for _, s := range DocumentStatuses() {
		if raw == string(s) {
			return s, true
		}
	}
	return "", false
}

const dateFormat = "2006-01-02"

// Metadata is the in-memory mirror of an artifact's YAML frontmatter block.
// CreatedDate is stamped on first write and preserved thereafter. ClosedDate
// is zero until the first transition to a closed document status and is
// stamped once at that transition. A blank DocumentStatus counts as open.
type Metadata struct {
	CreatedDate    time.Time
	DocumentStatus DocumentStatus
	ClosedDate     time.Time
	// Provenance fields carried by derived per-repo changelog entries: the
	// project that produced the entry, its source URL when set, and the spec
	// and plan identifiers. Empty on artifacts that don't carry provenance.
	Project       string
	ProjectSource string
	Spec          string
	Plan          string
	// Designs are the design documents this artifact references. Only a spec
	// carries them today. They are modelled here rather than left as loose
	// frontmatter because yamlShape is a closed schema: a key it does not name
	// is dropped the next time the block is rendered, so an unmodelled
	// reference would silently disappear on the next write.
	Designs []DesignRef
	// Specs are the specs that reference this document, the inbound reverse of
	// Designs. Only a design document Spektacular authored carries them today.
	// Note the neighbouring Spec field is a different fact one character away:
	// Spec is provenance, the single spec whose conversation produced this
	// document, while Specs is every spec that points at it. Modelled here for
	// the same reason Designs is: yamlShape is a closed schema.
	Specs []string
}

// DesignRef is one reference from an artifact to a design document. It names
// both halves of the address: the declared source the document belongs to, and
// its path within that source. A path alone could not say which source was
// searched, so it could never report a useful failure.
type DesignRef struct {
	Source string `yaml:"source" json:"source"`
	Path   string `yaml:"path" json:"path"`
}

// yamlShape is the on-disk representation used by MarshalYAML / UnmarshalYAML.
// It carries the two dates as YYYY-MM-DD strings so day precision is enforced
// at the type boundary rather than at every call site.
type yamlShape struct {
	CreatedDate    string         `yaml:"created_date"`
	DocumentStatus DocumentStatus `yaml:"document_status"`
	ClosedDate     string         `yaml:"closed_date,omitempty"`
	Project        string         `yaml:"project,omitempty"`
	ProjectSource  string         `yaml:"project_source,omitempty"`
	Spec           string         `yaml:"spec,omitempty"`
	Plan           string         `yaml:"plan,omitempty"`
	Designs        []DesignRef    `yaml:"designs,omitempty"`
	Specs          []string       `yaml:"specs,omitempty"`
}

// yamlInShape is the decode-side twin of yamlShape. It holds document_status
// as a raw node so a non-string value cannot fail the decode.
type yamlInShape struct {
	CreatedDate    string    `yaml:"created_date"`
	DocumentStatus yaml.Node `yaml:"document_status"`
	ClosedDate     string    `yaml:"closed_date"`
	Project        string    `yaml:"project"`
	ProjectSource  string    `yaml:"project_source"`
	Spec           string    `yaml:"spec"`
	Plan           string    `yaml:"plan"`
	// Designs is a raw node for the same reason DocumentStatus is: reads
	// across this block are lenient, so a malformed or non-list value must
	// read as no references rather than failing the whole parse and making the
	// artifact unreadable.
	Designs yaml.Node `yaml:"designs"`
	// Specs is a raw node for the same reason Designs is.
	Specs yaml.Node `yaml:"specs"`
}

// MarshalYAML implements yaml.Marshaler.
func (m Metadata) MarshalYAML() (interface{}, error) {
	out := yamlShape{
		CreatedDate:    m.CreatedDate.Format(dateFormat),
		DocumentStatus: m.DocumentStatus,
		Project:        m.Project,
		ProjectSource:  m.ProjectSource,
		Spec:           m.Spec,
		Plan:           m.Plan,
		Designs:        m.Designs,
		Specs:          m.Specs,
	}
	if !m.ClosedDate.IsZero() {
		out.ClosedDate = m.ClosedDate.Format(dateFormat)
	}
	return out, nil
}

// UnmarshalYAML implements yaml.Unmarshaler. It parses the two date fields as
// YYYY-MM-DD. Reading is lenient about document status: a missing,
// unrecognised, retired or non-string value reads as blank and never fails
// the parse, and a legacy `status` key is ignored. Input validation is strict
// and lives in ParseDocumentStatus's callers instead.
func (m *Metadata) UnmarshalYAML(node *yaml.Node) error {
	var in yamlInShape
	if err := node.Decode(&in); err != nil {
		return err
	}
	created, err := time.Parse(dateFormat, in.CreatedDate)
	if err != nil {
		return fmt.Errorf("parsing created_date %q: %w", in.CreatedDate, err)
	}
	m.CreatedDate = created
	m.DocumentStatus = ""
	if in.DocumentStatus.Kind == yaml.ScalarNode {
		if s, ok := ParseDocumentStatus(in.DocumentStatus.Value); ok {
			m.DocumentStatus = s
		}
	}
	if in.ClosedDate != "" {
		closed, err := time.Parse(dateFormat, in.ClosedDate)
		if err != nil {
			return fmt.Errorf("parsing closed_date %q: %w", in.ClosedDate, err)
		}
		m.ClosedDate = closed
	}
	m.Project = in.Project
	m.ProjectSource = in.ProjectSource
	m.Spec = in.Spec
	m.Plan = in.Plan
	m.Designs = decodeDesignRefs(in.Designs)
	m.Specs = decodeSpecNames(in.Specs)
	return nil
}

// decodeDesignRefs reads the designs list leniently. An absent, empty,
// malformed or non-list value yields no references instead of an error: a spec
// whose frontmatter someone hand-edited badly must still be readable, which is
// the same bargain document_status strikes. Entries missing either half of the
// address are dropped, since a reference that cannot name its source and path
// cannot be resolved.
func decodeDesignRefs(node yaml.Node) []DesignRef {
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	var refs []DesignRef
	for _, item := range node.Content {
		var ref DesignRef
		if err := item.Decode(&ref); err != nil {
			continue
		}
		if ref.Source == "" || ref.Path == "" {
			continue
		}
		refs = append(refs, ref)
	}
	return refs
}

// decodeSpecNames reads the specs list leniently, on the same bargain
// decodeDesignRefs strikes: an absent, empty, malformed or non-list value
// yields no back-links instead of an error, so a design whose frontmatter
// someone hand-edited badly stays readable. Non-scalar and empty entries are
// dropped, since a back-link that cannot name a spec cannot be resolved.
func decodeSpecNames(node yaml.Node) []string {
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	var names []string
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			continue
		}
		if item.Value == "" {
			continue
		}
		names = append(names, item.Value)
	}
	return names
}

// validateDocumentStatus returns an error if s is not one of the four named
// document statuses. The blank value is rejected: it is a state an artifact
// can read as, not one a caller can set.
func validateDocumentStatus(s DocumentStatus) error {
	if _, ok := ParseDocumentStatus(string(s)); ok {
		return nil
	}
	return fmt.Errorf("invalid document status %q; must be one of %v", s, DocumentStatuses())
}

// isClosed reports whether s is final, superseded or archived. Draft and
// blank are open.
func isClosed(s DocumentStatus) bool {
	return s == StatusFinal || s == StatusStale || s == StatusSuperseded || s == StatusArchived
}
