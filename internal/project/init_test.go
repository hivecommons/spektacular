package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/knowledge"
	"github.com/stretchr/testify/require"
)

// mustInit runs Init and fails the test on error, discarding notices. Tests
// that assert on notices call Init directly.
func mustInit(t *testing.T, dir, name string, force bool) {
	t.Helper()
	_, err := Init(dir, name, force)
	require.NoError(t, err)
}

func TestInit_CreatesDirectoryStructure(t *testing.T) {
	dir := t.TempDir()

	_, err := Init(dir, "", false)
	require.NoError(t, err)

	expectedDirs := []string{
		".spektacular",
		".spektacular/plans",
		".spektacular/specs",
		".spektacular/knowledge",
		".spektacular/knowledge/learnings",
		".spektacular/knowledge/architecture",
		".spektacular/knowledge/gotchas",
	}
	for _, d := range expectedDirs {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, "expected dir %s to exist", d)
		require.True(t, info.IsDir(), "%s should be a directory", d)
	}
}

func TestInit_CreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	configPath := filepath.Join(dir, ".spektacular", "config.yaml")
	_, err = os.Stat(configPath)
	require.NoError(t, err, "config.yaml should exist")

	cfg, err := config.FromYAMLFile(configPath)
	require.NoError(t, err)
	require.Equal(t, "timestamp", cfg.Spec.IDMethod)
}

func TestInit_CreatesGitignore(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	gitignorePath := filepath.Join(dir, ".spektacular", ".gitignore")
	_, err = os.Stat(gitignorePath)
	require.NoError(t, err)
}

func TestInit_SeedsConventionsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	conventionsDir := filepath.Join(dir, ".spektacular", "knowledge", "conventions")
	info, err := os.Stat(conventionsDir)
	require.NoError(t, err, "conventions directory should exist")
	require.True(t, info.IsDir(), "conventions should be a directory")
}

func TestInit_CreatesKnowledgeREADMEs(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	for _, c := range knowledge.Categories {
		readmePath := filepath.Join(dir, ".spektacular", "knowledge", c.Name, "README.md")
		data, err := os.ReadFile(readmePath)
		require.NoError(t, err, "README.md should exist in %s", c.Name)
		require.Contains(t, string(data), "**Purpose:**", "README for %s should be rendered from its registry definition", c.Name)
	}
}

// TestInit_CreatesNewCategoryDirsWithoutCircularReadmes asserts the Phase 1.2
// acceptance criteria: Init creates the two new category directories (glossary
// and decisions) under .spektacular/knowledge, and no category README carries
// the old circular placeholder text.
func TestInit_CreatesNewCategoryDirsWithoutCircularReadmes(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "", false)

	for _, name := range []string{"glossary", "decisions"} {
		info, err := os.Stat(filepath.Join(dir, ".spektacular", "knowledge", name))
		require.NoError(t, err, "expected new category dir %s to exist", name)
		require.True(t, info.IsDir(), "%s should be a directory", name)
	}

	for _, c := range knowledge.Categories {
		readmePath := filepath.Join(dir, ".spektacular", "knowledge", c.Name, "README.md")
		data, err := os.ReadFile(readmePath)
		require.NoError(t, err, "README.md should exist in %s", c.Name)
		require.NotContains(t, string(data), "This directory contains", "README for %s should not carry the old circular placeholder", c.Name)
	}
}

func TestInit_AlreadyExists_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	_, err = Init(dir, "", false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestInit_Force_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	_, err := Init(dir, "", false)
	require.NoError(t, err)

	_, err = Init(dir, "", true)
	require.NoError(t, err)
}

// TestInit_DefaultConfig_CreatesSpecsAndPlansDirs asserts that with default
// config Init still creates the conventional specs/plans directories
// (Phase 2.2, criterion 3).
func TestInit_DefaultConfig_CreatesSpecsAndPlansDirs(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "", false)

	for _, d := range []string{".spektacular/specs", ".spektacular/plans"} {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, "expected default dir %s", d)
		require.True(t, info.IsDir())
	}
}

