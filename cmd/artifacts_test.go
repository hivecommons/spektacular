package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jumppad-labs/spektacular/internal/metadata"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// artifactsListConfigYAML is the config used by every acceptance test in this
// file: all three stored-artifact classes (spec, plan, changelog) point at
// docs/-rooted directories, so a single seed helper can lay out any mix of
// artifacts without reconfiguring per subtest.
const artifactsListConfigYAML = "spec:\n  config:\n    directory: ../docs/specs\nplan:\n  config:\n    directory: ../docs/plans\nchangelog:\n  config:\n    directory: ../docs/changelog\n"

// artifactsResponse is the shape `spektacular artifacts list` writes on
// success: `{"error": false, "artifacts": [...]}`. Each entry carries at
// minimum kind, name, and path; frontmatter-bearing entries additionally
// carry created_date, document_status, and (when non-zero) closed_date.
type artifactsResponse struct {
	Error     bool             `json:"error"`
	Artifacts []map[string]any `json:"artifacts"`
}

// runArtifactsListJSON invokes `artifacts list [args...]` through runRoot
// (the same wrapper Execute uses in production) and decodes the response
// envelope. It requires success — a failing invocation fails the test so
// callers can focus on asserting the returned artifacts array.
func runArtifactsListJSON(t *testing.T, args ...string) artifactsResponse {
	t.Helper()
	stdout, stderr, code := runRootCmd(t, append([]string{"artifacts", "list"}, args...)...)
	require.Equalf(t, 0, code, "artifacts list failed: stdout=%s stderr=%s", stdout, stderr)
	require.Empty(t, stderr)

	var resp artifactsResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &resp))
	require.False(t, resp.Error)
	return resp
}

// artifactKinds returns the sorted list of kind discriminants across every
// artifact in resp. Sorted so test assertions are stable regardless of scan
// order across the three appendXArtifacts calls.
func artifactKinds(resp artifactsResponse) []string {
	kinds := make([]string, 0, len(resp.Artifacts))
	for _, a := range resp.Artifacts {
		if k, ok := a["kind"].(string); ok {
			kinds = append(kinds, k)
		}
	}
	sort.Strings(kinds)
	return kinds
}

// findArtifactByKindAndName locates the unique entry matching (kind, name),
// or nil if none is present. Names are unique per kind in every seed used in
// this file, so this returns at most one match.
func findArtifactByKindAndName(artifacts []map[string]any, kind, name string) map[string]any {
	for _, a := range artifacts {
		if a["kind"] == kind && a["name"] == name {
			return a
		}
	}
	return nil
}

// TestArtifactsList_EmptyStoreReturnsEmptyEnvelope asserts acceptance criterion
// 5: with the store configured but no artifacts on disk anywhere, the command
// returns a well-formed envelope with an empty artifacts array — never a nil
// slice or a missing key. Same shape as a populated store, just empty.
func TestArtifactsList_EmptyStoreReturnsEmptyEnvelope(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.NotNil(t, resp.Artifacts, "the artifacts key must always be present, even when empty")
	require.Empty(t, resp.Artifacts)
}

// TestArtifactsList_UnfilteredReturnsAllKindsTagged asserts acceptance criteria
// 1 and 2: an unfiltered scan produces one entry per artifact across all three
// classes, each tagged with the correct kind discriminant and carrying kind /
// name / path / metadata fields.
func TestArtifactsList_UnfilteredReturnsAllKindsTagged(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	m := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}

	// One spec, one plan directory with all four sibling docs, two changelog entries.
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260709000000-feature.md"), m, []byte("spec body"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "plan.md"), m, []byte("plan body"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "context.md"), m, []byte("context body"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "research.md"), m, []byte("research body"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "test-plan.md"), m, []byte("test-plan body"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260710000000-first.md"), m, []byte("changelog 1"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260711000000-second.md"), m, []byte("changelog 2"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.Len(t, resp.Artifacts, 7, "one entry per seeded artifact across all three classes")

	require.Equal(t,
		[]string{
			"changelog", "changelog",
			"plan.context", "plan.plan", "plan.research", "plan.test-plan",
			"spec",
		},
		artifactKinds(resp),
		"kind discriminants must be tagged 1xspec + 4xplan.* + 2xchangelog")

	// Sanity: every entry carries kind, name, path — the always-present triple.
	for i, a := range resp.Artifacts {
		require.Containsf(t, a, "kind", "entry %d missing kind", i)
		require.Containsf(t, a, "name", "entry %d missing name", i)
		require.Containsf(t, a, "path", "entry %d missing path", i)
	}
}

