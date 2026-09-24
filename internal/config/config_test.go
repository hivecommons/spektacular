package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/stretchr/testify/require"
)

// projectConfigPath lays out a project root with its settings folder and
// returns the root and the path config.yaml should be written to:
// <root>/.spektacular/config.yaml. Store directories in that file resolve
// against <root>/.spektacular.
func projectConfigPath(t *testing.T) (root, path string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, ".spektacular")
	require.NoError(t, os.MkdirAll(dir, 0755))
	return root, filepath.Join(dir, "config.yaml")
}

func TestNewDefault_HasExpectedDefaults(t *testing.T) {
	cfg := NewDefault()

	require.Equal(t, "spektacular", cfg.Command)
	require.False(t, cfg.Debug.Enabled)
	require.Equal(t, "timestamp", cfg.Spec.IDMethod)
	require.Equal(t, SpecTriggerThresholdModerate, cfg.SpecTriggerThreshold)
	require.Equal(t, AutoCommitOff, cfg.AutoCommit)
	require.Empty(t, cfg.Name)
	require.Empty(t, cfg.Knowledge.Sources)
}

func TestFromYAMLFile_LoadsAndExpandsEnvVars(t *testing.T) {
	t.Setenv("TEST_CMD", "go run .")

	yaml := `name: testproj
command: "${TEST_CMD}"
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "go run .", cfg.Command)
	require.Equal(t, "timestamp", cfg.Spec.IDMethod)
}

func TestFromYAMLFile_MissingSpecConfigUsesDefaults(t *testing.T) {
	yaml := `name: testproj
command: "go run ."
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "go run .", cfg.Command)
	require.Equal(t, "timestamp", cfg.Spec.IDMethod)
}

func TestFromYAMLFile_UnknownSpecIDMethodReturnsError(t *testing.T) {
	yaml := `name: testproj
spec:
  provider: file
  id_method: unsupported
  config:
    directory: specs
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.id_method")
}

func TestFromYAMLFile_UnknownSpecTriggerThresholdReturnsError(t *testing.T) {
	yaml := `name: testproj
spec_trigger_threshold: unsupported
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec_trigger_threshold")
}

// Phase 1.1 criterion 1: each of the three auto_commit values loads without
// error and is kept verbatim. The `off` fixture is deliberately unquoted: it
// is a YAML 1.1 boolean, and must still arrive as the Go string "off".
func TestFromYAMLFile_EachAutoCommitValueLoads(t *testing.T) {
	for _, want := range []string{"off", "workflow", "full"} {
		t.Run(want, func(t *testing.T) {
			yaml := "name: testproj\n" +
				"auto_commit: " + want + "\n" +
				"repos:\n" +
				"  - name: testproj\n" +
				"    location: ..\n"
			_, path := projectConfigPath(t)
			require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

			cfg, err := FromYAMLFile(path)
			require.NoError(t, err)
			require.Equal(t, want, cfg.AutoCommit)
		})
	}
}

// Phase 1.1 criterion 2: a project that never mentions auto_commit loads as
// off, so an existing project keeps working without an upgrade.
func TestFromYAMLFile_AbsentAutoCommitLoadsAsOff(t *testing.T) {
	yaml := `name: testproj
repos:
  - name: testproj
    location: ..`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, AutoCommitOff, cfg.AutoCommit)
}

// Phase 1.1 criterion 3: any other auto_commit value is refused with a
// config_invalid error that lists the three allowed values and names the key
// and file to correct.
func TestFromYAMLFile_UnknownAutoCommitReturnsError(t *testing.T) {
	yaml := `name: testproj
auto_commit: sometimes
repos:
  - name: testproj
    location: ..`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	_, err := FromYAMLFile(path)
	require.Error(t, err)

	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `auto_commit must be one of "off", "workflow", or "full"`, er.Message)
	require.Equal(t, "auto_commit", er.Resource)
	require.Equal(t, "set auto_commit in .spektacular/config.yaml to off, workflow or full (or remove the key to use off)", er.NextAction)
}

