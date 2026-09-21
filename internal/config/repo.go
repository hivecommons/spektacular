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

// RepoConfigFileName is the filename of the repo-level configuration file,
// stored in the repo's .spektacular directory beside config.yaml when the
// repo is colocated with a project.
const RepoConfigFileName = "repo.yaml"

// RepoConfig is the repo-level configuration. It holds only repo-scoped
// concerns — the repo's own descriptive metadata, knowledge sources, and
// changelog provider — and carries no pointer back to any project:
// membership is recorded only in project configs, so a repo can belong to
// multiple projects.
//
// A repo has two locations. The directory holding this file's .spektacular/
// folder is where the repo's own Spektacular files live: repo.yaml, the
// knowledge store, and the changelog. Source, when set, names where the
// repo's code lives instead of assuming it is that same directory: a file
// location (a plain path or a file:// path, absolute, relative to the
// directory holding repo.yaml, or using ${VAR}) or a git location that
// Spektacular clones into the project's working folder on first use. When
// Source is unset the code and the Spektacular files are colocated.
type RepoConfig struct {
	// Schema is the settings format version (see CurrentRepoSchema).
	// ToYAMLFile always stamps the current value.
	Schema int `yaml:"schema"`
	// WrittenBy is the Spektacular version that last saved this file. It is
	// diagnostic only and never triggers an upgrade.
	WrittenBy   string              `yaml:"written_by,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Role        string              `yaml:"role,omitempty"`
	Tags        []string            `yaml:"tags,omitempty"`
	Source      RepoSourceConfig    `yaml:"source,omitempty"`
	Knowledge   RepoKnowledgeConfig `yaml:"knowledge"`
	Changelog   ChangelogConfig     `yaml:"changelog"`
}

// NewDefaultRepoConfig returns a RepoConfig populated with default values:
// the repo's own knowledge store and a file-backed changelog.
func NewDefaultRepoConfig() RepoConfig {
	return RepoConfig{
		Knowledge: RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config: FileKnowledgeConfig{
				Location: DefaultRepoKnowledgeLocation,
			},
		},
		Changelog: ChangelogConfig{
			Provider: ProviderFile,
			Config: FileChangelogConfig{
				Directory: DefaultRepoChangelogDir,
			},
		},
	}
}

// RepoConfigFromYAMLFile loads a RepoConfig from a YAML file, expanding
// ${VAR} patterns.
func RepoConfigFromYAMLFile(path string) (RepoConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RepoConfig{}, fmt.Errorf("reading repo config file %s: %w", path, err)
	}

	expanded := expandEnvVars(string(raw))

	// Loading only ever reads the current format; upgrading an older file is
	// the migrate package's job, never the loader's.
	if err := checkSchema(expanded, path, "repo", CurrentRepoSchema); err != nil {
		return RepoConfig{}, err
	}

	cfg := NewDefaultRepoConfig()
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return RepoConfig{}, fmt.Errorf("parsing repo config file %s: %w", path, err)
	}
	if err := rejectLegacyRepoKnowledgeBlock(expanded, path); err != nil {
		return RepoConfig{}, err
	}
	if err := cfg.Validate(); err != nil {
		return RepoConfig{}, fmt.Errorf("validating repo config file %s: %w", path, err)
	}
	return cfg, nil
}

// rejectLegacyRepoKnowledgeBlock refuses a repo config whose knowledge store is
// still declared as a list of labelled sources. Because the typed config seeds
// from NewDefaultRepoConfig before unmarshalling, and the superseded keys no
// longer have a field to land in, a stale file would otherwise parse cleanly and
// silently read and write the default store instead of the one it declares —
// losing a custom location without any error at all. Nothing is rewritten on
// disk, so the same file fails identically on every run until a person edits it.
func rejectLegacyRepoKnowledgeBlock(raw, path string) error {
	var shape struct {
		Knowledge map[string]any `yaml:"knowledge"`
	}
	if err := yaml.Unmarshal([]byte(raw), &shape); err != nil {
		return nil // the typed unmarshal already accepted the document
	}
	found := ""
	switch {
	case shape.Knowledge["sources"] != nil:
		found = "sources"
	case shape.Knowledge["scope"] != nil:
		found = "scope"
	default:
		return nil
	}
	return output.NewError("config_invalid",
		fmt.Sprintf("%s: knowledge uses the removed '%s' key; a repo declares exactly one knowledge store, as a single provider block", path, found)).
		WithResource(path).
		WithNextAction(fmt.Sprintf("in %s, replace the knowledge block with:\n\nknowledge:\n  provider: file\n  config:\n    location: %s\n\nthe store is addressed by the name the project registered this repo under, so it takes no name of its own", path, DefaultRepoKnowledgeLocation))
}

// Validate checks whether the repo config contains supported values.
func (c RepoConfig) Validate() error {
	if err := c.Knowledge.Validate(); err != nil {
		return err
	}
	return c.Changelog.Validate()
}

// WithDefaults returns a RepoConfig whose knowledge section is guaranteed to
// name a store: when the repo declares none it synthesises the repo's own store
// under repoRoot. repoRoot is the folder holding repo.yaml, which every relative
// path in that file is resolved from.
func (c RepoConfig) WithDefaults(repoRoot string) RepoConfig {
	if c.Knowledge.Provider == "" {
		c.Knowledge = RepoKnowledgeConfig{
			Provider: ProviderFile,
			Config: FileKnowledgeConfig{
				Location: filepath.Join(repoRoot, DefaultRepoKnowledgeLocation),
			},
		}
	}
	return c
}

// DefaultRepoSource is the source a scaffolded repo.yaml declares. The
// scaffolder writes repo.yaml into a .spektacular folder inside the repo's
// code, so the code is that folder's parent.
var DefaultRepoSource = RepoSourceConfig{
	Provider: ProviderFile,
	Config:   RepoSourceLocation{Location: ".."},
}

// RepoSourceConfig declares where a repo's code lives, in the provider block
// shape every other section of these files uses: a provider naming the kind
// of location, and a config carrying it.
//
//	source:
//	  provider: file
//	  config:
//	    location: ..
//
// An unset source means the code is the folder holding repo.yaml.
type RepoSourceConfig struct {
	Provider string             `yaml:"provider,omitempty"`
	Config   RepoSourceLocation `yaml:"config,omitempty"`
}

// FileSource and GitSource build a source block for a directory on disk and
// for a git location to clone.
func FileSource(location string) RepoSourceConfig {
	return RepoSourceConfig{Provider: ProviderFile, Config: RepoSourceLocation{Location: location}}
}

func GitSource(location string) RepoSourceConfig {
	return RepoSourceConfig{Provider: ProviderGit, Config: RepoSourceLocation{Location: location}}
}

// RepoSourceLocation carries a source's location: a directory for the file
// provider, resolved from the folder holding repo.yaml when relative, or a
// git location for the git provider.
type RepoSourceLocation struct {
	Location string `yaml:"location,omitempty"`
}

// IsZero reports whether no source is declared, so an unset source is
// omitted from a marshalled repo.yaml rather than written as an empty block.
func (c RepoSourceConfig) IsZero() bool {
	return c.Provider == "" && c.Config.Location == ""
}

// UnmarshalYAML accepts the provider block and rejects the older scalar form
// (`source: file://..`) with an error naming the shape that replaced it,
// rather than silently ignoring a value the repo depends on.
func (c *RepoSourceConfig) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		provider, location := ProviderFile, value.Value
		if kind := classifyLegacySource(location); kind == SourceGit {
			provider = ProviderGit
		} else {
			location = strings.TrimPrefix(location, fileSourceScheme)
		}
		return fmt.Errorf("source must be a provider block, not a plain value: replace `source: %s` with\n\nsource:\n  provider: %s\n  config:\n    location: %s\n", value.Value, provider, location)
	}
	type plain RepoSourceConfig
	return value.Decode((*plain)(c))
}

