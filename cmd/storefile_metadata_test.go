package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// kindFixture describes one of the three per-kind write-command families
// (spec / plan / changelog) so the Phase 1.3 acceptance tests can exercise
// each through the same table-driven scaffold.
type kindFixture struct {
	// kind is the CLI subcommand root: "spec", "plan", or "changelog".
	kind string
	// configYAML is the config body written to .spektacular/config.yaml that
	// points this kind's `directory` under a docs-rooted subtree.
	configYAML string
	// artifactName is the argument passed to `<kind> file write`. Every kind
	// that carries an ID prefix uses a name matching the default id_method
	// (timestamp) so validateIDPrefix accepts it.
	artifactName string
	// storeRelPath is the on-disk path relative to the project root where the
	// artifact ends up after a write.
	storeRelPath string
}

// kindFixtures enumerates the three per-kind file-write commands under test.
// Each row uses a docs-rooted directory (not the default `.spektacular`
// data dir) to prove the metadata layer is applied regardless of where the
// configured directory lands, and uses a timestamp-shaped ID prefix so it
// clears validateIDPrefix under the default id_method.
func kindFixtures() []kindFixture {
	return []kindFixture{
		{
			kind:         "spec",
			configYAML:   "spec:\n  config:\n    directory: ../docs/specs\n",
			artifactName: "20260709000000-feature.md",
			storeRelPath: filepath.Join("docs", "specs", "20260709000000-feature.md"),
		},
		{
			kind:         "plan",
			configYAML:   "plan:\n  config:\n    directory: ../docs/plans\n",
			artifactName: "20260709000000-feature/plan.md",
			storeRelPath: filepath.Join("docs", "plans", "20260709000000-feature", "plan.md"),
		},
		{
			kind:       "changelog",
			configYAML: "changelog:\n  config:\n    directory: ../docs/changelog\n",
			// Central (no --repo) writes land flat under the configured
			// changelog directory; no project subfolder — that is a
			// repo-routed concern (see repoRoutedStore in cmd/storefile.go).
			artifactName: "20260709000000-release-notes.md",
			storeRelPath: filepath.Join("docs", "changelog", "20260709000000-release-notes.md"),
		},
	}
}

// today returns today's date at UTC midnight, matching the truncation the
// metadata Merge helper applies before stamping created_date / closed_date.
// Tests compare stamped dates to this value.
func today() time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour)
}

// TestStoreFileWrite_FreshWriteStampsMetadata asserts criterion 1: a fresh
// write through `<kind> file write` produces a stored file whose top begins
// with a YAML frontmatter block containing created_date set to today and
// document_status: draft. Exercised for all three per-kind write commands.
func TestStoreFileWrite_FreshWriteStampsMetadata(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("fresh body"), 0o644))

			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(filepath.Join(dir, fx.storeRelPath))
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta, "fresh write must produce a frontmatter block")
			require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(today()),
				"created_date must be today (%s), got %s", today(), meta.CreatedDate)
			require.True(t, meta.ClosedDate.IsZero(),
				"draft artifact must have zero closed_date, got %s", meta.ClosedDate)
			require.Equal(t, "fresh body", string(body))
		})
	}
}

// TestStoreFileWrite_ExistingArtifactPreservesCreatedDate asserts criterion
// 2: writing an existing artifact preserves the created_date it already had.
// The test primes the store with an artifact carrying an older created_date,
// then writes new body content through the CLI and asserts the created_date
// is unchanged. Exercised for all three per-kind write commands.
func TestStoreFileWrite_ExistingArtifactPreservesCreatedDate(t *testing.T) {
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)

	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			// Seed the store with an artifact that already carries a
			// created_date from earlier in the year, so any preservation
			// failure will be obvious (today != earlier).
			seeded, err := metadata.Render(metadata.Metadata{
				CreatedDate:    earlier,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("original body"))
			require.NoError(t, err)

			storeAbs := filepath.Join(dir, fx.storeRelPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(storeAbs), 0o755))
			require.NoError(t, os.WriteFile(storeAbs, seeded, 0o644))

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("updated body"), 0o644))

			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(storeAbs)
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta, "rewrite must retain a frontmatter block")
			require.True(t, meta.CreatedDate.Equal(earlier),
				"created_date must be preserved (%s), got %s", earlier, meta.CreatedDate)
			require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
			require.Equal(t, "updated body", string(body))
		})
	}
}

