package design

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// Every fixture in this file is built under t.TempDir(), so no test reads or
// writes the real repository tree and no test depends on another's state or on
// execution order.

// Document bodies used across the round-trip and outside-the-root cases. They
// are written out in full here so every expectation in this file is a literal
// the test owns, never a value derived from the package under test.

// frontmatterDoc carries YAML frontmatter of the team's own, which Spektacular
// must neither strip nor rewrite.
const frontmatterDoc = `---
title: Checkout API
owner: payments
tags: [design, api]
---

# Checkout API

The settled request and response shape.
`

// plainDoc carries no frontmatter at all, which Spektacular must not add.
const plainDoc = `# Wire frames

Just a heading and a line, with no frontmatter of any kind.
`

// crlfDoc carries CRLF line endings, a tab, trailing spaces and a blank line,
// none of which may be normalised on the way through.
const crlfDoc = "first line   \r\nsecond\tline\r\n\r\n   indented and trailing   \r\n"

// requireRefusal asserts err is a structured design refusal carrying the
// expected code and a non-empty next action, and returns the envelope so a
// caller can assert on the guidance it offers.
func requireRefusal(t *testing.T, err error, code string) *output.ErrorResponse {
	t.Helper()
	require.Error(t, err)
	var envelope *output.ErrorResponse
	require.ErrorAs(t, err, &envelope)
	require.Equal(t, code, envelope.Code)
	require.NotEmpty(t, envelope.NextAction, "a refusal must tell the caller how to reissue the request")
	return envelope
}

// fileSource builds one file-provider design source declaration.
func fileSource(name, location string) config.SourceConfig {
	return config.SourceConfig{
		Name:     name,
		Provider: "file",
		Config:   config.FileKnowledgeConfig{Location: location},
	}
}

// designConfig builds a Config declaring only the given design sources, which
// is all NewSet reads.
func designConfig(sources ...config.SourceConfig) config.Config {
	return config.Config{Design: config.DesignConfig{Sources: sources}}
}

// writeFile creates dir/name with content, creating parent directories.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// snapshot reads every file under dir, keyed by its slash-separated path
// relative to dir. It walks the filesystem directly rather than going through
// the package under test, so it is an independent oracle for "this directory
// was not touched".
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		require.NoError(t, relErr)
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}))
	return out
}

// Criterion: two design sources in different locations both resolve, and each
// is readable independently of the other.
func TestNewSet_TwoSourcesInDifferentLocationsAreIndependentlyReadable(t *testing.T) {
	root := t.TempDir()
	apiDir := filepath.Join(root, "api-designs")
	uxDir := filepath.Join(root, "ux-designs")
	writeFile(t, apiDir, "shape.md", "# API shape\n")
	writeFile(t, uxDir, "flow.md", "# UX flow\n")

	set, err := NewSet(designConfig(fileSource("api", apiDir), fileSource("ux", uxDir)), root)
	require.NoError(t, err)

	require.Equal(t, []Source{
		{Name: "api", Provider: "file", Location: apiDir},
		{Name: "ux", Provider: "file", Location: uxDir},
	}, set.Sources())

	apiContent, err := set.Read(Document{Source: "api", Path: "shape.md"})
	require.NoError(t, err)
	require.Equal(t, "# API shape\n", string(apiContent))

	uxContent, err := set.Read(Document{Source: "ux", Path: "flow.md"})
	require.NoError(t, err)
	require.Equal(t, "# UX flow\n", string(uxContent))

	// Independently readable means each source sees only its own documents.
	apiHasUXDoc, err := set.Exists(Document{Source: "api", Path: "flow.md"})
	require.NoError(t, err)
	require.False(t, apiHasUXDoc)

	uxHasAPIDoc, err := set.Exists(Document{Source: "ux", Path: "shape.md"})
	require.NoError(t, err)
	require.False(t, uxHasAPIDoc)
}

