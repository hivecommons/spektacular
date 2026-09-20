package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/design"
	"github.com/jumppad-labs/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// This file tests the `design ref` command group in cmd/design_ref.go: the
// references a spec carries to the project's design documents. The contract
// these tests hold the commands to is that a reference is a record in the
// spec's frontmatter and nothing more — the spec's prose never acquires any of
// the design's content, the design document is never touched by recording or
// dropping a reference, and `ref list` is the one place a reference that no
// longer resolves is reported.
//
// Every command is driven through resetRootCmd + runRootCmd, never by calling
// a RunE directly and never by setting a flag variable by hand, because the
// cobra commands are package-level globals whose flag values would otherwise
// leak into whichever test `-shuffle=on` runs next.

// designRefItem mirrors one entry of the `designs` list `ref add` and
// `ref remove` return.
type designRefItem struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// designRefWriteResult mirrors the `ref add` / `ref remove` JSON envelope: the
// spec the request named, plus the reference list as it stands after the write.
type designRefWriteResult struct {
	Spec    string          `json:"spec"`
	Designs []designRefItem `json:"designs"`
}

// designRefListItem mirrors one entry of the `ref list` JSON envelope: the
// address as recorded, whether a document is actually there, and the absolute
// location that was searched.
type designRefListItem struct {
	Source   string `json:"source"`
	Path     string `json:"path"`
	Resolved bool   `json:"resolved"`
	Location string `json:"location"`
}

// designRefListResult mirrors the `ref list` JSON envelope.
type designRefListResult struct {
	Spec       string              `json:"spec"`
	Refs       []designRefListItem `json:"refs"`
	Unresolved int                 `json:"unresolved"`
	NextAction string              `json:"next_action"`
}

// The two design documents every fixture project in this file declares. The
// overview carries a sentinel line found nowhere else, so a test can assert
// that none of a design document's content leaked into a spec.
const (
	designRefOverviewDoc = "# Overview\n\nDESIGN-DOC-SENTINEL: this line belongs to the design document alone.\n"
	designRefPaymentsDoc = "# Payments v2\n\nThe settled request shape.\n"
)

// designRefSpecFixture is a stored spec carrying no design references: a valid
// frontmatter block (created_date is required, and must be YYYY-MM-DD) and its
// own prose. The two halves are kept separate so a test can name the body on
// its own as the expected bytes after a write.
const (
	designRefSpecFrontmatter = "---\ncreated_date: 2026-07-01\ndocument_status: draft\n---\n\n"
	designRefSpecBody        = "# Billing\n\n## Overview\n\nThe spec's own prose, and nothing else.\n"
	designRefSpecFixture     = designRefSpecFrontmatter + designRefSpecBody
)

// specFixtureCarrying returns a stored spec whose frontmatter already records
// the hand-written `designs:` fragment refs, for the cases that must start from
// a spec with references rather than build one up through `ref add`.
func specFixtureCarrying(refs string) string {
	return "---\ncreated_date: 2026-07-01\ndocument_status: draft\n" + refs + "---\n\n" + designRefSpecBody
}

// The lifecycle block and body of a design Spektacular authored. Only such a
// document carries back-links, so the tests that exercise them seed a document
// of this shape and assemble the expected bytes from the same hand-written
// strings — never by running `design author`, which is the code under test's
// neighbour.
const (
	designRefAuthoredKeys = "created_date: \"2026-01-05\"\ndocument_status: draft\n"
	designRefAuthoredBody = "# Payments v2\n\nThe settled request shape.\n"
)

// seedAuthoredDesign writes an authored design at relPath within a design
// source, whose lifecycle block is designRefAuthoredKeys followed by the
// hand-written `specs:` fragment backLinks (empty for a document nothing
// references yet). It returns the document's full path.
func seedAuthoredDesign(t *testing.T, loc, relPath, backLinks string) string {
	t.Helper()
	return seedDesignDoc(t, loc, relPath,
		designAuthoredDoc(designRefAuthoredKeys+backLinks, designRefAuthoredBody))
}

// requireAuthoredDesign asserts the stored bytes at path are the seeded
// lifecycle block plus backLinks and nothing else: the back-link list is the
// only thing a reference write may change about a design document.
func requireAuthoredDesign(t *testing.T, path, backLinks string) {
	t.Helper()
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t,
		designAuthoredDoc(designRefAuthoredKeys+backLinks, designRefAuthoredBody),
		string(onDisk))
}

