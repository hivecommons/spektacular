package cmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file tests the `design` command family in cmd/design.go: reaching the
// project's declared design sources from the command line. The contract these
// tests hold the commands to is that Spektacular owns the *reference* to a
// design document and never the document itself — a write stores the staged
// bytes unchanged, a read returns them unchanged, and no other file in the
// source directory is touched.
//
// Every command is driven through resetRootCmd + runRootCmd, never by calling
// a RunE directly, because the cobra commands are package-level globals whose
// flag values would otherwise leak into whichever test `-shuffle=on` runs
// next.

// designSourceItem mirrors one entry of the `design sources` JSON envelope.
type designSourceItem struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Location string `json:"location"`
}

// designSourcesResult mirrors the `design sources` JSON envelope.
type designSourcesResult struct {
	Sources []designSourceItem `json:"sources"`
}

// designDocumentItem mirrors one entry of the `design list` JSON envelope.
// Each document carries the source it came from, so a fanned-out listing is
// unambiguous.
type designDocumentItem struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// designListResult mirrors the `design list` JSON envelope.
type designListResult struct {
	Documents []designDocumentItem `json:"documents"`
}

// designRawListResult mirrors the same envelope without modelling a document
// at all. A listing reports lifecycle fields only for a design Spektacular
// authored, and the contract is that the keys are absent from every other
// entry rather than present and empty — a distinction a struct with string
// fields cannot express, since an omitted key and an empty one decode
// identically. Decoding each entry as a bare map makes the whole key set the
// assertion.
type designRawListResult struct {
	Documents []map[string]any `json:"documents"`
}

// designWriteResult mirrors the `design write` JSON envelope: the address the
// request named, plus the absolute location the bytes landed at.
type designWriteResult struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Location string `json:"location"`
}

// designAuthorResult mirrors the `design author` JSON envelope: the address
// and location `design write` reports, plus the two lifecycle facts the
// stamped block carries, so a caller need not follow an author with a read.
type designAuthorResult struct {
	Source         string `json:"source"`
	Path           string `json:"path"`
	Location       string `json:"location"`
	DocumentStatus string `json:"document_status"`
	CreatedDate    string `json:"created_date"`
}

// designAuthoredDoc is the on-disk shape `design author` is expected to
// produce: keys is every frontmatter line between the delimiters, body is what
// follows the blank line after them. The expectation is assembled from
// hand-written strings here rather than through metadata.Render, so no part of
// it is derived from the code under test.
func designAuthoredDoc(keys, body string) string {
	return "---\n" + keys + "---\n\n" + body
}

// designSourceDecl is one design source declaration for a fixture config. The
// location is written into config.yaml verbatim, so a test can declare a
// relative location (resolved from the folder holding config.yaml) or an
// absolute one pointing outside the project root.
type designSourceDecl struct {
	name     string
	location string
}

// writeDesignConfig declares sources under `design:` in dir's project config,
// reusing writeSpecCommandConfig for everything a project needs besides the
// design block (the settings schema, the skills version, a slug-safe name and
// a registered repo).
func writeDesignConfig(t *testing.T, dir string, sources ...designSourceDecl) {
	t.Helper()
	body := "design:\n  sources:\n"
	for _, s := range sources {
		body += fmt.Sprintf("    - name: %s\n      provider: file\n      config:\n        location: %s\n", s.name, s.location)
	}
	writeSpecCommandConfig(t, dir, body)
}

// seedDesignDoc writes content at loc/relPath, creating parent folders. It
// writes with os.WriteFile rather than through the design commands, so the
// fixture is independent of the code under test.
func seedDesignDoc(t *testing.T, loc, relPath, content string) string {
	t.Helper()
	full := filepath.Join(loc, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	return full
}

// stageDesignDoc writes content to a file in its own temp folder and returns
// its path, ready to hand to `design write --from`.
func stageDesignDoc(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// twoSourceDesignProject lays out a project rooted at a t.TempDir() and chdirs
// into it, declaring two design sources: "api", at a location relative to the
// folder holding config.yaml, and "ux", at an absolute location outside the
// project root entirely — a folder the team already keeps, which is exactly
// what a design source is allowed to point at. The api source holds a
// top-level document and one nested in a subdirectory; the ux source holds a
// single top-level document. It returns the project root and the two resolved
// source locations.
func twoSourceDesignProject(t *testing.T) (root, apiLoc, uxLoc string) {
	t.Helper()
	root = t.TempDir()
	apiLoc = filepath.Join(root, "docs", "design")
	uxLoc = filepath.Join(t.TempDir(), "design-docs")

	seedDesignDoc(t, apiLoc, "overview.md", "# Overview\n")
	seedDesignDoc(t, apiLoc, "payments/v2.md", "# Payments v2\n")
	seedDesignDoc(t, uxLoc, "flows.md", "# Flows\n")

	t.Chdir(root)
	writeDesignConfig(t, root,
		designSourceDecl{name: "api", location: "../docs/design"},
		designSourceDecl{name: "ux", location: uxLoc},
	)
	return root, apiLoc, uxLoc
}

// Criterion: `design sources` lists every declared source with its name, its
// provider and the absolute location the declaration resolved to — including a
// relative location resolved from the folder holding config.yaml.
func TestDesignSources_ListsEveryDeclaredSourceWithResolvedLocation(t *testing.T) {
	root, apiLoc, uxLoc := twoSourceDesignProject(t)
	require.Equal(t, filepath.Join(root, "docs", "design"), apiLoc)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "sources")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designSourcesResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []designSourceItem{
		{Name: "api", Provider: "file", Location: filepath.Join(root, "docs", "design")},
		{Name: "ux", Provider: "file", Location: uxLoc},
	}, got.Sources)
}

