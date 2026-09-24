package repo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/stretchr/testify/require"
)

// newRegisterProject lays out an empty project on disk — the .spektacular
// folder registration writes config.yaml into — and returns its root
// alongside a valid, repo-less project config. Validation only passes once a
// repo has been registered, which is exactly what Register does.
func newRegisterProject(t *testing.T) (string, *config.Config) {
	t.Helper()
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".spektacular"), 0o755))
	cfg := config.NewDefault()
	cfg.Name = "testproj"
	return projectRoot, &cfg
}

// projectConfigPath is the hand-maintained location of a project's config.yaml.
func projectConfigPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".spektacular", "config.yaml")
}

// Registering a repo that is not yet in the registry appends its entry,
// records the location as the .spektacular folder inside the repo's code,
// creates the footprint there, and writes the project config to disk.
func TestRegister_FreshRegistrationAppendsCreatesFootprintAndWritesConfig(t *testing.T) {
	projectRoot, cfg := newRegisterProject(t)
	code := t.TempDir()
	git := newFakeGit(t)

	res, err := Register(cfg, projectRoot, git, Registration{
		Name:        "lib",
		Location:    code,
		Description: "the library repo",
	})
	require.NoError(t, err)
	require.True(t, res.Registered)
	require.Equal(t, FootprintCreated, res.Footprint)
	require.Empty(t, res.MetadataNote, "a described repo raises no metadata notice")

	// The in-memory registry gained exactly one entry, located at the
	// .spektacular folder inside the repo's code.
	footprint := filepath.Join(code, ".spektacular")
	require.Len(t, cfg.Repos, 1)
	require.Equal(t, "lib", cfg.Repos[0].Name)
	require.Equal(t, footprint, cfg.Repos[0].Location)

	// The footprint was scaffolded at that location.
	require.FileExists(t, filepath.Join(footprint, config.RepoConfigFileName))
	for _, cat := range footprintCategories {
		require.FileExists(t, filepath.Join(footprint, "knowledge", cat, "README.md"))
	}

	// The repo's own config carries the description registration was given.
	repoCfg, err := config.RepoConfigFromYAMLFile(filepath.Join(footprint, config.RepoConfigFileName))
	require.NoError(t, err)
	require.Equal(t, "the library repo", repoCfg.Description)

	// The project config was written, and reloads carrying the new entry.
	written, err := config.FromYAMLFile(projectConfigPath(projectRoot))
	require.NoError(t, err)
	require.Len(t, written.Repos, 1)
	require.Equal(t, "lib", written.Repos[0].Name)
	require.Equal(t, footprint, written.Repos[0].Location)
}

// Registering an identical entry a second time changes nothing: the project
// config is not rewritten and the footprint is reported unchanged.
func TestRegister_IdenticalEntryIsNoOp(t *testing.T) {
	projectRoot, cfg := newRegisterProject(t)
	code := t.TempDir()
	git := newFakeGit(t)

	in := Registration{Name: "lib", Location: code, Description: "the library repo"}

	_, err := Register(cfg, projectRoot, git, in)
	require.NoError(t, err)

	// Backdate config.yaml to a known time so a rewrite is unmistakable, and
	// keep its bytes for comparison.
	cfgPath := projectConfigPath(projectRoot)
	stamp := time.Date(2020, time.January, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, os.Chtimes(cfgPath, stamp, stamp))
	before, err := os.ReadFile(cfgPath)
	require.NoError(t, err)

	res, err := Register(cfg, projectRoot, git, in)
	require.NoError(t, err)
	require.True(t, res.Registered)
	require.Equal(t, FootprintUnchanged, res.Footprint)

	info, err := os.Stat(cfgPath)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(stamp), "an identical registration must not rewrite the project config")

	after, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Equal(t, before, after)

	require.Len(t, cfg.Repos, 1, "an identical registration must not append a duplicate")
}

// Re-registering the same name with different dependencies updates that
// entry in place rather than appending a second one, and the change reaches
// disk.
func TestRegister_ChangedEntryUpdatesInPlace(t *testing.T) {
	projectRoot, cfg := newRegisterProject(t)
	code := t.TempDir()
	git := newFakeGit(t)

	_, err := Register(cfg, projectRoot, git, Registration{
		Name:         "lib",
		Location:     code,
		Dependencies: []string{"core"},
	})
	require.NoError(t, err)
	require.Len(t, cfg.Repos, 1)

	_, err = Register(cfg, projectRoot, git, Registration{
		Name:         "lib",
		Location:     code,
		Dependencies: []string{"core", "utils"},
	})
	require.NoError(t, err)

	require.Len(t, cfg.Repos, 1, "re-registering a name must update in place, never append")
	require.Equal(t, []string{"core", "utils"}, cfg.Repos[0].Dependencies)

	written, err := config.FromYAMLFile(projectConfigPath(projectRoot))
	require.NoError(t, err)
	require.Len(t, written.Repos, 1)
	require.Equal(t, []string{"core", "utils"}, written.Repos[0].Dependencies)
}

