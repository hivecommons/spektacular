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