// TestInit_NonDefaultConfig_CreatesConfiguredDirs writes a config.yaml with
// non-default spec/plan directories into a temp project, runs Init, and asserts
// the configured directories are created on disk (Phase 2.2, criterion 3).
// Spec and plan directories are project-root-relative paths.
func TestInit_NonDefaultConfig_CreatesConfiguredDirs(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	cfg := config.NewDefault()
	cfg.Name = "testproj"
	cfg.Spec.Config.Directory = "my-specs"
	cfg.Plan.Config.Directory = "my-plans"
	require.NoError(t, cfg.ToYAMLFile(filepath.Join(spektacularDir, "config.yaml")))

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	for _, d := range []string{"my-specs", "my-plans"} {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, "expected configured dir %s", d)
		require.True(t, info.IsDir(), "%s should be a directory", d)
	}
	// The default directories must NOT be created.
	_, err := os.Stat(filepath.Join(dir, config.DefaultSpecDir))
	require.True(t, os.IsNotExist(err), "default specs dir should not be created")
	_, err = os.Stat(filepath.Join(dir, config.DefaultPlanDir))
	require.True(t, os.IsNotExist(err), "default plans dir should not be created")
}

// TestInit_ScaffoldsCategoriesAtConfiguredLocation asserts that when the repo
// config's knowledge store is configured at a non-default location, the
// knowledge base — every category directory and its README definition — is
// scaffolded there, and not at the hardcoded .spektacular/knowledge path.
func TestInit_ScaffoldsCategoriesAtConfiguredLocation(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Knowledge = config.RepoKnowledgeConfig{
		Provider: config.ProviderFile,
		Config:   config.FileKnowledgeConfig{Location: "kb"},
	}
	require.NoError(t, repoCfg.ToYAMLFile(filepath.Join(spektacularDir, config.RepoConfigFileName)))

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	// Every category directory and its README definition exist under the
	// configured location.
	for _, c := range knowledge.Categories {
		readme := filepath.Join(dir, ".spektacular", "kb", c.Name, "README.md")
		data, err := os.ReadFile(readme)
		require.NoError(t, err, "category %s should be scaffolded at the configured location", c.Name)
		require.Contains(t, string(data), "**Purpose:**")
	}

	// The hardcoded default location is not created when config points elsewhere.
	_, err := os.Stat(filepath.Join(dir, ".spektacular", "knowledge"))
	require.True(t, os.IsNotExist(err), "default .spektacular/knowledge should not be created when the location is customised")
}

// TestInit_DefaultConfig_CreatesProjectKnowledgeDir asserts that with no
// knowledge config the synthesised default project source directory exists.
func TestInit_DefaultConfig_CreatesProjectKnowledgeDir(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "", false)

	info, err := os.Stat(filepath.Join(dir, ".spektacular", "knowledge"))
	require.NoError(t, err, "default project knowledge directory should exist")
	require.True(t, info.IsDir())

	// repo.yaml's relative location resolves from its own folder, so nothing
	// is scaffolded at the project root.
	_, err = os.Stat(filepath.Join(dir, "knowledge"))
	require.True(t, os.IsNotExist(err), "init must not create a knowledge base at the project root")
}

