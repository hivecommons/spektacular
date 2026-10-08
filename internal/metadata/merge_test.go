package metadata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// draftWithDesigns is an existing artifact that already carries the
// twoDesignRefs list, for the Merge cases that must preserve, replace or
// clear it.
const draftWithDesigns = "---\n" +
	"created_date: 2026-07-01\n" +
	"document_status: draft\n" +
	twoDesignRefsYAML +
	"---\n\n" +
	"# Existing body\n"

// designRefsPtr returns a pointer to refs, since UpdateOptions.Designs is
// *[]DesignRef: nil means "no change", and only a non-nil pointer replaces
// the stored list.
func designRefsPtr(refs []DesignRef) *[]DesignRef { return &refs }

// TestMerge_BodyOnlyRewritePreservesDesigns is the durability case the
// pointer semantics exist for: an ordinary artifact write passes no Designs
// update at all, and the references already on the artifact must survive the
// body being replaced wholesale.
func TestMerge_BodyOnlyRewritePreservesDesigns(t *testing.T) {
	got, err := Merge([]byte(draftWithDesigns), []byte("# Updated body\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoDesignRefs(), meta.Designs,
		"a nil opts.Designs must preserve the stored references")
	require.Equal(t, "# Updated body\n", string(body))
}

// TestMerge_NonNilDesignsReplacesList asserts a non-nil opts.Designs replaces
// the stored list wholesale rather than merging into it.
func TestMerge_NonNilDesignsReplacesList(t *testing.T) {
	replacement := []DesignRef{{Source: "ux", Path: "flows/checkout.md"}}

	got, err := Merge([]byte(draftWithDesigns), []byte("# Existing body\n"), UpdateOptions{
		Today:   fixedToday(),
		Designs: designRefsPtr(replacement),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, replacement, meta.Designs)
}

// TestMerge_EmptyDesignsClearsList asserts a non-nil but empty opts.Designs
// is the explicit "clear" signal, and that a cleared list leaves no designs
// key behind in the rendered block.
func TestMerge_EmptyDesignsClearsList(t *testing.T) {
	got, err := Merge([]byte(draftWithDesigns), []byte("# Existing body\n"), UpdateOptions{
		Today:   fixedToday(),
		Designs: designRefsPtr([]DesignRef{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "designs",
		"a cleared list must be physically absent from the block, not an empty key")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Designs)
}

// TestMerge_FreshWriteRecordsDesigns asserts the first-write branch records
// the references and still applies the usual first-write stamp.
func TestMerge_FreshWriteRecordsDesigns(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today:   fixedToday(),
		Designs: designRefsPtr(twoDesignRefs()),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoDesignRefs(), meta.Designs)
	require.True(t, meta.CreatedDate.Equal(fixedToday()),
		"created_date must be stamped on a fresh write, got %s", meta.CreatedDate)
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// TestMerge_DocumentStatusTransitionCarriesDesigns covers the one genuinely
// new interaction between design references and the pre-existing merge
// invariants: a transition to a closed status stamps closed_date as it always
// did, and carries the references through unchanged on the same write.
func TestMerge_DocumentStatusTransitionCarriesDesigns(t *testing.T) {
	got, err := Merge([]byte(draftWithDesigns), []byte("# Existing body\n"), UpdateOptions{
		Today:          fixedToday(),
		DocumentStatus: documentStatusPtr(StatusFinal),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoDesignRefs(), meta.Designs)
	require.Equal(t, StatusFinal, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(fixedToday()),
		"closed_date must still be stamped on the transition, got %s", meta.ClosedDate)
	require.True(t, meta.CreatedDate.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)))
}

// draftWithSpecs is an existing document that already carries the
// twoSpecNames back-link list, for the Merge cases that must preserve,
// replace or clear it.
const draftWithSpecs = "---\n" +
	"created_date: 2026-07-01\n" +
	"document_status: draft\n" +
	twoSpecNamesYAML +
	"---\n\n" +
	"# Existing body\n"

// specNamesPtr returns a pointer to names, since UpdateOptions.Specs is
// *[]string: nil means "no change", and only a non-nil pointer replaces the
// stored list.
func specNamesPtr(names []string) *[]string { return &names }

// TestMerge_BodyOnlyRewritePreservesSpecs is the durability case the pointer
// semantics exist for: an ordinary design revision passes no Specs update at
// all, and the back-links already on the document must survive the body being
// replaced wholesale.
func TestMerge_BodyOnlyRewritePreservesSpecs(t *testing.T) {
	got, err := Merge([]byte(draftWithSpecs), []byte("# Updated body\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoSpecNames(), meta.Specs,
		"a nil opts.Specs must preserve the stored back-links")
	require.Equal(t, "# Updated body\n", string(body))
}

// TestMerge_NonNilSpecsReplacesList asserts a non-nil opts.Specs replaces the
// stored list wholesale rather than merging into it.
func TestMerge_NonNilSpecsReplacesList(t *testing.T) {
	replacement := []string{"000072_checkout-flow"}

	got, err := Merge([]byte(draftWithSpecs), []byte("# Existing body\n"), UpdateOptions{
		Today: fixedToday(),
		Specs: specNamesPtr(replacement),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, replacement, meta.Specs)
}

// TestMerge_EmptySpecsClearsList asserts a non-nil but empty opts.Specs is
// the explicit "clear" signal, and that a cleared list leaves no specs key
// behind in the rendered block.
func TestMerge_EmptySpecsClearsList(t *testing.T) {
	got, err := Merge([]byte(draftWithSpecs), []byte("# Existing body\n"), UpdateOptions{
		Today: fixedToday(),
		Specs: specNamesPtr([]string{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "specs",
		"a cleared list must be physically absent from the block, not an empty key")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Specs)
}

// TestMerge_FreshWriteRecordsSpecs asserts the first-write branch records the
// back-links and still applies the usual first-write stamp.
func TestMerge_FreshWriteRecordsSpecs(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today: fixedToday(),
		Specs: specNamesPtr(twoSpecNames()),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoSpecNames(), meta.Specs)
	require.True(t, meta.CreatedDate.Equal(fixedToday()),
		"created_date must be stamped on a fresh write, got %s", meta.CreatedDate)
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// TestMerge_DocumentStatusTransitionCarriesSpecs covers the interaction
// between back-links and the pre-existing merge invariants: a transition to a
// closed status stamps closed_date as it always did, and carries the
// back-links through unchanged on the same write.
func TestMerge_DocumentStatusTransitionCarriesSpecs(t *testing.T) {
	got, err := Merge([]byte(draftWithSpecs), []byte("# Existing body\n"), UpdateOptions{
		Today:          fixedToday(),
		DocumentStatus: documentStatusPtr(StatusFinal),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoSpecNames(), meta.Specs)
	require.Equal(t, StatusFinal, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(fixedToday()),
		"closed_date must still be stamped on the transition, got %s", meta.ClosedDate)
	require.True(t, meta.CreatedDate.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)))
}

// draftWithEpicAndSources is an existing spec that already carries testEpic
// and the twoSourceRefs list, for the Merge cases that must preserve, replace
// or clear them.
const draftWithEpicAndSources = "---\n" +
	"created_date: 2026-07-01\n" +
	"document_status: draft\n" +
	"epic: " + testEpic + "\n" +
	twoSourceRefsYAML +
	"---\n\n" +
	"# Existing body\n"

// epicPtr returns a pointer to epic, since UpdateOptions.Epic is *string: nil
// means "no change", and only a non-nil pointer replaces the stored value.
func epicPtr(epic string) *string { return &epic }

// sourceRefsPtr returns a pointer to refs, since UpdateOptions.Sources is
// *[]SourceRef: nil means "no change", and only a non-nil pointer replaces
// the stored list.
func sourceRefsPtr(refs []SourceRef) *[]SourceRef { return &refs }

// TestMerge_BodyOnlyRewritePreservesEpicAndSources is the durability case the
// pointer semantics exist for: the spec workflow rewrites the whole body and
// passes no Epic or Sources update, and the epic membership and sources
// already on the spec must survive the body being replaced wholesale.
func TestMerge_BodyOnlyRewritePreservesEpicAndSources(t *testing.T) {
	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Updated body\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, testEpic, meta.Epic, "a nil opts.Epic must preserve the stored epic")
	require.Equal(t, twoSourceRefs(), meta.Sources, "a nil opts.Sources must preserve the stored sources")
	require.Equal(t, "# Updated body\n", string(body))
}

// TestMerge_NonNilEpicReplacesValue asserts a non-nil opts.Epic replaces the
// stored epic and leaves the sources untouched.
func TestMerge_NonNilEpicReplacesValue(t *testing.T) {
	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Existing body\n"), UpdateOptions{
		Today: fixedToday(),
		Epic:  epicPtr("000072_checkout-epic"),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, "000072_checkout-epic", meta.Epic)
	require.Equal(t, twoSourceRefs(), meta.Sources)
}

// TestMerge_EmptyEpicClearsValue asserts a pointer to "" is the explicit
// "clear" signal, and that a cleared epic leaves no epic key behind in the
// rendered block.
func TestMerge_EmptyEpicClearsValue(t *testing.T) {
	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Existing body\n"), UpdateOptions{
		Today: fixedToday(),
		Epic:  epicPtr(""),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "epic",
		"a cleared epic must be physically absent from the block, not an empty key")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Epic)
	require.Equal(t, twoSourceRefs(), meta.Sources)
}

// TestMerge_NonNilSourcesReplacesList asserts a non-nil opts.Sources replaces
// the stored list wholesale rather than merging into it, and leaves the epic
// untouched.
func TestMerge_NonNilSourcesReplacesList(t *testing.T) {
	replacement := []SourceRef{{URI: "https://example.com/rfc", RetrievedDate: "2026-07-02"}}

	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Existing body\n"), UpdateOptions{
		Today:   fixedToday(),
		Sources: sourceRefsPtr(replacement),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, replacement, meta.Sources)
	require.Equal(t, testEpic, meta.Epic)
}

// TestMerge_EmptySourcesClearsList asserts a non-nil but empty opts.Sources
// is the explicit "clear" signal, and that a cleared list leaves no sources
// key behind in the rendered block.
func TestMerge_EmptySourcesClearsList(t *testing.T) {
	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Existing body\n"), UpdateOptions{
		Today:   fixedToday(),
		Sources: sourceRefsPtr([]SourceRef{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "sources",
		"a cleared list must be physically absent from the block, not an empty key")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Sources)
	require.Equal(t, testEpic, meta.Epic)
}

// TestMerge_FreshWriteRecordsEpicAndSources asserts the first-write branch
// records the epic and sources and still applies the usual first-write stamp.
func TestMerge_FreshWriteRecordsEpicAndSources(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today:   fixedToday(),
		Epic:    epicPtr(testEpic),
		Sources: sourceRefsPtr(twoSourceRefs()),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, testEpic, meta.Epic)
	require.Equal(t, twoSourceRefs(), meta.Sources)
	require.True(t, meta.CreatedDate.Equal(fixedToday()),
		"created_date must be stamped on a fresh write, got %s", meta.CreatedDate)
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// TestMerge_FreshWriteWithClearingEpicAndSourcesOmitsKeys asserts the clear
// signal is harmless on a first write: a pointer to "" and a pointer to an
// empty list record nothing, and neither key appears in the block.
func TestMerge_FreshWriteWithClearingEpicAndSourcesOmitsKeys(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today:   fixedToday(),
		Epic:    epicPtr(""),
		Sources: sourceRefsPtr([]SourceRef{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "epic")
	require.NotContains(t, string(got), "sources")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Epic)
	require.Empty(t, meta.Sources)
}

// TestMerge_DocumentStatusTransitionCarriesEpicAndSources covers the
// interaction between the epic, sources and the pre-existing merge
// invariants: a transition to a closed status stamps closed_date as it always
// did, and carries both fields through unchanged on the same write.
func TestMerge_DocumentStatusTransitionCarriesEpicAndSources(t *testing.T) {
	got, err := Merge([]byte(draftWithEpicAndSources), []byte("# Existing body\n"), UpdateOptions{
		Today:          fixedToday(),
		DocumentStatus: documentStatusPtr(StatusFinal),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, testEpic, meta.Epic)
	require.Equal(t, twoSourceRefs(), meta.Sources)
	require.Equal(t, StatusFinal, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(fixedToday()),
		"closed_date must still be stamped on the transition, got %s", meta.ClosedDate)
	require.True(t, meta.CreatedDate.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)))
}

