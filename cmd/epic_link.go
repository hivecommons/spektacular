package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/artifact"
	"github.com/hivecommons/spektacular/internal/config"
	"github.com/hivecommons/spektacular/internal/epic"
	"github.com/hivecommons/spektacular/internal/metadata"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/status"
	"github.com/hivecommons/spektacular/internal/store"
)

// An epic lists its specs and each of those specs names the epic. The two
// sides are kept in agreement here, in one place, following the shape
// `design ref add` established (cmd/design_ref.go): validate everything
// before the first write, write the owning document first and the back-links
// after it, keep the original bytes of everything touched, and restore them
// in reverse order if a later write fails. Unlike a design reference this
// works on a set, so a membership conflict anywhere in the set refuses with
// nothing written at all.

// docTxn records every document a multi-document write touches so a failure
// part-way can put each back exactly as it was. There is no transaction in
// the store, so atomicity is by compensation.
type docTxn struct {
	st      store.Store
	touched []txnEntry
}

type txnEntry struct {
	path     string
	original []byte
	existed  bool
}

func newDocTxn(st store.Store) *docTxn { return &docTxn{st: st} }

// remember records path's current bytes the first time it is touched.
func (t *docTxn) remember(path string) error {
	for _, e := range t.touched {
		if e.path == path {
			return nil
		}
	}
	raw, err := t.st.Read(path)
	switch {
	case err == nil:
		t.touched = append(t.touched, txnEntry{path: path, original: raw, existed: true})
	case errors.Is(err, store.ErrNotFound):
		t.touched = append(t.touched, txnEntry{path: path})
	default:
		return err
	}
	return nil
}

// write stores content at path, remembering what was there first.
func (t *docTxn) write(path string, content []byte) error {
	if err := t.remember(path); err != nil {
		return err
	}
	return t.st.Write(path, content)
}

// delete removes path, remembering what was there first.
func (t *docTxn) delete(path string) error {
	if err := t.remember(path); err != nil {
		return err
	}
	return t.st.Delete(path)
}

// rollback restores every touched document in reverse order: original bytes
// back where a document existed, removal where it did not. It returns the
// paths it could not restore.
func (t *docTxn) rollback() []string {
	var failed []string
	for i := len(t.touched) - 1; i >= 0; i-- {
		e := t.touched[i]
		var err error
		if e.existed {
			err = t.st.Write(e.path, e.original)
		} else if t.st.Exists(e.path) {
			err = t.st.Delete(e.path)
		}
		if err != nil {
			failed = append(failed, e.path)
		}
	}
	return failed
}

// paths lists every touched document, for a message naming them.
func (t *docTxn) paths() []string {
	out := make([]string, len(t.touched))
	for i, e := range t.touched {
		out[i] = e.path
	}
	return out
}

// fail turns a write error part-way through a transaction into the reported
// refusal, after restoring everything. The two outcomes carry different codes
// on purpose: epic_link_failed says retry, epic_link_rollback_failed says the
// named documents must be reconciled by hand.
func (t *docTxn) fail(cause error) error {
	if failed := t.rollback(); len(failed) > 0 {
		return output.NewError("epic_link_rollback_failed",
			fmt.Sprintf("a write failed (%s), and restoring the documents afterwards also failed for %s; the epic and its specs may now disagree",
				cause.Error(), strings.Join(failed, ", "))).
			WithResource(failed[0]).
			WithNextAction(fmt.Sprintf("reconcile these documents by hand so the epic's specs list and each spec's epic field agree: %s", strings.Join(t.paths(), ", ")))
	}
	return output.NewError("epic_link_failed",
		fmt.Sprintf("a write failed (%s); every document was restored exactly as it was and nothing was recorded", cause.Error())).
		WithNextAction("fix the cause (for example make the store writable) and reissue the same command; no manual repair is needed")
}

// abort restores everything and returns refusal, the caller's own reason
// for stopping, unless the restore itself fails, which is reported instead.
func (t *docTxn) abort(refusal error) error {
	if failed := t.rollback(); len(failed) > 0 {
		return output.NewError("epic_link_rollback_failed",
			fmt.Sprintf("the operation was refused (%s), and restoring the documents already written also failed for %s",
				refusal.Error(), strings.Join(failed, ", "))).
			WithResource(failed[0]).
			WithNextAction(fmt.Sprintf("reconcile these documents by hand so the epic's specs list and each spec's epic field agree: %s", strings.Join(t.paths(), ", ")))
	}
	return refusal
}