// Criterion 1: after init, the project directory holds both config files,
// each containing only its own level's settings — config.yaml carries the
// project identity with no knowledge sources seeded, and repo.yaml carries
// knowledge and changelog with nothing project-level.
func TestInit_WritesProjectAndRepoConfigsWithOwnSettings(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "", false)

	// config.yaml: identity present, no knowledge sources seeded by default.
	configPath := filepath.Join(dir, ".spektacular", "config.yaml")
	cfg, err := config.FromYAMLFile(configPath)
	require.NoError(t, err)
	require.NotEmpty(t, cfg.Name, "config.yaml must record the project name")
	require.Empty(t, cfg.Knowledge.Sources, "config.yaml must not seed knowledge sources by default — the repo's own store lives in repo.yaml")
	rawConfig, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(rawConfig), "name:")

	// repo.yaml: the repo's own knowledge store and changelog, nothing more.
	repoPath := filepath.Join(dir, ".spektacular", config.RepoConfigFileName)
	repoCfg, err := config.RepoConfigFromYAMLFile(repoPath)
	require.NoError(t, err)
	require.Equal(t, config.RepoKnowledgeConfig{
		Provider: config.ProviderFile,
		Config:   config.FileKnowledgeConfig{Location: config.DefaultRepoKnowledgeLocation},
	}, repoCfg.Knowledge)
	require.Equal(t, config.ProviderFile, repoCfg.Changelog.Provider)
	require.Equal(t, config.DefaultRepoChangelogDir, repoCfg.Changelog.Config.Directory)

	rawRepo, err := os.ReadFile(repoPath)
	require.NoError(t, err)
	require.Contains(t, string(rawRepo), "knowledge:")
	require.Contains(t, string(rawRepo), "changelog:")
	for _, projectField := range []string{"name:", "spec:", "plan:"} {
		require.NotContains(t, string(rawRepo), projectField,
			"repo.yaml must not carry the project-level %s setting", projectField)
	}
}

// TestInit_DerivesSlugNameFromDirBasename asserts that with no explicit name
// and no stored config, the project name is the slugified directory basename.
func TestInit_DerivesSlugNameFromDirBasename(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Cool Project")
	require.NoError(t, os.MkdirAll(dir, 0755))

	mustInit(t, dir, "", false)

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "my-cool-project", cfg.Name)
}

// TestInit_ExplicitNameSetsProjectName asserts that a non-empty name argument
// wins over basename derivation.
func TestInit_ExplicitNameSetsProjectName(t *testing.T) {
	dir := t.TempDir()

	mustInit(t, dir, "custom-name", false)

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "custom-name", cfg.Name)
}

// TestInit_BackfillsMissingNamePreservingOtherSettings asserts that a
// pre-existing config.yaml with no name gets the slugified basename backfilled
// while the file's other settings are preserved.
func TestInit_BackfillsMissingNamePreservingOtherSettings(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backfill-proj")
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// A hand-written config with no name and a non-default spec directory.
	raw := "spec:\n  provider: file\n  config:\n    directory: docs/specs\n"
	require.NoError(t, os.WriteFile(filepath.Join(spektacularDir, "config.yaml"), []byte(raw), 0644))

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	cfg, err := config.FromYAMLFile(filepath.Join(spektacularDir, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "backfill-proj", cfg.Name, "name should be backfilled from the directory basename")
	require.Equal(t, "docs/specs", cfg.Spec.Config.Directory, "existing settings must survive the backfill")
}

// TestInit_ExplicitNameOverridesStoredName asserts that an explicit name on a
// re-init replaces the name already recorded in config.yaml.
func TestInit_ExplicitNameOverridesStoredName(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "old-name", false)

	mustInit(t, dir, "new-name", true)

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "new-name", cfg.Name)
}

// TestInit_KeepsStoredNameWithoutExplicitName asserts that a re-init with no
// explicit name keeps the stored name rather than re-deriving it from the
// directory basename.
func TestInit_KeepsStoredNameWithoutExplicitName(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "stored-name", false)

	mustInit(t, dir, "", true)

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, "stored-name", cfg.Name)
}

// Criterion 3: a freshly initialized project's config registers its
// colocated repo as the single registry entry, named after the project and
// located at ".".
func TestInit_SeedsColocatedRepoInRegistry(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "my-project", false)

	cfg, err := config.FromYAMLFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, []config.RepoEntry{{Name: "my-project", Location: "."}}, cfg.Repos)
}