// Criterion (the load-bearing one): a source location entirely outside the
// project root resolves and reads. This is the success metric "teams adopt
// design documents without relocating anything", and it is what rules out a
// store-directory config shape, which would refuse a location outside the
// project. Reading must also leave the team's existing folder byte-identical.
func TestNewSet_SourceOutsideProjectRootResolvesAndReadsWithoutMutatingIt(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".spektacular"), 0755))

	// A folder the team already keeps, nowhere near the project root.
	teamDir := t.TempDir()
	require.NotEqual(t, projectRoot, teamDir)
	require.False(t, strings.HasPrefix(teamDir, projectRoot+string(filepath.Separator)),
		"the fixture must sit outside the project root for this case to mean anything")

	writeFile(t, teamDir, "checkout-api.md", frontmatterDoc)
	writeFile(t, teamDir, "wireframes.md", plainDoc)
	writeFile(t, teamDir, "flows/returns.md", "# Returns flow\n\nStep one.\n")

	// The expected contents of the team's folder, hand-written, both before
	// and after Spektacular reads from it.
	expected := map[string]string{
		"checkout-api.md":  frontmatterDoc,
		"wireframes.md":    plainDoc,
		"flows/returns.md": "# Returns flow\n\nStep one.\n",
	}
	require.Equal(t, expected, snapshot(t, teamDir), "fixture should match the hand-written expectation before any read")

	set, err := NewSet(designConfig(fileSource("team", teamDir)), projectRoot)
	require.NoError(t, err)
	require.Equal(t, []Source{{Name: "team", Provider: "file", Location: teamDir}}, set.Sources())

	// A document with frontmatter of its own comes back byte-identical.
	withFrontmatter, err := set.Read(Document{Source: "team", Path: "checkout-api.md"})
	require.NoError(t, err)
	require.Equal(t, frontmatterDoc, string(withFrontmatter))

	// So does one that carries no frontmatter.
	withoutFrontmatter, err := set.Read(Document{Source: "team", Path: "wireframes.md"})
	require.NoError(t, err)
	require.Equal(t, plainDoc, string(withoutFrontmatter))

	nested, err := set.Read(Document{Source: "team", Path: "flows/returns.md"})
	require.NoError(t, err)
	require.Equal(t, "# Returns flow\n\nStep one.\n", string(nested))

	// Nothing was added, removed or rewritten in the team's folder.
	require.Equal(t, expected, snapshot(t, teamDir))
}

// Criterion: a relative location resolves from the settings folder
// (<root>/.spektacular), not from the project root and not from the process
// working directory. A decoy folder of the same relative name sits at the
// project root; the source must not resolve to it.
func TestNewSet_RelativeLocationResolvesFromSettingsFolderNotWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	settingsDesign := filepath.Join(root, ".spektacular", "design")
	writeFile(t, settingsDesign, "real.md", "# From the settings folder\n")

	// A decoy at the project root, which a resolver using the wrong base would
	// pick up instead. The process working directory is the repository, which
	// has no such folder, so an absolute-path assertion rules that out too.
	writeFile(t, filepath.Join(root, "design"), "decoy.md", "# From the project root\n")

	set, err := NewSet(designConfig(fileSource("d", "design")), root)
	require.NoError(t, err)

	require.Equal(t, filepath.Join(root, ".spektacular", "design"), set.Sources()[0].Location)

	docs, err := set.List("d")
	require.NoError(t, err)
	require.Equal(t, []Document{{Source: "d", Path: "real.md"}}, docs)

	content, err := set.Read(Document{Source: "d", Path: "real.md"})
	require.NoError(t, err)
	require.Equal(t, "# From the settings folder\n", string(content))
}

// Criterion: an absolute location is used as written.
func TestNewSet_AbsoluteLocationIsUsedAsWritten(t *testing.T) {
	root := t.TempDir()
	absDir := t.TempDir()
	writeFile(t, absDir, "shape.md", "# Absolute\n")

	set, err := NewSet(designConfig(fileSource("abs", absDir)), root)
	require.NoError(t, err)

	require.Equal(t, absDir, set.Sources()[0].Location)

	content, err := set.Read(Document{Source: "abs", Path: "shape.md"})
	require.NoError(t, err)
	require.Equal(t, "# Absolute\n", string(content))
}

// Criterion: a ${VAR} location expands. Expansion is the config layer's job
// (config.ParseYAMLFile), not NewSet's, so this case goes through a written
// config.yaml and the real loader before resolving the set.
func TestNewSet_EnvVarLocationInConfigFileExpandsAndResolves(t *testing.T) {
	designDir := t.TempDir()
	writeFile(t, designDir, "shape.md", "# From the env var\n")
	t.Setenv("SPEK_TEST_DESIGN_LOCATION", designDir)

	root := t.TempDir()
	settings := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(settings, 0755))
	configPath := filepath.Join(settings, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(`schema: 3
name: designproj
command: "go run ."
design:
  sources:
    - name: team
      provider: file
      config:
        location: "${SPEK_TEST_DESIGN_LOCATION}"
repos:
  - name: designproj
    location: ..
`), 0644))

	cfg, err := config.FromYAMLFile(configPath)
	require.NoError(t, err)

	set, err := NewSet(cfg, root)
	require.NoError(t, err)
	require.Equal(t, []Source{{Name: "team", Provider: "file", Location: designDir}}, set.Sources())

	content, err := set.Read(Document{Source: "team", Path: "shape.md"})
	require.NoError(t, err)
	require.Equal(t, "# From the env var\n", string(content))
}