// TestStoreFileWrite_StatusFlagTransitionsAndStampsClosedDate asserts
// criterion 3: writing with `--document-status final` on an artifact currently
// draft transitions the status and stamps closed_date to today.
// Exercised for all three per-kind write commands.
func TestStoreFileWrite_StatusFlagTransitionsAndStampsClosedDate(t *testing.T) {
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)

	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seeded, err := metadata.Render(metadata.Metadata{
				CreatedDate:    earlier,
				DocumentStatus: metadata.StatusDraft,
			}, []byte("original body"))
			require.NoError(t, err)

			storeAbs := filepath.Join(dir, fx.storeRelPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(storeAbs), 0o755))
			require.NoError(t, os.WriteFile(storeAbs, seeded, 0o644))

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("final body"), 0o644))

			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath, "--document-status", "final"})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(storeAbs)
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta)
			require.Equal(t, metadata.StatusFinal, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(earlier),
				"created_date must be preserved across status transition (%s), got %s", earlier, meta.CreatedDate)
			require.True(t, meta.ClosedDate.Equal(today()),
				"closed_date must be stamped to today (%s), got %s", today(), meta.ClosedDate)
			require.Equal(t, "final body", string(body))
		})
	}
}

// TestStoreFileWrite_BareArtifactIsUpgradedInPlace asserts criterion 4:
// writing an existing artifact that has no prior frontmatter is treated as
// a first write — the artifact gains a metadata block on its next write,
// created_date is stamped to today, status defaults to draft, and no
// error is raised. Exercised for all three per-kind write commands.
func TestStoreFileWrite_BareArtifactIsUpgradedInPlace(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			// Pre-populate the store with a bare (no-frontmatter) file to
			// simulate an artifact that pre-dates the metadata schema.
			storeAbs := filepath.Join(dir, fx.storeRelPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(storeAbs), 0o755))
			require.NoError(t, os.WriteFile(storeAbs, []byte("legacy body without frontmatter\n"), 0o644))

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("upgraded body"), 0o644))

			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath})

			require.NoError(t, rootCmd.Execute(),
				"bare-artifact upgrade must not error")

			content, err := os.ReadFile(storeAbs)
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta, "bare artifact must gain a frontmatter block on next write")
			require.Equal(t, metadata.StatusDraft, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(today()),
				"created_date on bare-artifact upgrade must be today (%s), got %s", today(), meta.CreatedDate)
			require.True(t, meta.ClosedDate.IsZero())
			require.Equal(t, "upgraded body", string(body))
		})
	}
}

// TestStoreFileWrite_FreshWriteWithClosedStatusStampsBothDates asserts the
// symmetric first-write case for --document-status: passing --document-status final on a
// brand-new artifact stamps both created_date and closed_date to today.
// This complements criterion 3 (which covers the transition path) by
// exercising the first-write branch of the same flag.
func TestStoreFileWrite_FreshWriteWithClosedStatusStampsBothDates(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("closed body"), 0o644))

			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath, "--document-status", "final"})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(filepath.Join(dir, fx.storeRelPath))
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta)
			require.Equal(t, metadata.StatusFinal, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(today()))
			require.True(t, meta.ClosedDate.Equal(today()),
				"fresh write with --document-status final must stamp closed_date to today, got %s", meta.ClosedDate)
			require.Equal(t, "closed body", string(body))
		})
	}
}

// documentStatusNextAction is the remediation every rejected
// --document-status value must carry, naming the allowed values in
// lifecycle order.
const documentStatusNextAction = "Pass --document-status with one of draft, final, stale, superseded, archived."

// rejectedDocumentStatuses are --document-status values the CLI must refuse:
// the two retired values and an unknown one.
var rejectedDocumentStatuses = []string{"completed", "in-progress", "bogus"}

