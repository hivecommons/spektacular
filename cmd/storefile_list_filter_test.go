package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/stretchr/testify/require"
)

// listResponse is the shape the `<kind> file list` command writes to stdout in
// Phase 2.1: `{"files": [ {name, path, ...metadata}, ... ], "error": false }`.
// Only files carry metadata fields; directory entries carry only name and path.
type listResponse struct {
	Files []map[string]any `json:"files"`
	Error bool             `json:"error"`
}

// runListJSON runs `<kind> file list [args...]` via runRoot (mirroring
// production wrapping) and decodes the response envelope into a listResponse.
// It fails the test if the command returns a non-zero exit code, so callers
// can focus on asserting the shape and contents of the returned files list.
func runListJSON(t *testing.T, kind string, args ...string) listResponse {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, append([]string{kind, "file", "list"}, args...)...)
	require.Equalf(t, 0, code, "list command failed: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, stderr)

	var resp listResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &resp))
	require.False(t, resp.Error)
	return resp
}

// listFilterFixture describes one of the three per-kind file-list command
// families for the Phase 2.1 acceptance tests. Each row uses a docs-rooted
// directory (not the default .spektacular data dir) and a listPath the tests
// pass to `<kind> file list` — for spec/changelog that's "" (list the store
// root, where the artifacts sit directly), for plan it's the plan's feature
// name so the metadata-bearing documents inside are listed instead of the
// per-plan feature folders at the top level. Listed names are bare: the
// seeded file "draft.md" lists as "draft".
type listFilterFixture struct {
	kind       string
	configYAML string
	// artifactPath is a function that returns the on-disk store-relative path
	// for a given filename, so plan's per-plan subdirectory nesting can be
	// expressed once and reused for every seed.
	artifactPath func(name string) string
	// listPath is passed as the positional argument to `<kind> file list`:
	// the plan feature whose documents are listed (empty means the store
	// root, which is the only listing spec and changelog have).
	listPath string
}

// listFilterFixtures enumerates the three per-kind list-filter command
// surfaces. Spec and changelog list directly under their configured
// directory; plan lists inside a specific plan subdirectory to exercise
// metadata filtering on the plan.md / context.md files inside a plan folder.
func listFilterFixtures() []listFilterFixture {
	return []listFilterFixture{
		{
			kind:         "spec",
			configYAML:   "spec:\n  config:\n    directory: ../docs/specs\n",
			artifactPath: func(name string) string { return filepath.Join("docs", "specs", name) },
			listPath:     "",
		},
		{
			kind:       "changelog",
			configYAML: "changelog:\n  config:\n    directory: ../docs/changelog\n",
			// Central (no --repo) changelog records live flat under the
			// configured directory; a project subfolder appears only under a
			// member repo's changelog store when --repo is used.
			artifactPath: func(name string) string { return filepath.Join("docs", "changelog", name) },
			listPath:     "",
		},
		{
			kind:       "plan",
			configYAML: "plan:\n  config:\n    directory: ../docs/plans\n",
			// Seed each artifact into the same plan subdirectory so a
			// non-empty listPath returns them all as file entries with
			// metadata rather than a single directory entry at the top level.
			artifactPath: func(name string) string {
				return filepath.Join("docs", "plans", "20260709000000-feature", name)
			},
			listPath: "20260709000000-feature",
		},
	}
}

// findFile returns the entry in files whose name matches, or nil.
func findFile(files []map[string]any, name string) map[string]any {
	for _, f := range files {
		if f["name"] == name {
			return f
		}
	}
	return nil
}

// fileNames returns the sorted names of every entry in files, for stable set
// comparisons in the tests below.
func fileNames(files []map[string]any) []string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		if n, ok := f["name"].(string); ok {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names
}