// specsListedBy scans the `specs:` list out of a stored design document, and
// designsReferencedBy scans the `designs:` list out of a stored spec. Both
// scan the bytes by hand rather than calling metadata.Split, so the
// consistency check that compares the two sides has an oracle the code under
// test cannot influence.
func specsListedBy(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	out := []string{}
	inList := false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case line == "specs:":
			inList = true
		case inList && strings.HasPrefix(line, "    - "):
			out = append(out, strings.TrimPrefix(line, "    - "))
		case inList:
			inList = false
		}
	}
	return out
}

func designsReferencedBy(t *testing.T, path string) []designRefItem {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	out := []designRefItem{}
	inList := false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case line == "designs:":
			inList = true
		case inList && strings.HasPrefix(line, "    - source: "):
			out = append(out, designRefItem{Source: strings.TrimPrefix(line, "    - source: ")})
		case inList && strings.HasPrefix(line, "      path: "):
			require.NotEmpty(t, out, "a path line must follow a source line")
			out[len(out)-1].Path = strings.TrimPrefix(line, "      path: ")
		case inList:
			inList = false
		}
	}
	return out
}

// designRefProject lays out a project rooted at a t.TempDir() and chdirs into
// it, declaring a single design source, "api", at a location relative to the
// folder holding config.yaml and holding two documents. No spec is seeded: each
// test writes the specs it needs with writeSpecFixture. It returns the project
// root and the resolved source location.
func designRefProject(t *testing.T) (root, apiLoc string) {
	t.Helper()
	root = t.TempDir()
	apiLoc = filepath.Join(root, "docs", "design")

	seedDesignDoc(t, apiLoc, "overview.md", designRefOverviewDoc)
	seedDesignDoc(t, apiLoc, "payments/v2.md", designRefPaymentsDoc)

	t.Chdir(root)
	writeDesignConfig(t, root, designSourceDecl{name: "api", location: "../docs/design"})
	return root, apiLoc
}

// writeSpecFixture stores content as name.md in the project's configured spec
// directory (the default, .spektacular/specs) and returns its path. It writes
// with os.WriteFile rather than through `spec file write`, so the fixture is
// independent of the code under test.
func writeSpecFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, ".spektacular", "specs", name+".md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// runDesignRef drives one `design ref` invocation through the root command,
// asserting only that nothing reached stderr.
func runDesignRef(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	resetRootCmd(t)
	out, errOut, code := runRootCmd(t, append([]string{"design", "ref"}, args...)...)
	require.Empty(t, errOut)
	return out, code
}

// designRefWrite runs a `ref add` or `ref remove` expected to succeed, and
// decodes its envelope.
func designRefWrite(t *testing.T, args ...string) designRefWriteResult {
	t.Helper()
	stdout, code := runDesignRef(t, args...)
	require.Equal(t, 0, code)
	var got designRefWriteResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	return got
}

// designRefListRaw runs `ref list` for one spec, asserting the success
// envelope every listing must produce, and returns its raw stdout so a caller
// can inspect which keys are present as well as their values.
func designRefListRaw(t *testing.T, spec string) string {
	t.Helper()
	stdout, code := runDesignRef(t, "list", "--data", `{"spec":"`+spec+`"}`)
	require.Equal(t, 0, code, "ref list always reports, even when nothing resolves")

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
	require.Equal(t, "false", string(envelope["error"]), "ref list always returns a success envelope")
	return stdout
}

// designRefList runs `ref list` for one spec and decodes its envelope.
func designRefList(t *testing.T, spec string) designRefListResult {
	t.Helper()
	var got designRefListResult
	require.NoError(t, json.Unmarshal([]byte(designRefListRaw(t, spec)), &got))
	return got
}

// refuseDesignRef runs a `design ref` invocation expected to be refused, and
// returns the failure envelope.
func refuseDesignRef(t *testing.T, args ...string) output.ErrorResponse {
	t.Helper()
	stdout, code := runDesignRef(t, args...)
	require.Equal(t, 1, code)
	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.True(t, er.IsError)
	return er
}

// Criterion: `design ref add` records the reference on the spec, and reading
// the stored spec back shows it in the YAML frontmatter — the spec's record,
// not its prose.
func TestDesignRefAdd_RecordsTheReferenceInTheSpecsFrontmatter(t *testing.T) {
	root, _ := designRefProject(t)
	specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	got := designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v2.md"}`)
	require.Equal(t, designRefWriteResult{
		Spec:    "000054_billing",
		Designs: []designRefItem{{Source: "api", Path: "payments/v2.md"}},
	}, got)

	stored, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Contains(t, string(stored),
		"designs:\n    - source: api\n      path: payments/v2.md\n",
		"the reference must be recorded in the spec's frontmatter block")
}