// requireInvalidDocumentStatus asserts a runRootCmd result is the
// invalid_document_status error envelope for bad.
func requireInvalidDocumentStatus(t *testing.T, bad, stdout, stderr string, code int) {
	t.Helper()
	require.Equal(t, 1, code)
	require.Empty(t, stderr)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	require.Equal(t, "invalid_document_status", er.Code)
	require.Equal(t, `--document-status "`+bad+`" is not one of the allowed values`, er.Message)
	require.Equal(t, documentStatusNextAction, er.NextAction)
}

// TestStoreFileWrite_RejectsInvalidDocumentStatusFlag asserts that
// --document-status must be one of the enum values; the retired
// completed and in-progress and an unknown value are rejected with
// invalid_document_status and a remediation listing the allowed values. A
// seeded artifact keeps its exact bytes, and a fresh destination is not
// created. This guards the CLI-layer validation that
// metadataOptsForDocumentStatus performs before Merge sees the value.
func TestStoreFileWrite_RejectsInvalidDocumentStatusFlag(t *testing.T) {
	for _, fx := range kindFixtures() {
		for _, bad := range rejectedDocumentStatuses {
			t.Run(fx.kind+"/"+bad, func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)

				srcPath := filepath.Join(t.TempDir(), "source.md")
				require.NoError(t, os.WriteFile(srcPath, []byte("new body"), 0o644))

				t.Run("fresh destination is not created", func(t *testing.T) {
					resetRootCmd(t)
					stdout, stderr, code := runRootCmd(t, fx.kind, "file", "write", fx.artifactName, "--from", srcPath, "--document-status", bad)
					requireInvalidDocumentStatus(t, bad, stdout, stderr, code)
					require.NoFileExists(t, filepath.Join(dir, fx.storeRelPath),
						"a rejected --document-status must not create the destination file")
				})

				t.Run("seeded artifact is byte-for-byte unchanged", func(t *testing.T) {
					seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
						CreatedDate:    time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
						DocumentStatus: metadata.StatusDraft,
					}, []byte("original body\n"))
					abs := filepath.Join(dir, fx.storeRelPath)
					before, err := os.ReadFile(abs)
					require.NoError(t, err)

					resetRootCmd(t)
					stdout, stderr, code := runRootCmd(t, fx.kind, "file", "write", fx.artifactName, "--from", srcPath, "--document-status", bad)
					requireInvalidDocumentStatus(t, bad, stdout, stderr, code)

					after, err := os.ReadFile(abs)
					require.NoError(t, err)
					require.Equal(t, before, after,
						"a rejected --document-status must leave the file's bytes untouched")
				})
			})
		}
	}
}

// seedArtifactWithMetadata renders a Metadata block over body and writes it
// on disk at storeRelPath under dir. It's a shortcut for tests that need a
// pre-existing artifact with a specific created_date / status baked in.
func seedArtifactWithMetadata(t *testing.T, dir, storeRelPath string, m metadata.Metadata, body []byte) {
	t.Helper()
	rendered, err := metadata.Render(m, body)
	require.NoError(t, err)
	abs := filepath.Join(dir, storeRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, rendered, 0o644))
}

// TestStoreFileSetDocumentStatus_MutatesFrontmatterOnly_PreservesBody asserts
// acceptance criterion 1: `set-document-status` mutates only the frontmatter block of
// the target artifact and leaves the body bytes untouched. For each kind, we
// seed a draft artifact with a known older created_date and a body
// containing shell-sensitive and multi-line content, then flip to `final`
// and assert (a) the frontmatter now shows document_status: final with today's
// closed_date, (b) created_date is unchanged, and (c) the body bytes are
// byte-identical to what was seeded.
func TestStoreFileSetDocumentStatus_MutatesFrontmatterOnly_PreservesBody(t *testing.T) {
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	body := []byte("body line 1 with `backticks` and $dollar\nline 2\n")

	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
				CreatedDate:    earlier,
				DocumentStatus: metadata.StatusDraft,
			}, body)

			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", "final"})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(filepath.Join(dir, fx.storeRelPath))
			require.NoError(t, err)

			meta, gotBody, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta)
			require.Equal(t, metadata.StatusFinal, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(earlier),
				"created_date must be preserved (%s), got %s", earlier, meta.CreatedDate)
			require.True(t, meta.ClosedDate.Equal(today()),
				"closed_date must be stamped to today (%s), got %s", today(), meta.ClosedDate)
			require.Equal(t, body, gotBody,
				"body bytes must be byte-identical to the seeded body")
		})
	}
}