// epicLinks is the membership change one write makes: the epic's members
// before and after.
type epicLinks struct {
	epic    string
	oldList []string
	newList []string
}

// diff returns the specs that join and the specs that leave the epic.
func (l epicLinks) diff() (added, removed []string) {
	old := make(map[string]bool, len(l.oldList))
	for _, n := range l.oldList {
		old[n] = true
	}
	next := make(map[string]bool, len(l.newList))
	for _, n := range l.newList {
		next[n] = true
		if !old[n] {
			added = append(added, n)
		}
	}
	for _, n := range l.oldList {
		if !next[n] {
			removed = append(removed, n)
		}
	}
	return added, removed
}

// specFile is a spec read for a membership change.
type specFile struct {
	name string
	path string
	raw  []byte
	fm   *metadata.Metadata
	body []byte
}

func readSpecFile(st store.Store, cfg config.Config, name string) (specFile, error) {
	path := artifact.Address{Kind: artifact.KindSpec, Feature: name}.StorePath(cfg.Spec.Config.Directory)
	raw, err := st.Read(path)
	if err != nil {
		return specFile{}, err
	}
	fm, body, err := metadata.Split(raw)
	if err != nil {
		return specFile{}, err
	}
	return specFile{name: name, path: path, raw: raw, fm: fm, body: body}, nil
}

func (s specFile) epic() string {
	if s.fm == nil {
		return ""
	}
	return s.fm.Epic
}

// epicPath is where the named epic is stored.
func epicPath(cfg config.Config, name string) string {
	return artifact.Address{Kind: artifact.KindEpic, Feature: name}.StorePath(cfg.Epic.Config.Directory)
}

// checkEpicLinks refuses a membership change that cannot be made, before
// anything is written. Every joining spec must exist, must not already belong
// to another epic, and must not be an epic itself.
func checkEpicLinks(st store.Store, cfg config.Config, l epicLinks) error {
	added, _ := l.diff()
	for _, name := range added {
		spec, err := readSpecFile(st, cfg, name)
		if errors.Is(err, store.ErrNotFound) {
			if name != l.epic && st.Exists(epicPath(cfg, name)) || name == l.epic {
				return output.NewError("epic_nested",
					fmt.Sprintf("%q is an epic, not a spec; an epic's specs list may only name specs, and epics do not nest", name)).
					WithResource(name).
					WithNextAction(fmt.Sprintf("remove %q from the specs list and list the specs themselves; run `%s epic read %s` to see the specs it holds", name, cfg.Command, name))
			}
			return output.NewError("epic_spec_not_found",
				fmt.Sprintf("no spec named %q is stored in this project", name)).
				WithResource(name).
				WithNextAction(fmt.Sprintf("run `%s spec file list` to see the stored specs, and list only those in the epic", cfg.Command))
		}
		if err != nil {
			return err
		}
		if other := spec.epic(); other != "" && other != l.epic {
			return output.NewError("epic_membership_conflict",
				fmt.Sprintf("spec %q already belongs to epic %q; a spec belongs to at most one epic", name, other)).
				WithResource(name).
				WithNextAction(fmt.Sprintf("remove it from %q first with `%s epic write %s --from <its current body> --data '{\"specs\":[…the other specs…]}'`, or leave it out of this epic", other, cfg.Command, other))
		}
	}
	return nil
}

// writeEpicLink sets (epic non-empty) or clears (epic empty) a spec's epic,
// rewriting only its frontmatter. The body is carried through untouched.
func writeEpicLink(t *docTxn, spec specFile, epic string) error {
	merged, err := metadata.Merge(spec.raw, spec.body, metadata.UpdateOptions{Epic: &epic})
	if err != nil {
		return err
	}
	return t.write(spec.path, merged)
}

// writeEpicLinkFn indirects the per-spec back-link write so a test can make
// the Nth write fail, the only way to exercise the compensating rollback. A
// test substituting it must restore it in t.Cleanup.
var writeEpicLinkFn = writeEpicLink