// Criterion: a location that is not there is refused with
// design_source_unreachable, naming the resolved path and the base a relative
// location resolved from.
func TestNewSet_MissingLocationRefusesAsUnreachable(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".spektacular"), 0755))

	resolved := filepath.Join(root, ".spektacular", "design", "missing")
	base := filepath.Join(root, ".spektacular")

	_, err := NewSet(designConfig(fileSource("team", "design/missing")), root)
	envelope := requireRefusal(t, err, "design_source_unreachable")

	require.Equal(t, resolved, envelope.Resource)
	require.Contains(t, envelope.Message, resolved, "the refusal must name the path the declaration resolved to")
	require.Contains(t, envelope.Message, `"team"`)
	require.Contains(t, envelope.NextAction, resolved)
	require.Contains(t, envelope.NextAction, base, "a relative location's refusal must name the base it resolved from")
}

// Criterion: a location that is a file rather than a directory is refused the
// same way.
func TestNewSet_LocationThatIsAFileRefusesAsUnreachable(t *testing.T) {
	root := t.TempDir()
	holder := t.TempDir()
	notADir := filepath.Join(holder, "design.md")
	require.NoError(t, os.WriteFile(notADir, []byte("# Not a directory\n"), 0644))

	_, err := NewSet(designConfig(fileSource("team", notADir)), root)
	envelope := requireRefusal(t, err, "design_source_unreachable")

	require.Equal(t, notADir, envelope.Resource)
	require.Contains(t, envelope.Message, notADir)
	require.Contains(t, envelope.NextAction, notADir)
}

// Criterion: a provider this build does not implement is refused with
// design_provider_unsupported, by name rather than by silently dropping the
// source.
func TestNewSet_UnknownProviderRefusesAsUnsupported(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# Shape\n")

	_, err := NewSet(designConfig(config.SourceConfig{
		Name:     "team",
		Provider: "notion",
		Config:   config.FileKnowledgeConfig{Location: dir},
	}), root)
	envelope := requireRefusal(t, err, "design_provider_unsupported")

	require.Equal(t, "team", envelope.Resource)
	require.Contains(t, envelope.Message, `"notion"`)
	require.Contains(t, envelope.Message, `"team"`)
	require.Contains(t, envelope.NextAction, `"file"`)
}

// Criterion: an unknown source name is refused with design_source_unknown, and
// the refusal lists the names the project did declare.
func TestSet_UnknownSourceNameRefusesAndListsDeclaredNames(t *testing.T) {
	root := t.TempDir()
	apiDir := t.TempDir()
	uxDir := t.TempDir()
	writeFile(t, apiDir, "shape.md", "# API shape\n")
	writeFile(t, uxDir, "flow.md", "# UX flow\n")

	set, err := NewSet(designConfig(fileSource("api", apiDir), fileSource("ux", uxDir)), root)
	require.NoError(t, err)

	_, readErr := set.Read(Document{Source: "apis", Path: "shape.md"})
	readEnvelope := requireRefusal(t, readErr, "design_source_unknown")
	require.Equal(t, "apis", readEnvelope.Resource)
	require.Contains(t, readEnvelope.Message, `"apis"`)
	require.Contains(t, readEnvelope.NextAction, `"api"`)
	require.Contains(t, readEnvelope.NextAction, `"ux"`)

	_, listErr := set.List("apis")
	listEnvelope := requireRefusal(t, listErr, "design_source_unknown")
	require.Contains(t, listEnvelope.NextAction, `"api"`)
	require.Contains(t, listEnvelope.NextAction, `"ux"`)
}