// draftWithAmendments is an existing spec that already carries testEpic and
// the twoAmendments list, for the Merge cases that must preserve, replace or
// clear them.
const draftWithAmendments = "---\n" +
	"created_date: 2026-07-01\n" +
	"document_status: draft\n" +
	"epic: " + testEpic + "\n" +
	twoAmendmentsYAML +
	"---\n\n" +
	"- [ ] first task\n"

// amendmentsPtr returns a pointer to list, since UpdateOptions.Amendments is
// *[]Amendment: nil means "no change", and only a non-nil pointer replaces
// the stored list.
func amendmentsPtr(list []Amendment) *[]Amendment { return &list }

// TestMerge_BodyOnlyRewritePreservesAmendments is the durability case the
// pointer semantics exist for: an ordinary spec write — here the implement
// workflow ticking a checkbox — passes no Amendments update, and the recorded
// amendments must survive the body being replaced wholesale.
func TestMerge_BodyOnlyRewritePreservesAmendments(t *testing.T) {
	got, err := Merge([]byte(draftWithAmendments), []byte("- [x] first task\n"), UpdateOptions{Today: fixedToday()})
	require.NoError(t, err)

	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoAmendments(), meta.Amendments, "a nil opts.Amendments must preserve the stored amendments")
	require.Equal(t, testEpic, meta.Epic)
	require.Equal(t, "- [x] first task\n", string(body))
}