// TestArtifactsList_KindFilterNarrowsToRequestedClasses asserts acceptance
// criterion 4 and the invalid-kind error path: --kind narrows the result to
// only the requested classes (single or comma-separated), and any unknown
// value in the list returns an invalid_kind error envelope.
func TestArtifactsList_KindFilterNarrowsToRequestedClasses(t *testing.T) {
	seed := func(t *testing.T, dir string) {
		created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
		m := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260709000000-feature.md"), m, []byte("spec"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "plan.md"), m, []byte("plan"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "context.md"), m, []byte("context"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "research.md"), m, []byte("research"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "test-plan.md"), m, []byte("test-plan"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260710000000-first.md"), m, []byte("cl1"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260711000000-second.md"), m, []byte("cl2"))
	}

	t.Run("single kind narrows to just that class", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, artifactsListConfigYAML)
		seed(t, dir)

		resetRootCmd(t)
		resp := runArtifactsListJSON(t, "--kind", "plan.context")

		require.Len(t, resp.Artifacts, 1)
		require.Equal(t, "plan.context", resp.Artifacts[0]["kind"])
		require.Equal(t, "context.md", resp.Artifacts[0]["name"])
	})

	t.Run("comma-separated list unions requested classes", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, artifactsListConfigYAML)
		seed(t, dir)

		resetRootCmd(t)
		resp := runArtifactsListJSON(t, "--kind", "spec,changelog")

		require.Len(t, resp.Artifacts, 3, "1 spec + 2 changelog = 3")
		require.Equal(t, []string{"changelog", "changelog", "spec"}, artifactKinds(resp))
	})

	t.Run("invalid kind value returns invalid_kind error", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		writeSpecCommandConfig(t, dir, artifactsListConfigYAML)
		seed(t, dir)

		resetRootCmd(t)
		stdout, _, code := runRootCmd(t, "artifacts", "list", "--kind", "bogus")
		require.Equal(t, 1, code)

		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
		require.Equal(t, "invalid_kind", er.Code)
		require.Contains(t, er.Message, "bogus")
	})
}

// TestArtifactsList_StatusFilterAppliedAcrossAllKinds asserts acceptance
// criterion 3: --document-status filters uniformly across every scanned class, not
// just one — a mixed seed of draft and final artifacts across
// specs, plan siblings, and changelog entries returns exactly the final
// subset when --document-status final is set.
func TestArtifactsList_StatusFilterAppliedAcrossAllKinds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	ip := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
	done := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}

	// One draft + one final of each class (specs, plan siblings, changelog).
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260701000000-open.md"), ip, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260702000000-done.md"), done, []byte("done"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "plan.md"), ip, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-feature", "context.md"), done, []byte("done"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260710000000-open.md"), ip, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260711000000-done.md"), done, []byte("done"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t, "--document-status", "final")

	require.Len(t, resp.Artifacts, 3, "3 final artifacts across 3 classes")

	// Every returned entry must carry document_status: final.
	for _, a := range resp.Artifacts {
		require.Equal(t, "final", a["document_status"])
	}
	// And they must span the classes seeded as final — not all in one class.
	require.Equal(t, []string{"changelog", "plan.context", "spec"}, artifactKinds(resp))
}

