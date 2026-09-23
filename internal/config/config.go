package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jumppad-labs/spektacular/internal/output"
	"gopkg.in/yaml.v3"
)

var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

const (
	SpecIDMethodTimestamp = "timestamp"
	SpecIDMethodCounter   = "counter"
	SpecIDMethodExternal  = "external"
)

const (
	SpecTriggerThresholdStrict   = "strict"
	SpecTriggerThresholdModerate = "moderate"
	SpecTriggerThresholdLenient  = "lenient"
)

// AutoCommit* are the values of the auto_commit project setting, which
// decides when Spektacular makes a git commit on the user's behalf.
// AutoCommitOff is the default: an absent key means no automatic commits.
const (
	AutoCommitOff      = "off"
	AutoCommitWorkflow = "workflow"
	AutoCommitFull     = "full"
)

// ProviderFile is the only storage provider this release ships. The provider
// field on the spec, plan, and knowledge sections names a backend; today it
// must always be this value.
const ProviderFile = "file"

// ProviderGit is the only repo provider this release ships. The provider
// field on a repos entry names how the repo is resolved to a local
// directory; today it must always be this value (or empty, which defaults
// to it).
const ProviderGit = "git"

const (
	// DefaultSpecDir, DefaultPlanDir and DefaultChangelogDir are the store
	// directories used when none is configured. Like every relative path in
	// config.yaml they are relative to the folder holding config.yaml, so
	// they land in .spektacular/specs, .spektacular/plans and
	// .spektacular/changelog.
	DefaultSpecDir      = "specs"
	DefaultPlanDir      = "plans"
	DefaultChangelogDir = "changelog"

	// DefaultRepoKnowledgeLocation and DefaultRepoChangelogDir are the
	// repo-scoped defaults written into a repo.yaml. Every relative path in
	// that file is resolved from the folder holding it, so these are bare
	// folder names beside repo.yaml — not project-root paths.
	DefaultRepoKnowledgeLocation = "knowledge"
	DefaultRepoChangelogDir      = "changelog"
)

// DebugConfig holds debug logging configuration.
type DebugConfig struct {
	Enabled bool `yaml:"enabled"`
}

// SpecConfig holds configuration for specification creation. It names a
// storage provider, the provider-agnostic spec identifier method, and the
// provider's own settings.
type SpecConfig struct {
	Provider string         `yaml:"provider"`
	IDMethod string         `yaml:"id_method"`
	Config   FileSpecConfig `yaml:"config"`
}

// FileSpecConfig is the file-provider configuration for the spec section.
type FileSpecConfig struct {
	// Directory is the store directory. In a loaded project Config it is
	// project-root-relative (e.g. ".spektacular/specs"), which is what every
	// store consumer expects; in config.yaml it is written relative to the
	// folder holding the file (e.g. "specs"). The loader converts one to the
	// other and ToYAMLFile converts back.
	Directory string `yaml:"directory"`
	// fileForm is the value as read from config.yaml, written back
	// unchanged while Directory still resolves to it.
	fileForm string
}

// PlanConfig holds configuration for plan creation. It names a storage
// provider and carries that provider's settings.
type PlanConfig struct {
	Provider string         `yaml:"provider"`
	Config   FilePlanConfig `yaml:"config"`
}

// FilePlanConfig is the file-provider configuration for the plan section.
type FilePlanConfig struct {
	// Directory is the store directory. In a loaded project Config it is
	// project-root-relative (e.g. ".spektacular/plans"), which is what every
	// store consumer expects; in config.yaml it is written relative to the
	// folder holding the file (e.g. "plans"). The loader converts one to the
	// other and ToYAMLFile converts back.
	Directory string `yaml:"directory"`
	// fileForm is the value as read from config.yaml, written back
	// unchanged while Directory still resolves to it.
	fileForm string
}

// ChangelogConfig holds configuration for changelog record storage. It names
// a storage provider and carries that provider's settings.
type ChangelogConfig struct {
	Provider string              `yaml:"provider"`
	Config   FileChangelogConfig `yaml:"config"`
}

// FileChangelogConfig is the file-provider configuration for the changelog section.
type FileChangelogConfig struct {
	// Directory is the store directory. In a loaded project Config it is
	// project-root-relative (e.g. ".spektacular/changelog"), which is what every
	// store consumer expects; in config.yaml it is written relative to the
	// folder holding the file (e.g. "changelog"). The loader converts one to the
	// other and ToYAMLFile converts back.
	// In a repo.yaml the directory is always relative to the folder
	// holding that file and is used as written.
	Directory string `yaml:"directory"`
	// fileForm is the value as read from config.yaml, written back
	// unchanged while Directory still resolves to it.
	fileForm string
}