// TestStoreFileList_UnfilteredReturnsAllWithMetadataFields asserts acceptance
// criterion 5 (and half of 6): unfiltered `<kind> file list` returns the same
// set of artifacts as before, plus the new metadata fields on entries whose
// stored bytes carry a frontmatter block. A bare (pre-shipping) artifact is
// included, but carries only name and path — no metadata fields.
func TestStoreFileList_UnfilteredReturnsAllWithMetadataFields(t *testing.T) {
	inProgressCreated := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	completedCreated := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	completedClosed := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft.md"), metadata.Metadata{
				CreatedDate:    inProgressCreated,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("body in progress"))

			seedArtifactWithMetadata(t, dir, fx.artifactPath("final.md"), metadata.Metadata{
				CreatedDate:    completedCreated,
				DocumentStatus: metadata.StatusFinal,
				ClosedDate:     completedClosed,
			}, []byte("body final"))

			// Bare (pre-shipping) artifact: raw bytes, no frontmatter block.
			bareAbs := filepath.Join(dir, fx.artifactPath("bare.md"))
			require.NoError(t, os.MkdirAll(filepath.Dir(bareAbs), 0o755))
			require.NoError(t, os.WriteFile(bareAbs, []byte("bare legacy body"), 0o644))

			resetRootCmd(t)
			var resp listResponse
			if fx.listPath == "" {
				resp = runListJSON(t, fx.kind)
			} else {
				resp = runListJSON(t, fx.kind, fx.listPath)
			}

			require.ElementsMatch(t, []string{"bare", "final", "draft"}, fileNames(resp.Files),
				"unfiltered list must include every seeded artifact, bare ones included")

			draft := findFile(resp.Files, "draft")
			require.NotNil(t, draft)
			require.Equal(t, "draft", draft["document_status"])
			require.Equal(t, "2026-01-10", draft["created_date"])
			_, hasClosed := draft["closed_date"]
			require.False(t, hasClosed, "a draft artifact must not carry a closed_date field")

			final := findFile(resp.Files, "final")
			require.NotNil(t, final)
			require.Equal(t, "final", final["document_status"])
			require.Equal(t, "2026-01-20", final["created_date"])
			require.Equal(t, "2026-02-05", final["closed_date"])

			bare := findFile(resp.Files, "bare")
			require.NotNil(t, bare)
			_, hasStatus := bare["document_status"]
			require.False(t, hasStatus, "a bare artifact must not carry a document_status field")
			_, hasCreated := bare["created_date"]
			require.False(t, hasCreated, "a bare artifact must not carry a created_date field")
		})
	}
}

// TestStoreFileList_StatusFilterExcludesNonMatching asserts acceptance
// criterion 1 and the second half of criterion 6: `--document-status final`
// returns only artifacts whose stored status is `final`, and the bare
// (no-frontmatter) artifact is silently excluded because it can't satisfy
// any metadata predicate.
func TestStoreFileList_StatusFilterExcludesNonMatching(t *testing.T) {
	inProgressCreated := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	completedCreated := time.Date(2026, time.January, 20, 0, 0, 0, 0, time.UTC)
	completedClosed := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft.md"), metadata.Metadata{
				CreatedDate:    inProgressCreated,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("body in progress"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("final.md"), metadata.Metadata{
				CreatedDate:    completedCreated,
				DocumentStatus: metadata.StatusFinal,
				ClosedDate:     completedClosed,
			}, []byte("body final"))

			bareAbs := filepath.Join(dir, fx.artifactPath("bare.md"))
			require.NoError(t, os.MkdirAll(filepath.Dir(bareAbs), 0o755))
			require.NoError(t, os.WriteFile(bareAbs, []byte("bare legacy body"), 0o644))

			resetRootCmd(t)
			args := []string{"--document-status", "final"}
			if fx.listPath != "" {
				args = append([]string{fx.listPath}, args...)
			}
			resp := runListJSON(t, fx.kind, args...)

			require.Equal(t, []string{"final"}, fileNames(resp.Files),
				"a --document-status final filter must return only the final artifact and exclude the bare one")
			require.Equal(t, "final", resp.Files[0]["document_status"])
		})
	}
}

