package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jumppad-labs/spektacular/internal/config"
	"github.com/jumppad-labs/spektacular/internal/output"
)

// Registration is the request to register a repo in a project: the registry
// entry's identity and the folder the repo's code lives in, plus the fields
// written into the repo's own config — its descriptive metadata and, when
// given, the source its code lives at.
type Registration struct {
	Name         string
	Location     string
	Source       string
	Description  string
	Role         string
	Tags         []string
	Dependencies []string
}

// RegistrationResult reports what registering a repo did: whether it is
// registered, what happened to its footprint, and the warn-only notice
// raised when the repo ends up with no descriptive metadata at all.
type RegistrationResult struct {
	Registered   bool
	Footprint    string
	MetadataNote string
}

// Register registers a repo in the project config and creates or repairs its
// minimal footprint. It is the single write path for registration, shared by
// the `repo add` command and the guided add workflow, so neither can drift
// from the other.
//
// git is the runner used to materialize a git source; callers pass their own
// so a test can substitute a stub rather than clone for real.
func Register(cfg *config.Config, projectRoot string, git GitRunner, in Registration) (RegistrationResult, error) {
	// Registration is pointed at the repo's code. Its Spektacular footprint
	// is scaffolded into a .spektacular folder inside it, and that folder —
	// the one that ends up holding repo.yaml — is what the registry records,
	// since a registered location names the folder holding repo.yaml and
	// nothing is appended when reading it back.
	entry := config.RepoEntry{
		Name:         in.Name,
		Location:     filepath.Join(in.Location, ".spektacular"),
		Dependencies: in.Dependencies,
	}

	// Idempotent registration by name: an identical entry is a no-op, a
	// changed entry updates in place, a new one appends. The config write is
	// skipped entirely when nothing changed.
	changed := true
	found := false
	for i, existing := range cfg.Repos {
		if existing.Name == entry.Name {
			found = true
			// Preserve provider settings the input shape doesn't carry.
			entry.Provider = existing.Provider
			entry.Config = existing.Config
			if reposEqual(existing, entry) {
				changed = false
			} else {
				cfg.Repos[i] = entry
			}
			break
		}
	}
	if !found {
		cfg.Repos = append(cfg.Repos, entry)
	}

	if err := cfg.Validate(); err != nil {
		return RegistrationResult{}, err
	}

	// A target repo.yaml written by a newer Spektacular is refused before
	// anything is written, so a refused registration leaves no registry
	// entry pointing at a file this build cannot read.
	targetConfig := filepath.Join(entry.ResolvedLocation(projectRoot), config.RepoConfigFileName)
	if n, err := config.PeekSchema(targetConfig); err == nil && n > config.CurrentRepoSchema {
		return RegistrationResult{}, &config.FormatError{Path: targetConfig, Kind: "repo", Found: n, Want: config.CurrentRepoSchema}
	}

	if changed {
		cfgPath := filepath.Join(config.ProjectConfigDir(projectRoot), "config.yaml")
		if err := cfg.ToYAMLFile(cfgPath); err != nil {
			return RegistrationResult{}, fmt.Errorf("writing config: %w", err)
		}
	}

	// Create or repair the target repo's minimal footprint at its registered
	// location, creating that folder when missing: the location is the
	// footprint folder holding the repo's repo.yaml, so making it exist is
	// registration's job.
	set, err := New(*cfg, projectRoot, git)
	if err != nil {
		return RegistrationResult{}, err
	}
	location := entry.ResolvedLocation(projectRoot)
	if err := os.MkdirAll(location, 0755); err != nil {
		return RegistrationResult{}, fmt.Errorf("creating repo location %s: %w", location, err)
	}

	// A scaffolded footprint sits inside the repo's code, so its default
	// source is the parent folder. An explicit source in this same
	// registration overrides it below.
	scaffold := config.NewDefaultRepoConfig()
	scaffold.Source = config.DefaultRepoSource
	status, err := EnsureFootprint(location, scaffold)
	if err != nil {
		return RegistrationResult{}, err
	}

	// Write the request's descriptive fields and source into the target
	// repo's own config, updating an already-footprinted repo rather than
	// only a freshly created one. EnsureFootprint's caller-supplied config
	// has no effect once a repo.yaml already exists, so this is a separate
	// step. It runs before resolution so a git source given in this same
	// registration is honoured by the clone below.
	repoConfigPath := filepath.Join(location, config.RepoConfigFileName)
	repoCfg, err := config.RepoConfigFromYAMLFile(repoConfigPath)
	if err != nil {
		return RegistrationResult{}, fmt.Errorf("reading repo config: %w", err)
	}
	updated := repoCfg
	if in.Description != "" {
		updated.Description = in.Description
	}
	if in.Role != "" {
		updated.Role = in.Role
	}
	if len(in.Tags) > 0 {
		updated.Tags = in.Tags
	}
	if in.Source != "" {
		src, err := config.SourceFromInput(in.Source)
		if err != nil {
			return RegistrationResult{}, output.NewError("invalid_data", err.Error()).
				WithNextAction(`give "source" as a path (plain or file://) or a git location (git://, ssh://, https://, or user@host:path)`)
		}
		updated.Source = src
	}
	if !repoConfigDescriptiveFieldsEqual(repoCfg, updated) {
		if err := updated.ToYAMLFile(repoConfigPath); err != nil {
			return RegistrationResult{}, fmt.Errorf("writing repo config: %w", err)
		}
	}

	// Resolve once so a git source is materialized (cloned into the
	// project's working folder) on registration rather than on first use.
	if _, err := set.Resolve(entry.Name); err != nil {
		return RegistrationResult{}, err
	}

	result := RegistrationResult{Registered: true, Footprint: status}
	if DescriptiveFieldsEmpty(updated) {
		result.MetadataNote = fmt.Sprintf("repo %q has no descriptive metadata set; pass description/role/tags so it's consistently described", entry.Name)
	}
	return result, nil
}

// reposEqual reports whether two registry entries are identical field for
// field. RepoEntry carries membership only; descriptive metadata lives in
// the repo's own config and is compared separately.
func reposEqual(a, b config.RepoEntry) bool {
	if a.Name != b.Name || a.Location != b.Location || a.Provider != b.Provider {
		return false
	}
	return stringSlicesEqual(a.Dependencies, b.Dependencies)
}

// repoConfigDescriptiveFieldsEqual reports whether the fields registration
// writes — the descriptive metadata and the source — match, so the repo's
// config is rewritten only when registration actually changed something.
func repoConfigDescriptiveFieldsEqual(a, b config.RepoConfig) bool {
	if a.Description != b.Description || a.Role != b.Role || a.Source != b.Source {
		return false
	}
	return stringSlicesEqual(a.Tags, b.Tags)
}

// DescriptiveFieldsEmpty reports whether a repo config carries no
// descriptive metadata at all, the case this project wants surfaced with a
// warn-only notice so every repo ends up consistently described. Source is
// a location, not a description, so it does not count.
func DescriptiveFieldsEmpty(c config.RepoConfig) bool {
	return c.Description == "" && c.Role == "" && len(c.Tags) == 0
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