// KnowledgeConfig holds the ordered list of the project's own shared knowledge
// stores. It is the project-tier declaration; a repo declares its single store
// with RepoKnowledgeConfig instead.
type KnowledgeConfig struct {
	Sources []SourceConfig `yaml:"sources,omitempty"`
}

// DesignConfig holds the ordered list of the design sources the project
// declares. A design source points at a folder of design documents the team
// already keeps, wherever it keeps it, and Spektacular reads and writes
// documents there without imposing any structure on them.
//
// It is a project-tier declaration only: a repo declares no design sources of
// its own, so SourceConfig.Tier is left unset for every entry here and a
// source's identity is its name alone.
//
// A source's config.location may be absolute or relative; a relative location
// is relative to the folder holding config.yaml, the same base every other
// relative path in that file uses. Unlike the spec, plan and changelog store
// directories, it is never re-expressed on write: the declaration is written
// back exactly as the author wrote it, and a location outside the project root
// is allowed, because a design source points at a folder the team already has.
type DesignConfig struct {
	Sources []SourceConfig `yaml:"sources,omitempty"`
}

// RepoKnowledgeConfig is a repo's single knowledge store declaration. It
// deliberately mirrors ChangelogConfig, which sits beside it in the same file:
// one provider block, no list and no label, because a repo has exactly one
// store and is addressed by the name the project registered it under, not by
// a name it chooses for itself.
type RepoKnowledgeConfig struct {
	Provider string              `yaml:"provider"`
	Config   FileKnowledgeConfig `yaml:"config"`
}

// Validate checks the repo's knowledge declaration names a supported provider
// and a location to read from.
func (k RepoKnowledgeConfig) Validate() error {
	if k.Provider != ProviderFile {
		return fmt.Errorf("knowledge.provider %q is not supported (only %q)", k.Provider, ProviderFile)
	}
	if k.Config.Location == "" {
		return fmt.Errorf("knowledge.config.location must not be empty")
	}
	return nil
}

// SourceConfig is a single knowledge store. Each store names its own provider,
// so stores can use different backends independently.
//
// Name is the name the store is addressed by, and it is the same word wherever
// it appears: in this declaration, in a write, and in a narrowing. A
// project-declared store carries the name written here; a repo-declared store
// has its name stamped from the registry during aggregation, since a repo is
// addressed by the name the project registered it under and does not choose one.
type SourceConfig struct {
	Name     string              `yaml:"name"`
	Provider string              `yaml:"provider"`
	Config   FileKnowledgeConfig `yaml:"config"`
	// Tier is the store's addressing tier, stamped programmatically during
	// aggregation where every tier is visible at once, and never declared in a
	// config file. It is a plain string because the knowledge package that
	// defines the tier type imports this one.
	Tier string `yaml:"-"`
}

// FileKnowledgeConfig is the file-provider configuration for a knowledge source.
type FileKnowledgeConfig struct {
	Location string `yaml:"location"`
}

// RepoEntry is a single member repo in the project's registry. It carries
// membership only — identity, the folder holding the repo's own Spektacular
// files, and project-scoped dependencies — deliberately provider-agnostic
// siblings of the provider block, mirroring how knowledge sources keep scope
// outside their provider config. A repo's descriptive metadata (description,
// role, tags) and the location of its code (RepoConfig.Source)
// live in the repo's own configuration, not here, so they are never
// duplicated across the projects that register it.
//
// Location is the folder holding the repo's .spektacular/ directory, absolute
// or relative to the folder holding config.yaml (so the project's own root
// is `..`), and is required. Local is the older name
// for the same setting: it is accepted on load, folded into Location, and
// never written back. The former address key is no longer accepted; a repo's
// git origin belongs in its repo.yaml as source.
type RepoEntry struct {
	Name         string        `yaml:"name"`
	Location     string        `yaml:"location,omitempty"`
	Local        string        `yaml:"local,omitempty"`
	Dependencies []string      `yaml:"dependencies,omitempty"`
	Provider     string        `yaml:"provider,omitempty"`
	Config       GitRepoConfig `yaml:"config,omitempty"`
}