// TestStoreFileList_CreatedDateRangeInclusive asserts acceptance criterion 2:
// `--created-after X --created-before Y` returns only artifacts whose
// created_date falls inclusively within the range. Uses X == Y so a matching
// artifact must be exactly on the boundary — the strongest form of the
// inclusivity claim.
func TestStoreFileList_CreatedDateRangeInclusive(t *testing.T) {
	jan1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	jan15 := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	jan31 := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.artifactPath("jan1.md"), metadata.Metadata{
				CreatedDate:    jan1,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("jan1"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("jan15.md"), metadata.Metadata{
				CreatedDate:    jan15,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("jan15"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("jan31.md"), metadata.Metadata{
				CreatedDate:    jan31,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("jan31"))

			resetRootCmd(t)
			args := []string{"--created-after", "2026-01-15", "--created-before", "2026-01-15"}
			if fx.listPath != "" {
				args = append([]string{fx.listPath}, args...)
			}
			resp := runListJSON(t, fx.kind, args...)

			require.Equal(t, []string{"jan15"}, fileNames(resp.Files),
				"only the boundary-inclusive artifact must appear when after == before")
			require.Equal(t, "2026-01-15", resp.Files[0]["created_date"])
		})
	}
}

// TestStoreFileList_ClosedDateRangeExcludesInProgress asserts acceptance
// criterion 3: `--closed-after X` returns only artifacts whose closed_date
// falls at or after X. A draft artifact (no closed_date) must not be
// returned even though it "trivially" satisfies "> X" for a very old X.
func TestStoreFileList_ClosedDateRangeExcludesInProgress(t *testing.T) {
	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	completedClosed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.artifactPath("open-a.md"), metadata.Metadata{
				CreatedDate:    created,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("still open a"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("open-b.md"), metadata.Metadata{
				CreatedDate:    created,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("still open b"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("closed.md"), metadata.Metadata{
				CreatedDate:    created,
				DocumentStatus: metadata.StatusFinal,
				ClosedDate:     completedClosed,
			}, []byte("closed"))

			resetRootCmd(t)
			args := []string{"--closed-after", "2026-01-01"}
			if fx.listPath != "" {
				args = append([]string{fx.listPath}, args...)
			}
			resp := runListJSON(t, fx.kind, args...)

			require.Equal(t, []string{"closed"}, fileNames(resp.Files),
				"only the artifact with a non-zero closed_date on or after the bound must appear")
			require.Equal(t, "2026-02-01", resp.Files[0]["closed_date"])
		})
	}
}

// TestStoreFileList_CombinedFiltersIntersect asserts acceptance criterion 4:
// combined filter flags AND together, not OR. Seeds four artifacts spanning
// (status × date) combinations and asserts `--document-status final --created-after
// 2026-02-01` returns only the artifact that satisfies both — never the
// union of the two individual filter sets.
func TestStoreFileList_CombinedFiltersIntersect(t *testing.T) {
	jan15 := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	feb15 := time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC)
	closedDate := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			// (status, created_date) matrix — only final-feb matches.
			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft-jan.md"), metadata.Metadata{
				CreatedDate:    jan15,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("ip jan"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft-feb.md"), metadata.Metadata{
				CreatedDate:    feb15,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("ip feb"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("final-jan.md"), metadata.Metadata{
				CreatedDate:    jan15,
				DocumentStatus: metadata.StatusFinal,
				ClosedDate:     closedDate,
			}, []byte("done jan"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("final-feb.md"), metadata.Metadata{
				CreatedDate:    feb15,
				DocumentStatus: metadata.StatusFinal,
				ClosedDate:     closedDate,
			}, []byte("done feb"))

			resetRootCmd(t)
			args := []string{"--document-status", "final", "--created-after", "2026-02-01"}
			if fx.listPath != "" {
				args = append([]string{fx.listPath}, args...)
			}
			resp := runListJSON(t, fx.kind, args...)

			require.Equal(t, []string{"final-feb"}, fileNames(resp.Files),
				"the intersection of --document-status final AND --created-after 2026-02-01 must contain only final-feb — not the union")
		})
	}
}