// TestArtifactsList_CreatedDateRangeFiltersAcrossAllKinds asserts acceptance
// criterion 3 again for the created-date filter, and additionally that the
// inclusive-range semantics from Phase 2.1 (`parseListFilter`) are inherited
// verbatim: a seed spanning Jan 1 / Jan 15 / Jan 31 with a range of Jan 10 -
// Jan 20 returns only the Jan 15 entry, across all classes.
func TestArtifactsList_CreatedDateRangeFiltersAcrossAllKinds(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	jan1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	jan15 := time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC)
	jan31 := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.UTC)

	// One artifact per date in each of the three classes — nine total, of
	// which three (one per class) sit on the Jan 15 boundary.
	for _, d := range []struct {
		when time.Time
		suf  string
	}{
		{jan1, "jan1"},
		{jan15, "jan15"},
		{jan31, "jan31"},
	} {
		m := metadata.Metadata{CreatedDate: d.when, DocumentStatus: metadata.StatusDraft}
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260701000000-"+d.suf+".md"), m, []byte(d.suf))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260709000000-"+d.suf, "plan.md"), m, []byte(d.suf))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260710000000-"+d.suf+".md"), m, []byte(d.suf))
	}

	resetRootCmd(t)
	resp := runArtifactsListJSON(t, "--created-after", "2026-01-10", "--created-before", "2026-01-20")

	require.Len(t, resp.Artifacts, 3, "one Jan 15 artifact per class must survive the range")
	for _, a := range resp.Artifacts {
		require.Equal(t, "2026-01-15", a["created_date"], "only Jan 15 artifacts must appear")
	}
	require.Equal(t, []string{"changelog", "plan.plan", "spec"}, artifactKinds(resp),
		"the surviving Jan 15 entries must span every configured class")
}

// TestArtifactsList_CombinedFiltersIntersect asserts acceptance criterion 3's
// AND semantics: combining --kind and --document-status yields the intersection, not
// the union. Only specs that are also final must appear — a final
// changelog and a draft spec both fall outside the intersection.
func TestArtifactsList_CombinedFiltersIntersect(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	ip := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
	done := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}

	// Two specs: one draft, one final. Two changelog entries: same.
	// Only spec+final satisfies --kind spec --document-status final.
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260701000000-open.md"), ip, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260702000000-done.md"), done, []byte("done"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260710000000-open.md"), ip, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "20260711000000-done.md"), done, []byte("done"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t, "--kind", "spec", "--document-status", "final")

	require.Len(t, resp.Artifacts, 1, "only spec + final survives the intersection")
	require.Equal(t, "spec", resp.Artifacts[0]["kind"])
	require.Equal(t, "20260702000000-done.md", resp.Artifacts[0]["name"])
	require.Equal(t, "final", resp.Artifacts[0]["document_status"])
}

// TestArtifactsList_BareArtifactsSurfaceInUnfilteredButNotFiltered asserts the
// bare-artifact behaviour scanArtifact inherits from Phase 2.1: a
// no-frontmatter artifact appears in an unfiltered scan carrying only kind /
// name / path (no metadata fields), but is silently excluded from any
// filtered scan because it can't satisfy a metadata predicate.
func TestArtifactsList_BareArtifactsSurfaceInUnfilteredButNotFiltered(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	ip := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260701000000-draft.md"), ip, []byte("open"))

	// Bare spec: raw body with no frontmatter block at all.
	bareAbs := filepath.Join(dir, "docs", "specs", "20260702000000-bare.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(bareAbs), 0o755))
	require.NoError(t, os.WriteFile(bareAbs, []byte("bare legacy body"), 0o644))

	// Unfiltered: both surface, bare carrying only kind/name/path.
	resetRootCmd(t)
	resp := runArtifactsListJSON(t)
	require.Len(t, resp.Artifacts, 2)

	bare := findArtifactByKindAndName(resp.Artifacts, "spec", "20260702000000-bare.md")
	require.NotNil(t, bare)
	require.Contains(t, bare, "kind")
	require.Contains(t, bare, "name")
	require.Contains(t, bare, "path")
	_, hasStatus := bare["document_status"]
	require.False(t, hasStatus, "a bare artifact must not carry a document_status field")
	_, hasCreated := bare["created_date"]
	require.False(t, hasCreated, "a bare artifact must not carry a created_date field")

	// Filtered: bare artifact is silently dropped.
	resetRootCmd(t)
	resp = runArtifactsListJSON(t, "--document-status", "draft")
	require.Len(t, resp.Artifacts, 1, "any metadata filter must exclude bare artifacts")
	require.Equal(t, "20260701000000-draft.md", resp.Artifacts[0]["name"])
}

// TestArtifactsList_MetadataFieldsPresentPerEntry asserts acceptance
// criterion 2 for both a final (closed_date present) and a draft
// (closed_date absent) artifact — a positive assertion that every carried
// field lands in the entry, complementing the bare-artifact test's negative
// assertion.
func TestArtifactsList_MetadataFieldsPresentPerEntry(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)

	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260701000000-open.md"),
		metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}, []byte("open"))
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260702000000-done.md"),
		metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}, []byte("done"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.Len(t, resp.Artifacts, 2)

	open := findArtifactByKindAndName(resp.Artifacts, "spec", "20260701000000-open.md")
	require.NotNil(t, open)
	require.Equal(t, "spec", open["kind"])
	require.Equal(t, "20260701000000-open.md", open["name"])
	require.Contains(t, open, "path")
	require.Equal(t, "2026-01-10", open["created_date"])
	require.Equal(t, "draft", open["document_status"])
	_, hasClosed := open["closed_date"]
	require.False(t, hasClosed, "a draft artifact must not carry closed_date")

	done := findArtifactByKindAndName(resp.Artifacts, "spec", "20260702000000-done.md")
	require.NotNil(t, done)
	require.Equal(t, "spec", done["kind"])
	require.Equal(t, "20260702000000-done.md", done["name"])
	require.Contains(t, done, "path")
	require.Equal(t, "2026-01-10", done["created_date"])
	require.Equal(t, "final", done["document_status"])
	require.Equal(t, "2026-02-05", done["closed_date"])
}

