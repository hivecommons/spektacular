package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeTree creates dir and writes every file in files, whose keys are
// slash-separated paths relative to dir. Intermediate directories are
// created as needed, so a key like "src/app.ts" seeds a subdirectory.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
}

// A Go repo checked out into a directory whose name has nothing to do with
// its module is described by what it declares, not by where it happens to
// sit: the identity is the module path's last segment, never the folder
// name. This is the oracle behind "proposals are drawn from the repository,
// not the folder name".
func TestExamine_GoRepoIdentityComesFromModuleNotFolderName(t *testing.T) {
	// The folder name and the module name are deliberately unrelated.
	dir := filepath.Join(t.TempDir(), "checked-out-elsewhere")
	writeTree(t, dir, map[string]string{
		"README.md":     "# Checked Out Elsewhere\n\nA tool for managing specifications.\nIt keeps intent beside the code.\n\n## Install\n\nrun it\n",
		"go.mod":        "module github.com/hivecommons/spektacular\n\ngo 1.23\n",
		"main.go":       "package main\n",
		"internal/a.go": "package internal\n",
		"internal/b.go": "package internal\n",
	})

	ev := Examine(dir)

	require.True(t, ev.Readable)
	require.Equal(t, "spektacular", ev.Identity, "identity must be the module path's last segment")
	require.NotEqual(t, "checked-out-elsewhere", ev.Identity, "identity must never be the folder name")
	require.NotEqual(t, filepath.Base(dir), ev.Identity)
	require.Equal(t, "go.mod", ev.Manifest)
	require.Empty(t, ev.Summary, "go.mod declares no description")
	require.Equal(t, "A tool for managing specifications. It keeps intent beside the code.", ev.Readme)
	require.Equal(t, []string{"internal/", "README.md", "go.mod", "main.go"}, ev.TopLevel)
	require.Equal(t, []string{"Go"}, ev.Languages)
}

// A JavaScript repo's package.json is the authority for both its identity
// and its summary, again independently of the folder it sits in.
func TestExamine_PackageJSONSuppliesIdentityAndSummary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "some-checkout-dir")
	writeTree(t, dir, map[string]string{
		"package.json": `{
  "name": "widget-kit",
  "version": "2.1.0",
  "description": "A kit of composable widgets for the web."
}
`,
		"src/index.js": "export default 1;\n",
	})

	ev := Examine(dir)

	require.True(t, ev.Readable)
	require.Equal(t, "package.json", ev.Manifest)
	require.Equal(t, "widget-kit", ev.Identity)
	require.Equal(t, "A kit of composable widgets for the web.", ev.Summary)
	require.NotEqual(t, "some-checkout-dir", ev.Identity)
	require.Empty(t, ev.Readme, "there is no README to read")
	require.Equal(t, []string{"src/", "package.json"}, ev.TopLevel)
	require.Equal(t, []string{"JavaScript"}, ev.Languages)
}

// A repo with a README but no manifest is still readable: the prose is
// reported, and the manifest fields stay empty rather than being guessed at.
func TestExamine_ReadmeWithoutManifestIsReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "prose-only")
	writeTree(t, dir, map[string]string{
		"README.md": "# Prose Only\n\nNotes and diagrams for the platform team.\n\nMore below.\n",
	})

	ev := Examine(dir)

	require.True(t, ev.Readable)
	require.Equal(t, "Notes and diagrams for the platform team.", ev.Readme)
	require.Empty(t, ev.Manifest)
	require.Empty(t, ev.Identity)
	require.Empty(t, ev.Summary)
	require.Equal(t, []string{"README.md"}, ev.TopLevel)
	require.Empty(t, ev.Languages)
}

// A repo with a manifest but no README is still readable: the identity and
// summary the manifest declares carry the examination on their own.
func TestExamine_ManifestWithoutReadmeIsReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "no-readme-here")
	writeTree(t, dir, map[string]string{
		"Cargo.toml": "[package]\nname = \"ferris-tools\"\nversion = \"0.3.0\"\ndescription = \"Small helpers for Rust services.\"\n",
	})

	ev := Examine(dir)

	require.True(t, ev.Readable)
	require.Empty(t, ev.Readme)
	require.Equal(t, "Cargo.toml", ev.Manifest)
	require.Equal(t, "ferris-tools", ev.Identity)
	require.Equal(t, "Small helpers for Rust services.", ev.Summary)
	require.Equal(t, []string{"Cargo.toml"}, ev.TopLevel)
	require.Empty(t, ev.Languages)
}

// An empty directory yields the zero Evidence with Readable false — an
// ordinary outcome, not a panic and not an error.
func TestExamine_EmptyDirectoryYieldsZeroEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "empty-repo")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	ev := Examine(dir)

	require.Equal(t, Evidence{}, ev)
	require.False(t, ev.Readable)
}

// A directory that is not on disk yields the zero Evidence with Readable
// false, without panicking and without any error channel to consult.
func TestExamine_MissingDirectoryYieldsZeroEvidence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")
	require.NoDirExists(t, dir)

	ev := Examine(dir)

	require.Equal(t, Evidence{}, ev)
	require.False(t, ev.Readable)
}