// Criterion: recording a reference puts none of the design document's content
// into the spec — the body comes through the write byte for byte.
func TestDesignRefAdd_LeavesTheSpecBodyUntouched(t *testing.T) {
	root, _ := designRefProject(t)
	specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"overview.md"}`)

	stored, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(stored), "---\n\n"+designRefSpecBody),
		"the spec's body must be the fixture body, unchanged, after the frontmatter block")
	require.NotContains(t, string(stored), "DESIGN-DOC-SENTINEL",
		"no part of the design document's content may be copied into the spec")
}

// Criterion: `ref add` validates the source against the project's declared
// design sources before it touches the spec, so an undeclared source is
// refused and the stored spec is left byte-identical.
func TestDesignRefAdd_UndeclaredSourceIsRefusedAndWritesNothing(t *testing.T) {
	root, _ := designRefProject(t)
	specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	er := refuseDesignRef(t, "add", "--data", `{"spec":"000054_billing","source":"marketing","path":"overview.md"}`)
	require.Equal(t, "design_source_unknown", er.Code)
	require.Equal(t, "marketing", er.Resource)
	require.NotEmpty(t, er.NextAction)

	stored, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Equal(t, designRefSpecFixture, string(stored),
		"a refused add must leave the stored spec byte-identical")
}

// Criterion: `ref add` of a pair the spec already carries is a silent no-op
// that still succeeds and returns the unchanged list.
func TestDesignRefAdd_DuplicateIsASilentNoOp(t *testing.T) {
	root, _ := designRefProject(t)
	specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	const data = `{"spec":"000054_billing","source":"api","path":"payments/v2.md"}`
	first := designRefWrite(t, "add", "--data", data)
	afterFirst, err := os.ReadFile(specPath)
	require.NoError(t, err)

	second := designRefWrite(t, "add", "--data", data)
	require.Equal(t, first, second, "a duplicate add must return the unchanged list")
	require.Equal(t, []designRefItem{{Source: "api", Path: "payments/v2.md"}}, second.Designs)

	afterSecond, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Equal(t, string(afterFirst), string(afterSecond),
		"a duplicate add must not rewrite the stored spec")
}

// Criterion: `ref remove` drops only the named reference and leaves its
// siblings in place; removing a reference the spec does not carry succeeds and
// changes nothing.
func TestDesignRefRemove_KeepsSiblingsAndToleratesAnAbsentReference(t *testing.T) {
	root, _ := designRefProject(t)
	specPath := writeSpecFixture(t, root, "000054_billing", specFixtureCarrying(
		"designs:\n"+
			"  - source: api\n"+
			"    path: overview.md\n"+
			"  - source: api\n"+
			"    path: payments/v2.md\n"))

	got := designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"overview.md"}`)
	require.Equal(t, []designRefItem{{Source: "api", Path: "payments/v2.md"}}, got.Designs,
		"the sibling reference must survive the removal")

	stored, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Contains(t, string(stored), "designs:\n    - source: api\n      path: payments/v2.md\n")
	require.NotContains(t, string(stored), "overview.md")

	again := designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"overview.md"}`)
	require.Equal(t, got, again, "removing an absent reference must report the unchanged list")

	afterSecond, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.Equal(t, string(stored), string(afterSecond),
		"removing an absent reference must not rewrite the stored spec")
}

// Criterion: `ref list` reports every reference the spec carries with whether
// it resolves and the absolute location searched, counts the unresolved ones,
// and carries a next action only when at least one does not resolve.
func TestDesignRefList_ReportsResolutionCountAndNextAction(t *testing.T) {
	t.Run("every reference resolves", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", specFixtureCarrying(
			"designs:\n"+
				"  - source: api\n"+
				"    path: overview.md\n"+
				"  - source: api\n"+
				"    path: payments/v2.md\n"))

		stdout := designRefListRaw(t, "000054_billing")
		var got designRefListResult
		require.NoError(t, json.Unmarshal([]byte(stdout), &got))
		require.Equal(t, designRefListResult{
			Spec: "000054_billing",
			Refs: []designRefListItem{
				{Source: "api", Path: "overview.md", Resolved: true, Location: filepath.Join(apiLoc, "overview.md")},
				{Source: "api", Path: "payments/v2.md", Resolved: true, Location: filepath.Join(apiLoc, "payments", "v2.md")},
			},
			Unresolved: 0,
		}, got)

		var envelope map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(stdout), &envelope))
		require.NotContains(t, envelope, "next_action",
			"a listing with nothing unresolved must omit next_action entirely")
	})

	t.Run("a reference whose document is missing", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", specFixtureCarrying(
			"designs:\n"+
				"  - source: api\n"+
				"    path: overview.md\n"+
				"  - source: api\n"+
				"    path: payments/v3.md\n"))

		got := designRefList(t, "000054_billing")
		require.Equal(t, []designRefListItem{
			{Source: "api", Path: "overview.md", Resolved: true, Location: filepath.Join(apiLoc, "overview.md")},
			{Source: "api", Path: "payments/v3.md", Resolved: false, Location: filepath.Join(apiLoc, "payments", "v3.md")},
		}, got.Refs)
		require.Equal(t, 1, got.Unresolved)
		require.NotEmpty(t, got.NextAction, "an unresolved reference must name a runnable next step")
	})

	t.Run("a reference whose source is no longer declared", func(t *testing.T) {
		root, _ := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", specFixtureCarrying(
			"designs:\n"+
				"  - source: legacy\n"+
				"    path: retired.md\n"))

		got := designRefList(t, "000054_billing")
		require.Equal(t, []designRefListItem{
			{Source: "legacy", Path: "retired.md", Resolved: false, Location: ""},
		}, got.Refs, "a reference the project can no longer address is still listed, with no location")
		require.Equal(t, 1, got.Unresolved)
		require.NotEmpty(t, got.NextAction)
	})
}

// Criterion: `design read` of a document that has been removed from disk fails
// with design_not_found, even though a spec still records a reference to it.
func TestDesignRead_RemovedDocumentIsNotFound(t *testing.T) {
	root, apiLoc := designRefProject(t)
	writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v2.md"}`)
	require.NoError(t, os.Remove(filepath.Join(apiLoc, "payments", "v2.md")))

	resetRootCmd(t)
	stdout, stderr, code := runRootCmd(t, "design", "read", "--data", `{"source":"api","path":"payments/v2.md"}`)
	require.Equal(t, 1, code)
	require.Empty(t, stderr)

	var er output.ErrorResponse
	require.NoError(t, json.Unmarshal([]byte(stdout), &er))
	require.Equal(t, "design_not_found", er.Code)
	require.Equal(t, filepath.Join(apiLoc, "payments", "v2.md"), er.Resource)
	require.NotEmpty(t, er.NextAction)
}