// Criterion: `design list` with no --source returns the documents from every
// declared source, each tagged with the source it came from, and recurses into
// subdirectories.
func TestDesignList_FansOutAcrossEverySourceAndRecurses(t *testing.T) {
	twoSourceDesignProject(t)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "list")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designListResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []designDocumentItem{
		{Source: "api", Path: "overview.md"},
		{Source: "api", Path: "payments/v2.md"},
		{Source: "ux", Path: "flows.md"},
	}, got.Documents)
}

// Criterion: `design list --source <name>` narrows the listing to that one
// declared source.
func TestDesignList_SourceFlagNarrowsToOneSource(t *testing.T) {
	twoSourceDesignProject(t)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "list", "--source", "ux")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designListResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []designDocumentItem{
		{Source: "ux", Path: "flows.md"},
	}, got.Documents)
}

// Criterion: `design list` reports the lifecycle fields of a design
// Spektacular authored, and omits them entirely for a document the project
// already had — including one carrying frontmatter of the team's own, which is
// not a lifecycle block and must not be read as one.
//
// The optional keys are covered in both directions: an authored document
// carrying every one of them reports all of them, and an authored document
// carrying only the two mandatory ones reports only those, so an absent close
// date or back-link is omitted rather than reported blank.
//
// The team.md entry is the listing half of the team-frontmatter criterion; the
// overwrite half is TestDesignWrite_ReplacesDocumentsSpektacularDidNotAuthor.
func TestDesignList_ReportsLifecycleFieldsOnlyForAuthoredDocuments(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")

	// Every fixture is written straight to disk, block and all, so no part of
	// the expectation below is produced by the code under test.
	seedDesignDoc(t, apiLoc, "authored/full.md", designAuthoredDoc(
		"created_date: \"2026-01-05\"\ndocument_status: final\nclosed_date: \"2026-02-01\"\n"+
			"spec: 000010_origin\nspecs:\n    - 000012_alpha\n    - 000030_beta\n",
		"# Full\n"))
	seedDesignDoc(t, apiLoc, "authored/minimal.md", designAuthoredDoc(
		"created_date: \"2026-03-09\"\ndocument_status: draft\n", "# Minimal\n"))
	seedDesignDoc(t, apiLoc, "plain.md", "# Plain\n\nNo frontmatter anywhere.\n")
	seedDesignDoc(t, apiLoc, "team.md",
		"---\ntitle: Payments v2\nauthor: the payments team\n---\n\n# Team\n\nTheirs.\n")

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "list")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designRawListResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []map[string]any{
		{
			"source":          "api",
			"path":            "authored/full.md",
			"created_date":    "2026-01-05",
			"document_status": "final",
			"closed_date":     "2026-02-01",
			"spec":            "000010_origin",
			"specs":           []any{"000012_alpha", "000030_beta"},
		},
		{
			"source":          "api",
			"path":            "authored/minimal.md",
			"created_date":    "2026-03-09",
			"document_status": "draft",
		},
		{"source": "api", "path": "plain.md"},
		{"source": "api", "path": "team.md"},
	}, got.Documents)
}