// SourceKind classifies a repo's declared source.
type SourceKind int

const (
	// SourceNone means no source is declared: the code is the folder
	// holding repo.yaml.
	SourceNone SourceKind = iota
	// SourceFile means the source is a directory on disk.
	SourceFile
	// SourceGit means the source is a git location to clone.
	SourceGit
)

// String returns the kind's name for error messages and logs.
func (k SourceKind) String() string {
	switch k {
	case SourceFile:
		return "file"
	case SourceGit:
		return "git"
	default:
		return "none"
	}
}

const fileSourceScheme = "file://"

// gitSourceSchemes are the URL schemes that mark a location as a git one.
var gitSourceSchemes = []string{"git://", "ssh://", "http://", "https://", "git+"}

// scpStyleSource matches git's scp-style remote form, user@host:path.
var scpStyleSource = regexp.MustCompile(`^[^/@:]+@[^/:]+:`)

// explicitScheme matches a leading URL scheme such as "s3://".
var explicitScheme = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)

// ParseSource classifies the declared source and resolves it. For the file
// provider it returns the absolute directory, anchoring a relative location
// at configDir (the folder holding repo.yaml); for the git provider it
// returns the location unchanged. It returns SourceNone and an empty value
// when no source is declared. Environment variables are already expanded by
// the loader, so none are expanded here.
func (c RepoConfig) ParseSource(configDir string) (SourceKind, string, error) {
	provider := strings.TrimSpace(c.Source.Provider)
	location := strings.TrimSpace(c.Source.Config.Location)

	if provider == "" && location == "" {
		return SourceNone, "", nil
	}
	if provider == "" {
		return SourceNone, "", fmt.Errorf("source declares a location but no provider: set source.provider to %q or %q", ProviderFile, ProviderGit)
	}
	if location == "" {
		return SourceNone, "", fmt.Errorf("source declares provider %q but no location: set source.config.location", provider)
	}

	switch provider {
	case ProviderFile:
		return SourceFile, resolveFileSource(strings.TrimPrefix(location, fileSourceScheme), configDir), nil
	case ProviderGit:
		return SourceGit, location, nil
	default:
		return SourceNone, "", fmt.Errorf("unsupported source provider %q: use %q or %q", provider, ProviderFile, ProviderGit)
	}
}