// applyEpicLinks writes the epic and then brings every affected spec into
// agreement with it, inside t. checkEpicLinks must already have passed. On a
// failure it restores everything t touched, including writes the caller made
// before calling it, and returns the reported refusal.
func applyEpicLinks(t *docTxn, cfg config.Config, l epicLinks, epicBytes []byte) (linked, unlinked []string, err error) {
	if err := t.write(epicPath(cfg, l.epic), epicBytes); err != nil {
		return nil, nil, t.fail(err)
	}
	added, removed := l.diff()
	for _, name := range added {
		spec, readErr := readSpecFile(t.st, cfg, name)
		if readErr != nil {
			return nil, nil, t.fail(readErr)
		}
		if spec.epic() == l.epic {
			continue
		}
		if err := writeEpicLinkFn(t, spec, l.epic); err != nil {
			return nil, nil, t.fail(err)
		}
		linked = append(linked, name)
	}
	for _, name := range removed {
		spec, readErr := readSpecFile(t.st, cfg, name)
		if errors.Is(readErr, store.ErrNotFound) {
			continue
		}
		if readErr != nil {
			return nil, nil, t.fail(readErr)
		}
		if spec.epic() != l.epic {
			continue
		}
		if err := writeEpicLinkFn(t, spec, ""); err != nil {
			return nil, nil, t.fail(err)
		}
		unlinked = append(unlinked, name)
	}
	return linked, unlinked, nil
}

// refuseIfInEpic refuses to delete a spec that still belongs to an epic: the
// epic would list a spec that is not there. It names the epic write that
// takes the spec out first.
func refuseIfInEpic(cfg config.Config, st store.Store, storePath string) error {
	raw, err := st.Read(storePath)
	if err != nil {
		return nil
	}
	fm, _, err := metadata.Split(raw)
	if err != nil || fm == nil || fm.Epic == "" {
		return nil
	}
	name := strings.TrimSuffix(filepath.Base(storePath), filepath.Ext(storePath))
	return output.NewError("spec_in_epic_delete",
		fmt.Sprintf("spec %q belongs to epic %q, which still lists it; deleting it would leave the epic naming a spec that is not there", name, fm.Epic)).
		WithResource(name).
		WithNextAction(fmt.Sprintf("remove it from the epic first with `%s epic write %s --from <the epic's current body> --data '{\"specs\":[…every spec except %s…]}'` (run `%s epic read %s` for the current list), then delete it",
			cfg.Command, fm.Epic, name, cfg.Command, fm.Epic))
}

// refuseCompletedEpic refuses to add a spec to an epic whose specs are all
// implemented, unless the caller confirmed it. Completion is derived on read
// by the status builder, so once a spec is added the epic simply reads as in
// progress again; nothing has to be reset. retry is the caller's command with
// the confirmation added, for the next action.
func refuseCompletedEpic(cfg config.Config, st store.Store, epicName string, confirmed bool, retry string) error {
	if confirmed {
		return nil
	}
	done, err := status.EpicComplete(status.Options{Config: cfg, Store: st}, epicName)
	if err != nil || !done {
		return err
	}
	return output.NewError("epic_complete",
		fmt.Sprintf("epic %s is complete: every spec is implemented; adding a spec reopens it until the new spec is implemented too", epicName)).
		WithResource(epicName).
		WithNextAction(fmt.Sprintf("ask the user whether to add to the completed epic; only if they agree, re-run %s", retry))
}

// joinSpecToEpic appends a newly written spec to the named epic's specs with
// no dependencies and sets the spec's epic, inside t, so the two sides agree
// from the start. On a refusal or a failed write it restores everything t
// touched, including documents the caller remembered before calling it.
func joinSpecToEpic(t *docTxn, cfg config.Config, epicName, specName string) error {
	current, _, err := readEpic(cfg, t.st, epicName)
	if err != nil {
		return t.abort(err)
	}
	next := current
	next.Specs = append(slices.Clone(current.Specs), epic.EpicSpec{Name: specName, DependsOn: []string{}})
	if err := epic.Validate(next.Specs); err != nil {
		return t.abort(err)
	}
	if next, err = epic.Stamp(&current, next, nil, time.Now().UTC()); err != nil {
		return t.abort(err)
	}
	links := epicLinks{epic: epicName, oldList: current.SpecNames(), newList: next.SpecNames()}
	if err := checkEpicLinks(t.st, cfg, links); err != nil {
		return t.abort(err)
	}
	rendered, err := next.Render()
	if err != nil {
		return t.abort(err)
	}
	_, _, err = applyEpicLinks(t, cfg, links, rendered)
	return err
}