// GitRepoConfig is the git-provider configuration for a repos entry. It is
// empty in this release and reserved for provider-specific settings.
type GitRepoConfig struct{}

// ProjectConfigFileName is the name of the project settings file inside
// ProjectConfigDir.
const ProjectConfigFileName = "config.yaml"

// ProjectConfigDirName is the folder, inside a project root, that holds
// config.yaml.
const ProjectConfigDirName = ".spektacular"

// ProjectConfigDir returns the folder holding the project's config.yaml:
// <projectRoot>/.spektacular. Relative paths written in config.yaml are
// resolved from this folder — from the file that declares them — so
// `..` is the project's own root and `../repos/<name>` a sibling folder.
func ProjectConfigDir(projectRoot string) string {
	return filepath.Join(projectRoot, ProjectConfigDirName)
}

// ResolvedLocation returns the entry's location as an absolute path. An
// absolute location is returned cleaned; a relative one is resolved from the
// folder holding config.yaml (see ProjectConfigDir), never from the process
// working directory or the project root.
func (e RepoEntry) ResolvedLocation(projectRoot string) string {
	if filepath.IsAbs(e.Location) {
		return filepath.Clean(e.Location)
	}
	return filepath.Join(ProjectConfigDir(projectRoot), e.Location)
}

// Config is the top-level project configuration. It carries the project's
// identity, agent behaviour, and the central spec/plan/changelog storage.
// The knowledge section lists only project-owned sources (team or global
// shares, for example); each repo's own knowledge sources are declared in
// that repo's RepoConfig instead.
//
// Source is the project's git address, recorded in changelog provenance
// only. It is not a code location: where a repo's code lives is declared by
// RepoConfig.Source in that repo's repo.yaml.
type Config struct {
	// Schema is the settings format version (see CurrentProjectSchema).
	// ToYAMLFile always stamps the current value.
	Schema int `yaml:"schema"`
	// WrittenBy is the Spektacular version that last saved this file. It is
	// diagnostic only and never triggers an upgrade.
	WrittenBy string `yaml:"written_by,omitempty"`
	// SkillsVersion is the Spektacular version that last installed the
	// project's agent skills. Only a skills install sets it; rewriting the
	// file for any other reason leaves it unchanged.
	SkillsVersion        string          `yaml:"skills_version,omitempty"`
	Name                 string          `yaml:"name"`
	Source               string          `yaml:"source,omitempty"`
	Command              string          `yaml:"command"`
	Agent                string          `yaml:"agent"`
	SpecTriggerThreshold string          `yaml:"spec_trigger_threshold"`
	AutoCommit           string          `yaml:"auto_commit"`
	Debug                DebugConfig     `yaml:"debug"`
	Spec                 SpecConfig      `yaml:"spec"`
	Plan                 PlanConfig      `yaml:"plan"`
	Changelog            ChangelogConfig `yaml:"changelog"`
	Knowledge            KnowledgeConfig `yaml:"knowledge,omitempty"`
	Design               DesignConfig    `yaml:"design,omitempty"`
	Repos                []RepoEntry     `yaml:"repos,omitempty"`
}

// AutoCommitMode returns the effective auto_commit mode, resolving an absent
// key to AutoCommitOff so callers never have to special-case the empty string.
func (c Config) AutoCommitMode() string {
	if c.AutoCommit == "" {
		return AutoCommitOff
	}
	return c.AutoCommit
}

// NewDefault returns a Config populated with default values, with its store
// directories in their in-memory, project-root-relative form.
func NewDefault() Config {
	return Config{
		Command:              "spektacular",
		SpecTriggerThreshold: SpecTriggerThresholdModerate,
		AutoCommit:           AutoCommitOff,
		Debug: DebugConfig{
			Enabled: false,
		},
		Spec: SpecConfig{
			Provider: ProviderFile,
			IDMethod: SpecIDMethodTimestamp,
			Config: FileSpecConfig{
				Directory: filepath.Join(ProjectConfigDirName, DefaultSpecDir),
			},
		},
		Plan: PlanConfig{
			Provider: ProviderFile,
			Config: FilePlanConfig{
				Directory: filepath.Join(ProjectConfigDirName, DefaultPlanDir),
			},
		},
		Changelog: ChangelogConfig{
			Provider: ProviderFile,
			Config: FileChangelogConfig{
				Directory: filepath.Join(ProjectConfigDirName, DefaultChangelogDir),
			},
		},
		// Knowledge is empty by default: the project level lists only sources
		// owned by the project itself (team or global shares the user adds by
		// hand). Each repo's own store is declared in its RepoConfig.
		//
		// Design is empty by default for the same reason: a design source
		// points at a folder of design documents the team already has, so the
		// project declares each one by hand and nothing is scaffolded for it.
	}
}