// TestStoreFileList_BareArtifactsExcludedFromAnyFilter asserts the second half
// of acceptance criterion 6: a bare (no-frontmatter) artifact appears in an
// unfiltered listing but is silently excluded from any listing that sets a
// metadata filter, because a bare artifact carries no metadata for the filter
// to match against. Runs unfiltered and filtered back-to-back within the same
// subtest — the reset helper isolates the two calls.
func TestStoreFileList_BareArtifactsExcludedFromAnyFilter(t *testing.T) {
	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft.md"), metadata.Metadata{
				CreatedDate:    time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC),
				DocumentStatus: metadata.StatusDraft,
			}, []byte("body"))

			bareAbs := filepath.Join(dir, fx.artifactPath("bare.md"))
			require.NoError(t, os.MkdirAll(filepath.Dir(bareAbs), 0o755))
			require.NoError(t, os.WriteFile(bareAbs, []byte("bare legacy body"), 0o644))

			// Unfiltered: both artifacts appear.
			resetRootCmd(t)
			var unfiltered listResponse
			if fx.listPath == "" {
				unfiltered = runListJSON(t, fx.kind)
			} else {
				unfiltered = runListJSON(t, fx.kind, fx.listPath)
			}
			require.ElementsMatch(t, []string{"bare", "draft"}, fileNames(unfiltered.Files),
				"unfiltered list must include the bare artifact")

			// Filtered: only the draft artifact appears; the bare one is silently excluded.
			resetRootCmd(t)
			args := []string{"--document-status", "draft"}
			if fx.listPath != "" {
				args = append([]string{fx.listPath}, args...)
			}
			filtered := runListJSON(t, fx.kind, args...)
			require.Equal(t, []string{"draft"}, fileNames(filtered.Files),
				"any metadata filter must exclude the bare artifact silently")
		})
	}
}

// TestStoreFileList_TopLevelPlanDirectoryEntriesCarryNameAndPathOnly is
// plan-specific coverage of the "directory entries become objects but carry
// only name and path" claim from the Phase 2.1 note. `plan file list` at the
// top of the plans directory yields one entry per plan sub-dir; those
// directory entries must not carry the metadata fields (created_date,
// status, closed_date) even though their contained plan.md does.
func TestStoreFileList_TopLevelPlanDirectoryEntriesCarryNameAndPathOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, "plan:\n  config:\n    directory: ../docs/plans\n")

	// Seed a plan.md inside a plan sub-directory carrying full metadata.
	seedArtifactWithMetadata(t,
		dir,
		filepath.Join("docs", "plans", "20260709000000-feature", "plan.md"),
		metadata.Metadata{
			CreatedDate:    time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC),
			DocumentStatus: metadata.StatusDraft,
		},
		[]byte("plan body"),
	)

	resetRootCmd(t)
	resp := runListJSON(t, "plan")

	require.Equal(t, []string{"20260709000000-feature"}, fileNames(resp.Files),
		"top-level plan list must return the plan sub-directory as its own entry")

	planDir := resp.Files[0]
	require.Equal(t, "20260709000000-feature", planDir["name"])
	require.Equal(t, "../docs/plans/20260709000000-feature", planDir["path"])
	// Directory entries must not carry metadata fields — those live on the
	// plan.md file inside, not on the containing directory.
	_, hasStatus := planDir["document_status"]
	require.False(t, hasStatus, "directory entries must not carry a document_status field")
	_, hasCreated := planDir["created_date"]
	require.False(t, hasCreated, "directory entries must not carry a created_date field")
	_, hasClosed := planDir["closed_date"]
	require.False(t, hasClosed, "directory entries must not carry a closed_date field")
}

// legacyStatusArtifact is a hand-written artifact from before the document
// status rename: it carries the retired `status` key and no document_status,
// so it must read as a blank document status.
const legacyStatusArtifact = "---\ncreated_date: \"2026-01-01\"\nstatus: completed\nclosed_date: \"2026-01-02\"\n---\n\nlegacy body\n"

// bogusDocumentStatusArtifact is a hand-written artifact whose
// document_status is not one of the four values, so it must read as blank.
const bogusDocumentStatusArtifact = "---\ncreated_date: \"2026-01-01\"\ndocument_status: bogus\n---\n\nbogus body\n"

