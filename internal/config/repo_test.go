package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// pinWriterVersion sets WriterVersion for the duration of a test so the
// `written_by` stamp in a written settings file is a known literal.
func pinWriterVersion(t *testing.T, v string) {
	t.Helper()
	prev := WriterVersion
	WriterVersion = v
	t.Cleanup(func() { WriterVersion = prev })
}

// withSchema prefixes a hand-written repo.yaml fixture with the current repo
// format version, so it loads past the format check and exercises what
// follows it. The literal is hand-maintained, not read from
// CurrentRepoSchema.
func withSchema(body string) string {
	return "schema: 2\n" + body
}

// withProjectSchema is withSchema for a config.yaml fixture: it prefixes the
// current project format version (hand-maintained, not read from
// CurrentProjectSchema).
func withProjectSchema(body string) string {
	return "schema: 3\n" + body
}

// Criterion 2: the default repo config seeds exactly the repo's own knowledge
// store and a file-backed changelog.
func TestNewDefaultRepoConfig_SeedsRepoStoreAndChangelog(t *testing.T) {
	cfg := NewDefaultRepoConfig()

	require.Equal(t, "file", cfg.Knowledge.Provider)
	require.Equal(t, "knowledge", cfg.Knowledge.Config.Location)

	require.Equal(t, "file", cfg.Changelog.Provider)
	require.Equal(t, "changelog", cfg.Changelog.Config.Directory)
}

// Criterion 2: a repo.yaml parses and validates independently of any project
// config — a minimal file in a directory with NO config.yaml loads fine, with
// defaults filling the absent sections.
func TestRepoConfigFromYAMLFile_LoadsWithoutProjectConfig(t *testing.T) {
	dir := t.TempDir()
	// Deliberately no config.yaml anywhere in dir: only repo.yaml exists.
	path := filepath.Join(dir, RepoConfigFileName)
	minimal := "changelog:\n  provider: file\n  config:\n    directory: docs/changelog\n"
	require.NoError(t, os.WriteFile(path, []byte(withSchema(minimal)), 0644))

	cfg, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)

	// The configured section is honoured...
	require.Equal(t, "docs/changelog", cfg.Changelog.Config.Directory)
	// ...and the absent knowledge section keeps the seeded default store.
	require.Equal(t, "file", cfg.Knowledge.Provider)
	require.Equal(t, "knowledge", cfg.Knowledge.Config.Location)
}

// Criterion 2: a minimal repo config with no descriptive fields set (they are
// all omitempty and zero-valued) contains only knowledge and changelog
// settings — the serialized repo.yaml carries exactly those two top-level
// sections, plus the format and writer stamps every settings file carries.
func TestRepoConfig_ToYAMLFileWritesOnlyKnowledgeAndChangelog(t *testing.T) {
	pinWriterVersion(t, "test-x")
	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, NewDefaultRepoConfig().ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &top))
	require.Len(t, top, 4)
	require.Equal(t, 2, top["schema"])
	require.Equal(t, "test-x", top["written_by"])
	require.Contains(t, top, "knowledge")
	require.Contains(t, top, "changelog")
}

// Criterion 2: a repo config round-trips through repo.yaml unchanged.
func TestRepoConfig_ToYAMLFileRoundTrip(t *testing.T) {
	pinWriterVersion(t, "test-x")
	cfg := RepoConfig{
		Knowledge: RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config:   FileKnowledgeConfig{Location: "kb/repo"},
		},
		Changelog: ChangelogConfig{
			Provider: ProviderFile,
			Config:   FileChangelogConfig{Directory: "docs/changelog"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	// The written file is stamped with the current format and writer.
	want := cfg
	want.Schema = 2
	want.WrittenBy = "test-x"
	require.Equal(t, want, loaded)
}

// Criterion 2: a repo config with descriptive metadata (description, role,
// tags) round-trips through repo.yaml unchanged, alongside the
// existing knowledge and changelog sections.
func TestRepoConfig_ToYAMLFileRoundTripWithDescriptiveFields(t *testing.T) {
	pinWriterVersion(t, "test-x")
	cfg := RepoConfig{
		Description: "Handles order processing and fulfillment.",
		Role:        "backend-service",
		Tags:        []string{"go", "orders", "team-checkout"},
		Knowledge: RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config:   FileKnowledgeConfig{Location: "kb/repo"},
		},
		Changelog: ChangelogConfig{
			Provider: ProviderFile,
			Config:   FileChangelogConfig{Directory: "docs/changelog"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	// The written file is stamped with the current format and writer.
	want := cfg
	want.Schema = 2
	want.WrittenBy = "test-x"
	require.Equal(t, want, loaded)
}

// Criterion 3: a repo knowledge store missing its required location fails
// RepoConfig validation with an error naming the config key.
func TestRepoConfigFromYAMLFile_MissingKnowledgeLocationReturnsError(t *testing.T) {
	body := "knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: \"\"\n"
	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, os.WriteFile(path, []byte(withSchema(body)), 0644))

	_, err := RepoConfigFromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "config.location")
}

// Criterion 3: an unknown changelog provider in repo.yaml fails RepoConfig
// validation with an error naming the config key.
func TestRepoConfigFromYAMLFile_UnknownChangelogProviderReturnsError(t *testing.T) {
	body := "changelog:\n  provider: bogus\n  config:\n    directory: changelog\n"
	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, os.WriteFile(path, []byte(withSchema(body)), 0644))

	_, err := RepoConfigFromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "changelog.provider")
}

