package knowledge

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Criterion 1: every category in the registry has a non-empty Purpose,
// Boundary, and EntryShape — the descriptive contract a contributor relies on.
func TestCategories_AllHaveDescriptiveFields(t *testing.T) {
	for _, c := range Categories {
		require.NotEmpty(t, c.Purpose, "category %q must have a Purpose", c.Name)
		require.NotEmpty(t, c.Boundary, "category %q must have a Boundary", c.Name)
		require.NotEmpty(t, c.EntryShape, "category %q must have an EntryShape", c.Name)
	}
}

// Criterion 2: the registry contains exactly the six known categories, by name,
// in registry order.
func TestCategories_ContainsExactlyTheSixKnownCategories(t *testing.T) {
	names := make([]string, len(Categories))
	for i, c := range Categories {
		names[i] = c.Name
	}
	require.Equal(t, []string{
		"conventions",
		"glossary",
		"architecture",
		"gotchas",
		"learnings",
		"decisions",
	}, names)
}

// Criterion 3: glossary is declared always-applied and decisions is declared
// looked-up — the two tiers, read from the registry.
func TestCategoryByName_DeclaresExpectedTiers(t *testing.T) {
	glossary, ok := CategoryByName("glossary")
	require.True(t, ok)
	require.Equal(t, CategoryTierAlwaysApplied, glossary.Tier)

	decisions, ok := CategoryByName("decisions")
	require.True(t, ok)
	require.Equal(t, CategoryTierLookedUp, decisions.Tier)
}

// Criterion 4: AlwaysApplied returns exactly conventions then glossary, in
// registry order.
func TestAlwaysApplied_ReturnsConventionsAndGlossaryInOrder(t *testing.T) {
	require.Equal(t, []string{"conventions", "glossary"}, AlwaysApplied())
}

// Criterion 5: CategoryByName reports found for a known name and not-found,
// with a zero Category, for an unknown name.
func TestCategoryByName_FoundForKnownMissingForUnknown(t *testing.T) {
	c, ok := CategoryByName("architecture")
	require.True(t, ok)
	require.Equal(t, "architecture", c.Name)

	zero, ok := CategoryByName("nonexistent")
	require.False(t, ok)
	require.Equal(t, Category{}, zero)
}

// Criterion 1: the description a category directory carries is rendered from
// the registry definition and states every field of it — the title, the tier,
// the purpose, the boundary, and the entry shape — so the file documents the
// category rather than carrying a placeholder. The expected fragments are
// hand-written, not recomputed from the renderer.
func TestCategoryREADME_RendersEveryRegistryField(t *testing.T) {
	glossary, ok := CategoryByName("glossary")
	require.True(t, ok)

	readme := glossary.README()
	require.True(t, strings.HasPrefix(readme, "# Glossary\n"),
		"the description must open with the capitalised category title, got %q", readme)
	require.Contains(t, readme, "**Tier:** always-applied")
	require.Contains(t, readme, "**Purpose:** The shared vocabulary of the project")
	require.Contains(t, readme, "**Belongs elsewhere:** Not an explanation of how a thing works")
	require.Contains(t, readme, "**Entry shape:** A term and a short gloss")
}

// Criterion 1: the descriptor is named in one place — a category's own
// generated description is "<registry category>/README.md" and nothing else.
// Path separators are normalised and a leading "./" is not significant.
func TestIsCategoryDescription_TrueOnlyForACategorysOwnDescription(t *testing.T) {
	require.True(t, IsCategoryDescription("conventions/README.md"))
	require.True(t, IsCategoryDescription("./conventions/README.md"),
		"a leading ./ must not hide a descriptor")

	require.False(t, IsCategoryDescription("conventions/naming.md"),
		"an ordinary entry inside a category is not the category's description")
	require.False(t, IsCategoryDescription("README.md"),
		"a README at the store root belongs to no category")
	require.False(t, IsCategoryDescription("notacategory/README.md"),
		"a directory that is not a registry category has no generated description")
}

// Criterion 2: a README a contributor placed deeper inside a category is their
// own file, not the category's generated description — the descriptor shape is
// exactly two segments.
func TestIsCategoryDescription_FalseForAContributorsNestedREADME(t *testing.T) {
	require.False(t, IsCategoryDescription("conventions/sub/README.md"))
}

// Phase 2.2 criterion 1: an entry carrying labels into an always-applied
// category has every one of those labels reported as unreachable, for both
// categories the registry puts in that tier. The next action is checked for
// its content, not its presence: it names the offending category and states
// both of the real options — moving the entry to a looked-up category, naming
// them, or dropping the labels.
func TestUnreachableTags_ReportsEveryLabelOnAnAlwaysAppliedEntry(t *testing.T) {
	for _, category := range []string{"conventions", "glossary"} {
		t.Run(category, func(t *testing.T) {
			content := []byte("---\ntags: [http, routing]\n---\n# Entry\n\nprose\n")

			report := UnreachableTags(category+"/x.md", content)
			require.NotNil(t, report)
			require.Equal(t, category, report.Category)
			require.Equal(t, []string{"http", "routing"}, report.Unreachable)

			require.Contains(t, report.NextAction, `the "`+category+`" category is always-applied`)
			require.Contains(t, report.NextAction,
				"deliberately excluded from search and from the label vocabulary")
			require.Contains(t, report.NextAction,
				"move the entry to a looked-up category, where labels are searchable (architecture, gotchas, learnings, decisions)",
				"the next action must name the categories a search does reach")
			require.Contains(t, report.NextAction, "or drop the labels",
				"the next action must offer the second option too")
			require.Contains(t, report.NextAction, "the entry itself is written either way",
				"the report is not a refusal, and must say so")
		})
	}
}

// Phase 2.2 criterion 3: the same labels on an entry bound for a looked-up
// category are perfectly reachable, so nothing is reported.
func TestUnreachableTags_NilForALookedUpCategory(t *testing.T) {
	content := []byte("---\ntags: [http, routing]\n---\n# Entry\n\nprose\n")
	require.Nil(t, UnreachableTags("gotchas/x.md", content))
}

// Phase 2.2 criterion 4: an entry declaring no labels has nothing to report,
// even in an always-applied category. Covers both shapes of "no labels": no
// frontmatter block at all, and a block too malformed to parse — the latter
// yields no tags rather than failing, exactly as every other reader treats it.
func TestUnreachableTags_NilWhenTheEntryDeclaresNoLabels(t *testing.T) {
	require.Nil(t, UnreachableTags("conventions/x.md", []byte("# Entry\n\nno frontmatter block at all\n")),
		"an entry with no frontmatter block declares no labels")

	// The flow sequence is never closed, which is a YAML scanner error.
	require.Nil(t, UnreachableTags("conventions/x.md", []byte("---\ntags: [http\n---\nprose here\n")),
		"a malformed block yields no tags, so there is nothing to report")
}

// Phase 2.2: a path with no category segment at all sits at the store root and
// belongs to no category, so no tier can be read off it and nothing is
// reported.
func TestUnreachableTags_NilForAPathWithNoCategorySegment(t *testing.T) {
	require.Nil(t, UnreachableTags("x.md", []byte("---\ntags: [http]\n---\n# Entry\n\nprose\n")))
}