// Phase 1.1 criterion 4: `off` is a YAML 1.1 boolean, so saving the settings
// must write it back as text — quoted or bare — and reading it back must
// yield the Go string "off", never a boolean.
func TestToYAMLFile_AutoCommitOffIsWrittenAsText(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	_, path := projectConfigPath(t)

	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Regexp(t, `auto_commit: "?off"?\n`, string(raw))

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "off", loaded.AutoCommit)
}

// Phase 1.1 criterion 2: AutoCommitMode resolves an absent key to off and
// returns every configured value unchanged, so callers never special-case the
// empty string.
func TestAutoCommitMode_ResolvesAbsentKeyToOff(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "off"},
		{"off", "off"},
		{"workflow", "workflow"},
		{"full", "full"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, Config{AutoCommit: tc.in}.AutoCommitMode(), "AutoCommitMode() for %q", tc.in)
	}
}

func TestFromYAMLFile_MissingFile_ReturnsError(t *testing.T) {
	_, err := FromYAMLFile("/nonexistent/path/config.yaml")
	require.Error(t, err)
}

// Criterion 4: spec/plan/changelog configuration behaviour is unchanged by
// the project/repo split — the default config still round-trips.
func TestToYAMLFile_RoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := cfg.ToYAMLFile(path)
	require.NoError(t, err)

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Command, loaded.Command)
	require.Equal(t, cfg.Debug.Enabled, loaded.Debug.Enabled)
	require.Equal(t, cfg.Spec.IDMethod, loaded.Spec.IDMethod)
}

// Criterion 1: spec, plan, and knowledge each round-trip a provider plus
// config block through YAML, with knowledge carrying multiple independently
// configured sources.
func TestToYAMLFile_ProviderSectionsRoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Spec = SpecConfig{
		Provider: ProviderFile,
		IDMethod: SpecIDMethodCounter,
		Config: FileSpecConfig{
			Directory: "docs/specs",
		},
	}
	cfg.Plan = PlanConfig{
		Provider: ProviderFile,
		Config:   FilePlanConfig{Directory: "docs/plans"},
	}
	cfg.Changelog = ChangelogConfig{
		Provider: ProviderFile,
		Config:   FileChangelogConfig{Directory: "docs/changelog"},
	}
	cfg.Knowledge = KnowledgeConfig{
		Sources: []SourceConfig{
			{
				Name:     "project",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: ".spektacular/knowledge"},
			},
			{
				Name:     "team",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: "/shared/team/knowledge"},
			},
		},
	}

	_, path := projectConfigPath(t)

	err := cfg.ToYAMLFile(path)
	require.NoError(t, err)

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)

	// The store directories come back in their project-root-relative form;
	// compare field by field, since a loaded config also remembers the value
	// it read from the file.
	require.Equal(t, ProviderFile, loaded.Spec.Provider)
	require.Equal(t, SpecIDMethodCounter, loaded.Spec.IDMethod)
	require.Equal(t, "docs/specs", loaded.Spec.Config.Directory)
	require.Equal(t, ProviderFile, loaded.Plan.Provider)
	require.Equal(t, "docs/plans", loaded.Plan.Config.Directory)
	require.Equal(t, ProviderFile, loaded.Changelog.Provider)
	require.Equal(t, "docs/changelog", loaded.Changelog.Config.Directory)
	require.Equal(t, cfg.Knowledge, loaded.Knowledge)
}

