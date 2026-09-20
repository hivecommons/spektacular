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

// designWriteResult mirrors the `design write` JSON envelope: the address the
// request named, plus the absolute location the bytes landed at.
type designWriteResult struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Location string `json:"location"`
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
		for _, field := range []string{"source", "path"} {
			require.Contains(t, items.Items.Properties, field)
			require.Equal(t, "string", items.Items.Properties[field].Type)
		}
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
}