// FromYAMLFile loads a Config from a YAML file, expanding ${VAR} patterns.
func FromYAMLFile(path string) (Config, error) {
	cfg, err := ParseYAMLFile(path)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validating config file %s: %w", path, err)
	}
	return cfg, nil
}

// ParseYAMLFile loads a Config from a YAML file without validating it,
// expanding ${VAR} patterns and prefilling defaults. It exists for init,
// which must be able to read a config that is missing its required name so
// it can backfill one; every other caller wants FromYAMLFile.
func ParseYAMLFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %s: %w", path, err)
	}

	expanded := expandEnvVars(string(raw))

	// Loading only ever reads the current format; upgrading an older file is
	// the migrate package's job, never the loader's.
	if err := checkSchema(expanded, path, "project", CurrentProjectSchema); err != nil {
		return Config{}, err
	}

	cfg := NewDefault()
	cfg.Spec.Config.Directory = DefaultSpecDir
	cfg.Plan.Config.Directory = DefaultPlanDir
	cfg.Changelog.Config.Directory = DefaultChangelogDir
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if err := rejectLegacyRepoAddress(expanded, path); err != nil {
		return Config{}, err
	}
	if err := rejectLegacyKnowledgeScope(expanded, path); err != nil {
		return Config{}, err
	}
	for i := range cfg.Repos {
		cfg.Repos[i] = cfg.Repos[i].foldLocationAlias()
	}
	cfg.resolveStoreDirs(filepath.Dir(path))
	return cfg, nil
}

// storeDir addresses one store directory of a Config.
type storeDir struct {
	key       string // the config.yaml section, for error messages
	directory *string
	fileForm  *string
}

// storeDirs enumerates the store directories that are bound to the project
// root. Membership in this list is load-bearing twice over, and a caller
// adding a new configured path here should want both effects:
//
//   - resolveStoreDirs and fileFormStoreDir re-express the value on read and
//     write, so the path written back is not necessarily the one the author
//     wrote; and
//   - Validate runs validateStoreDir over every entry, which refuses any
//     directory resolving outside the project root.
//
// A configured path that must survive verbatim, or that may point outside the
// project root, therefore does not belong here: it resolves in its own domain
// package instead. design.sources[].config.location is the worked example, and
// knowledge.sources[].config.location predates it.
func (c *Config) storeDirs() []storeDir {
	return []storeDir{
		{"spec", &c.Spec.Config.Directory, &c.Spec.Config.fileForm},
		{"plan", &c.Plan.Config.Directory, &c.Plan.Config.fileForm},
		{"changelog", &c.Changelog.Config.Directory, &c.Changelog.Config.fileForm},
	}
}

// resolveStoreDirs turns the spec, plan and changelog directories, which
// config.yaml states relative to the folder holding it (configDir), into the
// project-root-relative paths the stores use, remembering each value as read.
func (c *Config) resolveStoreDirs(configDir string) {
	for _, d := range c.storeDirs() {
		*d.fileForm = *d.directory
		*d.directory = resolveStoreDir(*d.directory, configDir)
	}
}

// resolveStoreDir resolves a store directory written relative to configDir
// into a path relative to the project root (configDir's parent). A value
// that resolves outside the project root is returned unchanged, for Validate
// to refuse.
func resolveStoreDir(value, configDir string) string {
	if value == "" {
		return value
	}
	abs := value
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(configDir, value)
	}
	rel, err := filepath.Rel(filepath.Dir(configDir), abs)
	if err != nil || escapesRoot(rel) {
		return value
	}
	return rel
}

// escapesRoot reports whether a project-root-relative store directory points
// outside the project root.
func escapesRoot(dir string) bool {
	return filepath.IsAbs(dir) || dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator))
}