// TestStoreFileSetDocumentStatus_RejectsInvalidStatus_LeavesFileUntouched
// asserts that a --document-status value outside the four-value enum, the
// retired completed and in-progress included, is rejected with
// invalid_document_status and a remediation listing the four values, and the
// on-disk bytes of the target artifact are unchanged. This guards the
// CLI-layer validation performed by metadataOptsForDocumentStatus before any
// write is attempted.
func TestStoreFileSetDocumentStatus_RejectsInvalidStatus_LeavesFileUntouched(t *testing.T) {
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	body := []byte("preserved body\n")

	for _, fx := range kindFixtures() {
		for _, bad := range rejectedDocumentStatuses {
			t.Run(fx.kind+"/"+bad, func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)

				seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
					CreatedDate:    earlier,
					DocumentStatus: metadata.StatusDraft,
				}, body)

				abs := filepath.Join(dir, fx.storeRelPath)
				before, err := os.ReadFile(abs)
				require.NoError(t, err)

				resetRootCmd(t)
				stdout, stderr, code := runRootCmd(t, fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", bad)
				requireInvalidDocumentStatus(t, bad, stdout, stderr, code)

				after, err := os.ReadFile(abs)
				require.NoError(t, err)
				require.Equal(t, before, after,
					"a rejected --document-status must leave the file's bytes untouched")
			})
		}
	}
}

// TestStoreFileSetDocumentStatus_OnBareArtifact_AttachesFrontmatter asserts
// acceptance criterion 3: running set-document-status on an artifact with no prior
// frontmatter attaches a new block with created_date=today, the requested
// status, and (when the status is a closed value) closed_date=today. The
// body bytes of the bare artifact must survive verbatim.
func TestStoreFileSetDocumentStatus_OnBareArtifact_AttachesFrontmatter(t *testing.T) {
	body := []byte("legacy body without frontmatter\n")

	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			abs := filepath.Join(dir, fx.storeRelPath)
			require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
			require.NoError(t, os.WriteFile(abs, body, 0o644))

			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", "archived"})

			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(abs)
			require.NoError(t, err)

			meta, gotBody, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta,
				"bare artifact must gain a frontmatter block after set-document-status")
			require.Equal(t, metadata.StatusArchived, meta.DocumentStatus)
			require.True(t, meta.CreatedDate.Equal(today()),
				"created_date on a bare-artifact set-document-status must be today (%s), got %s", today(), meta.CreatedDate)
			require.True(t, meta.ClosedDate.Equal(today()),
				"closed_date on a closed-status set-document-status must be today (%s), got %s", today(), meta.ClosedDate)
			require.Equal(t, body, gotBody,
				"body bytes of a bare artifact must survive set-document-status unchanged")
		})
	}
}

// TestStoreFileSetDocumentStatus_Idempotent asserts acceptance criterion 4: running
// set-document-status twice with the same value is a no-op on the second call. The
// closed_date stamped by the first call must survive the second (it is not
// re-stamped to a later "today" value even in the trivial same-day case),
// and the on-disk bytes after the second call must be byte-identical to the
// bytes after the first call.
func TestStoreFileSetDocumentStatus_Idempotent(t *testing.T) {
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	body := []byte("stable body\n")

	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
				CreatedDate:    earlier,
				DocumentStatus: metadata.StatusDraft,
			}, body)

			// First set-document-status: transitions to final, stamps closed_date.
			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", "final"})
			require.NoError(t, rootCmd.Execute())

			abs := filepath.Join(dir, fx.storeRelPath)
			afterFirst, err := os.ReadFile(abs)
			require.NoError(t, err)

			// Second set-document-status with the same value: must leave bytes
			// untouched — including the closed_date stamped on the first call.
			resetRootCmd(t)
			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", "final"})
			require.NoError(t, rootCmd.Execute())

			afterSecond, err := os.ReadFile(abs)
			require.NoError(t, err)

			require.Equal(t, afterFirst, afterSecond,
				"repeat set-document-status with the same value must be byte-idempotent")
		})
	}
}

