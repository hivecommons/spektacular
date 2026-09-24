package repo

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/output"
)

// MaterializeDirName is the project-relative folder that clones of git
// sources land in: .spektacular/repos/<repo-name>/. Project init gitignores
// it so a materialized clone can never enter the project's git history.
const MaterializeDirName = "repos"

// ResolvedRepo is a registered repo resolved to its two locations — the only
// access handle the registry hands out.
//
// Root is the directory holding the repo's .spektacular/ folder: repo.yaml,
// the knowledge store, and the changelog live under it, so every consumer
// of the repo's own Spektacular files (footprint repair, descriptive
// metadata, knowledge aggregation, the repo-routed changelog store,
// registration write-back) reads Root. Source is the directory holding the
// repo's code: the file source declared in repo.yaml, the clone of a git
// source, or Root itself when no source is declared. Consumers of the code
// (listing, git operations, the roster, and the instructions that send an
// agent into a repo) read Source.
type ResolvedRepo struct {
	Name         string
	Root         string // absolute directory holding .spektacular/
	Source       string // absolute directory holding the code
	Entry        config.RepoEntry
	Materialized bool   // true when Source is a project-managed clone of a git source
	StaleNote    string // non-empty ⇒ warn-only behind-remote (or check-failed) notice
}

// FootprintError reports a registered repo whose .spektacular footprint is
// missing or invalid. It carries what the CLI layer needs to surface the
// problem with an offer to repair, rather than a bare failure.
type FootprintError struct {
	Repo string // registry name
	Root string // resolved local root
	Err  error  // what is wrong with the footprint
}

func (e *FootprintError) Error() string {
	return fmt.Sprintf("repo %q at %s has a missing or invalid footprint: %v", e.Repo, e.Root, e.Err)
}

func (e *FootprintError) Unwrap() error { return e.Err }

// Set is the project's repo registry resolved against a project root. It is
// constructed from configuration, dispatches providers fail-fast, and
// resolves entries on demand — materializing clones only when a repo is
// actually used.
type Set struct {
	projectRoot string
	git         GitRunner
	entries     []config.RepoEntry // defaults applied, registry order
}

// New builds the registry from the project configuration. Unknown providers
// fail fast here, before any entry is resolved, mirroring knowledge.NewSet's
// contract.
func New(cfg config.Config, projectRoot string, git GitRunner) (*Set, error) {
	entries := make([]config.RepoEntry, 0, len(cfg.Repos))
	for _, e := range cfg.Repos {
		e = e.WithDefaults()
		if e.Provider != config.ProviderGit {
			return nil, fmt.Errorf("repo %q: provider %q is not supported (only %q)", e.Name, e.Provider, config.ProviderGit)
		}
		entries = append(entries, e)
	}
	return &Set{projectRoot: projectRoot, git: git, entries: entries}, nil
}