// fileFormStoreDir expresses an in-memory store directory relative to
// configDir for writing, reusing the value read from the file while it
// still resolves to the same place.
func fileFormStoreDir(directory, fileForm, configDir string) string {
	if fileForm != "" && resolveStoreDir(fileForm, configDir) == directory {
		return fileForm
	}
	if escapesRoot(directory) {
		return directory
	}
	rel, err := filepath.Rel(configDir, filepath.Join(filepath.Dir(configDir), directory))
	if err != nil {
		return directory
	}
	return filepath.ToSlash(rel)
}

// rejectLegacyRepoAddress fails a config whose registry still carries the
// removed address key. The value is not lost, only relocated: the error
// names the repo and the exact source line to add to its repo.yaml. The raw
// document is scanned because RepoEntry no longer has a field the key could
// land in.
// rejectLegacyKnowledgeScope refuses a project config whose shared knowledge
// stores are still identified by the removed 'scope' key. The typed config has
// no field that key could land in, so without this guard a stale file parses
// cleanly and behaves as though it declared an unnamed store. Nothing is
// rewritten on disk, so the same file fails identically on every run until a
// person edits it.
func rejectLegacyKnowledgeScope(raw, path string) error {
	var shape struct {
		Knowledge struct {
			Sources []map[string]any `yaml:"sources"`
		} `yaml:"knowledge"`
	}
	if err := yaml.Unmarshal([]byte(raw), &shape); err != nil {
		return nil // the typed unmarshal already accepted the document
	}
	for i, src := range shape.Knowledge.Sources {
		scope, ok := src["scope"]
		if !ok {
			continue
		}
		return output.NewError("config_invalid",
			fmt.Sprintf("%s: knowledge.sources[%d] uses the removed 'scope' key; a shared knowledge store is now identified by 'name'", path, i)).
			WithResource(path).
			WithNextAction(fmt.Sprintf("in %s, rename knowledge.sources[%d].scope to 'name', e.g.:\n\nknowledge:\n  sources:\n    - name: %v\n      provider: file\n      config:\n        location: <location>", path, i, scope))
	}
	return nil
}

func rejectLegacyRepoAddress(raw, path string) error {
	var shape struct {
		Repos []map[string]any `yaml:"repos"`
	}
	if err := yaml.Unmarshal([]byte(raw), &shape); err != nil {
		return nil // the typed unmarshal already accepted the document
	}
	for i, r := range shape.Repos {
		addr, ok := r["address"]
		if !ok {
			continue
		}
		name, _ := r["name"].(string)
		location, _ := r["location"].(string)
		if location == "" {
			location, _ = r["local"].(string)
		}
		if location == "" {
			location = "<location>"
		}
		return output.NewError("config_invalid",
			fmt.Sprintf("%s: repo %q uses the removed 'address' key; a repo's git origin now belongs in its own repo.yaml as 'source'", path, name)).
			WithResource(path).
			WithNextAction(fmt.Sprintf("remove repos[%d].address from %s and set 'source: %v' in %s/.spektacular/repo.yaml", i, path, addr, location))
	}
	return nil
}

// slugPattern matches slug/filesystem-safe identifiers: lowercase letters,
// digits, hyphens, and underscores, with no path separators.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// validateSlug checks that value is a slug/filesystem-safe identifier,
// returning an error that names field when it is not.
func validateSlug(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if !slugPattern.MatchString(value) {
		return fmt.Errorf("%s %q must contain only lowercase letters, digits, '-' or '_', and must start with a letter or digit", field, value)
	}
	return nil
}

// SlugifyName converts an arbitrary name (such as a directory basename) into
// a slug-safe identifier: lowercased, with every run of unsupported
// characters collapsed to a single hyphen.
func SlugifyName(name string) string {
	slug := strings.ToLower(name)
	slug = nonSlugRunPattern.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-_")
	if slug == "" {
		return "project"
	}
	return slug
}

var nonSlugRunPattern = regexp.MustCompile(`[^a-z0-9_-]+`)