// TestStoreFileSetDocumentStatus_MissingStatusIsError asserts that invoking
// set-document-status without --document-status is rejected by cobra's
// MarkFlagRequired gate with a required-flag error, and the file is left
// untouched. This binds the contract that --document-status is mandatory on
// set-document-status, distinct from `write` where it is optional.
func TestStoreFileSetDocumentStatus_MissingStatusIsError(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			// Seed an existing file so a "not found" error can't be what we
			// accidentally observe instead of a missing-flag error.
			seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
				CreatedDate:    today(),
				DocumentStatus: metadata.StatusDraft,
			}, []byte("body\n"))

			abs := filepath.Join(dir, fx.storeRelPath)
			before, err := os.ReadFile(abs)
			require.NoError(t, err)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, fx.kind, "file", "set-document-status", fx.artifactName)
			require.Equal(t, 1, code, "set-document-status without --document-status must error")
			require.Empty(t, stderr)

			var er output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &er))
			require.True(t, er.IsError)
			require.Equal(t, `required flag(s) "document-status" not set`, er.Message)

			after, err := os.ReadFile(abs)
			require.NoError(t, err)
			require.Equal(t, before, after,
				"a missing-status error must not modify the file")
		})
	}
}

// TestStoreFileSetDocumentStatus_MissingFileIsError asserts that running set-document-status
// against a non-existent path returns the standard `not_found` error envelope
// (via runRoot, which mirrors production Execute wrapping) with the requested
// path named in the resource field, and does not create the target file as a
// side-effect. This binds the contract that set-document-status is a mutation of an
// existing artifact, never a create.
func TestStoreFileSetDocumentStatus_MissingFileIsError(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", "final")

			require.Equal(t, 1, code)
			require.Empty(t, stderr)

			var er output.ErrorResponse
			require.NoError(t, json.Unmarshal([]byte(stdout), &er))
			require.True(t, er.IsError)
			require.Equal(t, "not_found", er.Code)
			require.Equal(t, fx.artifactName, er.Resource)

			require.NoFileExists(t, filepath.Join(dir, fx.storeRelPath),
				"a not_found error must not create the destination file")
		})
	}
}

// TestStoreFileWrite_IdempotentUnderRepeatedWrites is a regression guard: a
// source blob that already carries one or more leading frontmatter blocks
// (e.g. because the caller cp'd a stored artifact into their scratch file)
// must not accumulate. The write handler strips every leading frontmatter
// block from the source before merging, so the resulting stored artifact
// carries exactly one block regardless of how many the source had.
func TestStoreFileWrite_IdempotentUnderRepeatedWrites(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			// Source with three properly-stacked frontmatter blocks — each
			// block terminated by `---\n\n` before the next `---\n` opener,
			// matching the shape a buggy handler produces by re-wrapping its
			// own output. This is what actually accumulates on disk.
			one, err := metadata.Render(metadata.Metadata{
				CreatedDate:    today(),
				DocumentStatus: metadata.StatusDraft,
			}, []byte("real body"))
			require.NoError(t, err)
			// Prepend two more frontmatter blocks. Render already outputs
			// `---\n<yaml>---\n\n<body>`; nesting is `---\n<yaml>---\n\n` +
			// prior content.
			blockPrefix := []byte("---\ncreated_date: \"2026-07-28\"\nstatus: draft\n---\n\n")
			stacked := append([]byte{}, blockPrefix...)
			stacked = append(stacked, blockPrefix...)
			stacked = append(stacked, one...)

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, stacked, 0o644))

			setupImplementCmd(t)
			rootCmd.SetArgs([]string{fx.kind, "file", "write", fx.artifactName, "--from", srcPath})
			require.NoError(t, rootCmd.Execute())

			content, err := os.ReadFile(filepath.Join(dir, fx.storeRelPath))
			require.NoError(t, err)

			meta, body, err := metadata.Split(content)
			require.NoError(t, err)
			require.NotNil(t, meta, "write must produce exactly one leading frontmatter block")
			require.Equal(t, "real body", string(body),
				"body must be the original body, not the source's own frontmatter blocks")

			// After stripping the one leading block, no further frontmatter
			// should be present in the body.
			extra, _, err := metadata.Split(body)
			require.NoError(t, err)
			require.Nil(t, extra,
				"stored artifact must not carry a second frontmatter block underneath the first")
		})
	}
}