// TestMerge_NonNilAmendmentsReplacesList asserts a non-nil opts.Amendments
// replaces the stored list wholesale rather than merging into it, and leaves
// the epic untouched.
func TestMerge_NonNilAmendmentsReplacesList(t *testing.T) {
	replacement := append(twoAmendments(), Amendment{
		At:       "2026-10-10T09:00:00Z",
		Sections: []string{"Requirements"},
		Hash:     "sha256:ef",
	})

	got, err := Merge([]byte(draftWithAmendments), []byte("- [ ] first task\n"), UpdateOptions{
		Today:      fixedToday(),
		Amendments: amendmentsPtr(replacement),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, replacement, meta.Amendments)
	require.Equal(t, testEpic, meta.Epic)
}

// TestMerge_EmptyAmendmentsClearsList asserts a non-nil but empty
// opts.Amendments is the explicit "clear" signal, and that a cleared list
// leaves no amendments key behind in the rendered block.
func TestMerge_EmptyAmendmentsClearsList(t *testing.T) {
	got, err := Merge([]byte(draftWithAmendments), []byte("- [ ] first task\n"), UpdateOptions{
		Today:      fixedToday(),
		Amendments: amendmentsPtr([]Amendment{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "amendments",
		"a cleared list must be physically absent from the block, not an empty key")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Amendments)
	require.Equal(t, testEpic, meta.Epic)
}

// TestMerge_FreshWriteRecordsAmendments asserts the first-write branch
// records the amendments and still applies the usual first-write stamp.
func TestMerge_FreshWriteRecordsAmendments(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today:      fixedToday(),
		Amendments: amendmentsPtr(twoAmendments()),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoAmendments(), meta.Amendments)
	require.True(t, meta.CreatedDate.Equal(fixedToday()),
		"created_date must be stamped on a fresh write, got %s", meta.CreatedDate)
	require.Equal(t, StatusDraft, meta.DocumentStatus)
}

// TestMerge_FreshWriteWithClearingAmendmentsOmitsKey asserts the clear signal
// is harmless on a first write: a pointer to an empty list records nothing
// and no amendments key appears in the block.
func TestMerge_FreshWriteWithClearingAmendmentsOmitsKey(t *testing.T) {
	got, err := Merge(nil, []byte("# New body\n"), UpdateOptions{
		Today:      fixedToday(),
		Amendments: amendmentsPtr([]Amendment{}),
	})
	require.NoError(t, err)

	require.NotContains(t, string(got), "amendments")

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Empty(t, meta.Amendments)
}

// TestMerge_DocumentStatusTransitionCarriesAmendments asserts a transition to
// a closed status stamps closed_date as it always did and carries the
// amendments through unchanged on the same write.
func TestMerge_DocumentStatusTransitionCarriesAmendments(t *testing.T) {
	got, err := Merge([]byte(draftWithAmendments), []byte("- [x] first task\n"), UpdateOptions{
		Today:          fixedToday(),
		DocumentStatus: documentStatusPtr(StatusFinal),
	})
	require.NoError(t, err)

	meta, _, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, twoAmendments(), meta.Amendments)
	require.Equal(t, StatusFinal, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(fixedToday()),
		"closed_date must still be stamped on the transition, got %s", meta.ClosedDate)
	require.True(t, meta.CreatedDate.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)))
}