// Criterion 2: a config with a section absent yields the documented default.
func TestFromYAMLFile_AbsentProviderSectionsUseDefaults(t *testing.T) {
	yaml := `name: testproj
command: "go run ."
repos:
  - name: testproj
    location: ..`
	_, path := projectConfigPath(t)
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)

	require.Equal(t, ProviderFile, cfg.Spec.Provider)
	require.Equal(t, ".spektacular/specs", cfg.Spec.Config.Directory)
	require.Equal(t, SpecIDMethodTimestamp, cfg.Spec.IDMethod)
	require.Equal(t, ProviderFile, cfg.Plan.Provider)
	require.Equal(t, ".spektacular/plans", cfg.Plan.Config.Directory)
	require.Equal(t, ProviderFile, cfg.Changelog.Provider)
	require.Equal(t, ".spektacular/changelog", cfg.Changelog.Config.Directory)
	// The project-level knowledge list holds only project-owned sources and is
	// empty by default; the repo's own store lives in RepoConfig.
	require.Empty(t, cfg.Knowledge.Sources)
}

// Criterion 3: an unknown provider is rejected with a clear validation error.
func TestFromYAMLFile_UnknownSpecProviderReturnsError(t *testing.T) {
	yaml := `name: testproj
spec:
  provider: bogus
  config:
    directory: specs
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.provider")
}

// Criterion 3: an empty required config field is rejected.
func TestFromYAMLFile_EmptySpecDirectoryReturnsError(t *testing.T) {
	yaml := `name: testproj
spec:
  provider: file
  config:
    directory: ""
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "spec.config.directory")
}

// Criterion 3: an unknown changelog provider is rejected with a clear
// validation error.
func TestFromYAMLFile_UnknownChangelogProviderReturnsError(t *testing.T) {
	yaml := `name: testproj
changelog:
  provider: bogus
  config:
    directory: changelog
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "changelog.provider")
}

// Criterion 3: an empty required changelog config field is rejected.
func TestFromYAMLFile_EmptyChangelogDirectoryReturnsError(t *testing.T) {
	yaml := `name: testproj
changelog:
  provider: file
  config:
    directory: ""
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "changelog.config.directory")
}

// Criterion 3: a knowledge source missing its required location is rejected.
func TestKnowledgeConfig_ValidateRejectsMissingLocation(t *testing.T) {
	knowledge := KnowledgeConfig{
		Sources: []SourceConfig{
			{Name: "project", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: ""}},
		},
	}

	err := knowledge.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "config.location")
}

