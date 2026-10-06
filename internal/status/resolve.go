package status

import (
	"errors"
	"fmt"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
)

// Target is what a name resolves to: an epic, or a standalone spec.
type Target struct {
	// Epic is the epic to report, empty for a standalone spec.
	Epic string
	// Spec is the standalone spec to report, empty when Epic is set.
	Spec string
}

// Resolve finds what name refers to, searching the epic store, then the spec
// store, then the plan store:
//
//  1. an epic name reports that epic;
//  2. a spec name reports the spec's epic when it has one, else the spec on
//     its own;
//  3. a plan name resolves to its spec (the plan's `spec` frontmatter, or the
//     same name), then as 2.
//
// Nothing matching is artifact_not_found, naming the three stores searched.
func Resolve(cfg config.Config, st store.Reader, name string) (Target, error) {
	if _, err := st.Read(epicPath(cfg, name)); err == nil {
		return Target{Epic: name}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return Target{}, err
	}

	if t, ok, err := resolveSpec(cfg, st, name); err != nil || ok {
		return t, err
	}

	raw, err := st.Read(planPath(cfg, name))
	if err == nil {
		specName := name
		if fm, _, splitErr := metadata.Split(raw); splitErr == nil && fm != nil && fm.Spec != "" {
			specName = fm.Spec
		}
		if t, ok, err := resolveSpec(cfg, st, specName); err != nil || ok {
			return t, err
		}
		// The plan's spec cannot be read: report it on its own, where it
		// shows as missing alongside the plan that names it.
		return Target{Spec: specName}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return Target{}, err
	}

	return Target{}, output.NewError("artifact_not_found",
		fmt.Sprintf("no epic, spec or plan named %q was found (searched the epic store %q, the spec store %q and the plan store %q)",
			name, cfg.Epic.Config.Directory, cfg.Spec.Config.Directory, cfg.Plan.Config.Directory)).
		WithResource(name).
		WithNextAction(fmt.Sprintf("run `%[1]s epic list`, `%[1]s spec file list` or `%[1]s plan file list` to see what exists", cfg.Command))
}

// resolveSpec resolves a spec name: its epic when it names one that exists,
// else the spec on its own. ok is false when no such spec is stored.
func resolveSpec(cfg config.Config, st store.Reader, name string) (Target, bool, error) {
	raw, err := st.Read(specPath(cfg, name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return Target{}, false, nil
		}
		return Target{}, false, err
	}
	fm, _, err := metadata.Split(raw)
	if err == nil && fm != nil && fm.Epic != "" && st.Exists(epicPath(cfg, fm.Epic)) {
		return Target{Epic: fm.Epic}, true, nil
	}
	return Target{Spec: name}, true, nil
}

// readEpic reads and parses the named epic. A missing epic is
// artifact_not_found; a malformed one is metadata_read_failed.
func readEpic(cfg config.Config, st store.Reader, name string) (epic.Epic, error) {
	raw, err := st.Read(epicPath(cfg, name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return epic.Epic{}, output.NewError("artifact_not_found", fmt.Sprintf("epic %q was not found", name)).
				WithResource(name).
				WithNextAction(fmt.Sprintf("run `%s epic list` to see available epics", cfg.Command))
		}
		return epic.Epic{}, err
	}
	e, err := epic.Parse(raw)
	if err != nil {
		return epic.Epic{}, output.NewError("metadata_read_failed", fmt.Sprintf("could not read epic %q: %v", name, err)).
			WithResource(name).
			WithNextAction(fmt.Sprintf("repair the epic's frontmatter with `%s epic write %s`", cfg.Command, name))
	}
	return e, nil
}

func epicPath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindEpic, Feature: name}.StorePath(cfg.Epic.Config.Directory)
}

func specPath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindSpec, Feature: name}.StorePath(cfg.Spec.Config.Directory)
}

func planPath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindPlan, Feature: name, Document: "plan"}.StorePath(cfg.Plan.Config.Directory)
}