// Criterion 3: the project's source list and the repo's single store validate
// independently at their own levels — an invalid repo store fails RepoConfig
// validation while a valid project config is unaffected, and vice versa.
func TestRepoConfigValidation_IndependentOfProjectConfig(t *testing.T) {
	invalidRepoKnowledge := RepoKnowledgeConfig{
		Provider: ProviderFile,
		Config:   FileKnowledgeConfig{Location: ""},
	}
	validRepoKnowledge := RepoKnowledgeConfig{
		Provider: ProviderFile,
		Config:   FileKnowledgeConfig{Location: "/shared/team/knowledge"},
	}
	invalidProjectKnowledge := KnowledgeConfig{
		Sources: []SourceConfig{
			{Name: "project", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: ""}},
		},
	}
	validProjectKnowledge := KnowledgeConfig{
		Sources: []SourceConfig{
			{Name: "team", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/shared/team/knowledge"}},
		},
	}

	// An invalid repo store fails at the repo level; a valid project config is
	// unaffected.
	repoCfg := NewDefaultRepoConfig()
	repoCfg.Knowledge = invalidRepoKnowledge
	require.Error(t, repoCfg.Validate())

	projectCfg := NewDefault()
	projectCfg.Name = "testproj"
	projectCfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	projectCfg.Knowledge = validProjectKnowledge
	require.NoError(t, projectCfg.Validate())

	// And vice versa: invalid project sources fail at the project level while
	// a valid repo config is unaffected.
	projectCfg.Knowledge = invalidProjectKnowledge
	require.Error(t, projectCfg.Validate())

	repoCfg.Knowledge = validRepoKnowledge
	require.NoError(t, repoCfg.Validate())
}

// Phase 2.1 criterion 2: a repo that declares no knowledge store at all still
// resolves to its default store in the expected place — WithDefaults
// synthesises the single file-provider block rooted at repoRoot.
func TestRepoConfig_WithDefaultsSynthesisesRepoStore(t *testing.T) {
	cfg := RepoConfig{}.WithDefaults("/some/repo")

	require.Equal(t, RepoKnowledgeConfig{
		Provider: ProviderFile,
		Config:   FileKnowledgeConfig{Location: filepath.Join("/some/repo", "knowledge")},
	}, cfg.Knowledge)
}

// Criterion 2: WithDefaults leaves an already-configured repo store unchanged.
func TestRepoConfig_WithDefaultsKeepsConfiguredSources(t *testing.T) {
	configured := RepoConfig{
		Knowledge: RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config:   FileKnowledgeConfig{Location: "/elsewhere/kb"},
		},
		Changelog: NewDefaultRepoConfig().Changelog,
	}

	result := configured.WithDefaults("/some/repo")
	require.Equal(t, configured, result)
}

// Criterion 2: repo.yaml expands ${VAR} environment patterns like config.yaml
// does.
func TestRepoConfigFromYAMLFile_ExpandsEnvVars(t *testing.T) {
	t.Setenv("TEST_REPO_KB", "env/knowledge")

	body := "knowledge:\n" +
		"  provider: file\n" +
		"  config:\n" +
		"    location: \"${TEST_REPO_KB}\"\n"
	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, os.WriteFile(path, []byte(withSchema(body)), 0644))

	cfg, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "env/knowledge", cfg.Knowledge.Config.Location)
}

