package repo

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/knowledge"
	"github.com/hivecommons/spektacular/internal/migrate"
)

// Footprint statuses reported by EnsureFootprint.
const (
	FootprintCreated   = "created"
	FootprintRepaired  = "repaired"
	FootprintUnchanged = "unchanged"
)

// EnsureFootprint creates or repairs a repo's minimal Spektacular footprint
// at root — the folder that holds the repo's repo.yaml: that file plus the
// knowledge storage its sources declare — and nothing else (no agent guidance, no skills, no version
// file). It is idempotent and almost entirely additive: an existing repo.yaml is
// kept (and drives the scaffolding) unless it is broken, and a repo initialized
// by another project is left undisturbed.
//
// A knowledge *entry* is never overwritten. The one deliberate exception is a
// category's own generated description (knowledge.CategoryDescriptionFile),
// which is rendered from the category registry rather than written by anyone:
// one whose bytes have drifted from that rendering is brought back into line,
// and one that already matches is left untouched. Without this, a description
// that no longer matched the project's definition of its category could be
// repaired by no command at all, which would leave the refusal to delete a
// descriptor pointing at a remedy that does not work.
//
// The returned status reports what happened: created (no
// repo.yaml existed), repaired (repo.yaml existed but was broken, parts
// of the knowledge storage were missing, or a category description had
// drifted), or unchanged.
func EnsureFootprint(root string, repoCfg config.RepoConfig) (string, error) {
	repoConfigPath := filepath.Join(root, config.RepoConfigFileName)

	status := FootprintUnchanged
	if _, err := os.Stat(repoConfigPath); os.IsNotExist(err) {
		status = FootprintCreated
		if err := os.MkdirAll(root, 0755); err != nil {
			return "", fmt.Errorf("creating directory %s: %w", root, err)
		}
		if err := repoCfg.ToYAMLFile(repoConfigPath); err != nil {
			return "", err
		}
	} else if loaded, err := config.RepoConfigFromYAMLFile(repoConfigPath); err != nil {
		fe, isFormat := config.IsFormatError(err)
		switch {
		case isFormat && fe.Newer():
			// A file from a newer Spektacular is not broken, and must never
			// be overwritten.
			return "", err
		case isFormat:
			// An older-format repo.yaml is upgraded in place, not replaced,
			// so a repo is brought current whenever it is next set up.
			if _, err := migrate.UpgradeRepoFile(repoConfigPath, config.WriterVersion); err != nil {
				return "", err
			}
			upgraded, err := config.RepoConfigFromYAMLFile(repoConfigPath)
			if err != nil {
				return "", err
			}
			status = FootprintRepaired
			repoCfg = upgraded
		default:
			// A broken repo config is repaired by rewriting it from the given
			// defaults — the footprint must end the call valid.
			status = FootprintRepaired
			if err := repoCfg.ToYAMLFile(repoConfigPath); err != nil {
				return "", err
			}
		}
	} else {
		// A healthy existing config is the authority for its own footprint.
		repoCfg = loaded
	}

	// Scaffold the knowledge storage the repo config declares: the source
	// root plus a directory and README for every category in the registry.
	// Only missing pieces are created, so an already-initialized repo is
	// never disturbed.
	kc := repoCfg.WithDefaults(root).Knowledge
	if kc.Provider == config.ProviderFile {
		location := kc.Config.Location
		if !filepath.IsAbs(location) {
			location = filepath.Join(root, location)
		}
		for _, c := range knowledge.Categories {
			dir := filepath.Join(location, c.Name)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if status == FootprintUnchanged {
					status = FootprintRepaired
				}
				if err := os.MkdirAll(dir, 0755); err != nil {
					return "", fmt.Errorf("creating directory %s: %w", dir, err)
				}
			}
			// The category description is generated output, not content, so
			// it is the one file here that is brought back into line rather
			// than merely created. Compare against the registry's own
			// rendering: a description that already matches is left
			// byte-identical and does not move the status, so a repeated run
			// still reports unchanged.
			readmePath := filepath.Join(dir, knowledge.CategoryDescriptionFile)
			want := []byte(c.README())
			existing, err := os.ReadFile(readmePath)
			if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("reading %s %s: %w", c.Name, knowledge.CategoryDescriptionFile, err)
			}
			if err != nil || !bytes.Equal(existing, want) {
				if status == FootprintUnchanged {
					status = FootprintRepaired
				}
				if err := os.WriteFile(readmePath, want, 0644); err != nil {
					return "", fmt.Errorf("writing %s README: %w", c.Name, err)
				}
			}
		}
	}

	return status, nil
}