// Validate checks whether the config contains supported values.
func (c Config) Validate() error {
	if err := validateSlug("name", c.Name); err != nil {
		return err
	}
	switch c.SpecTriggerThreshold {
	case "", SpecTriggerThresholdStrict, SpecTriggerThresholdModerate, SpecTriggerThresholdLenient:
	default:
		return fmt.Errorf("spec_trigger_threshold must be one of %q, %q, or %q", SpecTriggerThresholdStrict, SpecTriggerThresholdModerate, SpecTriggerThresholdLenient)
	}
	switch c.AutoCommit {
	case "", AutoCommitOff, AutoCommitWorkflow, AutoCommitFull:
	default:
		return output.NewError("config_invalid",
			fmt.Sprintf("auto_commit must be one of %q, %q, or %q", AutoCommitOff, AutoCommitWorkflow, AutoCommitFull)).
			WithResource("auto_commit").
			WithNextAction(fmt.Sprintf("set auto_commit in .spektacular/config.yaml to %s, %s or %s (or remove the key to use %s)",
				AutoCommitOff, AutoCommitWorkflow, AutoCommitFull, AutoCommitOff))
	}
	if err := c.Spec.Validate(); err != nil {
		return err
	}
	if err := c.Plan.Validate(); err != nil {
		return err
	}
	if err := c.Changelog.Validate(); err != nil {
		return err
	}
	// Only the project's store directories are bound to the project root; a
	// repo.yaml's changelog is relative to that repo's own folder.
	for _, d := range c.storeDirs() {
		if err := validateStoreDir(d.key, *d.directory); err != nil {
			return err
		}
	}
	if err := c.Knowledge.Validate(); err != nil {
		return err
	}
	if err := c.Design.Validate(); err != nil {
		return err
	}
	if err := validateRepos(c.Repos); err != nil {
		return err
	}
	return nil
}

// validateRepos checks every registry entry for a slug-safe unique name, a
// usable location, and a supported provider.
func validateRepos(repos []RepoEntry) error {
	if len(repos) == 0 {
		return output.NewError("config_invalid", "no repos are registered in config.yaml; a project must register at least one repo").
			WithNextAction("run 'init' to register this project's own repo, or add a repos entry with a name and location")
	}
	seen := make(map[string]bool, len(repos))
	for i, r := range repos {
		if err := validateSlug(fmt.Sprintf("repos[%d].name", i), r.Name); err != nil {
			return err
		}
		if seen[r.Name] {
			return fmt.Errorf("repos: name %q is configured more than once", r.Name)
		}
		seen[r.Name] = true
		if r.Location == "" && r.Local == "" {
			return output.NewError("config_invalid", fmt.Sprintf("repo %q has no location", r.Name)).
				WithNextAction(fmt.Sprintf("set repos[%d].location to the folder holding %s's .spektacular/ directory", i, r.Name))
		}
		switch r.Provider {
		case "", ProviderGit:
		default:
			return fmt.Errorf("repo %q: provider %q is not supported (only %q)", r.Name, r.Provider, ProviderGit)
		}
	}
	return nil
}

// WithDefaults returns the entry with its provider defaulted to git when
// unset, mirroring how absent config sections resolve to defaults at load,
// and with the deprecated local alias folded into Location.
func (r RepoEntry) WithDefaults() RepoEntry {
	r = r.foldLocationAlias()
	if r.Provider == "" {
		r.Provider = ProviderGit
	}
	return r
}

// foldLocationAlias moves a value given under the older local key into
// Location and clears Local, so the alias is honoured on load and the
// current key is the only one ever written back.
func (r RepoEntry) foldLocationAlias() RepoEntry {
	if r.Location == "" && r.Local != "" {
		r.Location = r.Local
	}
	r.Local = ""
	return r
}

// validateStoreDir refuses a store directory outside the project root: the
// project store only ever lived inside it.
func validateStoreDir(key, dir string) error {
	if !escapesRoot(dir) {
		return nil
	}
	return output.NewError("config_invalid",
		fmt.Sprintf("%s.config.directory %q is outside the project", key, dir)).
		WithNextAction(fmt.Sprintf("set `%s.config.directory` to a folder inside the project, relative to the folder holding config.yaml (e.g. `%s`)", key, map[string]string{"spec": DefaultSpecDir, "plan": DefaultPlanDir, "changelog": DefaultChangelogDir}[key]))
}

// Validate checks whether the spec config names a supported provider and
// carries valid provider settings.
func (c SpecConfig) Validate() error {
	if c.Provider != ProviderFile {
		return fmt.Errorf("spec.provider %q is not supported (only %q)", c.Provider, ProviderFile)
	}
	if c.Config.Directory == "" {
		return fmt.Errorf("spec.config.directory must not be empty")
	}
	switch c.IDMethod {
	case "", SpecIDMethodTimestamp, SpecIDMethodCounter, SpecIDMethodExternal:
	default:
		return fmt.Errorf("spec.id_method must be one of %q, %q, or %q", SpecIDMethodTimestamp, SpecIDMethodCounter, SpecIDMethodExternal)
	}
	return nil
}