// TestArtifactsList_MissingConfiguredDirectoriesAreTreatedAsEmpty asserts the
// defence-in-depth behaviour for fresh workspaces: with the three directories
// configured but never materialised on disk (no docs/specs, docs/plans,
// docs/changelog), the command returns an empty envelope with no error —
// never a "directory does not exist" failure that would break every fresh
// project.
func TestArtifactsList_MissingConfiguredDirectoriesAreTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	// Deliberately do not create docs/specs, docs/plans, or docs/changelog.
	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.NotNil(t, resp.Artifacts)
	require.Empty(t, resp.Artifacts)
}

// Criterion 2: changelog artifacts remain visible to artifact listing under
// the new project-namespaced layout — the scanner lists one folder level down
// (`<changelogDir>/<projectName>/`), so an entry seeded there surfaces with
// kind changelog and its flat name.
func TestArtifactsList_ChangelogEntriesListedFromProjectNamespaceFolder(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", "000002_x.md"),
		metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}, []byte("changelog body"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.Len(t, resp.Artifacts, 1)
	entry := findArtifactByKindAndName(resp.Artifacts, "changelog", "000002_x.md")
	require.NotNil(t, entry, "the namespaced changelog entry must appear in the listing")
	require.Equal(t, "docs/changelog/testproj/000002_x.md", entry["path"])
	require.Equal(t, "2026-01-10", entry["created_date"])
	require.Equal(t, "draft", entry["document_status"])
}

// Criterion 2: a project whose changelog namespace folder does not exist yet
// lists cleanly — the changelog directory is present but no entry has ever
// been written for this project, so no `<projectName>/` folder exists, and
// the scan must treat that as empty rather than erroring.
func TestArtifactsList_MissingChangelogNamespaceFolderListsCleanly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	// The changelog directory itself exists, but holds no testproj/ folder.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs", "changelog"), 0o755))

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260709000000-feature.md"),
		metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}, []byte("spec body"))

	resetRootCmd(t)
	resp := runArtifactsListJSON(t)

	require.Len(t, resp.Artifacts, 1, "only the spec must appear; the missing namespace folder is empty, not an error")
	require.Equal(t, "spec", resp.Artifacts[0]["kind"])
}

// TestParseKindFlag_EmptyMeansAllKinds asserts the private helper's contract:
// an empty or whitespace-only raw value returns the full six-kind set that
// the unfiltered scan uses.
func TestParseKindFlag_EmptyMeansAllKinds(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t"} {
		t.Run(raw, func(t *testing.T) {
			set, err := parseKindFlag(raw)
			require.NoError(t, err)
			require.Len(t, set, 6, "an empty --kind must mean every recognised class")
			for _, k := range allArtifactKinds {
				require.Truef(t, set[k], "%q must be in the resulting set", k)
			}
		})
	}
}

