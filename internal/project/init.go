// Package project handles Spektacular project initialisation.
package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/repo"
	"github.com/hivecommons/spektacular/templates"
)

// Init creates the .spektacular directory structure in projectPath,
// establishing the project's identity and seeding the colocated repo's own
// config (a solo repo is a project of one). The project name is resolved as:
// the explicit name argument when non-empty, then the name already recorded
// in an existing config.yaml, then the slugified directory basename. After
// project scaffolding, Init cascades over the registered repos, creating or
// repairing each one's minimal footprint — never cloning: repos not
// materialized locally are skipped with a returned notice. If force is
// false and the directory already exists, an error is returned.
func Init(projectPath, name string, force bool) ([]string, error) {
	spektacularDir := filepath.Join(projectPath, ".spektacular")

	if _, err := os.Stat(spektacularDir); err == nil && !force {
		return nil, fmt.Errorf(".spektacular directory already exists at %s; use --force to overwrite", spektacularDir)
	}

	// Resolve the spec, plan, and knowledge configuration from an existing
	// config.yaml, or fall back to the defaults when no config exists yet.
	// The parse is deliberately unvalidated: a pre-existing config without a
	// name must be readable here so init can backfill one.
	configPath := filepath.Join(spektacularDir, "config.yaml")
	cfg := config.NewDefault()
	configExisted := false
	if _, err := os.Stat(configPath); err == nil {
		loaded, err := config.ParseYAMLFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("loading config: %w", err)
		}
		cfg = loaded
		configExisted = true
	}

	storedName := cfg.Name
	if name != "" {
		cfg.Name = name
	} else if cfg.Name == "" {
		cfg.Name = config.SlugifyName(filepath.Base(projectPath))
	}

	// Every project is uniformly a collection of repos: seed the colocated
	// repo as the first registry entry when none are registered yet.
	reposSeeded := false
	if len(cfg.Repos) == 0 {
		cfg.Repos = []config.RepoEntry{{Name: cfg.Name, Location: "."}}
		reposSeeded = true
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	// The colocated repo's own configuration: loaded when it already exists,
	// defaults otherwise. Its knowledge sources drive the scaffolding below —
	// the repo config, not the project config, is the knowledge authority for
	// the repo.
	repoConfigPath := filepath.Join(spektacularDir, config.RepoConfigFileName)
	// The colocated repo's footprint is written into .spektacular, so its
	// code is that folder's parent: the project directory itself.
	repoCfg := config.NewDefaultRepoConfig()
	repoCfg.Source = config.DefaultRepoSource
	repoConfigExisted := false
	if _, err := os.Stat(repoConfigPath); err == nil {
		loaded, err := config.RepoConfigFromYAMLFile(repoConfigPath)
		if err != nil {
			return nil, fmt.Errorf("loading repo config: %w", err)
		}
		repoCfg = loaded
		repoConfigExisted = true
	}

	// Resolve the knowledge store location from the colocated repo config so
	// the knowledge base is scaffolded wherever that configuration points it,
	// not at a hardcoded path. Only the repo's own store is created by init;
	// the project's shared stores are declared separately and expected to exist
	// independently. A relative location resolves against the folder holding
	// repo.yaml — the repo's root, as EnsureFootprint and the knowledge
	// aggregation resolve it — not the project root. By default this is
	// .spektacular/knowledge.
	var knowledgeRoots []string
	if kc := repoCfg.WithDefaults(spektacularDir).Knowledge; kc.Provider == config.ProviderFile {
		location := kc.Config.Location
		if !filepath.IsAbs(location) {
			location = filepath.Join(spektacularDir, location)
		}
		knowledgeRoots = append(knowledgeRoots, location)
	}

	dirs := []string{
		spektacularDir,
		// A loaded config holds its spec and plan directories as
		// project-root-relative paths (e.g. ".spektacular/plans"); the file
		// writes them relative to the folder holding config.yaml.
		filepath.Join(projectPath, cfg.Plan.Config.Directory),
		filepath.Join(projectPath, cfg.Spec.Config.Directory),
	}

	// Ensure the knowledge base exists under the repo's own store location:
	// the store root plus a directory for every category in the
	// knowledge registry, which is the single source of truth for the category
	// model (including the always-applied glossary and the looked-up decisions
	// category). MkdirAll is idempotent, so this also tops up any categories a
	// pre-existing project is missing.
	for _, root := range knowledgeRoots {
		dirs = append(dirs, root)
		for _, c := range knowledge.Categories {
			dirs = append(dirs, filepath.Join(root, c.Name))
		}
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// Write config.yaml if it does not already exist. An existing config is
	// left untouched except when the resolved project name differs from what
	// it stored (backfill of a missing name, or an explicit override) or the
	// colocated repo was just seeded into an empty registry.
	if !configExisted || cfg.Name != storedName || reposSeeded {
		if err := cfg.ToYAMLFile(configPath); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
	}

	// Write the colocated repo's config only if it does not already exist,
	// mirroring the config.yaml behaviour.
	if !repoConfigExisted {
		if err := repoCfg.ToYAMLFile(repoConfigPath); err != nil {
			return nil, fmt.Errorf("writing repo config: %w", err)
		}
	}

	// Write embedded .gitignore
	gitignoreContent, err := templates.FS.ReadFile(".spektacular/.gitignore")
	if err != nil {
		return nil, fmt.Errorf("reading embedded .gitignore: %w", err)
	}
	if err := os.WriteFile(filepath.Join(spektacularDir, ".gitignore"), gitignoreContent, 0644); err != nil {
		return nil, fmt.Errorf("writing .gitignore: %w", err)
	}

	// Write a README for each knowledge category under each configured project
	// source location, rendered from its registry definition so the directory
	// documents its own purpose, boundary, tier, and entry shape rather than
	// carrying a circular placeholder.
	for _, root := range knowledgeRoots {
		for _, c := range knowledge.Categories {
			readmePath := filepath.Join(root, c.Name, knowledge.CategoryDescriptionFile)
			if err := os.WriteFile(readmePath, []byte(c.README()), 0644); err != nil {
				return nil, fmt.Errorf("writing %s README: %w", c.Name, err)
			}
		}
	}

	// Cascade over the registered repos, creating or repairing each one's
	// minimal footprint with the same routine `repo add` uses. Init never
	// clones: address-only repos not yet materialized are skipped with a
	// notice, and their footprints are created when they are first used.
	var notices []string
	rset, err := repo.New(cfg, projectPath, repo.NewGitRunner())
	if err != nil {
		return nil, err
	}
	for _, e := range rset.Entries() {
		root, ok := rset.LocalRoot(e.Name)
		if !ok {
			notices = append(notices, fmt.Sprintf("repo %q is not materialized locally; footprint creation skipped until it is first used", e.Name))
			continue
		}
		if _, err := repo.EnsureFootprint(root, config.NewDefaultRepoConfig()); err != nil {
			return nil, fmt.Errorf("ensuring footprint for repo %q: %w", e.Name, err)
		}
		// Every repo should end up consistently described; a footprint with no
		// descriptive metadata is not an error, just something worth surfacing.
		if meta, ok := rset.DescriptiveMetadata(e.Name); !ok || (meta.Description == "" && meta.Role == "" && len(meta.Tags) == 0) {
			notices = append(notices, fmt.Sprintf("repo %q has no descriptive metadata set; run 'repo add' with description/role/tags to describe it", e.Name))
		}
	}

	return notices, nil
}