// Criterion: a document that is missing from a source that does exist is
// refused with design_not_found, distinctly from an unknown source name. Both
// failures are provoked against the same project, so the two codes cannot be
// confused.
func TestSet_MissingDocumentRefusesDistinctlyFromUnknownSource(t *testing.T) {
	root := t.TempDir()
	apiDir := t.TempDir()
	writeFile(t, apiDir, "shape.md", "# API shape\n")

	set, err := NewSet(designConfig(fileSource("api", apiDir)), root)
	require.NoError(t, err)

	_, missingErr := set.Read(Document{Source: "api", Path: "nope.md"})
	missing := requireRefusal(t, missingErr, "design_not_found")
	searched := filepath.Join(apiDir, "nope.md")
	require.Equal(t, searched, missing.Resource)
	require.Contains(t, missing.Message, searched, "a not-found refusal must name the absolute path it searched")
	require.Contains(t, missing.Message, `"api"`)
	require.Contains(t, missing.NextAction, "design list --source api")

	_, unknownErr := set.Read(Document{Source: "nope", Path: "shape.md"})
	unknown := requireRefusal(t, unknownErr, "design_source_unknown")

	require.NotEqual(t, unknown.Code, missing.Code,
		"a missing document and an unknown source must be distinguishable by code alone")
}

// Criterion: a source whose provider cannot write refuses the write by name
// with design_source_read_only. No shipping provider is read-only, so the
// source is constructed directly with a nil writer.
func TestSet_WriteToSourceWithNilWriterRefusesAsReadOnly(t *testing.T) {
	dir := t.TempDir()
	set := &Set{sources: []resolvedSource{{
		name:     "remote",
		provider: "file",
		location: dir,
		reader:   store.NewSourceStore(dir, "design:remote"),
		writer:   nil,
	}}}

	err := set.Write(Document{Source: "remote", Path: "shape.md"}, []byte("# Shape\n"))
	envelope := requireRefusal(t, err, "design_source_read_only")

	require.Equal(t, "remote", envelope.Resource)
	require.Contains(t, envelope.Message, `"remote"`)
	require.Contains(t, envelope.Message, `"shape.md"`)
	require.NoFileExists(t, filepath.Join(dir, "shape.md"), "a refused write must not have written anything")
}

// Criterion: a write/read round trip is byte-for-byte, with nothing added,
// removed or reformatted — including a document with its own frontmatter, one
// with none, and one with CRLF line endings and trailing whitespace.
func TestSet_WriteThenReadRoundTripsBytesUnchanged(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()

	set, err := NewSet(designConfig(fileSource("api", dir)), root)
	require.NoError(t, err)

	cases := map[string]string{
		"with-frontmatter.md": frontmatterDoc,
		"plain.md":            plainDoc,
		"crlf.md":             crlfDoc,
	}
	for path, content := range cases {
		require.NoError(t, set.Write(Document{Source: "api", Path: path}, []byte(content)))
	}

	for path, content := range cases {
		roundTripped, readErr := set.Read(Document{Source: "api", Path: path})
		require.NoError(t, readErr)
		require.Equal(t, content, string(roundTripped), "read of %s must return exactly what was written", path)

		// Independently of the package, the bytes on disk are the same bytes:
		// no frontmatter stamped, no line endings normalised, no whitespace
		// trimmed.
		onDisk, diskErr := os.ReadFile(filepath.Join(dir, path))
		require.NoError(t, diskErr)
		require.Equal(t, content, string(onDisk), "%s on disk must be byte-identical to what was written", path)
	}

	require.Equal(t, map[string]string{
		"with-frontmatter.md": frontmatterDoc,
		"plain.md":            plainDoc,
		"crlf.md":             crlfDoc,
	}, snapshot(t, dir), "writing must add no files beyond the documents written")
}

// Criterion: a document that has just been written appears in that source's
// listing.
func TestSet_WrittenDocumentAppearsInSourceListing(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# API shape\n")

	set, err := NewSet(designConfig(fileSource("api", dir)), root)
	require.NoError(t, err)

	before, err := set.List("api")
	require.NoError(t, err)
	require.Equal(t, []Document{{Source: "api", Path: "shape.md"}}, before)

	require.NoError(t, set.Write(Document{Source: "api", Path: "notes/extra.md"}, []byte("# Extra\n")))

	after, err := set.List("api")
	require.NoError(t, err)
	require.Equal(t, []Document{
		{Source: "api", Path: "notes/extra.md"},
		{Source: "api", Path: "shape.md"},
	}, after)
}