// A location whose footprint is only partly there — a valid repo.yaml but no
// knowledge tree — is topped up, and registration reports the repair.
func TestRegister_PartialFootprintReportsRepaired(t *testing.T) {
	projectRoot, cfg := newRegisterProject(t)
	code := t.TempDir()
	footprint := filepath.Join(code, ".spektacular")
	writeFootprint(t, footprint) // repo.yaml only, no knowledge tree

	res, err := Register(cfg, projectRoot, newFakeGit(t), Registration{
		Name:        "lib",
		Location:    code,
		Description: "the library repo",
	})
	require.NoError(t, err)
	require.Equal(t, FootprintRepaired, res.Footprint)

	for _, cat := range footprintCategories {
		require.FileExists(t, filepath.Join(footprint, "knowledge", cat, "README.md"))
	}
}

// The metadata notice fires only when description, role and tags are all
// empty; supplying any one of the three silences it.
func TestRegister_MetadataNoteOnlyWhenAllDescriptiveFieldsEmpty(t *testing.T) {
	t.Run("no descriptive metadata", func(t *testing.T) {
		projectRoot, cfg := newRegisterProject(t)

		res, err := Register(cfg, projectRoot, newFakeGit(t), Registration{
			Name:     "lib",
			Location: t.TempDir(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, res.MetadataNote)
		require.Contains(t, res.MetadataNote, "lib")
	})

	cases := []struct {
		name string
		in   Registration
	}{
		{name: "description only", in: Registration{Description: "the library repo"}},
		{name: "role only", in: Registration{Role: "library"}},
		{name: "tags only", in: Registration{Tags: []string{"go"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectRoot, cfg := newRegisterProject(t)
			in := tc.in
			in.Name = "lib"
			in.Location = t.TempDir()

			res, err := Register(cfg, projectRoot, newFakeGit(t), in)
			require.NoError(t, err)
			require.Empty(t, res.MetadataNote, "any one descriptive field silences the notice")
		})
	}
}

// DescriptiveFieldsEmpty is true only for a config with no description, role
// or tags at all. A source is a location, not a description, so it does not
// count as metadata.
func TestDescriptiveFieldsEmpty(t *testing.T) {
	withSource := config.NewDefaultRepoConfig()
	withSource.Source = config.DefaultRepoSource

	withDescription := config.NewDefaultRepoConfig()
	withDescription.Description = "the library repo"

	withRole := config.NewDefaultRepoConfig()
	withRole.Role = "library"

	withTags := config.NewDefaultRepoConfig()
	withTags.Tags = []string{"go"}

	cases := []struct {
		name string
		cfg  config.RepoConfig
		want bool
	}{
		{name: "zero value", cfg: config.RepoConfig{}, want: true},
		{name: "defaults only", cfg: config.NewDefaultRepoConfig(), want: true},
		{name: "source only", cfg: withSource, want: true},
		{name: "description", cfg: withDescription, want: false},
		{name: "role", cfg: withRole, want: false},
		{name: "tags", cfg: withTags, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, DescriptiveFieldsEmpty(tc.cfg))
		})
	}
}

// Phase 2.1: registering a location whose repo.yaml was written by a newer
// Spektacular fails with a FormatError, and the repo.yaml is left
// byte-identical rather than being treated as broken and rewritten.
func TestRegister_NewerFormatRepoYAMLIsRefusedAndUntouched(t *testing.T) {
	projectRoot, cfg := newRegisterProject(t)
	code := t.TempDir()
	footprint := filepath.Join(code, ".spektacular")
	require.NoError(t, os.MkdirAll(footprint, 0o755))
	path := filepath.Join(footprint, config.RepoConfigFileName)
	const body = "schema: 99\n" +
		"description: from the future\n"
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	_, err := Register(cfg, projectRoot, newFakeGit(t), Registration{
		Name:        "lib",
		Location:    code,
		Description: "the library repo",
	})
	require.Error(t, err)
	fe, ok := config.IsFormatError(err)
	require.True(t, ok, "expected a *config.FormatError, got %T: %v", err, err)
	require.True(t, fe.Newer())
	require.Equal(t, path, fe.Path)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, string(after), "a newer-format repo.yaml must never be rewritten")
	require.NoDirExists(t, filepath.Join(footprint, "knowledge"))

	require.NoFileExists(t, projectConfigPath(projectRoot), "a refused registration must not record the repo in config.yaml")
}