// TestStoreFileDocumentStatus_SetOnWriteAndChangeForEveryValue asserts that
// each of the four document statuses can be set with `<kind> file write
// --document-status` and then changed with `<kind> file
// set-document-status`, for every per-kind command family. Each write sets
// one value and the follow-up set-document-status moves to a different one,
// so both commands are exercised for all four values.
func TestStoreFileDocumentStatus_SetOnWriteAndChangeForEveryValue(t *testing.T) {
	transitions := []struct{ write, change string }{
		{write: "draft", change: "final"},
		{write: "final", change: "superseded"},
		{write: "superseded", change: "archived"},
		{write: "archived", change: "draft"},
	}

	for _, fx := range kindFixtures() {
		for _, tr := range transitions {
			t.Run(fx.kind+"/"+tr.write+"->"+tr.change, func(t *testing.T) {
				dir := t.TempDir()
				t.Chdir(dir)
				writeSpecCommandConfig(t, dir, fx.configYAML)

				srcPath := filepath.Join(t.TempDir(), "source.md")
				require.NoError(t, os.WriteFile(srcPath, []byte("body\n"), 0o644))
				abs := filepath.Join(dir, fx.storeRelPath)

				resetRootCmd(t)
				stdout, _, code := runRootCmd(t, fx.kind, "file", "write", fx.artifactName, "--from", srcPath, "--document-status", tr.write)
				require.Equalf(t, 0, code, "write failed: %s", stdout)

				content, err := os.ReadFile(abs)
				require.NoError(t, err)
				meta, _, err := metadata.Split(content)
				require.NoError(t, err)
				require.NotNil(t, meta)
				require.Equal(t, tr.write, string(meta.DocumentStatus))

				resetRootCmd(t)
				stdout, _, code = runRootCmd(t, fx.kind, "file", "set-document-status", fx.artifactName, "--document-status", tr.change)
				require.Equalf(t, 0, code, "set-document-status failed: %s", stdout)

				var resp map[string]any
				require.NoError(t, json.Unmarshal([]byte(stdout), &resp))
				require.Equal(t, tr.change, resp["document_status"])
				require.NotContains(t, resp, "status")

				content, err = os.ReadFile(abs)
				require.NoError(t, err)
				meta, body, err := metadata.Split(content)
				require.NoError(t, err)
				require.NotNil(t, meta)
				require.Equal(t, tr.change, string(meta.DocumentStatus))
				require.Equal(t, "body\n", string(body))
			})
		}
	}
}