// Criterion: `design read` returns the document's bytes exactly as they are on
// disk, with no JSON envelope wrapped around them.
func TestDesignRead_ReturnsRawBytesWithNoEnvelope(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	const body = "---\ntitle: Payments v2\n---\n\n# Payments v2\n\nSettled shape.\n"
	docPath := seedDesignDoc(t, apiLoc, "payments/v2.md", body)

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "read", "--data", `{"source":"api","path":"payments/v2.md"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	onDisk, err := os.ReadFile(docPath)
	require.NoError(t, err)
	require.Equal(t, string(onDisk), stdout, "read must return the file's bytes unchanged")
	require.Equal(t, body, stdout)

	// No JSON envelope: the bytes are the whole of stdout, so they do not
	// decode as the result object every other subcommand emits.
	var envelope map[string]any
	require.Error(t, json.Unmarshal([]byte(stdout), &envelope), "read must not wrap the document in a JSON envelope")
	require.NotContains(t, stdout, `"error"`)
}

// Criterion: reading a design Spektacular authored still returns its bytes
// exactly as stored, lifecycle block included. The listing now parses that
// block, and a read must keep handing back the whole document — the
// read-edit-author loop stages what it got back as the next author's input, so
// a read that stripped or reordered the block would silently rewrite history.
func TestDesignRead_ReturnsAuthoredDocumentBytesUnchanged(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	stored := designAuthoredDoc(
		"created_date: \"2026-01-05\"\ndocument_status: final\nclosed_date: \"2026-02-01\"\n"+
			"specs:\n    - 000012_alpha\n",
		"# Payments v2\n\nThe settled request shape.\n")
	seedDesignDoc(t, apiLoc, "authored/v2.md", stored)

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "read", "--data", `{"source":"api","path":"authored/v2.md"}`)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)
	require.Equal(t, stored, stdout)
}

// designRoundTripDocs are the three documents the round-trip criterion covers:
// one carrying its own YAML frontmatter (which Spektacular must not touch,
// merge into, or re-order), one carrying none (which must not acquire any), and
// one with CRLF line endings and trailing whitespace (which must not be
// normalised).
var designRoundTripDocs = []struct {
	name    string
	path    string
	content string
}{
	{
		name:    "own frontmatter",
		path:    "payments/v2.md",
		content: "---\ntitle: Payments v2\nstatus: settled\nowners:\n  - payments\n---\n\n# Payments v2\n\nThe settled request shape.\n",
	},
	{
		name:    "no frontmatter",
		path:    "checkout.md",
		content: "# Checkout\n\nNo frontmatter anywhere in this document.\n",
	},
	{
		name:    "crlf and trailing whitespace",
		path:    "legacy/imported.md",
		content: "# Imported   \r\n\r\nLine with trailing spaces   \r\nLine with a trailing tab\t\r\n\r\n",
	},
}

// Criterion: `design write --from` then `design read` round-trips a document
// byte for byte — nothing is added, removed or reformatted, whether the
// document carries its own frontmatter, none at all, or CRLF endings and
// trailing whitespace.
func TestDesignWriteRead_RoundTripsBytesUnchanged(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	require.NoError(t, os.MkdirAll(apiLoc, 0o755))

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	for _, doc := range designRoundTripDocs {
		t.Run(doc.name, func(t *testing.T) {
			from := stageDesignDoc(t, filepath.Base(doc.path), doc.content)
			data := fmt.Sprintf(`{"source":"api","path":%q}`, doc.path)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "design", "write", "--data", data, "--from", from)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)

			var written designWriteResult
			require.NoError(t, json.Unmarshal([]byte(stdout), &written))
			require.Equal(t, designWriteResult{
				Source:   "api",
				Path:     doc.path,
				Location: filepath.Join(root, "docs", "design", filepath.FromSlash(doc.path)),
			}, written)

			// The bytes on disk are the staged bytes: no frontmatter stamped,
			// no line endings rewritten, no trailing whitespace trimmed.
			onDisk, err := os.ReadFile(written.Location)
			require.NoError(t, err)
			require.Equal(t, doc.content, string(onDisk), "stored bytes must be the staged bytes, unchanged")

			resetRootCmd(t)
			stdout, stderr, code = runRootCmd(t, "design", "read", "--data", data)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)
			require.Equal(t, doc.content, stdout, "read must return the written bytes, unchanged")
			require.Len(t, stdout, len(doc.content), "nothing may be added to or removed from the document")
		})
	}
}

// Criterion: a document written with `design write` then appears in that
// source's `design list` output.
func TestDesignWrite_DocumentAppearsInSourceListing(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	seedDesignDoc(t, apiLoc, "overview.md", "# Overview\n")

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	from := stageDesignDoc(t, "v2.md", "# Payments v2\n\nSettled shape.\n")

	resetRootCmd(t)
	_, stderr, code := runRootCmd(t, "design", "write", "--data", `{"source":"api","path":"payments/v2.md"}`, "--from", from)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "list", "--source", "api")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designListResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, []designDocumentItem{
		{Source: "api", Path: "overview.md"},
		{Source: "api", Path: "payments/v2.md"},
	}, got.Documents)
}

// Criterion: nothing Spektacular writes alters any other file in the source
// directory. The directory is snapshotted before and after a write by walking
// it directly, and only the intended path may have appeared.
func TestDesignWrite_LeavesEveryOtherFileUntouched(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	seedDesignDoc(t, apiLoc, "overview.md", "# Overview\n\nUnchanged.\n")
	seedDesignDoc(t, apiLoc, "payments/v1.md", "---\ntitle: Payments v1\n---\n\n# Payments v1\n")
	seedDesignDoc(t, apiLoc, "notes.txt", "loose note, no frontmatter, trailing space \n")

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	// snapshotDir (init_test.go) walks the directory with filepath.WalkDir and
	// os.ReadFile, hashing each file's bytes — an oracle that never consults
	// the code under test.
	before := snapshotDir(t, apiLoc)
	require.Len(t, before, 3)
	for _, path := range []string{"overview.md", "payments/v1.md", "notes.txt"} {
		require.Contains(t, before, path)
	}

	const added = "# Payments v2\n\nThe new shape.\n"
	from := stageDesignDoc(t, "v2.md", added)

	resetRootCmd(t)
	_, stderr, code := runRootCmd(t, "design", "write", "--data", `{"source":"api","path":"payments/v2.md"}`, "--from", from)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	// The expected snapshot is the one taken before the write plus exactly one
	// new path, whose expected hash is derived from the staged literal.
	expected := map[string]string{"payments/v2.md": fmt.Sprintf("%x", sha256.Sum256([]byte(added)))}
	for path, digest := range before {
		expected[path] = digest
	}
	require.Equal(t, expected, snapshotDir(t, apiLoc),
		"only the addressed document may appear, and no other file may change")

	onDisk, err := os.ReadFile(filepath.Join(apiLoc, "payments", "v2.md"))
	require.NoError(t, err)
	require.Equal(t, added, string(onDisk))
}

// designUnauthoredDocs are the two shapes of document a verbatim write is
// still allowed to replace: one carrying no frontmatter at all, and one
// carrying frontmatter the team wrote. The second is the important half. A
// block Spektacular did not write cannot be parsed as a lifecycle record, and
// the only correct reading of that failure is "not ours, so not protected" —
// if it were read as a refusal instead, the guard would lock the team out of
// the very documents this feature promises not to disturb.
var designUnauthoredDocs = []struct {
	name   string
	path   string
	seeded string
}{
	{
		name:   "no frontmatter",
		path:   "checkout.md",
		seeded: "# Checkout\n\nThe first shape.\n",
	},
	{
		name:   "the team's own frontmatter",
		path:   "payments/v2.md",
		seeded: "---\ntitle: Payments v2\nauthor: the payments team\n---\n\n# Payments v2\n\nTheirs.\n",
	},
}

// Criterion: `design write` still replaces a document Spektacular did not
// author, storing the staged bytes exactly as supplied.
func TestDesignWrite_ReplacesDocumentsSpektacularDidNotAuthor(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	for _, doc := range designUnauthoredDocs {
		t.Run(doc.name, func(t *testing.T) {
			seedDesignDoc(t, apiLoc, doc.path, doc.seeded)

			const replacement = "# Rewritten\n\nThe second shape.\n"
			from := stageDesignDoc(t, filepath.Base(doc.path), replacement)
			data := fmt.Sprintf(`{"source":"api","path":%q}`, doc.path)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "design", "write", "--data", data, "--from", from)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)

			var written designWriteResult
			require.NoError(t, json.Unmarshal([]byte(stdout), &written))
			require.Equal(t, designWriteResult{
				Source:   "api",
				Path:     doc.path,
				Location: filepath.Join(root, "docs", "design", filepath.FromSlash(doc.path)),
			}, written)

			onDisk, err := os.ReadFile(written.Location)
			require.NoError(t, err)
			require.Equal(t, replacement, string(onDisk),
				"the stored bytes must be the staged bytes, with nothing of the replaced document left behind")
		})
	}
}

// Criterion: a document written with `design author` reads back carrying its
// capture date and its lifecycle status, both in the stored frontmatter and in
// the command's own envelope. Exercised against both source shapes — a
// location declared relative to the config.yaml folder, and an absolute one
// outside the project root — because the stamping happens on the way through
// the same store path a plain write uses.
func TestDesignAuthor_StampsCaptureDateAndDraftStatus(t *testing.T) {
	_, apiLoc, uxLoc := twoSourceDesignProject(t)
	// The command stamps from the real clock and has no --today seam, so the
	// expected date is computed the same way today() does for the store-file
	// tests rather than hardcoded.
	stamped := today().Format("2006-01-02")

	for _, tc := range []struct {
		source string
		loc    string
	}{
		{source: "api", loc: apiLoc},
		{source: "ux", loc: uxLoc},
	} {
		t.Run(tc.source, func(t *testing.T) {
			const body = "# Payments v2\n\nThe settled request shape.\n"
			from := stageDesignDoc(t, "v2.md", body)
			data := fmt.Sprintf(`{"source":%q,"path":"authored/v2.md"}`, tc.source)

			resetRootCmd(t)
			stdout, stderr, code := runRootCmd(t, "design", "author", "--data", data, "--from", from)
			require.Equal(t, 0, code)
			require.Empty(t, stderr)

			var got designAuthorResult
			require.NoError(t, json.Unmarshal([]byte(stdout), &got))
			require.Equal(t, designAuthorResult{
				Source:         tc.source,
				Path:           "authored/v2.md",
				Location:       filepath.Join(tc.loc, "authored", "v2.md"),
				DocumentStatus: "draft",
				CreatedDate:    stamped,
			}, got)

			// The stored bytes are the staged body with exactly one block in
			// front of it: a capture date, a status, and nothing else — no
			// spec key, because none was named.
			onDisk, err := os.ReadFile(filepath.Join(tc.loc, "authored", "v2.md"))
			require.NoError(t, err)
			require.Equal(t, designAuthoredDoc(
				"created_date: \""+stamped+"\"\ndocument_status: draft\n", body,
			), string(onDisk))
		})
	}
}

// Criterion: naming the spec a design came from records it on the document.
// The other half of the criterion — that leaving --spec out records nothing in
// its place — is the byte-exact expectation in
// TestDesignAuthor_StampsCaptureDateAndDraftStatus, whose stored document
// carries no spec key at all.
func TestDesignAuthor_RecordsTheSpecTheDesignCameFrom(t *testing.T) {
	_, apiLoc, _ := twoSourceDesignProject(t)
	stamped := today().Format("2006-01-02")

	const body = "# Payments v2\n\nThe settled request shape.\n"
	from := stageDesignDoc(t, "v2.md", body)

	resetRootCmd(t)
	_, stderr, code := runRootCmd(t, "design", "author",
		"--data", `{"source":"api","path":"authored/v2.md"}`,
		"--from", from,
		"--spec", "000055_design-authoring-skill")
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	onDisk, err := os.ReadFile(filepath.Join(apiLoc, "authored", "v2.md"))
	require.NoError(t, err)
	require.Equal(t, designAuthoredDoc(
		"created_date: \""+stamped+"\"\ndocument_status: draft\nspec: 000055_design-authoring-skill\n", body,
	), string(onDisk))
}

// Criterion: rewriting an existing authored design keeps its original capture
// date and its existing referencing-spec list while replacing its content. The
// back-links matter most: they are owned by the reference verbs, and a
// revision that dropped them would break every spec pointing at the document.
func TestDesignAuthor_RevisionKeepsCaptureDateAndBackLinks(t *testing.T) {
	_, apiLoc, _ := twoSourceDesignProject(t)

	// A document Spektacular authored months ago, since referenced by two
	// specs. Written directly to disk, so the fixture does not depend on the
	// code under test.
	const seededKeys = "created_date: \"2026-01-05\"\ndocument_status: draft\n" +
		"specs:\n    - 000012_alpha\n    - 000030_beta\n"
	seedDesignDoc(t, apiLoc, "authored/v2.md",
		designAuthoredDoc(seededKeys, "# Payments v2\n\nThe first shape.\n"))

	const revised = "# Payments v2\n\nThe second shape.\n"
	from := stageDesignDoc(t, "v2.md", revised)

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "author",
		"--data", `{"source":"api","path":"authored/v2.md"}`, "--from", from)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	var got designAuthorResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Equal(t, "2026-01-05", got.CreatedDate,
		"a revision must report the original capture date, not today's")

	onDisk, err := os.ReadFile(filepath.Join(apiLoc, "authored", "v2.md"))
	require.NoError(t, err)
	require.Equal(t, designAuthoredDoc(seededKeys, revised), string(onDisk))
}

// Criterion: setting a closed lifecycle status records when it closed, once,
// and returning the document to draft clears that. Each case seeds its own
// document so the three are independent of the order `-shuffle=on` runs them.
func TestDesignAuthor_ClosedStatusIsDatedOnceAndClearedByDraft(t *testing.T) {
	_, apiLoc, _ := twoSourceDesignProject(t)
	stamped := today().Format("2006-01-02")

	const body = "# Payments v2\n\nThe settled request shape.\n"

	// author revises path with content and the given status, and returns the
	// stored bytes.
	author := func(t *testing.T, relPath, status string) string {
		t.Helper()
		from := stageDesignDoc(t, "v2.md", body)
		data := fmt.Sprintf(`{"source":"api","path":%q}`, relPath)

		resetRootCmd(t)
		_, stderr, code := runRootCmd(t, "design", "author", "--data", data, "--from", from, "--document-status", status)
		require.Equal(t, 0, code)
		require.Empty(t, stderr)

		onDisk, err := os.ReadFile(filepath.Join(apiLoc, filepath.FromSlash(relPath)))
		require.NoError(t, err)
		return string(onDisk)
	}

	t.Run("first close is dated today", func(t *testing.T) {
		seedDesignDoc(t, apiLoc, "authored/first-close.md", designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: draft\n", "# Payments v2\n\nDraft.\n"))

		require.Equal(t, designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: final\nclosed_date: \""+stamped+"\"\n", body,
		), author(t, "authored/first-close.md", "final"))
	})

	t.Run("a later closed status keeps the first close date", func(t *testing.T) {
		seedDesignDoc(t, apiLoc, "authored/reclose.md", designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: final\nclosed_date: \"2026-02-01\"\n",
			"# Payments v2\n\nFinal.\n"))

		require.Equal(t, designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: archived\nclosed_date: \"2026-02-01\"\n", body,
		), author(t, "authored/reclose.md", "archived"))
	})

	t.Run("returning to draft clears the close date", func(t *testing.T) {
		seedDesignDoc(t, apiLoc, "authored/reopen.md", designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: final\nclosed_date: \"2026-02-01\"\n",
			"# Payments v2\n\nFinal.\n"))

		require.Equal(t, designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: draft\n", body,
		), author(t, "authored/reopen.md", "draft"))
	})
}

// Criterion: re-authoring content that already carries a block leaves exactly
// one block on disk. This is the `design read > file`, edit, author-again
// loop: the staged content is the stored document, block and all, and the
// prior block must be treated as metadata to discard rather than as body text
// a second block gets stacked on top of.
func TestDesignAuthor_ReAuthoringStampedContentLeavesOneBlock(t *testing.T) {
	_, apiLoc, _ := twoSourceDesignProject(t)
	stamped := today().Format("2006-01-02")

	const body = "# Payments v2\n\nThe settled request shape.\n"
	docPath := filepath.Join(apiLoc, "authored", "v2.md")

	from := stageDesignDoc(t, "v2.md", body)
	resetRootCmd(t)
	_, stderr, code := runRootCmd(t, "design", "author",
		"--data", `{"source":"api","path":"authored/v2.md"}`, "--from", from)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	// Stage the stored document straight back as the source. It is the input
	// to the second run, not the oracle for it — the expectation below is the
	// same hand-written single-block document as every other author test.
	stored, err := os.ReadFile(docPath)
	require.NoError(t, err)
	again := stageDesignDoc(t, "v2-again.md", string(stored))

	resetRootCmd(t)
	_, stderr, code = runRootCmd(t, "design", "author",
		"--data", `{"source":"api","path":"authored/v2.md"}`, "--from", again)
	require.Equal(t, 0, code)
	require.Empty(t, stderr)

	onDisk, err := os.ReadFile(docPath)
	require.NoError(t, err)
	require.Equal(t, designAuthoredDoc(
		"created_date: \""+stamped+"\"\ndocument_status: draft\n", body,
	), string(onDisk), "re-authoring stamped content must leave exactly one frontmatter block")
}

// Criterion: --schema returns the documented input and output shape for every
// subcommand, including that `read` publishes a string output because it emits
// raw bytes, and that `list` and `write` publish their command-line flags.
func TestDesignSchema_PublishesDocumentedShapes(t *testing.T) {
	root := t.TempDir()
	apiLoc := filepath.Join(root, "docs", "design")
	require.NoError(t, os.MkdirAll(apiLoc, 0o755))
	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

	readSchema := func(t *testing.T, sub string) commandSchema {
		t.Helper()
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "design", sub, "--schema")
		require.Equal(t, 0, code)
		require.Empty(t, stderr)
		var schema commandSchema
		require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
		return schema
	}

	t.Run("sources", func(t *testing.T) {
		schema := readSchema(t, "sources")
		require.Nil(t, schema.Input, "sources takes no JSON input")
		require.Empty(t, schema.Flags, "sources takes no command-line options")
		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		require.Contains(t, schema.Output.Properties, "sources")
		items := schema.Output.Properties["sources"]
		require.Equal(t, "array", items.Type)
		require.NotNil(t, items.Items)
		require.Equal(t, "object", items.Items.Type)
		for _, field := range []string{"name", "provider", "location"} {
			require.Contains(t, items.Items.Properties, field)
			require.Equal(t, "string", items.Items.Properties[field].Type)
		}
	})

	t.Run("list", func(t *testing.T) {
		schema := readSchema(t, "list")
		require.Nil(t, schema.Input, "list takes no JSON input")
		require.Contains(t, schema.Flags, "source", "list must publish its --source flag")
		require.Equal(t, "string", schema.Flags["source"].Type)
		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		require.Contains(t, schema.Output.Properties, "documents")
		items := schema.Output.Properties["documents"]
		require.Equal(t, "array", items.Type)
		require.NotNil(t, items.Items)
		require.Equal(t, "object", items.Items.Type)
		// The lifecycle fields an authored design reports are optional on the
		// item, so they are published alongside the two every document carries.
		for _, field := range []string{"source", "path", "created_date", "document_status", "closed_date", "spec"} {
			require.Contains(t, items.Items.Properties, field)
			require.Equal(t, "string", items.Items.Properties[field].Type)
		}
		require.Contains(t, items.Items.Properties, "specs")
		require.Equal(t, "array", items.Items.Properties["specs"].Type)
		require.NotNil(t, items.Items.Properties["specs"].Items)
		require.Equal(t, "string", items.Items.Properties["specs"].Items.Type)
	})

	t.Run("read", func(t *testing.T) {
		schema := readSchema(t, "read")
		require.NotNil(t, schema.Input)
		require.Equal(t, "object", schema.Input.Type)
		require.Equal(t, []string{"source", "path"}, schema.Input.Required)
		require.Equal(t, "string", schema.Input.Properties["source"].Type)
		require.Equal(t, "string", schema.Input.Properties["path"].Type)
		require.Empty(t, schema.Flags, "read takes no command-line options beyond --data")
		require.NotNil(t, schema.Output)
		require.Equal(t, "string", schema.Output.Type, "read emits the document's raw bytes, not an object")
		require.Empty(t, schema.Output.Properties)
	})

	t.Run("write", func(t *testing.T) {
		schema := readSchema(t, "write")
		require.NotNil(t, schema.Input)
		require.Equal(t, "object", schema.Input.Type)
		require.Equal(t, []string{"source", "path"}, schema.Input.Required)
		require.Equal(t, "string", schema.Input.Properties["source"].Type)
		require.Equal(t, "string", schema.Input.Properties["path"].Type)
		require.Contains(t, schema.Flags, "from", "write must publish its --from flag")
		require.Equal(t, "string", schema.Flags["from"].Type)
		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		for _, field := range []string{"source", "path", "location"} {
			require.Contains(t, schema.Output.Properties, field)
			require.Equal(t, "string", schema.Output.Properties[field].Type)
		}
	})

	// Criterion: the author command publishes its own input and output shapes
	// on request, as its sibling commands do — including the two lifecycle
	// fields its envelope adds over `write`, and the two flags that drive
	// them.
	t.Run("author", func(t *testing.T) {
		schema := readSchema(t, "author")
		require.NotNil(t, schema.Input)
		require.Equal(t, "object", schema.Input.Type)
		require.Equal(t, []string{"source", "path"}, schema.Input.Required)
		require.Equal(t, "string", schema.Input.Properties["source"].Type)
		require.Equal(t, "string", schema.Input.Properties["path"].Type)
		for _, flag := range []string{"from", "document-status", "spec"} {
			require.Contains(t, schema.Flags, flag, "author must publish its --%s flag", flag)
			require.Equal(t, "string", schema.Flags[flag].Type)
		}
		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		for _, field := range []string{"source", "path", "location", "document_status", "created_date"} {
			require.Contains(t, schema.Output.Properties, field)
			require.Equal(t, "string", schema.Output.Properties[field].Type)
		}
	})
}

// Criterion: every refusal surfaces as the documented failure code with a
// non-empty next action naming the runnable next step.
func TestDesignRefusals_CarryCodeAndNextAction(t *testing.T) {
	refuse := func(t *testing.T, args ...string) output.ErrorResponse {
		t.Helper()
		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, args...)
		require.Equal(t, 1, code)
		require.Empty(t, stderr)
		var er output.ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &er))
		require.True(t, er.IsError)
		require.NotEmpty(t, er.NextAction, "every refusal must name a runnable next step")
		return er
	}

	t.Run("read without --data", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

		er := refuse(t, "design", "read")
		require.Equal(t, "design_data_required", er.Code)
		require.Contains(t, er.NextAction, "--data")
	})

	t.Run("write without --from", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

		er := refuse(t, "design", "write", "--data", `{"source":"api","path":"payments/v2.md"}`)
		require.Equal(t, "design_from_required", er.Code)
		require.Contains(t, er.NextAction, "--from")
		// A refused write must not have created the addressed document.
		require.NoFileExists(t, filepath.Join(apiLoc, "payments", "v2.md"))
	})

	t.Run("undeclared source name", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root,
			designSourceDecl{name: "api", location: "../docs/design"},
			designSourceDecl{name: "ux", location: "../docs/design"},
		)

		er := refuse(t, "design", "read", "--data", `{"source":"marketing","path":"overview.md"}`)
		require.Equal(t, "design_source_unknown", er.Code)
		require.Equal(t, "marketing", er.Resource)
		require.Contains(t, er.NextAction, `"api"`, "next action must name the declared sources")
		require.Contains(t, er.NextAction, `"ux"`, "next action must name the declared sources")
	})

	t.Run("missing document", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		seedDesignDoc(t, apiLoc, "overview.md", "# Overview\n")
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

		er := refuse(t, "design", "read", "--data", `{"source":"api","path":"payments/v2.md"}`)
		require.Equal(t, "design_not_found", er.Code)
		searched := filepath.Join(root, "docs", "design", "payments", "v2.md")
		require.Contains(t, er.Message, searched, "the refusal must name the absolute location searched")
		require.Equal(t, searched, er.Resource)
		require.Contains(t, er.NextAction, "design list --source api")
	})

	t.Run("declared location that does not exist", func(t *testing.T) {
		root := t.TempDir()
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

		er := refuse(t, "design", "sources")
		require.Equal(t, "design_source_unreachable", er.Code)
		resolved := filepath.Join(root, "docs", "design")
		require.Contains(t, er.Message, resolved)
		require.Equal(t, resolved, er.Resource)
		require.Contains(t, er.NextAction, resolved)
		require.Contains(t, er.NextAction, filepath.Join(root, ".spektacular"),
			"a relative location's refusal must name the base it resolved from")
	})

	// Criterion: omitting the content file is refused with an explanation of
	// how to supply one. The author verb reads its content from a file for the
	// same reason `write` does, so it refuses identically.
	t.Run("author without --from", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})

		er := refuse(t, "design", "author", "--data", `{"source":"api","path":"payments/v2.md"}`)
		require.Equal(t, "design_from_required", er.Code)
		require.Contains(t, er.NextAction, "--from")
		require.NoFileExists(t, filepath.Join(apiLoc, "payments", "v2.md"))
	})

	// Criterion: writing to a source the project has not declared is refused,
	// and the refusal names the sources it could have used.
	t.Run("author to an undeclared source", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root,
			designSourceDecl{name: "api", location: "../docs/design"},
			designSourceDecl{name: "ux", location: "../docs/design"},
		)
		from := stageDesignDoc(t, "v2.md", "# Payments v2\n")

		er := refuse(t, "design", "author", "--data", `{"source":"marketing","path":"overview.md"}`, "--from", from)
		require.Equal(t, "design_source_unknown", er.Code)
		require.Equal(t, "marketing", er.Resource)
		require.Contains(t, er.NextAction, `"api"`, "next action must name the declared sources")
		require.Contains(t, er.NextAction, `"ux"`, "next action must name the declared sources")
	})

	// Criterion: a status outside the project's four allowed values is
	// refused, and the refusal lists the values that are allowed. The author
	// verb routes --document-status through the same validation the store-file
	// writes use, so it produces the project's existing refusal verbatim.
	t.Run("author with an invalid document status", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		require.NoError(t, os.MkdirAll(apiLoc, 0o755))
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})
		from := stageDesignDoc(t, "v2.md", "# Payments v2\n")

		resetRootCmd(t)
		stdout, stderr, code := runRootCmd(t, "design", "author",
			"--data", `{"source":"api","path":"payments/v2.md"}`, "--from", from,
			"--document-status", "bogus")
		requireInvalidDocumentStatus(t, "bogus", stdout, stderr, code)
		require.NoFileExists(t, filepath.Join(apiLoc, "payments", "v2.md"),
			"a rejected --document-status must not create the destination document")
	})

	// A document already carrying frontmatter the team wrote is theirs, and a
	// lifecycle block cannot be merged into it. The refusal must say so and
	// leave the document exactly as it found it.
	t.Run("author over frontmatter the team wrote", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		const teamDoc = "---\ntitle: Payments v2\nauthor: the payments team\n---\n\n# Payments v2\n\nTheirs.\n"
		seedDesignDoc(t, apiLoc, "payments/v2.md", teamDoc)
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})
		from := stageDesignDoc(t, "v2.md", "# Payments v2\n\nRewritten.\n")

		er := refuse(t, "design", "author", "--data", `{"source":"api","path":"payments/v2.md"}`, "--from", from)
		require.Equal(t, "design_frontmatter_not_authored", er.Code)
		resolved := filepath.Join(root, "docs", "design", "payments", "v2.md")
		require.Equal(t, resolved, er.Resource)
		require.Contains(t, er.Message, resolved, "the refusal must name the document it could not stamp")

		onDisk, err := os.ReadFile(resolved)
		require.NoError(t, err)
		require.Equal(t, teamDoc, string(onDisk),
			"a refused author must leave the team's document byte for byte")
	})

	// Criterion: a verbatim write over a design Spektacular authored is
	// refused, the refusal names the command that performs an authored write
	// instead, and nothing on disk changes. The two kinds of document share a
	// folder, so `design write` is the one command that could silently destroy
	// a lifecycle record and every back-link in it.
	t.Run("write over an authored design", func(t *testing.T) {
		root := t.TempDir()
		apiLoc := filepath.Join(root, "docs", "design")
		authored := designAuthoredDoc(
			"created_date: \"2026-01-05\"\ndocument_status: draft\nspecs:\n    - 000012_alpha\n",
			"# Payments v2\n\nOurs.\n")
		seedDesignDoc(t, apiLoc, "authored/v2.md", authored)
		t.Chdir(root)
		writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})
		from := stageDesignDoc(t, "v2.md", "# Payments v2\n\nClobbered.\n")

		er := refuse(t, "design", "write", "--data", `{"source":"api","path":"authored/v2.md"}`, "--from", from)
		require.Equal(t, "design_authored_overwrite", er.Code)
		resolved := filepath.Join(root, "docs", "design", "authored", "v2.md")
		require.Equal(t, resolved, er.Resource)
		require.Contains(t, er.Message, resolved, "the refusal must name the document it would not clobber")
		require.Contains(t, er.NextAction, "design author",
			"the refusal must name the command that performs an authored write instead")

		// The whole source directory is still the one file it was seeded with,
		// byte for byte: nothing written, nothing truncated, nothing added.
		require.Equal(t,
			map[string]string{"authored/v2.md": fmt.Sprintf("%x", sha256.Sum256([]byte(authored)))},
			snapshotDir(t, apiLoc),
			"a refused verbatim write must leave the source directory untouched")
	})
}