// Criterion: two specs can carry the same reference, each reported
// independently. A design Spektacular did not author is unchanged by either
// operation; one it did author records both specs and keeps the survivor when
// the other drops its reference.
func TestDesignRef_TwoSpecsShareOneDesignIndependently(t *testing.T) {
	t.Run("a design Spektacular did not author", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)
		writeSpecFixture(t, root, "000055_invoicing", designRefSpecFixture)

		// snapshotDir (init_test.go) walks the directory with filepath.WalkDir
		// and os.ReadFile, hashing each file's bytes — an oracle that never
		// consults the code under test.
		before := snapshotDir(t, apiLoc)

		designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v2.md"}`)
		designRefWrite(t, "add", "--data", `{"spec":"000055_invoicing","source":"api","path":"payments/v2.md"}`)

		shared := []designRefListItem{
			{Source: "api", Path: "payments/v2.md", Resolved: true, Location: filepath.Join(apiLoc, "payments", "v2.md")},
		}
		billing := designRefList(t, "000054_billing")
		require.Equal(t, "000054_billing", billing.Spec)
		require.Equal(t, shared, billing.Refs)

		invoicing := designRefList(t, "000055_invoicing")
		require.Equal(t, "000055_invoicing", invoicing.Spec)
		require.Equal(t, shared, invoicing.Refs)

		// Dropping one spec's reference leaves the other's standing.
		dropped := designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v2.md"}`)
		require.Empty(t, dropped.Designs)
		require.Equal(t, shared, designRefList(t, "000055_invoicing").Refs)

		require.Equal(t, before, snapshotDir(t, apiLoc),
			"recording or dropping a reference must not touch the design source at all")
	})

	t.Run("a design Spektacular authored", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md", "")
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)
		writeSpecFixture(t, root, "000055_invoicing", designRefSpecFixture)

		designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
		designRefWrite(t, "add", "--data", `{"spec":"000055_invoicing","source":"api","path":"authored/v2.md"}`)
		requireAuthoredDesign(t, docPath, "specs:\n    - 000054_billing\n    - 000055_invoicing\n")

		// Dropping one spec's reference leaves the other spec's back-link
		// standing on the same document.
		designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
		requireAuthoredDesign(t, docPath, "specs:\n    - 000055_invoicing\n")
	})
}