// Phase 2.2 criterion 3: two shared stores declared under the same name are
// rejected with a structured config_invalid refusal that names the duplicate
// and tells the author to rename one of them.
func TestKnowledgeConfig_ValidateRejectsDuplicateName(t *testing.T) {
	knowledge := KnowledgeConfig{
		Sources: []SourceConfig{
			{Name: "team", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/a"}},
			{Name: "team", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/b"}},
		},
	}

	err := knowledge.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `knowledge.sources declares the name "team" more than once`, er.Message)
	require.NotEmpty(t, er.NextAction)
	require.Contains(t, er.NextAction, "rename")
}

// Phase 2.2 criterion 3: a shared store declared without a name is rejected
// the same way, naming the offending entry's position in the list.
func TestKnowledgeConfig_ValidateRejectsMissingName(t *testing.T) {
	knowledge := KnowledgeConfig{
		Sources: []SourceConfig{
			{Name: "team", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/a"}},
			{Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/b"}},
		},
	}

	err := knowledge.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, "knowledge.sources[1] declares no name", er.Message)
	require.NotEmpty(t, er.NextAction)
}

// Criterion 1: a project config file with a missing name fails validation
// with an error naming the field.
func TestFromYAMLFile_MissingNameReturnsError(t *testing.T) {
	yaml := `command: "go run ."
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "name")
}

// TestParseYAMLFile_MissingNameParsesWithoutValidation asserts that the
// unvalidated parse path reads a config with no name — the shape init must
// accept so it can backfill one — while still prefilling defaults, where the
// validating loader rejects the same file.
func TestParseYAMLFile_MissingNameParsesWithoutValidation(t *testing.T) {
	yaml := `command: "go run ."
repos:
  - name: testproj
    location: ..`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	cfg, err := ParseYAMLFile(path)
	require.NoError(t, err)
	require.Empty(t, cfg.Name)
	require.Equal(t, "go run .", cfg.Command)
	require.Equal(t, ".spektacular/specs", cfg.Spec.Config.Directory, "defaults are still prefilled")
}

// Criterion 1: a non-slug-safe name fails validation with an error naming
// the field.
func TestFromYAMLFile_NonSlugNameReturnsError(t *testing.T) {
	yaml := `name: "Has Spaces/UPPER"
repos:
  - name: testproj
    location: ..`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "name")
}

// Criterion 1: the project's name and optional source round-trip through
// config.yaml unchanged.
func TestToYAMLFile_NameAndSourceRoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "my-project"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Source = "github.com/example/my-project"
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "my-project", loaded.Name)
	require.Equal(t, "github.com/example/my-project", loaded.Source)
}

// Criterion 1: SlugifyName converts arbitrary directory basenames into
// slug-safe project names. The expected values are hand-maintained.
func TestSlugifyName_HandMaintainedCases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"My Project", "my-project"},
		{"spektacular", "spektacular"},
		{"Has Spaces/UPPER", "has-spaces-upper"},
		{"snake_case_ok", "snake_case_ok"},
		{"___", "project"},
		{"", "project"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, SlugifyName(tc.in), "SlugifyName(%q)", tc.in)
	}
}

// Criterion 1: every slugified name passes the same validation that
// config.yaml loading applies to the name field.
func TestSlugifyName_ProducesValidNames(t *testing.T) {
	for _, in := range []string{"My Project", "spektacular", "___", "9lives", "-lead-trim-"} {
		cfg := NewDefault()
		cfg.Name = SlugifyName(in)
		cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
		require.NoError(t, cfg.Validate(), "slugified %q should validate", in)
	}
}

// Criterion 1: a registry entry whose name duplicates an earlier entry fails
// validation with an error naming the duplicated entry.
func TestValidateRepos_DuplicateNameReturnsError(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Repos = []RepoEntry{
		{Name: "api", Location: "."},
		{Name: "api", Location: "./api"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), `repos: name "api" is configured more than once`)
}

// Criterion 1: a registry entry with a non-slug-safe name fails validation
// with an error naming the entry's index.
func TestValidateRepos_NonSlugNameReturnsError(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Repos = []RepoEntry{
		{Name: "api", Location: "."},
		{Name: "Has Spaces/UPPER", Location: "./other"},
	}

	err := cfg.Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "repos[1].name")
}

// Phase 1.2 criterion 3: a registry entry with neither location nor the
// deprecated local key fails validation with a config_invalid error naming
// the entry and pointing at the field to set.
func TestValidateRepos_MissingLocationReturnsError(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Repos = []RepoEntry{
		{Name: "api"},
	}

	err := cfg.Validate()
	require.Error(t, err)

	var envelope *output.ErrorResponse
	require.True(t, errors.As(err, &envelope), "expected an *output.ErrorResponse, got %T", err)
	require.Equal(t, "config_invalid", envelope.Code)
	require.Equal(t, `repo "api" has no location`, envelope.Message)
	require.Contains(t, envelope.NextAction, "repos[0].location")
}

// Phase 1.2 criterion 1: a registry entry declared with location, and one
// declared with the deprecated local alias, both load to the same entry with
// Location set and Local empty; writing the loaded config back emits only
// the location key.
func TestFromYAMLFile_LocationAndLocalAliasLoadIdentically(t *testing.T) {
	want := RepoEntry{Name: "api", Location: "./api"}

	for _, tc := range []struct {
		key  string
		yaml string
	}{
		{key: "location", yaml: "name: testproj\nrepos:\n  - name: api\n    location: ./api\n"},
		{key: "local", yaml: "name: testproj\nrepos:\n  - name: api\n    local: ./api\n"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(tc.yaml)), 0644))

			cfg, err := FromYAMLFile(path)
			require.NoError(t, err)
			require.Equal(t, []RepoEntry{want}, cfg.Repos)
			require.Empty(t, cfg.Repos[0].Local, "the alias must be folded into Location on load")

			out := filepath.Join(dir, "rewritten.yaml")
			require.NoError(t, cfg.ToYAMLFile(out))
			raw, err := os.ReadFile(out)
			require.NoError(t, err)
			require.Contains(t, string(raw), "location: ./api")
			require.NotContains(t, string(raw), "local:", "the deprecated alias must never be written back")
		})
	}
}

// Phase 1.2 criterion 2: a registry entry still carrying the removed address
// key is rejected on load — by ParseYAMLFile and therefore FromYAMLFile —
// with a config_invalid error naming the repo and the key, and a next action
// that relocates the value to the repo's own repo.yaml as source.
func TestFromYAMLFile_LegacyAddressKeyIsRejected(t *testing.T) {
	yaml := "name: testproj\nrepos:\n  - name: api\n    address: git@example.com:org/api.git\n    local: ./repos/api\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	for name, load := range map[string]func(string) (Config, error){
		"ParseYAMLFile": ParseYAMLFile,
		"FromYAMLFile":  FromYAMLFile,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := load(path)
			require.Error(t, err)

			var envelope *output.ErrorResponse
			require.True(t, errors.As(err, &envelope), "expected an *output.ErrorResponse, got %T", err)
			require.Equal(t, "config_invalid", envelope.Code)
			require.Contains(t, envelope.Message, `"api"`)
			require.Contains(t, envelope.Message, "'address'")
			require.Contains(t, envelope.NextAction, "repos[0].address")
			require.Contains(t, envelope.NextAction, "source: git@example.com:org/api.git")
			require.Contains(t, envelope.NextAction, "./repos/api/.spektacular/repo.yaml")
		})
	}
}

// Phase 1.2 criterion 4 (WithDefaults): an entry built with the deprecated
// Local field has it folded into Location, cleared, and the provider
// defaulted to git.
func TestRepoEntry_WithDefaultsFoldsLocalAlias(t *testing.T) {
	got := RepoEntry{Name: "x", Local: "./x"}.WithDefaults()
	require.Equal(t, RepoEntry{Name: "x", Location: "./x", Provider: ProviderGit}, got)
	require.Empty(t, got.Local)
}

// Criterion 2: an unknown repo provider is rejected at load time with an
// error naming the provider.
func TestFromYAMLFile_UnknownRepoProviderReturnsError(t *testing.T) {
	yaml := `name: testproj
repos:
  - name: api
    location: "."
    provider: svn`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	_, err = FromYAMLFile(path)
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider")
}

// Criterion 2: an entry with no provider is accepted at load time — the
// provider is optional and defaults to git.
func TestFromYAMLFile_EmptyRepoProviderIsAccepted(t *testing.T) {
	yaml := `name: testproj
repos:
  - name: api
    location: "."`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644)
	require.NoError(t, err)

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 1)
	require.Empty(t, cfg.Repos[0].Provider)
}

// Criterion 2: WithDefaults resolves an empty provider to git and leaves an
// explicit provider untouched.
func TestRepoEntry_WithDefaultsResolvesProviderToGit(t *testing.T) {
	entry := RepoEntry{Name: "api", Location: "."}
	require.Equal(t, ProviderGit, entry.WithDefaults().Provider)

	explicit := RepoEntry{Name: "api", Location: ".", Provider: ProviderGit}
	require.Equal(t, ProviderGit, explicit.WithDefaults().Provider)
}

// A two-entry registry with every membership field populated round-trips
// through config.yaml unchanged. RepoEntry carries membership only —
// descriptive metadata (description/role/tags) lives in the
// repo's own repo.yaml and is covered by internal/config/repo_test.go
// instead.
func TestToYAMLFile_ReposRoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Repos = []RepoEntry{
		{
			Name:         "api",
			Location:     "./api",
			Dependencies: []string{"db"},
			Provider:     ProviderGit,
		},
		{
			Name:     "db",
			Location: "./db",
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Repos, loaded.Repos)
}

// Criterion 3: a project config may declare project-owned knowledge sources,
// and a populated project-level knowledge list round-trips through
// config.yaml.
func TestToYAMLFile_ProjectOwnedKnowledgeSourcesRoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Knowledge = KnowledgeConfig{
		Sources: []SourceConfig{
			{
				Name:     "team",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: "/shared/team/knowledge"},
			},
			{
				Name:     "global",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: "/shared/global/knowledge"},
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, cfg.ToYAMLFile(path))

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Knowledge, loaded.Knowledge)
}

// Phase 1.1 criteria 1 & 5: a project may declare several named design
// sources, each naming a backend and a location, and the declaration
// round-trips through config.yaml unchanged. A design block does not change
// the settings format version.
func TestToYAMLFile_ProjectDesignSourcesRoundTrip(t *testing.T) {
	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	cfg.Design = DesignConfig{
		Sources: []SourceConfig{
			{
				Name:     "architecture",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: "/shared/architecture"},
			},
			{
				Name:     "product",
				Provider: ProviderFile,
				Config:   FileKnowledgeConfig{Location: "/shared/product/design"},
			},
		},
	}

	_, path := projectConfigPath(t)
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "design:\n")
	require.Contains(t, string(raw), "schema: 3\n", "a design block does not bump the settings format version")

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, cfg.Design, loaded.Design)
	require.Equal(t, 3, loaded.Schema)
}

// Phase 1.1 criterion 2: a project that declares no design sources loads as
// it did before the design section existed — the list is absent, not empty —
// and writing the config back emits no design key at all.
func TestFromYAMLFile_AbsentDesignSectionDeclaresNoSources(t *testing.T) {
	yaml := `name: testproj
command: "go run ."
repos:
  - name: testproj
    location: ..`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Nil(t, cfg.Design.Sources)

	require.NoError(t, cfg.ToYAMLFile(path))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "design:", "an undeclared design section must not be written back")
}

// Phase 1.1 criterion 4: a design source's relative location is a path
// relative to the folder holding config.yaml, and is read and written back
// exactly as declared — unlike the spec/plan/changelog store directories, it
// is never re-expressed on write.
func TestToYAMLFile_DesignSourceRelativeLocationIsWrittenAsDeclared(t *testing.T) {
	yaml := `name: testproj
repos:
  - name: testproj
    location: ..
design:
  sources:
    - name: architecture
      provider: file
      config:
        location: ../docs/design`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, "../docs/design", cfg.Design.Sources[0].Config.Location)

	require.NoError(t, cfg.ToYAMLFile(path))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "location: ../docs/design")
}

// Phase 1.1 criterion 3: an invalid design source is refused when the
// settings load, not only when DesignConfig.Validate is called directly.
func TestFromYAMLFile_InvalidDesignSourceIsRejectedOnLoad(t *testing.T) {
	yaml := `name: testproj
repos:
  - name: testproj
    location: ..
design:
  sources:
    - name: architecture
      provider: notion
      config:
        location: ../docs/design`
	_, path := projectConfigPath(t)
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	_, err := FromYAMLFile(path)
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `design source "architecture": provider "notion" is not supported (only "file")`, er.Message)
	require.NotEmpty(t, er.NextAction)
}

// Phase 1.1 criterion 3: a design source declared without a name is refused,
// naming the offending entry's position and the key to add.
func TestDesignConfig_ValidateRejectsMissingName(t *testing.T) {
	design := DesignConfig{
		Sources: []SourceConfig{
			{Name: "architecture", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/a"}},
			{Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/b"}},
		},
	}

	err := design.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, "design.sources[1] declares no name", er.Message)
	require.Contains(t, er.NextAction, "name:")
}

// Phase 1.1 criterion 3: two design sources declared under the same name are
// refused, naming the duplicate and telling the author to rename one.
func TestDesignConfig_ValidateRejectsDuplicateName(t *testing.T) {
	design := DesignConfig{
		Sources: []SourceConfig{
			{Name: "architecture", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/a"}},
			{Name: "architecture", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: "/b"}},
		},
	}

	err := design.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `design.sources declares the name "architecture" more than once`, er.Message)
	require.Contains(t, er.NextAction, "rename")
}

// Phase 1.1 criterion 3: a design source naming a backend this release does
// not ship is refused, with the key to correct.
func TestDesignConfig_ValidateRejectsUnsupportedProvider(t *testing.T) {
	design := DesignConfig{
		Sources: []SourceConfig{
			{Name: "architecture", Provider: "notion", Config: FileKnowledgeConfig{Location: "/a"}},
		},
	}

	err := design.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `design source "architecture": provider "notion" is not supported (only "file")`, er.Message)
	require.Contains(t, er.NextAction, "design.sources[0].provider")
}

// Phase 1.1 criterion 3: a design source with no location is refused, with
// the key to set.
func TestDesignConfig_ValidateRejectsMissingLocation(t *testing.T) {
	design := DesignConfig{
		Sources: []SourceConfig{
			{Name: "architecture", Provider: ProviderFile, Config: FileKnowledgeConfig{Location: ""}},
		},
	}

	err := design.Validate()
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Equal(t, `design source "architecture": config.location must not be empty`, er.Message)
	require.Contains(t, er.NextAction, "design.sources[0].config.location")
}

// A project must register at least one repo: an otherwise valid config with
// no repos entry fails validation with a config_invalid error that says so.
func TestFromYAMLFile_NoReposReturnsError(t *testing.T) {
	yaml := `name: testproj`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(yaml)), 0644))

	_, err := FromYAMLFile(path)
	require.Error(t, err)
	var er *output.ErrorResponse
	require.ErrorAs(t, err, &er)
	require.Equal(t, "config_invalid", er.Code)
	require.Contains(t, er.Message, "no repos are registered")
}

// Phase 2.3 criteria 1, 2, 3 & 5: a project config whose shared knowledge
// stores are still identified by the removed 'scope' key is refused on load —
// by ParseYAMLFile and therefore FromYAMLFile — with a config_invalid error
// naming the file and the key that was found, and a next action showing the
// replacement block. The refusal repeats identically on a second load, the
// file is byte-for-byte untouched by the failure, and the corrected form loads.
func TestParseYAMLFile_LegacyKnowledgeScopeKeyIsRejected(t *testing.T) {
	const legacy = "name: testproj\n" +
		"repos:\n" +
		"  - name: api\n" +
		"    location: ./repos/api\n" +
		"knowledge:\n" +
		"  sources:\n" +
		"    - scope: team\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: ./team-knowledge\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(legacy)), 0644))

	for name, load := range map[string]func(string) (Config, error){
		"ParseYAMLFile": ParseYAMLFile,
		"FromYAMLFile":  FromYAMLFile,
	} {
		t.Run(name, func(t *testing.T) {
			// Criterion 3: the file's bytes as they stand before any load.
			before, err := os.ReadFile(path)
			require.NoError(t, err)

			// Load twice: the guard must be a pure refusal, not a one-shot
			// that repairs the file behind the caller's back.
			var first, second *output.ErrorResponse
			for _, envelope := range []**output.ErrorResponse{&first, &second} {
				_, err := load(path)
				require.Error(t, err)
				require.True(t, errors.As(err, envelope), "expected an *output.ErrorResponse, got %T", err)
			}

			// Criterion 2: the message names the file, the offending source,
			// and the key that was found.
			require.Equal(t, "config_invalid", first.Code)
			require.Contains(t, first.Message, path)
			require.Contains(t, first.Message, "knowledge.sources[0]")
			require.Contains(t, first.Message, "'scope'")
			require.Equal(t, path, first.Resource)
			// ...and the next action shows the block that is now required.
			require.Contains(t, first.NextAction, "knowledge:")
			require.Contains(t, first.NextAction, "sources:")
			require.Contains(t, first.NextAction, "- name: team")

			// Criterion 3: the same failure, and the same bytes on disk.
			require.Equal(t, first, second)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after, "a refused config must not be rewritten on disk")
		})
	}

	// Criterion 5: once the file is edited into the required form, the same
	// load succeeds and the store is addressed by the name it now carries.
	const corrected = "name: testproj\n" +
		"repos:\n" +
		"  - name: api\n" +
		"    location: ./repos/api\n" +
		"knowledge:\n" +
		"  sources:\n" +
		"    - name: team\n" +
		"      provider: file\n" +
		"      config:\n" +
		"        location: ./team-knowledge\n"
	require.NoError(t, os.WriteFile(path, []byte(withProjectSchema(corrected)), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, []SourceConfig{{
		Name:     "team",
		Provider: "file",
		Config:   FileKnowledgeConfig{Location: "./team-knowledge"},
	}}, cfg.Knowledge.Sources)
}

// Schema versioning criterion 1: a freshly written config.yaml records the
// current format version and the running Spektacular version.
func TestToYAMLFile_StampsSchemaAndWriter(t *testing.T) {
	pinWriterVersion(t, "test-x")

	cfg := NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []RepoEntry{{Name: "testproj", Location: ".."}}
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "schema: 3\n")
	require.Contains(t, string(raw), "written_by: test-x\n")

	loaded, err := FromYAMLFile(path)
	require.NoError(t, err)
	require.Equal(t, 3, loaded.Schema)
	require.Equal(t, "test-x", loaded.WrittenBy)
}

// Schema versioning criterion 2: rewriting config.yaml for an unrelated
// reason (registering a repo) leaves the recorded installed-skills version
// unchanged.
func TestToYAMLFile_PreservesSkillsVersion(t *testing.T) {
	pinWriterVersion(t, "test-x")

	body := "schema: 3\n" +
		"written_by: 0.9.0\n" +
		"skills_version: 0.9.0\n" +
		"name: testproj\n" +
		"repos:\n" +
		"  - name: testproj\n" +
		"    location: ..\n"
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0644))

	cfg, err := FromYAMLFile(path)
	require.NoError(t, err)
	cfg.Repos = append(cfg.Repos, RepoEntry{Name: "api", Location: "../api"})
	require.NoError(t, cfg.ToYAMLFile(path))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "skills_version: 0.9.0\n")
	require.Contains(t, string(raw), "written_by: test-x\n")
	require.Contains(t, string(raw), "location: ../api\n")
}

// A missing or non-positive schema value is the oldest format, 1; any
// positive value is kept as is.
func TestNormaliseSchema(t *testing.T) {
	require.Equal(t, 1, NormaliseSchema(0))
	require.Equal(t, 1, NormaliseSchema(-4))
	require.Equal(t, 1, NormaliseSchema(1))
	require.Equal(t, 2, NormaliseSchema(2))
	require.Equal(t, 3, NormaliseSchema(3))
}

// PeekSchema reads only the schema key: a file without one is format 1, a
// file that declares one reports it, and a missing file is an error.
func TestPeekSchema(t *testing.T) {
	dir := t.TempDir()

	unversioned := filepath.Join(dir, "unversioned.yaml")
	require.NoError(t, os.WriteFile(unversioned, []byte("name: testproj\n"), 0644))
	n, err := PeekSchema(unversioned)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	// The rest of the file is not validated, so an otherwise-invalid body
	// still reports its declared format.
	future := filepath.Join(dir, "future.yaml")
	require.NoError(t, os.WriteFile(future, []byte("schema: 3\nunknown_key: [1, 2]\n"), 0644))
	n, err = PeekSchema(future)
	require.NoError(t, err)
	require.Equal(t, 3, n)

	_, err = PeekSchema(filepath.Join(dir, "missing.yaml"))
	require.Error(t, err)
}