// TestParseKindFlag_InvalidValueReturnsError asserts that any unknown entry
// in the comma-separated list is rejected with invalid_kind and an actionable
// message naming both the offending value and the recognised alternatives.
func TestParseKindFlag_InvalidValueReturnsError(t *testing.T) {
	for _, bad := range []string{"bogus", "PLAN", "plan.other", "spec,mystery"} {
		t.Run(bad, func(t *testing.T) {
			_, err := parseKindFlag(bad)
			require.Error(t, err)
			var er *output.ErrorResponse
			require.ErrorAs(t, err, &er)
			require.Equal(t, "invalid_kind", er.Code)
			require.NotEmpty(t, er.NextAction)
		})
	}
}

// TestParseKindFlag_CommaSeparatedNarrows asserts a valid comma-separated
// list returns exactly the requested subset — every listed kind present,
// every other kind absent. Whitespace around entries is tolerated.
func TestParseKindFlag_CommaSeparatedNarrows(t *testing.T) {
	set, err := parseKindFlag("spec, plan.context ,changelog")
	require.NoError(t, err)
	require.Len(t, set, 3)
	require.True(t, set[artifactKindSpec])
	require.True(t, set[artifactKindPlanContext])
	require.True(t, set[artifactKindChangelog])
	require.False(t, set[artifactKindPlanPlan])
	require.False(t, set[artifactKindPlanResearch])
	require.False(t, set[artifactKindPlanTestPlan])
}

// artifactsNamesByKind returns "kind:name" for every artifact in resp,
// sorted, for stable set comparisons.
func artifactsNamesByKind(resp artifactsResponse) []string {
	out := make([]string, 0, len(resp.Artifacts))
	for _, a := range resp.Artifacts {
		out = append(out, fmt.Sprintf("%v:%v", a["kind"], a["name"]))
	}
	sort.Strings(out)
	return out
}

// TestArtifactsList_FiltersByEveryDocumentStatus asserts `artifacts list
// --document-status <v>` returns exactly the spec, plan and changelog stored
// with that value, for each of the four values, and that every entry carries
// a document_status key and no status key.
func TestArtifactsList_FiltersByEveryDocumentStatus(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	seeds := []struct {
		id string
		m  metadata.Metadata
	}{
		{"20260701000000-draft", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}},
		{"20260702000000-final", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}},
		{"20260703000000-superseded", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusSuperseded, ClosedDate: closed}},
		{"20260704000000-archived", metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusArchived, ClosedDate: closed}},
	}
	for _, s := range seeds {
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", s.id+".md"), s.m, []byte("spec"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", s.id, "plan.md"), s.m, []byte("plan"))
		seedArtifactWithMetadata(t, dir, filepath.Join("docs", "changelog", "testproj", s.id+".md"), s.m, []byte("changelog"))
	}

	resetRootCmd(t)
	all := runArtifactsListJSON(t)
	require.Len(t, all.Artifacts, 12)
	for _, a := range all.Artifacts {
		require.Contains(t, a, "document_status", "entry %v", a["name"])
		require.NotContains(t, a, "status", "entry %v", a["name"])
	}

	want := map[string][]string{
		"draft":      {"changelog:20260701000000-draft.md", "plan.plan:plan.md", "spec:20260701000000-draft.md"},
		"final":      {"changelog:20260702000000-final.md", "plan.plan:plan.md", "spec:20260702000000-final.md"},
		"superseded": {"changelog:20260703000000-superseded.md", "plan.plan:plan.md", "spec:20260703000000-superseded.md"},
		"archived":   {"changelog:20260704000000-archived.md", "plan.plan:plan.md", "spec:20260704000000-archived.md"},
	}
	for value, names := range want {
		t.Run(value, func(t *testing.T) {
			resetRootCmd(t)
			resp := runArtifactsListJSON(t, "--document-status", value)
			require.Equal(t, names, artifactsNamesByKind(resp))
			for _, a := range resp.Artifacts {
				require.Equal(t, value, a["document_status"])
			}
		})
	}
}