// Criterion: a reference whose document does not exist yet records
// successfully — only the source is validated at record time — and `ref list`
// is what reports it as unresolved.
func TestDesignRefAdd_RecordsAReferenceToADocumentThatDoesNotExistYet(t *testing.T) {
	root, apiLoc := designRefProject(t)
	writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	got := designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v3.md"}`)
	require.Equal(t, []designRefItem{{Source: "api", Path: "payments/v3.md"}}, got.Designs)
	require.NoFileExists(t, filepath.Join(apiLoc, "payments", "v3.md"),
		"recording a reference must not create the document")

	listed := designRefList(t, "000054_billing")
	require.Equal(t, []designRefListItem{
		{Source: "api", Path: "payments/v3.md", Resolved: false, Location: filepath.Join(apiLoc, "payments", "v3.md")},
	}, listed.Refs)
	require.Equal(t, 1, listed.Unresolved)
	require.NotEmpty(t, listed.NextAction)

	// There is no document to carry a back-link either way, so dropping the
	// reference again succeeds and still conjures nothing into the source.
	dropped := designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"payments/v3.md"}`)
	require.Empty(t, dropped.Designs)
	require.NoFileExists(t, filepath.Join(apiLoc, "payments", "v3.md"),
		"dropping a reference must not create the document")
}

// Criterion: every refusal surfaces as the documented failure code, exit code
// 1, and a non-empty next action naming the runnable next step.
func TestDesignRefRefusals_CarryCodeAndNextAction(t *testing.T) {
	t.Run("no --data at all", func(t *testing.T) {
		root, _ := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

		er := refuseDesignRef(t, "add")
		require.Equal(t, "design_data_required", er.Code)
		require.Contains(t, er.NextAction, "--data")
	})

	t.Run("no spec in --data", func(t *testing.T) {
		root, _ := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

		er := refuseDesignRef(t, "add", "--data", `{"source":"api","path":"overview.md"}`)
		require.Equal(t, "design_ref_spec_required", er.Code)
		require.Contains(t, er.NextAction, "spec file list")
	})

	t.Run("an incomplete document address", func(t *testing.T) {
		root, _ := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

		er := refuseDesignRef(t, "add", "--data", `{"spec":"000054_billing","source":"api"}`)
		require.Equal(t, "design_address_incomplete", er.Code)
		require.Contains(t, er.NextAction, "design sources")
	})

	t.Run("an unknown spec name", func(t *testing.T) {
		root, _ := designRefProject(t)
		writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

		er := refuseDesignRef(t, "add", "--data", `{"spec":"000099_nosuch","source":"api","path":"overview.md"}`)
		require.Equal(t, "design_ref_spec_not_found", er.Code)
		require.Equal(t, filepath.Join(".spektacular", "specs", "000099_nosuch.md"), er.Resource)
		require.Contains(t, er.NextAction, "spec file list")
	})
}

// Criterion: --schema returns the documented input and output shape for each
// of the three reference subcommands.
func TestDesignRefSchema_PublishesDocumentedShapes(t *testing.T) {
	designRefProject(t)

	readSchema := func(t *testing.T, sub string) commandSchema {
		t.Helper()
		stdout, code := runDesignRef(t, sub, "--schema")
		require.Equal(t, 0, code)
		var schema commandSchema
		require.NoError(t, json.Unmarshal([]byte(stdout), &schema))
		return schema
	}

	assertWriteSchema := func(t *testing.T, schema commandSchema) {
		t.Helper()
		require.NotNil(t, schema.Input)
		require.Equal(t, "object", schema.Input.Type)
		require.Equal(t, []string{"spec", "source", "path"}, schema.Input.Required)
		for _, field := range []string{"spec", "source", "path"} {
			require.Equal(t, "string", schema.Input.Properties[field].Type)
		}
		require.Empty(t, schema.Flags, "the reference verbs take no command-line options beyond --data")

		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		require.Equal(t, "string", schema.Output.Properties["spec"].Type)
		designs := schema.Output.Properties["designs"]
		require.Equal(t, "array", designs.Type)
		require.NotNil(t, designs.Items)
		require.Equal(t, "object", designs.Items.Type)
		for _, field := range []string{"source", "path"} {
			require.Equal(t, "string", designs.Items.Properties[field].Type)
		}
	}

	t.Run("add", func(t *testing.T) { assertWriteSchema(t, readSchema(t, "add")) })
	t.Run("remove", func(t *testing.T) { assertWriteSchema(t, readSchema(t, "remove")) })

	t.Run("list", func(t *testing.T) {
		schema := readSchema(t, "list")
		require.NotNil(t, schema.Input)
		require.Equal(t, "object", schema.Input.Type)
		require.Equal(t, []string{"spec"}, schema.Input.Required)
		require.Equal(t, "string", schema.Input.Properties["spec"].Type)
		require.NotContains(t, schema.Input.Properties, "source", "list addresses only a spec")
		require.Empty(t, schema.Flags)

		require.NotNil(t, schema.Output)
		require.Equal(t, "object", schema.Output.Type)
		require.Equal(t, "string", schema.Output.Properties["spec"].Type)
		require.Equal(t, "integer", schema.Output.Properties["unresolved"].Type)
		require.Equal(t, "string", schema.Output.Properties["next_action"].Type)
		refs := schema.Output.Properties["refs"]
		require.Equal(t, "array", refs.Type)
		require.NotNil(t, refs.Items)
		require.Equal(t, "object", refs.Items.Type)
		for field, kind := range map[string]string{
			"source":   "string",
			"path":     "string",
			"resolved": "boolean",
			"location": "string",
		} {
			require.Equal(t, kind, refs.Items.Properties[field].Type, "refs item field %q", field)
		}
	})
}

// Criterion: a bare spec name and the same name with a .md suffix address the
// same stored spec, and both spellings leave the referenced design carrying
// one back-link, written in the bare form `spec file list` reports.
func TestDesignRef_BareAndSuffixedSpecNamesAddressTheSameSpec(t *testing.T) {
	root, apiLoc := designRefProject(t)
	docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md", "")
	specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	designRefWrite(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
	designRefWrite(t, "add", "--data", `{"spec":"000054_billing.md","source":"api","path":"authored/v2.md"}`)
	requireAuthoredDesign(t, docPath, "specs:\n    - 000054_billing\n")

	// Listing under the suffixed name reports the reference recorded under the
	// bare one.
	listed := designRefList(t, "000054_billing.md")
	require.Equal(t, "000054_billing.md", listed.Spec, "the envelope echoes the name as given")
	require.Equal(t, []designRefListItem{
		{Source: "api", Path: "authored/v2.md", Resolved: true, Location: filepath.Join(apiLoc, "authored", "v2.md")},
	}, listed.Refs)

	// And removing under the suffixed name clears the reference from the one
	// stored file, and the back-link stored under the bare one with it.
	dropped := designRefWrite(t, "remove", "--data", `{"spec":"000054_billing.md","source":"api","path":"authored/v2.md"}`)
	require.Empty(t, dropped.Designs)

	stored, err := os.ReadFile(specPath)
	require.NoError(t, err)
	require.NotContains(t, string(stored), "designs")
	requireAuthoredDesign(t, docPath, "")
}

// Criterion: recording a reference adds the spec to the authored design's list
// of referencing specs, and recording the same reference a second time changes
// nothing — the design's list carries the spec exactly once.
func TestDesignRefAdd_RecordsTheSpecOnTheAuthoredDesignOnce(t *testing.T) {
	root, apiLoc := designRefProject(t)
	docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md", "")
	writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

	const data = `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`
	designRefWrite(t, "add", "--data", data)
	requireAuthoredDesign(t, docPath, "specs:\n    - 000054_billing\n")

	// The second add is a no-op on both documents, not a second entry.
	afterFirst := snapshotDir(t, apiLoc)
	designRefWrite(t, "add", "--data", data)
	require.Equal(t, afterFirst, snapshotDir(t, apiLoc),
		"a duplicate add must not rewrite the design document")
}

// Criterion: removing a reference removes that spec from the design's list of
// referencing specs and leaves every other spec on it intact.
func TestDesignRefRemove_DropsOnlyThatSpecFromTheDesignsBackLinks(t *testing.T) {
	root, apiLoc := designRefProject(t)
	docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md",
		"specs:\n    - 000012_alpha\n    - 000054_billing\n    - 000030_beta\n")
	writeSpecFixture(t, root, "000054_billing", specFixtureCarrying(
		"designs:\n"+
			"  - source: api\n"+
			"    path: authored/v2.md\n"))

	designRefWrite(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
	requireAuthoredDesign(t, docPath, "specs:\n    - 000012_alpha\n    - 000030_beta\n")
}

// Criterion: recording or removing a reference to a design that carries no
// lifecycle record succeeds and leaves that document byte for byte. The two
// shapes are distinct: a document with no frontmatter at all parses as "not
// ours", and one carrying the team's own YAML header fails to parse as a
// lifecycle block and must be read the same way rather than rewritten.
func TestDesignRef_ADesignWithNoLifecycleRecordIsLeftUntouched(t *testing.T) {
	for _, tc := range []struct {
		name    string
		relPath string
		doc     string
	}{
		{
			name:    "no frontmatter at all",
			relPath: "plain.md",
			doc:     "# Plain\n\nNo frontmatter anywhere.\n",
		},
		{
			name:    "the team's own frontmatter",
			relPath: "team.md",
			doc:     "---\ntitle: Payments v2\nauthor: the payments team\n---\n\n# Team\n\nTheirs.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, apiLoc := designRefProject(t)
			seedDesignDoc(t, apiLoc, tc.relPath, tc.doc)
			writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

			before := snapshotDir(t, apiLoc)
			data := `{"spec":"000054_billing","source":"api","path":"` + tc.relPath + `"}`

			designRefWrite(t, "add", "--data", data)
			require.Equal(t, before, snapshotDir(t, apiLoc),
				"recording a reference must leave a design with no lifecycle record byte for byte")

			designRefWrite(t, "remove", "--data", data)
			require.Equal(t, before, snapshotDir(t, apiLoc),
				"removing a reference must leave a design with no lifecycle record byte for byte")
		})
	}
}

// Criterion: an authored design never lists a spec that does not reference it,
// and never omits one that does. This is the invariant the whole back-link
// feature exists to hold, so it is asserted over a sequence of adds and
// removes across two specs and two designs rather than over a single write.
//
// The exact lists are pinned first, by hand: the cross-check below compares
// the two sides of each pair against each other, so it would hold vacuously if
// every list came out empty.
func TestDesignRef_BackLinksAgreeWithTheSpecsThatReferenceThem(t *testing.T) {
	root, apiLoc := designRefProject(t)
	designPaths := map[string]string{
		"authored/alpha.md": seedAuthoredDesign(t, apiLoc, "authored/alpha.md", ""),
		"authored/beta.md":  seedAuthoredDesign(t, apiLoc, "authored/beta.md", ""),
	}
	specPaths := map[string]string{
		"000054_billing":   writeSpecFixture(t, root, "000054_billing", designRefSpecFixture),
		"000055_invoicing": writeSpecFixture(t, root, "000055_invoicing", designRefSpecFixture),
	}

	for _, step := range []struct{ verb, spec, doc string }{
		{"add", "000054_billing", "authored/alpha.md"},
		{"add", "000055_invoicing", "authored/alpha.md"},
		{"add", "000055_invoicing", "authored/beta.md"},
		{"add", "000054_billing", "authored/beta.md"},
		{"remove", "000054_billing", "authored/alpha.md"},
		{"remove", "000055_invoicing", "authored/beta.md"},
		{"add", "000054_billing", "authored/alpha.md"},
	} {
		designRefWrite(t, step.verb, "--data",
			`{"spec":"`+step.spec+`","source":"api","path":"`+step.doc+`"}`)
	}

	require.Equal(t, []string{"000055_invoicing", "000054_billing"},
		specsListedBy(t, designPaths["authored/alpha.md"]))
	require.Equal(t, []string{"000054_billing"},
		specsListedBy(t, designPaths["authored/beta.md"]))
	require.Equal(t, []designRefItem{
		{Source: "api", Path: "authored/beta.md"},
		{Source: "api", Path: "authored/alpha.md"},
	}, designsReferencedBy(t, specPaths["000054_billing"]))
	require.Equal(t, []designRefItem{
		{Source: "api", Path: "authored/alpha.md"},
	}, designsReferencedBy(t, specPaths["000055_invoicing"]))

	for doc, docPath := range designPaths {
		listed := specsListedBy(t, docPath)
		for spec, specPath := range specPaths {
			references := slices.Contains(designsReferencedBy(t, specPath),
				designRefItem{Source: "api", Path: doc})
			require.Equal(t, references, slices.Contains(listed, spec),
				"design %q and spec %q disagree about whether the spec references the design", doc, spec)
		}
	}
}

// errForcedBackLink is the failure the seam substituted below returns. Its
// text is hand-written here and asserted in the reported message, which is how
// the underlying cause is shown to be carried through rather than swallowed.
var errForcedBackLink = errors.New("forced back-link failure")

// chmodUnwritable makes path read-only for the rest of the test and restores
// its mode afterwards, so the test's own temp tree can still be removed.
func chmodUnwritable(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o444))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
}

// Criterion: the two writes a reference operation makes cannot be made atomic,
// so the behaviour when the second one fails is defined rather than left to
// chance. The operation fails as a whole and the spec is put back exactly as
// it was; when putting the spec back also fails, that is reported as its own
// distinct outcome naming both documents to repair by hand. Recording and
// removing a reference carry the same guarantees, so both are exercised.
//
// The two codes must be told apart by an automated caller and not only by
// reading their wording, so each path's observed code is captured and the two
// are compared at the end: one says reissue the command, the other says repair
// two named files, and branching on the code must not conflate them.
func TestDesignRef_BackLinkFailureLeavesNoDisagreementBehind(t *testing.T) {
	var backLinkFailed, rollbackFailed string

	t.Run("add: the design's record cannot be updated", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md", "")
		specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)
		chmodUnwritable(t, docPath)

		er := refuseDesignRef(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
		backLinkFailed = er.Code
		require.Equal(t, "design_ref_backlink_failed", er.Code)
		require.Equal(t, docPath, er.Resource)
		require.Contains(t, er.Message, docPath,
			"the failure must name the design document that could not be updated")
		require.Contains(t, er.NextAction, "reissue",
			"the runnable next step is to retry the same command, not to repair anything")

		stored, err := os.ReadFile(specPath)
		require.NoError(t, err)
		require.Equal(t, designRefSpecFixture, string(stored),
			"a failed back-link write must leave the spec exactly as it was before the command")
	})

	t.Run("remove: the design's record cannot be updated", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md",
			"specs:\n    - 000054_billing\n")
		// The spec starts out carrying the reference, so the removal has both
		// halves of the pair to undo rather than being a no-op.
		carried := specFixtureCarrying(
			"designs:\n" +
				"  - source: api\n" +
				"    path: authored/v2.md\n")
		specPath := writeSpecFixture(t, root, "000054_billing", carried)
		chmodUnwritable(t, docPath)

		er := refuseDesignRef(t, "remove", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
		require.Equal(t, "design_ref_backlink_failed", er.Code)
		require.Equal(t, docPath, er.Resource)
		require.Contains(t, er.Message, docPath)
		require.Contains(t, er.NextAction, "reissue")

		stored, err := os.ReadFile(specPath)
		require.NoError(t, err)
		require.Equal(t, carried, string(stored),
			"a failed back-link removal must leave the spec exactly as it was before the command")
	})

	t.Run("restoring the spec afterwards also fails", func(t *testing.T) {
		root, apiLoc := designRefProject(t)
		docPath := seedAuthoredDesign(t, apiLoc, "authored/v2.md", "")
		specPath := writeSpecFixture(t, root, "000054_billing", designRefSpecFixture)

		// The filesystem alone cannot reach this branch. A store write is
		// os.MkdirAll followed by os.WriteFile, so any permission state that
		// fails the compensating write fails the identical first spec write
		// too, and the run never gets as far as the compensation. The spec has
		// to become unwritable *between* the two writes, and the back-link
		// write is the only thing that runs there — hence the seam.
		original := writeBackLinkFn
		t.Cleanup(func() { writeBackLinkFn = original })
		writeBackLinkFn = func(_ *design.Set, _ design.Document, _ string, _ bool) error {
			chmodUnwritable(t, specPath)
			return errForcedBackLink
		}

		er := refuseDesignRef(t, "add", "--data", `{"spec":"000054_billing","source":"api","path":"authored/v2.md"}`)
		rollbackFailed = er.Code
		require.Equal(t, "design_ref_backlink_rollback_failed", er.Code)
		require.Equal(t, specPath, er.Resource)
		require.Contains(t, er.Message, docPath,
			"the failure must name the design left out of agreement")
		require.Contains(t, er.Message, specPath,
			"the failure must name the spec that could not be restored")
		require.Contains(t, er.Message, errForcedBackLink.Error(),
			"the underlying cause must be carried through, not swallowed")
		require.Contains(t, er.NextAction, "by hand",
			"the runnable next step is a manual reconciliation, not a retry")
		require.Contains(t, er.NextAction, docPath)
		require.Contains(t, er.NextAction, specPath)
	})

	require.NotEqual(t, backLinkFailed, rollbackFailed,
		"an agent branching on the code must not treat 'reissue the command' and 'reconcile two files by hand' as the same outcome")
}