// classifyLegacySource guesses whether a bare location names a git remote or
// a directory. It backs the migration error above and the `repo add` input,
// where a source is still given as a single convenient value.
func classifyLegacySource(v string) SourceKind {
	v = strings.TrimSpace(v)
	if v == "" {
		return SourceNone
	}
	if strings.HasPrefix(v, fileSourceScheme) {
		return SourceFile
	}
	for _, scheme := range gitSourceSchemes {
		if strings.HasPrefix(v, scheme) {
			return SourceGit
		}
	}
	if scpStyleSource.MatchString(v) {
		return SourceGit
	}
	return SourceFile
}

// SourceFromInput turns the single `source` value a `repo add` payload
// carries into the provider block stored in repo.yaml, so the command line
// stays terse while the file keeps the same shape as every other section.
func SourceFromInput(v string) (RepoSourceConfig, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return RepoSourceConfig{}, nil
	}
	kind := classifyLegacySource(v)
	if kind == SourceFile {
		if m := explicitScheme.FindString(v); m != "" && !strings.HasPrefix(v, fileSourceScheme) {
			return RepoSourceConfig{}, fmt.Errorf("unsupported source scheme %q in %q: use a file:// path, a plain path, or a git location (git://, ssh://, https://, or user@host:path)", strings.TrimSuffix(m, "://"), v)
		}
		return RepoSourceConfig{Provider: ProviderFile, Config: RepoSourceLocation{Location: strings.TrimPrefix(v, fileSourceScheme)}}, nil
	}
	return RepoSourceConfig{Provider: ProviderGit, Config: RepoSourceLocation{Location: v}}, nil
}

// resolveFileSource turns a file source into an absolute, cleaned directory
// path, anchoring a relative path at configDir.
func resolveFileSource(path, configDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(configDir, path)
}

// ToYAMLFile writes the RepoConfig to a YAML file, stamping the current
// settings format and the running Spektacular version.
func (c RepoConfig) ToYAMLFile(path string) error {
	c.Schema = CurrentRepoSchema
	c.WrittenBy = WriterVersion
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshalling repo config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing repo config file %s: %w", path, err)
	}
	return nil
}