// TestArtifactsList_LegacyAndUnknownStatusReadAsBlank asserts the cross-kind
// list reads stored frontmatter leniently: in every class, an artifact with
// only the retired `status` key and one with an unknown document_status list
// with an empty document_status when unfiltered, and are excluded by
// --document-status draft and --document-status final.
func TestArtifactsList_LegacyAndUnknownStatusReadAsBlank(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	created := time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	draft := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusDraft}
	final := metadata.Metadata{CreatedDate: created, DocumentStatus: metadata.StatusFinal, ClosedDate: closed}

	writeRawArtifact(t, dir, filepath.Join("docs", "specs", "20260701000000-legacy.md"), legacyStatusArtifact)
	writeRawArtifact(t, dir, filepath.Join("docs", "specs", "20260702000000-bogus.md"), bogusDocumentStatusArtifact)
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "specs", "20260703000000-draft.md"), draft, []byte("spec"))
	writeRawArtifact(t, dir, filepath.Join("docs", "plans", "20260701000000-legacy", "plan.md"), legacyStatusArtifact)
	writeRawArtifact(t, dir, filepath.Join("docs", "plans", "20260701000000-legacy", "context.md"), bogusDocumentStatusArtifact)
	seedArtifactWithMetadata(t, dir, filepath.Join("docs", "plans", "20260701000000-legacy", "research.md"), final, []byte("research"))
	writeRawArtifact(t, dir, filepath.Join("docs", "changelog", "testproj", "20260701000000-legacy.md"), legacyStatusArtifact)
	writeRawArtifact(t, dir, filepath.Join("docs", "changelog", "testproj", "20260702000000-bogus.md"), bogusDocumentStatusArtifact)

	resetRootCmd(t)
	all := runArtifactsListJSON(t)
	require.Equal(t, []string{
		"changelog:20260701000000-legacy.md",
		"changelog:20260702000000-bogus.md",
		"plan.context:context.md",
		"plan.plan:plan.md",
		"plan.research:research.md",
		"spec:20260701000000-legacy.md",
		"spec:20260702000000-bogus.md",
		"spec:20260703000000-draft.md",
	}, artifactsNamesByKind(all))
	blank := map[string]bool{
		"changelog:20260701000000-legacy.md": true,
		"changelog:20260702000000-bogus.md":  true,
		"plan.context:context.md":            true,
		"plan.plan:plan.md":                  true,
		"spec:20260701000000-legacy.md":      true,
		"spec:20260702000000-bogus.md":       true,
	}
	for _, a := range all.Artifacts {
		key := fmt.Sprintf("%v:%v", a["kind"], a["name"])
		require.NotContains(t, a, "status", key)
		if blank[key] {
			require.Equal(t, "", a["document_status"], key)
		}
	}

	resetRootCmd(t)
	drafts := runArtifactsListJSON(t, "--document-status", "draft")
	require.Equal(t, []string{"spec:20260703000000-draft.md"}, artifactsNamesByKind(drafts))

	resetRootCmd(t)
	finals := runArtifactsListJSON(t, "--document-status", "final")
	require.Equal(t, []string{"plan.research:research.md"}, artifactsNamesByKind(finals))
}

// TestArtifactsList_RejectsInvalidDocumentStatus asserts `artifacts list`
// refuses the retired completed and in-progress and an unknown value with
// invalid_document_status and a remediation listing the four values, leaving
// the seeded artifact byte-for-byte unchanged.
func TestArtifactsList_RejectsInvalidDocumentStatus(t *testing.T) {
	for _, bad := range rejectedDocumentStatuses {
		t.Run(bad, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, artifactsListConfigYAML)
			rel := filepath.Join("docs", "specs", "20260701000000-legacy.md")
			writeRawArtifact(t, dir, rel, legacyStatusArtifact)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "artifacts", "list", "--document-status", bad)
			requireInvalidDocumentStatus(t, bad, stdout, stderr, code)

			after, err := os.ReadFile(filepath.Join(dir, rel))
			require.NoError(t, err)
			require.Equal(t, legacyStatusArtifact, string(after))
		})
	}
}

// TestArtifactsList_RetiredStatusFlagIsUnknown asserts the old `--status`
// flag is gone from `artifacts list` and fails as an unknown flag.
func TestArtifactsList_RetiredStatusFlagIsUnknown(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, artifactsListConfigYAML)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "artifacts", "list", "--status", "final")
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "unknown flag: --status", er.Message)
}