// Validate checks whether the plan config names a supported provider and
// carries valid provider settings.
func (c PlanConfig) Validate() error {
	if c.Provider != ProviderFile {
		return fmt.Errorf("plan.provider %q is not supported (only %q)", c.Provider, ProviderFile)
	}
	if c.Config.Directory == "" {
		return fmt.Errorf("plan.config.directory must not be empty")
	}
	return nil
}

// Validate checks whether the changelog config names a supported provider
// and carries valid provider settings.
func (c ChangelogConfig) Validate() error {
	if c.Provider != ProviderFile {
		return fmt.Errorf("changelog.provider %q is not supported (only %q)", c.Provider, ProviderFile)
	}
	if c.Config.Directory == "" {
		return fmt.Errorf("changelog.config.directory must not be empty")
	}
	return nil
}

// Validate checks every shared knowledge store for a supported provider,
// required fields, and a name unique within the project tier. Uniqueness is
// checked within this list rather than globally, so naming a shared store after
// a repo is allowed: a store's identity is its tier and its name together.
func (c KnowledgeConfig) Validate() error {
	seen := make(map[string]bool, len(c.Sources))
	for i, src := range c.Sources {
		if src.Name == "" {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("knowledge.sources[%d] declares no name", i),
			).WithNextAction("give every entry under knowledge.sources a `name:`, which is the name that store is addressed by")
		}
		if seen[src.Name] {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("knowledge.sources declares the name %q more than once", src.Name),
			).WithNextAction(fmt.Sprintf("rename one of the two %q entries under knowledge.sources; names must be unique within the project tier", src.Name))
		}
		seen[src.Name] = true
		if src.Provider != ProviderFile {
			return fmt.Errorf("knowledge store %q: provider %q is not supported (only %q)", src.Name, src.Provider, ProviderFile)
		}
		if src.Config.Location == "" {
			return fmt.Errorf("knowledge store %q: config.location must not be empty", src.Name)
		}
	}
	return nil
}

// Validate checks every declared design source for a name, a name unique
// within the list, a supported provider, and a location to read and write
// documents at. An empty list is valid and means the project declares no
// design sources.
//
// Every refusal carries a next action, including the provider and location
// cases: a design source is declared by hand, so an author who mistypes one
// needs to be told which key to correct. That is why this reads like
// KnowledgeConfig.Validate without sharing its two bare errors.
func (c DesignConfig) Validate() error {
	seen := make(map[string]bool, len(c.Sources))
	for i, src := range c.Sources {
		if src.Name == "" {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("design.sources[%d] declares no name", i),
			).WithNextAction("give every entry under design.sources a `name:`, which is the name that source is addressed by")
		}
		if seen[src.Name] {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("design.sources declares the name %q more than once", src.Name),
			).WithNextAction(fmt.Sprintf("rename one of the two %q entries under design.sources; a design source is addressed by its name alone, so names must be unique", src.Name))
		}
		seen[src.Name] = true
		if src.Provider != ProviderFile {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("design source %q: provider %q is not supported (only %q)", src.Name, src.Provider, ProviderFile),
			).WithNextAction(fmt.Sprintf("set design.sources[%d].provider to %q, the only design storage backend this release ships", i, ProviderFile))
		}
		if src.Config.Location == "" {
			return output.NewError(
				"config_invalid",
				fmt.Sprintf("design source %q: config.location must not be empty", src.Name),
			).WithNextAction(fmt.Sprintf("set design.sources[%d].config.location to the folder holding %s's design documents; a relative path resolves from the folder holding config.yaml", i, src.Name))
		}
	}
	return nil
}

// ToYAMLFile writes the Config to a YAML file, stamping the current settings
// format and the running Spektacular version, and writing the store
// directories back relative to the folder holding the file.
func (c Config) ToYAMLFile(path string) error {
	c.Schema = CurrentProjectSchema
	c.WrittenBy = WriterVersion
	configDir := filepath.Dir(path)
	for _, d := range c.storeDirs() {
		*d.directory = fileFormStoreDir(*d.directory, *d.fileForm, configDir)
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file %s: %w", path, err)
	}
	return nil
}

// expandEnvVars replaces ${VAR} patterns in s with the current environment values.
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1] // strip ${ and }
		return os.Getenv(name)
	})
}