// Phase 1.2 criterion 4: a freshly initialized project's config.yaml
// registers the colocated repo under the location key — never the
// deprecated local alias or the removed address key.
func TestInit_WritesLocationKeyForColocatedRepo(t *testing.T) {
	dir := t.TempDir()
	mustInit(t, dir, "my-project", false)

	raw, err := os.ReadFile(filepath.Join(dir, ".spektacular", "config.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "repos:\n")
	require.Contains(t, string(raw), "location: .")
	require.NotContains(t, string(raw), "local:")
	require.NotContains(t, string(raw), "address:")
}

// Criterion 3: re-running init on a config that already has registry entries
// leaves them unchanged rather than re-seeding the colocated repo.
func TestInit_ExistingReposLeftUnchanged(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	cfg := config.NewDefault()
	cfg.Name = "testproj"
	cfg.Repos = []config.RepoEntry{
		{Name: "api", Location: "./api"},
		{Name: "db", Location: "./db"},
	}
	require.NoError(t, cfg.ToYAMLFile(filepath.Join(spektacularDir, "config.yaml")))

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	loaded, err := config.FromYAMLFile(filepath.Join(spektacularDir, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, cfg.Repos, loaded.Repos, "a populated registry must survive re-init unchanged")
}

// Criterion 3: an older config with a name but no registry gets the colocated
// repo seeded on re-init, while its other settings are preserved.
func TestInit_SeedsReposIntoOlderConfigWithoutRegistry(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	// An older config: name present, repos absent.
	raw := "name: legacy-proj\nspec:\n  provider: file\n  config:\n    directory: docs/specs\n"
	require.NoError(t, os.WriteFile(filepath.Join(spektacularDir, "config.yaml"), []byte(raw), 0644))

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	cfg, err := config.FromYAMLFile(filepath.Join(spektacularDir, "config.yaml"))
	require.NoError(t, err)
	require.Equal(t, []config.RepoEntry{{Name: "legacy-proj", Location: "."}}, cfg.Repos,
		"the colocated repo should be seeded into an older config's empty registry")
	require.Equal(t, "docs/specs", cfg.Spec.Config.Directory, "existing settings must survive the seeding")
}

// TestInit_ExistingRepoConfigLeftUntouched asserts that a pre-existing
// repo.yaml is never rewritten by init, and that its knowledge store — not
// the defaults — drives the scaffolding.
func TestInit_ExistingRepoConfigLeftUntouched(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Knowledge.Config.Location = "custom-kb"
	repoPath := filepath.Join(spektacularDir, config.RepoConfigFileName)
	require.NoError(t, repoCfg.ToYAMLFile(repoPath))
	before, err := os.ReadFile(repoPath)
	require.NoError(t, err)

	// Force is required because .spektacular already exists.
	mustInit(t, dir, "", true)

	after, err := os.ReadFile(repoPath)
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "an existing repo.yaml must not be rewritten")

	info, err := os.Stat(filepath.Join(dir, ".spektacular", "custom-kb"))
	require.NoError(t, err, "scaffolding should follow the existing repo config's location")
	require.True(t, info.IsDir())
}

// A fresh project has no pre-existing repo.yaml, so the auto-seeded
// colocated repo (Location: ".") ends up with no descriptive metadata. Init
// must surface a notice naming it, warn-only, alongside the footprint
// creation.
func TestInit_ColocatedRepoNoMetadata_ReturnsNotice(t *testing.T) {
	dir := t.TempDir()

	notices, err := Init(dir, "my-project", false)
	require.NoError(t, err)

	found := false
	for _, n := range notices {
		if strings.Contains(n, "no descriptive metadata set") && strings.Contains(n, `"my-project"`) {
			found = true
		}
	}
	require.True(t, found, "expected a notice naming the colocated repo's missing descriptive metadata, got: %v", notices)
}

// A pre-existing repo.yaml with descriptive metadata already populated must
// not trigger the "no descriptive metadata set" notice for that repo.
func TestInit_ColocatedRepoWithMetadata_ReturnsNoMetadataNotice(t *testing.T) {
	dir := t.TempDir()
	spektacularDir := filepath.Join(dir, ".spektacular")
	require.NoError(t, os.MkdirAll(spektacularDir, 0755))

	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Description = "the main project repo"
	repoCfg.Role = "primary"
	require.NoError(t, repoCfg.ToYAMLFile(filepath.Join(spektacularDir, config.RepoConfigFileName)))

	notices, err := Init(dir, "my-project", true)
	require.NoError(t, err)

	for _, n := range notices {
		require.NotContains(t, n, "no descriptive metadata set", "a repo with descriptive metadata already set must not receive the notice")
	}
}