// A README that is nothing but a heading states no prose about itself, so
// Readme comes back empty and Readable is decided purely by whatever other
// evidence the repo carries.
func TestExamine_HeadingOnlyReadmeYieldsNoProse(t *testing.T) {
	cases := []struct {
		name         string
		tree         map[string]string
		wantReadable bool
		wantIdentity string
	}{
		{
			name:         "heading alone is the whole repo",
			tree:         map[string]string{"README.md": "# Just A Title\n"},
			wantReadable: false,
			wantIdentity: "",
		},
		{
			name: "heading alone beside a manifest",
			tree: map[string]string{
				"README.md": "# Just A Title\n",
				"go.mod":    "module example.com/team/toolbelt\n",
			},
			wantReadable: true,
			wantIdentity: "toolbelt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "heading-only")
			writeTree(t, dir, tc.tree)

			ev := Examine(dir)

			require.Empty(t, ev.Readme, "a heading is a title, not prose about the repo")
			require.Equal(t, tc.wantIdentity, ev.Identity)
			require.Equal(t, tc.wantReadable, ev.Readable)
		})
	}
}

// The languages reported reflect what the repository actually contains:
// they are weighed strongest first over the top two directory levels,
// dependency and build directories cannot decide the answer, and the repo's
// own .spektacular_ignore removes what it names.
func TestExamine_LanguagesReflectRepositoryContents(t *testing.T) {
	cases := []struct {
		name string
		tree map[string]string
		want []string
	}{
		{
			// Go 3, TypeScript 2, CSS 1, Shell 1 — ties break by name.
			name: "mixed tree strongest first",
			tree: map[string]string{
				"main.go":          "package main\n",
				"cmd/root.go":      "package cmd\n",
				"cmd/serve.go":     "package cmd\n",
				"web/app.ts":       "export {}\n",
				"web/util.ts":      "export {}\n",
				"web/style.css":    "body{}\n",
				"scripts/build.sh": "#!/bin/sh\n",
			},
			want: []string{"Go", "TypeScript", "CSS", "Shell"},
		},
		{
			// Five JavaScript files under node_modules outnumber the two
			// TypeScript files, and still must not appear.
			name: "node_modules does not decide the answer",
			tree: map[string]string{
				"index.ts":          "export {}\n",
				"src/app.ts":        "export {}\n",
				"node_modules/a.js": "module.exports={}\n",
				"node_modules/b.js": "module.exports={}\n",
				"node_modules/c.js": "module.exports={}\n",
				"node_modules/d.js": "module.exports={}\n",
				"node_modules/e.js": "module.exports={}\n",
			},
			want: []string{"TypeScript"},
		},
		{
			// Same shape with vendor/, which holds dependencies rather than
			// the repo's own code.
			name: "vendor does not decide the answer",
			tree: map[string]string{
				"main.go":       "package main\n",
				"internal/x.go": "package internal\n",
				"vendor/a.rs":   "fn a() {}\n",
				"vendor/b.rs":   "fn b() {}\n",
				"vendor/c.rs":   "fn c() {}\n",
				"vendor/d.rs":   "fn d() {}\n",
				"vendor/e.rs":   "fn e() {}\n",
			},
			want: []string{"Go"},
		},
		{
			// Without the ignore file Ruby (3) would outrank Python (1).
			name: "spektacular_ignore excludes what it names",
			tree: map[string]string{
				".spektacular_ignore": "legacy/\n",
				"app.py":              "print(1)\n",
				"legacy/a.rb":         "puts 1\n",
				"legacy/b.rb":         "puts 2\n",
				"legacy/c.rb":         "puts 3\n",
			},
			want: []string{"Python"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "languages")
			writeTree(t, dir, tc.tree)

			ev := Examine(dir)

			require.Equal(t, tc.want, ev.Languages)
		})
	}
}

// The examination reads only what a repository states about itself: the
// contents of a source file never reach any Evidence field, even though the
// file is counted towards the languages present.
func TestExamine_SourceFileContentsNeverLeakIntoEvidence(t *testing.T) {
	const sentinel = "SENTINEL_SOURCE_BODY_MUST_NOT_LEAK"

	dir := filepath.Join(t.TempDir(), "sentinel-repo")
	writeTree(t, dir, map[string]string{
		"README.md":     "# Sentinel Repo\n\nA service that does one thing.\n",
		"go.mod":        "module example.com/team/sentinel\n",
		"secret.go":     "package main\n\nconst Secret = \"" + sentinel + "\"\n",
		"internal/b.go": "package internal // " + sentinel + "\n",
	})

	ev := Examine(dir)

	require.Equal(t, []string{"Go"}, ev.Languages, "the file is counted, not read")
	for field, value := range map[string]string{
		"Readme":   ev.Readme,
		"Manifest": ev.Manifest,
		"Identity": ev.Identity,
		"Summary":  ev.Summary,
	} {
		require.NotContains(t, value, sentinel, field+" must not carry source file contents")
	}
	for _, entry := range ev.TopLevel {
		require.NotContains(t, entry, sentinel)
	}
	for _, lang := range ev.Languages {
		require.NotContains(t, lang, sentinel)
	}
	require.Equal(t, []string{"internal/", "README.md", "go.mod", "secret.go"}, ev.TopLevel)
}

// The examination never recurses past the second directory level, so a
// language that exists only three levels deep is not reported and the deep
// tree is not walked.
func TestExamine_ThirdLevelIsNotWalked(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "deep-repo")
	writeTree(t, dir, map[string]string{
		"main.go":         "package main\n",
		"a/top.go":        "package a\n",
		"a/b/c.rs":        "fn main() {}\n",
		"a/b/deeper/d.py": "print(1)\n",
	})

	ev := Examine(dir)

	require.Equal(t, []string{"Go"}, ev.Languages)
	require.NotContains(t, ev.Languages, "Rust", "a third-level file must not be counted")
	require.NotContains(t, ev.Languages, "Python", "a fourth-level file must not be counted")
	require.Equal(t, []string{"a/", "main.go"}, ev.TopLevel)
	require.False(t, strings.Contains(strings.Join(ev.TopLevel, " "), "c.rs"))
}