// Entries returns the registry entries in configuration order, with
// defaults applied. It performs no resolution and touches nothing on disk.
func (s *Set) Entries() []config.RepoEntry {
	out := make([]config.RepoEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

// Present reports whether the named repo's location is already on disk —
// without resolving it and without ever invoking git. Callers that must
// stay side-effect-free (like listing) use this to avoid triggering a clone.
func (s *Set) Present(name string) bool {
	_, ok := s.LocalRoot(name)
	return ok
}

// locationRoot returns the entry's registered location as an absolute path,
// resolving a relative value from the folder holding config.yaml.
func (s *Set) locationRoot(e config.RepoEntry) string {
	return e.ResolvedLocation(s.projectRoot)
}

// cloneDir returns where a git source for the named repo is materialized.
func (s *Set) cloneDir(name string) string {
	return filepath.Join(s.projectRoot, ".spektacular", MaterializeDirName, name)
}

// LocalRoot returns the named repo's on-disk root — its registered location
// when that directory exists — without resolving it and without ever
// invoking git. The second return is false when the location is not on disk
// (or the repo is not registered).
func (s *Set) LocalRoot(name string) (string, bool) {
	for _, e := range s.entries {
		if e.Name != name {
			continue
		}
		if e.Location == "" {
			return "", false
		}
		root := s.locationRoot(e)
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root, true
		}
		return "", false
	}
	return "", false
}

// LocalSource mirrors LocalRoot for the repo's code: the file source its
// repo.yaml declares, the clone of its git source when that clone exists,
// or the root when no source is declared — resolved without ever invoking
// git. The second return is false when the repo is not on disk, or when its
// git source has not been cloned yet. A missing, unreadable, or invalid
// repo.yaml falls back to the root, mirroring DescriptiveMetadata's
// tolerance.
func (s *Set) LocalSource(name string) (string, bool) {
	root, ok := s.LocalRoot(name)
	if !ok {
		return "", false
	}
	cfg, err := config.RepoConfigFromYAMLFile(filepath.Join(root, config.RepoConfigFileName))
	if err != nil {
		return root, true
	}
	kind, v, err := cfg.ParseSource(root)
	if err != nil {
		return root, true
	}
	switch kind {
	case config.SourceFile:
		return v, true
	case config.SourceGit:
		clone := s.cloneDir(name)
		if info, err := os.Stat(clone); err == nil && info.IsDir() {
			return clone, true
		}
		return "", false
	}
	return root, true
}

// DescriptiveMetadata returns the named repo's own descriptive metadata —
// what it says about itself in its own repo.yaml — if the repo is on disk
// and its config is readable. It never clones or fetches: an unmaterialized
// repo, or one whose repo.yaml is missing or unreadable, reports absent
// rather than an error, mirroring how loadFootprint tolerates the same
// conditions for its own purposes. This is the single place that decides
// how a repo's descriptive metadata is read, so every caller that surfaces
// it (repo listing, the plan workflow's repo roster) sees the same view.
func (s *Set) DescriptiveMetadata(name string) (config.RepoConfig, bool) {
	root, ok := s.LocalRoot(name)
	if !ok {
		return config.RepoConfig{}, false
	}
	cfg, err := config.RepoConfigFromYAMLFile(filepath.Join(root, config.RepoConfigFileName))
	if err != nil {
		return config.RepoConfig{}, false
	}
	return cfg, true
}

// Resolve resolves the named repo to its two locations: the root is its
// registered location, which must exist; the source is what its repo.yaml
// declares — a file source used as-is, or a git source cloned into the
// project's working folder only when absent, never fetched or pulled — and
// the root itself when nothing is declared. The repo's footprint is
// validated on the way; a missing or broken footprint returns a
// *FootprintError carrying a repair offer, never a silent continuation.
func (s *Set) Resolve(name string) (ResolvedRepo, error) {
	for _, e := range s.entries {
		if e.Name == name {
			return s.resolve(e)
		}
	}
	return ResolvedRepo{}, fmt.Errorf("repo %q is not registered in this project", name)
}

// ResolveAll resolves every registered repo in registry order, failing on
// the first repo that cannot be resolved.
func (s *Set) ResolveAll() ([]ResolvedRepo, error) {
	resolved := make([]ResolvedRepo, 0, len(s.entries))
	for _, e := range s.entries {
		r, err := s.resolve(e)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, r)
	}
	return resolved, nil
}

// Footprint loads the named repo's own configuration from its footprint,
// requiring the registered location to exist on disk and the repo.yaml
// inside it to be present and valid. It returns the same errors Resolve
// does for those conditions — never a silent "absent" — but never clones
// or runs git, so side-effect-free callers like listing can use it to
// surface a misregistered repo instead of hiding it.
func (s *Set) Footprint(name string) (config.RepoConfig, error) {
	for _, e := range s.entries {
		if e.Name != name {
			continue
		}
		r, err := s.locate(e)
		if err != nil {
			return config.RepoConfig{}, err
		}
		return s.loadFootprint(r)
	}
	return config.RepoConfig{}, fmt.Errorf("repo %q is not registered in this project", name)
}

