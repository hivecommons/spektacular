package plan

import (
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/stepkit"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// TestPlanDocStillScaffold verifies that planDocStillScaffold strips a leading
// YAML frontmatter block before comparing against the rendered scaffold, so an
// artifact wrapped in metadata is treated the same as its bare equivalent
// (Phase 1.2 acceptance criteria 1-3).
func TestPlanDocStillScaffold(t *testing.T) {
	const (
		planDir  = "plans"
		planName = "000099_test"
	)

	// Iterate across every planDoc entry so plan.md, context.md and research.md
	// all get the same frontmatter-tolerance guarantee.
	for _, doc := range planDocs {
		doc := doc
		docPath := doc.path(planDir, planName)

		scaffold, err := stepkit.RenderTemplate(doc.scaffold, map[string]any{"name": planName})
		require.NoError(t, err)
		scaffoldBytes := []byte(scaffold)

		fm := metadata.Metadata{
			CreatedDate:    time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC),
			DocumentStatus: metadata.StatusDraft,
		}

		// bare scaffold on disk — must be reported as still scaffold.
		t.Run(docPath+"/bare_scaffold_is_scaffold", func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			require.NoError(t, st.Write(docPath, scaffoldBytes))

			still, err := planDocStillScaffold(st, doc, planDir, planName)
			require.NoError(t, err)
			require.True(t, still, "bare scaffold bytes must be reported as still scaffold")
		})

		// scaffold with a well-formed frontmatter block prepended — must also be
		// reported as still scaffold. This is the Phase 1.2 change.
		t.Run(docPath+"/scaffold_with_frontmatter_is_scaffold", func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			wrapped, err := metadata.Render(fm, scaffoldBytes)
			require.NoError(t, err)
			require.NoError(t, st.Write(docPath, wrapped))

			still, err := planDocStillScaffold(st, doc, planDir, planName)
			require.NoError(t, err)
			require.True(t, still, "scaffold wrapped in frontmatter must be reported as still scaffold")
		})

		// A body that materially differs from the scaffold, without frontmatter —
		// must be reported as no-longer-scaffold.
		t.Run(docPath+"/filled_body_without_frontmatter_is_not_scaffold", func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			filled := append([]byte{}, scaffoldBytes...)
			filled = append(filled, "\n\n## Real content\n"...)
			require.NoError(t, st.Write(docPath, filled))

			still, err := planDocStillScaffold(st, doc, planDir, planName)
			require.NoError(t, err)
			require.False(t, still, "a filled body must not be reported as still scaffold")
		})

		// The same materially-different body, wrapped in frontmatter — must still
		// be reported as no-longer-scaffold.
		t.Run(docPath+"/filled_body_with_frontmatter_is_not_scaffold", func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")
			filled := append([]byte{}, scaffoldBytes...)
			filled = append(filled, "\n\n## Real content\n"...)
			wrapped, err := metadata.Render(fm, filled)
			require.NoError(t, err)
			require.NoError(t, st.Write(docPath, wrapped))

			still, err := planDocStillScaffold(st, doc, planDir, planName)
			require.NoError(t, err)
			require.False(t, still, "a filled body wrapped in frontmatter must not be reported as still scaffold")
		})

		// No file at all — pre-existing behaviour: treat as still scaffold.
		t.Run(docPath+"/missing_file_is_scaffold", func(t *testing.T) {
			st := store.NewFileStore(t.TempDir(), "project")

			still, err := planDocStillScaffold(st, doc, planDir, planName)
			require.NoError(t, err)
			require.True(t, still, "a missing artifact must be reported as still scaffold")
		})
	}
}