// Phase 1.1 criterion 1: a repo.yaml with no source key loads exactly as
// before — Source is empty and ParseSource reports SourceNone.
func TestRepoConfigFromYAMLFile_NoSourceIsNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	minimal := "changelog:\n  provider: file\n  config:\n    directory: docs/changelog\n"
	require.NoError(t, os.WriteFile(path, []byte(withSchema(minimal)), 0644))

	cfg, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	require.True(t, cfg.Source.IsZero(), "an absent source block leaves the source unset")

	kind, value, err := cfg.ParseSource(dir)
	require.NoError(t, err)
	require.Equal(t, SourceNone, kind)
	require.Equal(t, "", value)
}

// Phase 1.1 criterion 1: a config whose Source is set serialises exactly
// three content keys — source, knowledge, and changelog — beside the schema
// and written_by stamps.
func TestRepoConfig_ToYAMLFileWritesSourceWhenSet(t *testing.T) {
	pinWriterVersion(t, "test-x")
	cfg := NewDefaultRepoConfig()
	cfg.Source = FileSource("/srv/code/api")

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var top map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &top))
	require.Len(t, top, 5)
	require.Equal(t, 2, top["schema"])
	require.Equal(t, "test-x", top["written_by"])
	require.Contains(t, top, "source")
	require.Contains(t, top, "knowledge")
	require.Contains(t, top, "changelog")
	require.Equal(t, map[string]any{
		"provider": "file",
		"config":   map[string]any{"location": "/srv/code/api"},
	}, top["source"], "source round-trips as a provider block")
}

// An absent source block classifies as SourceNone with an empty value and
// no error; a half-declared one names the missing half instead.
func TestRepoConfig_ParseSource_Unset(t *testing.T) {
	kind, value, err := RepoConfig{}.ParseSource("/cfg")
	require.NoError(t, err)
	require.Equal(t, SourceNone, kind)
	require.Equal(t, "", value)

	_, _, err = RepoConfig{Source: FileSource("   ")}.ParseSource("/cfg")
	require.ErrorContains(t, err, "no location", "a provider with no location must be reported")

	_, _, err = RepoConfig{Source: RepoSourceConfig{Config: RepoSourceLocation{Location: "../code"}}}.ParseSource("/cfg")
	require.ErrorContains(t, err, "no provider", "a location with no provider must be reported")
}

// Phase 1.1 criterion 2: an absolute path classifies as a file source and
// resolves to that same directory, cleaned.
func TestRepoConfig_ParseSource_AbsoluteFile(t *testing.T) {
	code := t.TempDir()
	configDir := t.TempDir()

	kind, value, err := RepoConfig{Source: FileSource(code)}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, code, value)

	// A trailing separator is cleaned away; configDir plays no part.
	kind, value, err = RepoConfig{Source: FileSource(code + string(filepath.Separator))}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, code, value)
}

// Phase 1.1 criterion 2: a path relative to the configuration file's
// directory classifies as a file source and resolves against configDir —
// ../code beside a configDir of <parent>/repo resolves to <parent>/code.
func TestRepoConfig_ParseSource_RelativeFile(t *testing.T) {
	parent := t.TempDir()
	configDir := filepath.Join(parent, "repo")
	expected := filepath.Join(parent, "code")

	kind, value, err := RepoConfig{Source: FileSource("../code")}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, expected, value)

	// A plain child path anchors under configDir as well.
	kind, value, err = RepoConfig{Source: FileSource("src")}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, filepath.Join(parent, "repo", "src"), value)
}

// Phase 1.1 criterion 2: the file:// form of an absolute and of a relative
// path classifies as a file source and resolves to the same directory as the
// plain form.
func TestRepoConfig_ParseSource_FileScheme(t *testing.T) {
	code := t.TempDir()
	parent := t.TempDir()
	configDir := filepath.Join(parent, "repo")

	kind, value, err := RepoConfig{Source: FileSource(code)}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, code, value)

	kind, value, err = RepoConfig{Source: FileSource("../code")}.ParseSource(configDir)
	require.NoError(t, err)
	require.Equal(t, SourceFile, kind)
	require.Equal(t, filepath.Join(parent, "code"), value)
}