// Criterion: a fanned-out List("") returns the documents of every source, each
// tagged with the source it came from and recursing into subdirectories, while
// naming a source narrows the listing to that one.
func TestSet_ListFansOutOverEverySourceAndNarrowsToOne(t *testing.T) {
	root := t.TempDir()
	apiDir := t.TempDir()
	uxDir := t.TempDir()
	writeFile(t, apiDir, "endpoints.md", "# Endpoints\n")
	writeFile(t, apiDir, "v2/payments.md", "# Payments v2\n")
	writeFile(t, uxDir, "flow.md", "# Flow\n")
	writeFile(t, uxDir, "wire/checkout.md", "# Checkout wireframe\n")

	set, err := NewSet(designConfig(fileSource("api", apiDir), fileSource("ux", uxDir)), root)
	require.NoError(t, err)

	all, err := set.List("")
	require.NoError(t, err)
	require.Equal(t, []Document{
		{Source: "api", Path: "endpoints.md"},
		{Source: "api", Path: "v2/payments.md"},
		{Source: "ux", Path: "flow.md"},
		{Source: "ux", Path: "wire/checkout.md"},
	}, all)

	narrowed, err := set.List("api")
	require.NoError(t, err)
	require.Equal(t, []Document{
		{Source: "api", Path: "endpoints.md"},
		{Source: "api", Path: "v2/payments.md"},
	}, narrowed)
}

// Criterion: a project declaring no design sources yields an empty set, lists
// empty, and returns no error.
func TestNewSet_NoDeclaredSourcesYieldsEmptySet(t *testing.T) {
	root := t.TempDir()

	set, err := NewSet(designConfig(), root)
	require.NoError(t, err)
	require.NotNil(t, set)
	require.Empty(t, set.Sources())

	docs, err := set.List("")
	require.NoError(t, err)
	require.Empty(t, docs)
}

// Criterion: a document removed by its source and path disappears from that
// source's listing, and no sibling document goes with it.
func TestSet_DeleteRemovesDocumentAndItLeavesTheListing(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# API shape\n")
	writeFile(t, dir, "notes/extra.md", "# Extra\n")

	set, err := NewSet(designConfig(fileSource("api", dir)), root)
	require.NoError(t, err)

	before, err := set.List("api")
	require.NoError(t, err)
	require.Equal(t, []Document{
		{Source: "api", Path: "notes/extra.md"},
		{Source: "api", Path: "shape.md"},
	}, before)

	require.NoError(t, set.Delete(Document{Source: "api", Path: "notes/extra.md"}))

	after, err := set.List("api")
	require.NoError(t, err)
	require.Equal(t, []Document{{Source: "api", Path: "shape.md"}}, after)

	// Independently of the package, the survivor is still on disk byte for
	// byte and the removed document is gone.
	require.Equal(t, map[string]string{"shape.md": "# API shape\n"}, snapshot(t, dir))
}

// Criterion: removing a document that is not there reports success and changes
// nothing, and can be repeated safely — so a maintenance pass that retries is
// not punished for it.
func TestSet_DeleteOfAbsentPathSucceedsAndIsRepeatable(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# API shape\n")

	set, err := NewSet(designConfig(fileSource("api", dir)), root)
	require.NoError(t, err)

	require.NoError(t, set.Delete(Document{Source: "api", Path: "never-existed.md"}))

	// The same document twice: removed once, then removed again.
	require.NoError(t, set.Delete(Document{Source: "api", Path: "shape.md"}))
	require.NoError(t, set.Delete(Document{Source: "api", Path: "shape.md"}))

	require.Equal(t, map[string]string{}, snapshot(t, dir))
}

// Criterion: a source whose provider cannot write refuses the removal by name
// with design_source_read_only, and the document is still there. No shipping
// provider is read-only, so the source is constructed directly with a nil
// writer, exactly as the read-only write case is.
func TestSet_DeleteFromSourceWithNilWriterRefusesAsReadOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# Shape\n")
	set := &Set{sources: []resolvedSource{{
		name:     "remote",
		provider: "file",
		location: dir,
		reader:   store.NewSourceStore(dir, "design:remote"),
		writer:   nil,
	}}}

	err := set.Delete(Document{Source: "remote", Path: "shape.md"})
	envelope := requireRefusal(t, err, "design_source_read_only")

	require.Equal(t, "remote", envelope.Resource)
	require.Contains(t, envelope.Message, `"remote"`)
	require.Contains(t, envelope.Message, `"shape.md"`)
	require.Equal(t, map[string]string{"shape.md": "# Shape\n"}, snapshot(t, dir),
		"a refused removal must leave the source untouched")
}