// TestStoreFile_RetiredStatusFlagAndSubcommandAreUnknown asserts the old
// command surface is gone: `--status` on `<kind> file write` and `<kind>
// file list`, and the old `set-status` subcommand invoked as it used to be,
// all fail naming the unknown flag, and the seeded artifact is untouched.
// `--status` is passed before any other flag so a parse failure cannot leave
// a sibling flag set for later tests.
func TestStoreFile_RetiredStatusFlagAndSubcommandAreUnknown(t *testing.T) {
	for _, fx := range kindFixtures() {
		t.Run(fx.kind, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			writeSpecCommandConfig(t, dir, fx.configYAML)

			seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
				CreatedDate:    time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
				DocumentStatus: metadata.StatusDraft,
			}, []byte("body\n"))
			abs := filepath.Join(dir, fx.storeRelPath)
			before, err := os.ReadFile(abs)
			require.NoError(t, err)

			srcPath := filepath.Join(t.TempDir(), "source.md")
			require.NoError(t, os.WriteFile(srcPath, []byte("new body"), 0o644))

			cases := map[string][]string{
				"write --status":      {fx.kind, "file", "write", fx.artifactName, "--status", "final", "--from", srcPath},
				"list --status":       {fx.kind, "file", "list", "--status", "final"},
				"set-status --status": {fx.kind, "file", "set-status", fx.artifactName, "--status", "final"},
			}
			for name, args := range cases {
				t.Run(name, func(t *testing.T) {
					stdout, stderr, code := runRootCmd(t, args...)
					require.Equal(t, 1, code)
					require.Empty(t, stderr)

					var er output.ErrorResponse
					require.NoError(t, json.Unmarshal([]byte(stdout), &er))
					require.True(t, er.IsError)
					require.Equal(t, "unknown flag: --status", er.Message)

					after, err := os.ReadFile(abs)
					require.NoError(t, err)
					require.Equal(t, before, after)
				})
			}

			t.Run("set-status without flags", func(t *testing.T) {
				stdout, stderr, code := runRootCmd(t, fx.kind, "file", "set-status", fx.artifactName)
				require.Equal(t, 1, code)
				require.Empty(t, stderr)

				var er output.ErrorResponse
				require.NoError(t, json.Unmarshal([]byte(stdout), &er))
				require.True(t, er.IsError)
				require.Equal(t, "unknown_subcommand", er.Code)
				require.Contains(t, er.Message, `unknown subcommand "set-status"`)
				require.Contains(t, er.NextAction, "set-document-status")

				after, err := os.ReadFile(abs)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		})
	}
}

// specKindFixture returns the spec row of kindFixtures. Design references are
// a spec-only field today, so the tests that exercise them pick that one row
// out of the shared table rather than looping over every kind.
func specKindFixture(t *testing.T) kindFixture {
	t.Helper()
	for _, fx := range kindFixtures() {
		if fx.kind == "spec" {
			return fx
		}
	}
	t.Fatal("kindFixtures no longer contains a spec row")
	return kindFixture{}
}

// TestStoreFileWrite_SpecDesignReferencesSurviveOrdinaryWrite asserts that
// design references already recorded on a spec survive a later ordinary
// `spec file write`. An ordinary write passes UpdateOptions with a nil
// Designs — it says nothing about references at all — so the merge must carry
// the stored list forward rather than drop it when the body is replaced. This
// is the write path the spec workflow uses on every commit, so a regression
// here silently loses every reference on the next edit.
func TestStoreFileWrite_SpecDesignReferencesSurviveOrdinaryWrite(t *testing.T) {
	fx := specKindFixture(t)
	earlier := time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC)
	designs := []metadata.DesignRef{
		{Source: "api", Path: "payments/v2.md"},
		{Source: "platform", Path: "ingress.md"},
	}

	dir := t.TempDir()
	t.Chdir(dir)
	writeSpecCommandConfig(t, dir, fx.configYAML)

	seedArtifactWithMetadata(t, dir, fx.storeRelPath, metadata.Metadata{
		CreatedDate:    earlier,
		DocumentStatus: metadata.StatusDraft,
		Designs:        designs,
	}, []byte("original body\n"))

	srcPath := filepath.Join(t.TempDir(), "source.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("updated body\n"), 0o644))

	resetRootCmd(t)
	stdout, _, code := runRootCmd(t, fx.kind, "file", "write", fx.artifactName, "--from", srcPath)
	require.Equalf(t, 0, code, "write failed: %s", stdout)

	content, err := os.ReadFile(filepath.Join(dir, fx.storeRelPath))
	require.NoError(t, err)

	meta, body, err := metadata.Split(content)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.Equal(t, designs, meta.Designs,
		"an ordinary write must preserve the spec's design references")
	require.Equal(t, "updated body\n", string(body))
}