// Phase 1.1 criterion 2: a ${VAR} source, plain and in file:// form, is
// expanded by the loader and then classifies as a file source resolving to
// the variable's directory.
func TestRepoConfigFromYAMLFile_SourceEnvVar(t *testing.T) {
	code := t.TempDir()
	t.Setenv("TEST_REPO_SRC", code)

	for _, form := range []string{"${TEST_REPO_SRC}", "file://${TEST_REPO_SRC}"} {
		dir := t.TempDir()
		path := filepath.Join(dir, RepoConfigFileName)
		body := "source:\n  provider: file\n  config:\n    location: \"" + form + "\"\n"
		require.NoError(t, os.WriteFile(path, []byte(withSchema(body)), 0644))

		cfg, err := RepoConfigFromYAMLFile(path)
		require.NoError(t, err)

		kind, value, err := cfg.ParseSource(dir)
		require.NoError(t, err)
		require.Equal(t, SourceFile, kind, form)
		require.Equal(t, code, value, form)
	}
}

// Phase 1.1 criterion 3: git://, ssh://, http://, https://, and git+ssh://
// values classify as a git source and are returned unchanged.
func TestRepoConfig_ParseSource_GitSchemes(t *testing.T) {
	cases := []string{
		"git://github.com/org/api.git",
		"ssh://git@github.com/org/api.git",
		"http://github.com/org/api.git",
		"https://github.com/org/api.git",
		"git+ssh://git@github.com/org/api.git",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			// A `repo add` payload gives the source as one value; the
			// transport is what marks it as a git location.
			block, err := SourceFromInput(src)
			require.NoError(t, err)
			require.Equal(t, GitSource(src), block)

			kind, value, err := RepoConfig{Source: block}.ParseSource("/cfg")
			require.NoError(t, err)
			require.Equal(t, SourceGit, kind)
			require.Equal(t, src, value)
		})
	}
}

// Phase 1.1 criterion 3: an scp-style user@host:path value classifies as a
// git source and is returned unchanged.
func TestRepoConfig_ParseSource_ScpStyle(t *testing.T) {
	kind, value, err := RepoConfig{Source: GitSource("git@github.com:org/api.git")}.ParseSource("/cfg")
	require.NoError(t, err)
	require.Equal(t, SourceGit, kind)
	require.Equal(t, "git@github.com:org/api.git", value)
}

// A source declaring a provider that is neither file nor git is rejected
// with an error naming the unsupported provider.
func TestRepoConfig_ParseSource_UnsupportedProvider(t *testing.T) {
	kind, value, err := RepoConfig{Source: RepoSourceConfig{Provider: "s3", Config: RepoSourceLocation{Location: "bucket/code"}}}.ParseSource("/cfg")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported source provider")
	require.Contains(t, err.Error(), "s3")
	require.Equal(t, SourceNone, kind)
	require.Equal(t, "", value)
}