// Criterion: removing from a source the project has not declared is refused by
// name, nothing is removed, and the refusal lists the declared source names.
func TestSet_DeleteFromUnknownSourceRefusesAndListsDeclaredNames(t *testing.T) {
	root := t.TempDir()
	apiDir := t.TempDir()
	uxDir := t.TempDir()
	writeFile(t, apiDir, "shape.md", "# API shape\n")
	writeFile(t, uxDir, "flow.md", "# UX flow\n")

	set, err := NewSet(designConfig(fileSource("api", apiDir), fileSource("ux", uxDir)), root)
	require.NoError(t, err)

	envelope := requireRefusal(t, set.Delete(Document{Source: "apis", Path: "shape.md"}), "design_source_unknown")
	require.Equal(t, "apis", envelope.Resource)
	require.Contains(t, envelope.Message, `"apis"`)
	require.Contains(t, envelope.NextAction, `"api"`)
	require.Contains(t, envelope.NextAction, `"ux"`)

	require.Equal(t, map[string]string{"shape.md": "# API shape\n"}, snapshot(t, apiDir))
	require.Equal(t, map[string]string{"flow.md": "# UX flow\n"}, snapshot(t, uxDir))
}

// Criterion: an address missing its path is refused rather than guessed at —
// a source name alone can never say which document to remove, and taking it as
// "all of them" is the one reading a removal must never make.
func TestSet_DeleteWithEmptyPathRefusesAsIncompleteAddress(t *testing.T) {
	root := t.TempDir()
	dir := t.TempDir()
	writeFile(t, dir, "shape.md", "# API shape\n")

	set, err := NewSet(designConfig(fileSource("api", dir)), root)
	require.NoError(t, err)

	envelope := requireRefusal(t, set.Delete(Document{Source: "api", Path: ""}), "design_address_incomplete")
	require.Contains(t, envelope.Message, `"path"`)
	require.Contains(t, envelope.NextAction, `"api"`)

	require.Equal(t, map[string]string{"shape.md": "# API shape\n"}, snapshot(t, dir))
}

// recordingWriter is a store.Writer that is not a FileStore and holds no
// directory at all: it records the paths it is asked to delete and returns the
// error it is primed with. Substituted for a source's writer, it is what makes
// "the removal goes through the storage abstraction" observable rather than
// inferred, since a filesystem-only assertion could not tell a store call from
// a direct os.Remove.
type recordingWriter struct {
	written []string
	deleted []string
	err     error
}

func (w *recordingWriter) Write(path string, _ []byte) error {
	w.written = append(w.written, path)
	return w.err
}

func (w *recordingWriter) Delete(path string) error {
	w.deleted = append(w.deleted, path)
	return w.err
}

// Criterion: removal reaches the source through the same store.Writer seam
// reading and writing use, proven against a source whose backend is not a
// local directory — the writer receives the store-relative path, the document
// on disk is not touched behind its back, and the backend's own failure is
// returned rather than swallowed.
func TestSet_DeleteRoutesThroughTheStoreWriter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "payments/v2.md", "# Payments v2\n")

	writer := &recordingWriter{}
	set := &Set{sources: []resolvedSource{{
		name:     "remote",
		provider: "file",
		location: dir,
		reader:   store.NewSourceStore(dir, "design:remote"),
		writer:   writer,
	}}}

	require.NoError(t, set.Delete(Document{Source: "remote", Path: "payments/v2.md"}))
	require.Equal(t, []string{"payments/v2.md"}, writer.deleted,
		"the removal must be handed to the source's writer as a store-relative path")
	require.Empty(t, writer.written, "a removal must not write anything")
	require.Equal(t, map[string]string{"payments/v2.md": "# Payments v2\n"}, snapshot(t, dir),
		"nothing may be removed from the filesystem behind the writer's back")

	writer.err = errBackendRefused
	require.ErrorIs(t, set.Delete(Document{Source: "remote", Path: "payments/v2.md"}), errBackendRefused,
		"a backend's own failure must be returned, not swallowed into a success")
}

// errBackendRefused is the failure recordingWriter is primed with above. Its
// identity is the assertion, so it is declared here rather than built inline.
var errBackendRefused = errors.New("backend refused the removal")
