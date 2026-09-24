package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
)

// documentStatusValues renders the allowed document statuses, in lifecycle
// order, for flag help text and error remediation.
func documentStatusValues() string {
	names := make([]string, 0, len(metadata.DocumentStatuses()))
	for _, s := range metadata.DocumentStatuses() {
		names = append(names, string(s))
	}
	return strings.Join(names, ", ")
}

// parseDocumentStatusFlag validates a `--document-status` value supplied on
// the command line. Input is strict: anything outside the named values,
// the retired in-progress and completed included, is rejected with an
// actionable error, unlike stored frontmatter, which reads such values as
// blank.
func parseDocumentStatusFlag(raw string) (metadata.DocumentStatus, error) {
	s, ok := metadata.ParseDocumentStatus(raw)
	if !ok {
		return "", output.NewError("invalid_document_status",
			fmt.Sprintf("--document-status %q is not one of the allowed values", raw)).
			WithNextAction(fmt.Sprintf("Pass --document-status with one of %s.", documentStatusValues()))
	}
	return s, nil
}

// artifactFilter represents the five combinable filter flags exposed on
// `<kind> file list` (and reused by `spektacular artifacts list`). Every field
// is optional; a zero-value filter matches every artifact — the `active` guard
// lets callers preserve the pre-shipping bare-artifact behaviour when no flag
// is set.
type artifactFilter struct {
	documentStatus metadata.DocumentStatus
	createdAfter   time.Time
	createdBefore  time.Time
	closedAfter    time.Time
	closedBefore   time.Time
}

// active reports whether any filter flag has been set. A caller that wants to
// preserve the pre-shipping behaviour on bare-string listings ("show every
// entry, metadata-less ones included") should skip metadata matching when
// active returns false.
func (f artifactFilter) active() bool {
	return f.documentStatus != "" ||
		!f.createdAfter.IsZero() ||
		!f.createdBefore.IsZero() ||
		!f.closedAfter.IsZero() ||
		!f.closedBefore.IsZero()
}

// matches reports whether m satisfies every set filter (AND semantics — the
// intersection, not the union).
func (f artifactFilter) matches(m metadata.Metadata) bool {
	if f.documentStatus != "" && m.DocumentStatus != f.documentStatus {
		return false
	}
	created := m.CreatedDate
	if !f.createdAfter.IsZero() && created.Before(f.createdAfter) {
		return false
	}
	if !f.createdBefore.IsZero() && created.After(f.createdBefore) {
		return false
	}
	closed := m.ClosedDate
	if !f.closedAfter.IsZero() {
		if closed.IsZero() || closed.Before(f.closedAfter) {
			return false
		}
	}
	if !f.closedBefore.IsZero() {
		if closed.IsZero() || closed.After(f.closedBefore) {
			return false
		}
	}
	return true
}

// parseListFilter validates and parses the five raw filter-flag values from
// the `<kind> file list` and `spektacular artifacts list` command surfaces.
// Empty strings mean "no filter", so a blank-status artifact is listed only
// when no document status filter is set. A document status outside the four
// values or a date not in YYYY-MM-DD form is rejected with an actionable
// error.
func parseListFilter(documentStatus, createdAfter, createdBefore, closedAfter, closedBefore string) (artifactFilter, error) {
	var f artifactFilter
	if documentStatus != "" {
		s, err := parseDocumentStatusFlag(documentStatus)
		if err != nil {
			return artifactFilter{}, err
		}
		f.documentStatus = s
	}
	parseDate := func(flag, raw string) (time.Time, error) {
		if raw == "" {
			return time.Time{}, nil
		}
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return time.Time{}, output.NewError("invalid_date",
				fmt.Sprintf("--%s %q is not a valid YYYY-MM-DD date", flag, raw)).
				WithNextAction("Pass a date in YYYY-MM-DD form, e.g. 2026-07-01.")
		}
		return t.UTC(), nil
	}
	var err error
	if f.createdAfter, err = parseDate("created-after", createdAfter); err != nil {
		return artifactFilter{}, err
	}
	if f.createdBefore, err = parseDate("created-before", createdBefore); err != nil {
		return artifactFilter{}, err
	}
	if f.closedAfter, err = parseDate("closed-after", closedAfter); err != nil {
		return artifactFilter{}, err
	}
	if f.closedBefore, err = parseDate("closed-before", closedBefore); err != nil {
		return artifactFilter{}, err
	}
	return f, nil
}