// writeRawArtifact writes content verbatim at storeRelPath under dir.
func writeRawArtifact(t *testing.T, dir, storeRelPath, content string) {
	t.Helper()
	abs := filepath.Join(dir, storeRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}

// listArgs prepends fx.listPath (when set) to the given filter args.
func (fx listFilterFixture) listArgs(args ...string) []string {
	if fx.listPath == "" {
		return args
	}
	return append([]string{fx.listPath}, args...)
}

// TestStoreFileList_FiltersByEveryDocumentStatus asserts `<kind> file list
// --document-status <v>` returns exactly the artifact stored with that value
// for each of the four values, and that every listed file entry carries a
// document_status key and no status key.
func TestStoreFileList_FiltersByEveryDocumentStatus(t *testing.T) {
	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	seeds := []struct {
		name string
		m    metadata.Metadata
	}{
		{"draft.md", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}},
		{"final.md", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}},
		{"superseded.md", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusSuperseded, ClosedDate: closed}},
		{"archived.md", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusArchived, ClosedDate: closed}},
	}
	want := map[string]string{
		"draft":      "draft",
		"final":      "final",
		"superseded": "superseded",
		"archived":   "archived",
	}

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)
			for _, s := range seeds {
				seedArtifactWithMetadata(t, dir, fx.artifactPath(s.name), s.m, []byte("body"))
			}

			resetRootCmd(t)
			all := runListJSON(t, fx.kind, fx.listArgs()...)
			require.Len(t, all.Files, 4)
			for _, f := range all.Files {
				require.Contains(t, f, "document_status", "entry %v", f["name"])
				require.NotContains(t, f, "status", "entry %v", f["name"])
			}

			for value, name := range want {
				t.Run(value, func(t *testing.T) {
					resetRootCmd(t)
					resp := runListJSON(t, fx.kind, fx.listArgs("--document-status", value)...)
					require.Equal(t, []string{name}, fileNames(resp.Files))
					require.Equal(t, value, resp.Files[0]["document_status"])
				})
			}
		})
	}
}

// TestStoreFileList_LegacyAndUnknownStatusReadAsBlank asserts stored
// frontmatter is read leniently: an artifact with only the retired `status`
// key, and one with an unknown document_status, both list with an empty
// document_status, are included when no filter is set, and are excluded by
// --document-status draft and --document-status final.
func TestStoreFileList_LegacyAndUnknownStatusReadAsBlank(t *testing.T) {
	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)

	for _, fx := range listFilterFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)
			writeRawArtifact(t, dir, fx.artifactPath("legacy.md"), legacyStatusArtifact)
			writeRawArtifact(t, dir, fx.artifactPath("bogus.md"), bogusDocumentStatusArtifact)
			seedArtifactWithMetadata(t, dir, fx.artifactPath("draft.md"),
				metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}, []byte("body"))
			seedArtifactWithMetadata(t, dir, fx.artifactPath("final.md"),
				metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}, []byte("body"))

			resetRootCmd(t)
			all := runListJSON(t, fx.kind, fx.listArgs()...)
			require.Equal(t, []string{"bogus", "draft", "final", "legacy"}, fileNames(all.Files))
			for _, name := range []string{"legacy", "bogus"} {
				entry := findFile(all.Files, name)
				require.NotNil(t, entry)
				require.Equal(t, "", entry["document_status"], name)
				require.NotContains(t, entry, "status", name)
			}

			resetRootCmd(t)
			drafts := runListJSON(t, fx.kind, fx.listArgs("--document-status", "draft")...)
			require.Equal(t, []string{"draft"}, fileNames(drafts.Files))

			resetRootCmd(t)
			finals := runListJSON(t, fx.kind, fx.listArgs("--document-status", "final")...)
			require.Equal(t, []string{"final"}, fileNames(finals.Files))
		})
	}
}

// TestStoreFileList_RejectsInvalidDocumentStatus asserts `<kind> file list`
// refuses the retired completed and in-progress and an unknown value with
// invalid_document_status and a remediation listing the four values, and
// leaves the seeded artifact byte-for-byte unchanged.
func TestStoreFileList_RejectsInvalidDocumentStatus(t *testing.T) {
	for _, fx := range listFilterFixtures() {
		for _, bad := range rejectedDocumentStatuses {
			t.Run(fx.kind+"/"+bad, func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)
				writeRawArtifact(t, dir, fx.artifactPath("legacy.md"), legacyStatusArtifact)

				resetRootCmd(t)
				args := append([]string{fx.kind, "file", "list"}, fx.listArgs("--document-status", bad)...)
				stdout, stderr, code := runRootCmd(t, args...)
				requireInvalidDocumentStatus(t, bad, stdout, stderr, code)

				after, err := os.ReadFile(filepath.Join(dir, fx.artifactPath("legacy.md")))
				require.NoError(t, err)
				require.Equal(t, legacyStatusArtifact, string(after))
			})
		}
	}
}
