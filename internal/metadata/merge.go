package metadata

import (
	"time"
)

// UpdateOptions carries caller-supplied field updates for Merge. Every field
// is optional. DocumentStatus nil means "no document status change" (or
// "draft" on a first-ever write); an existing blank status stays blank. Today is caller-injected for determinism in tests; when
// zero, time.Now().UTC() truncated to day is used.
type UpdateOptions struct {
	DocumentStatus *DocumentStatus
	Today          time.Time
	// Provenance updates for derived changelog entries. Same semantics as
	// DocumentStatus: an empty value means "no change", a non-empty incoming value
	// wins, and existing values are preserved otherwise.
	Project       string
	ProjectSource string
	Spec          string
	Plan          string
	// Designs replaces the artifact's design references wholesale. It is a
	// pointer because the three states are genuinely distinct and the string
	// fields above cannot express them: nil means "no change", which is what
	// an ordinary artifact write passes so existing references survive a
	// body-only rewrite; a non-nil slice replaces the list; and a non-nil
	// empty slice clears it.
	Designs *[]DesignRef
	// Specs replaces the document's back-links wholesale, with the same three
	// states as Designs: nil means "no change", which is what an ordinary
	// authored-design rewrite passes so back-links survive a body-only
	// rewrite; a non-nil slice replaces the list; and a non-nil empty slice
	// clears it.
	Specs *[]string
	// Epic sets or clears the spec's epic, with the same nil-means-no-change
	// rule: nil keeps the current value so a body-only rewrite never drops
	// the membership, a non-nil value replaces it, and a pointer to "" clears
	// it. Only the epic link writer passes it.
	Epic *string
	// Sources replaces the spec's sources wholesale, with the same three
	// states as Designs. Only spec new passes it.
	Sources *[]SourceRef
	// Amendments replaces the spec's recorded amendments wholesale, with the
	// same three states as Designs: nil means "no change", which is what every
	// ordinary spec write passes so the record survives a body rewrite. Only
	// spec amend passes it.
	Amendments *[]Amendment
}

// Merge computes the on-disk bytes for a write by combining an existing store
// blob with caller-supplied field updates and a new body. Invariants:
//
//   - On first write (existing has no metadata), CreatedDate is stamped to
//     today and DocumentStatus defaults to StatusDraft unless overridden.
//   - On subsequent writes, CreatedDate is preserved.
//   - DocumentStatus is validated when opts.DocumentStatus is non-nil.
//   - ClosedDate is stamped exactly once, on the first transition to a closed
//     status (final, superseded, archived); later closed states, and an
//     artifact that already carries a ClosedDate, keep the existing date.
//   - A transition back to draft clears ClosedDate.
//   - With no DocumentStatus update, the existing status is preserved, blank
//     included, along with its ClosedDate.
//   - Design references are preserved across a body-only rewrite, and replaced
//     only when opts.Designs is non-nil.
//   - Back-links are preserved across a body-only rewrite, and replaced only
//     when opts.Specs is non-nil.
//   - A spec's epic, sources and amendments are preserved across a body-only
//     rewrite, and replaced only when opts.Epic, opts.Sources or
//     opts.Amendments is non-nil.
//   - Malformed existing frontmatter propagates as an error rather than being
//     silently replaced.
func Merge(existing []byte, newBody []byte, opts UpdateOptions) ([]byte, error) {
	today := opts.Today
	if today.IsZero() {
		today = time.Now().UTC()
	}
	today = today.UTC().Truncate(24 * time.Hour)

	var current Metadata
	fresh := true
	if len(existing) > 0 {
		fm, _, err := Split(existing)
		if err != nil {
			return nil, err
		}
		if fm != nil {
			current = *fm
			fresh = false
		}
	}

	var result Metadata
	if fresh {
		result.Project = opts.Project
		result.ProjectSource = opts.ProjectSource
		result.Spec = opts.Spec
		result.Plan = opts.Plan
		if opts.Designs != nil {
			result.Designs = *opts.Designs
		}
		if opts.Specs != nil {
			result.Specs = *opts.Specs
		}
		if opts.Epic != nil {
			result.Epic = *opts.Epic
		}
		if opts.Sources != nil {
			result.Sources = *opts.Sources
		}
		if opts.Amendments != nil {
			result.Amendments = *opts.Amendments
		}
		result.CreatedDate = today
		result.DocumentStatus = StatusDraft
		if opts.DocumentStatus != nil {
			if err := validateDocumentStatus(*opts.DocumentStatus); err != nil {
				return nil, err
			}
			result.DocumentStatus = *opts.DocumentStatus
			if isClosed(result.DocumentStatus) {
				result.ClosedDate = today
			}
		}
	} else {
		result.CreatedDate = current.CreatedDate
		result.DocumentStatus = current.DocumentStatus
		result.ClosedDate = current.ClosedDate
		result.Project = current.Project
		result.ProjectSource = current.ProjectSource
		result.Spec = current.Spec
		result.Plan = current.Plan
		// Carried forward unless the caller explicitly replaces the list.
		// This is the single site that makes a design reference durable: the
		// spec workflow commits by writing a freshly assembled body over the
		// stored file, and a reference not preserved here would vanish on that
		// write. The same holds for the back-links on an authored design: a
		// revision rewrites the whole body and passes no Specs update, so a
		// list not preserved here would vanish on every revision.
		result.Designs = current.Designs
		if opts.Designs != nil {
			result.Designs = *opts.Designs
		}
		result.Specs = current.Specs
		if opts.Specs != nil {
			result.Specs = *opts.Specs
		}
		result.Epic = current.Epic
		if opts.Epic != nil {
			result.Epic = *opts.Epic
		}
		result.Sources = current.Sources
		if opts.Sources != nil {
			result.Sources = *opts.Sources
		}
		result.Amendments = current.Amendments
		if opts.Amendments != nil {
			result.Amendments = *opts.Amendments
		}
		if opts.Project != "" {
			result.Project = opts.Project
		}
		if opts.ProjectSource != "" {
			result.ProjectSource = opts.ProjectSource
		}
		if opts.Spec != "" {
			result.Spec = opts.Spec
		}
		if opts.Plan != "" {
			result.Plan = opts.Plan
		}
		if opts.DocumentStatus != nil {
			if err := validateDocumentStatus(*opts.DocumentStatus); err != nil {
				return nil, err
			}
			result.DocumentStatus = *opts.DocumentStatus
			if isClosed(result.DocumentStatus) {
				if result.ClosedDate.IsZero() {
					result.ClosedDate = today
				}
			} else {
				result.ClosedDate = time.Time{}
			}
		}
	}

	return Render(result, newBody)
}