// Phase 1.1 criterion 4: a repo config that sets source round-trips through
// repo.yaml without losing it, alongside the other sections.
func TestRepoConfig_ToYAMLFileRoundTripWithSource(t *testing.T) {
	pinWriterVersion(t, "test-x")
	cfg := RepoConfig{
		Description: "Handles order processing and fulfillment.",
		Source:      GitSource("git@github.com:org/api.git"),
		Knowledge: RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config:   FileKnowledgeConfig{Location: "kb/repo"},
		},
		Changelog: ChangelogConfig{
			Provider: ProviderFile,
			Config:   FileChangelogConfig{Directory: "docs/changelog"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	// The written file is stamped with the current format and writer.
	want := cfg
	want.Schema = 2
	want.WrittenBy = "test-x"
	require.Equal(t, want, loaded)
	require.Equal(t, GitSource("git@github.com:org/api.git"), loaded.Source)
}

// Phase 2.1 criterion 1: a repo declares its knowledge store as a single
// provider block — no name, no label, no list — shaped exactly like the
// changelog block beside it. The expected file is written out by hand so a
// stray `sources:` list, a scope key, or a change of indentation fails here.
func TestRepoConfig_KnowledgeIsASingleProviderBlock(t *testing.T) {
	pinWriterVersion(t, "test-x")
	cfg := NewDefaultRepoConfig()
	cfg.Source = DefaultRepoSource

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "schema: 2\n"+
		"written_by: test-x\n"+
		"source:\n"+
		"    provider: file\n"+
		"    config:\n"+
		"        location: ..\n"+
		"knowledge:\n"+
		"    provider: file\n"+
		"    config:\n"+
		"        location: knowledge\n"+
		"changelog:\n"+
		"    provider: file\n"+
		"    config:\n"+
		"        directory: changelog\n", string(raw))

	loaded, err := RepoConfigFromYAMLFile(path)
	require.NoError(t, err)
	want := cfg
	want.Schema = 2
	want.WrittenBy = "test-x"
	require.Equal(t, want, loaded)
}

// Phase 2.3 criteria 1, 2, 3, 4 & 5: a repo.yaml that still declares its
// knowledge in the superseded form is refused on load. Both shapes are covered:
// a `sources` list — whether it carries more than one store or only one — and a
// single block that labels itself with the removed `scope` key. Neither shape
// has a field to land in on RepoConfig, so without the guard the file would
// parse cleanly and silently read the seeded default store instead.
//
// Each case asserts the refusal is a config_invalid *output.ErrorResponse
// naming the file and the key found, with a next action showing the required
// single provider block; that a second load fails identically and leaves the
// file byte-for-byte unchanged; and that the corrected form then loads.
func TestRepoConfigFromYAMLFile_LegacyKnowledgeBlockIsRejected(t *testing.T) {
	for name, tc := range map[string]struct {
		yaml  string
		found string
	}{
		// Criterion 4: a repo declaring more than one knowledge store.
		"two sources": {
			yaml: "knowledge:\n" +
				"  sources:\n" +
				"    - name: api\n" +
				"      provider: file\n" +
				"      config:\n" +
				"        location: knowledge\n" +
				"    - name: extra\n" +
				"      provider: file\n" +
				"      config:\n" +
				"        location: other-knowledge\n",
			found: "'sources'",
		},
		// Criterion 4: a single store, but still wrapped in the list form.
		"one source in a list": {
			yaml: "knowledge:\n" +
				"  sources:\n" +
				"    - name: api\n" +
				"      provider: file\n" +
				"      config:\n" +
				"        location: knowledge\n",
			found: "'sources'",
		},
		// Criterion 4: a single provider block that labels the store it
		// declares, which a repo has no business doing.
		"scope on the single block": {
			yaml: "knowledge:\n" +
				"  scope: repo\n" +
				"  provider: file\n" +
				"  config:\n" +
				"    location: knowledge\n",
			found: "'scope'",
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, RepoConfigFileName)
			require.NoError(t, os.WriteFile(path, []byte(withSchema(tc.yaml)), 0644))

			// Criterion 3: the file's bytes as they stand before any load.
			before, err := os.ReadFile(path)
			require.NoError(t, err)

			var first, second *output.ErrorResponse
			for _, envelope := range []**output.ErrorResponse{&first, &second} {
				_, err := RepoConfigFromYAMLFile(path)
				require.Error(t, err)
				require.True(t, errors.As(err, envelope), "expected an *output.ErrorResponse, got %T", err)
			}

			// Criterion 2: the message names the file and the key found.
			require.Equal(t, "config_invalid", first.Code)
			require.Contains(t, first.Message, path)
			require.Contains(t, first.Message, tc.found)
			require.Contains(t, first.Message, "exactly one knowledge store")
			require.Equal(t, path, first.Resource)
			// ...and the next action prints the single provider block now
			// required, addressed by the registry name rather than its own.
			require.Contains(t, first.NextAction, "knowledge:")
			require.Contains(t, first.NextAction, "provider: file")
			require.Contains(t, first.NextAction, "location: knowledge")
			require.Contains(t, first.NextAction, "takes no name of its own")

			// Criterion 3: the same failure, and the same bytes on disk.
			require.Equal(t, first, second)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after, "a refused repo config must not be rewritten on disk")

			// Criterion 5: the corrected single-block form loads cleanly, and
			// the store it declares is the one that is read.
			const corrected = "knowledge:\n" +
				"  provider: file\n" +
				"  config:\n" +
				"    location: other-knowledge\n"
			require.NoError(t, os.WriteFile(path, []byte(withSchema(corrected)), 0644))

			cfg, err := RepoConfigFromYAMLFile(path)
			require.NoError(t, err)
			require.Equal(t, "file", cfg.Knowledge.Provider)
			require.Equal(t, "other-knowledge", cfg.Knowledge.Config.Location)
		})
	}
}

// Phase 1.1 (schema versioning) criterion 1: a freshly written repo.yaml
// records the current format version and the running Spektacular version.
func TestRepoConfig_ToYAMLFileStampsSchemaAndWriter(t *testing.T) {
	pinWriterVersion(t, "test-x")

	dir := t.TempDir()
	path := filepath.Join(dir, RepoConfigFileName)
	require.NoError(t, NewDefaultRepoConfig().ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "schema: 2\n")
	require.Contains(t, string(raw), "written_by: test-x\n")
}