// locate checks the entry's registered location exists on disk and returns
// the unresolved repo rooted there. A relative location is resolved from
// the project root, and the error for a missing one says so, naming both
// the registered value and the absolute path it resolved to.
func (s *Set) locate(e config.RepoEntry) (ResolvedRepo, error) {
	if e.Location == "" {
		return ResolvedRepo{}, output.NewError("config_invalid", fmt.Sprintf("repo %q has no location", e.Name)).
			WithNextAction(fmt.Sprintf("set repos[].location for %q to the folder holding its .spektacular/ directory", e.Name))
	}
	root := s.locationRoot(e)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return ResolvedRepo{}, output.NewError("repo_location_missing", fmt.Sprintf("repo %q: location %q resolves to %s, which does not exist (a relative location is resolved from the folder holding config.yaml, %s)", e.Name, e.Location, root, config.ProjectConfigDir(s.projectRoot))).
			WithResource(root).
			WithNextAction(fmt.Sprintf("correct repos[].location for %q, or create %s and run 'repo add' to scaffold its footprint", e.Name, root))
	}
	return ResolvedRepo{Name: e.Name, Root: root, Source: root, Entry: e}, nil
}

func (s *Set) resolve(e config.RepoEntry) (ResolvedRepo, error) {
	r, err := s.locate(e)
	if err != nil {
		return ResolvedRepo{}, err
	}
	cfg, err := s.loadFootprint(r)
	if err != nil {
		return r, err
	}

	kind, v, err := cfg.ParseSource(r.Root)
	if err != nil {
		return ResolvedRepo{}, fmt.Errorf("repo %q: %w", e.Name, err)
	}
	switch kind {
	case config.SourceFile:
		r.Source = v
	case config.SourceGit:
		// Materialize by cloning into the project's working folder — only
		// when the clone is absent. An existing clone is reused as-is;
		// keeping it current is the user's responsibility.
		clone := s.cloneDir(e.Name)
		if _, err := os.Stat(clone); os.IsNotExist(err) {
			if err := s.git.Clone(v, clone); err != nil {
				return ResolvedRepo{}, fmt.Errorf("repo %q: cloning %s: %w", e.Name, v, err)
			}
		}
		r.Source = clone
		r.Materialized = true
		r.StaleNote = s.staleNote(clone, v)
	}
	return r, nil
}

// staleNote compares a materialized clone's head against its git source and
// composes a warn-only notice. Every failure degrades to a notice or
// silence — the staleness check never fails resolution, and no fetch or
// pull ever runs.
func (s *Set) staleNote(root, address string) string {
	local, err := s.git.LocalHead(root)
	if err != nil {
		return ""
	}
	remote, err := s.git.RemoteHead(address)
	if err != nil {
		return fmt.Sprintf("could not check %s against its remote: %v", root, err)
	}
	if local != remote {
		return fmt.Sprintf("the clone at %s is not at its remote's HEAD (local %s, remote %s); Spektacular never fetches or pulls — update it yourself if needed", root, local, remote)
	}
	return ""
}

// loadFootprint validates the resolved repo's minimal footprint — a
// parseable, valid .spektacular/repo.yaml under its root — returning the
// parsed config, or a *FootprintError when it is missing or broken.
func (s *Set) loadFootprint(r ResolvedRepo) (config.RepoConfig, error) {
	path := filepath.Join(r.Root, config.RepoConfigFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return config.RepoConfig{}, &FootprintError{Repo: r.Name, Root: r.Root, Err: fmt.Errorf("missing %s", config.RepoConfigFileName)}
	}
	cfg, err := config.RepoConfigFromYAMLFile(path)
	if err != nil {
		return config.RepoConfig{}, &FootprintError{Repo: r.Name, Root: r.Root, Err: err}
	}
	return cfg, nil
}
