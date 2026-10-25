package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
)

// UsageIndexFile is the name of the usage index each knowledge store keeps at
// its root. It records, per entry path, when the entry was created, when the
// index first saw it, and how often searches surfaced it (discoveries) and
// reads returned it (retrievals). The index travels with the store, so the
// history moves with the knowledge base.
//
// The index is Spektacular's own bookkeeping, not a knowledge entry: it is
// changed only through this file's operations, never through Write or Delete,
// and it never appears in a listing, a search or the always-applied reader.
const UsageIndexFile = ".usage-index.json"

// usageIndexVersion is the format version a freshly made index carries.
const usageIndexVersion = 1

// usageRecentCap bounds each entry's list of recent events, so the index stays
// small however often an entry is used. The total count keeps rising past it.
const usageRecentCap = 20

// ErrCodeIndexReserved is returned when a read, write or delete addresses the
// usage index, or the lock and temp files beside it, as though it were an
// entry.
const ErrCodeIndexReserved = "knowledge_index_reserved"

// UsageIndex is one store's usage index document.
type UsageIndex struct {
	Version int                     `json:"version"`
	Entries map[string]*UsageRecord `json:"entries"`
}

// UsageRecord is what the index knows about one entry. Created is when the
// entry was first written (or, for an entry that predates the index, the best
// date its history offers); TrackedSince is when the index first saw it, which
// is what protects an entry that predates the index from being pruned for
// non-use before its usage has been watched for long enough.
type UsageRecord struct {
	Created      time.Time `json:"created"`
	TrackedSince time.Time `json:"tracked_since"`
	Discoveries  UsageLog  `json:"discoveries"`
	Retrievals   UsageLog  `json:"retrievals"`
}

// UsageLog is the running history of one kind of use: a total count, the time
// of the latest use, and a capped list of the most recent events.
type UsageLog struct {
	Count  int          `json:"count"`
	Last   *time.Time   `json:"last,omitempty"`
	Recent []UsageEvent `json:"recent,omitempty"`
}

// UsageEvent is one use. A retrieval made while a workflow runs names the
// workflow kind and its spec; the fields are omitted when no workflow is known.
type UsageEvent struct {
	At       time.Time `json:"at"`
	Workflow string    `json:"workflow,omitempty"`
	Spec     string    `json:"spec,omitempty"`
}

// WorkflowRef names the workflow a retrieval was made under: its kind (plan,
// implement, …) and the spec it is for.
type WorkflowRef struct {
	Kind string
	Spec string
}

// CreatedDater reports the date an entry was first added, from the store's
// version history, given the store's location and the entry's store-relative
// path. It reports false when the history has nothing to say. It is injected by
// the command layer so this package stays free of git.
type CreatedDater func(storeLocation, entryPath string) (time.Time, bool)

// EntryUsage is one entry's usage, tagged with the store it lives in. Tracked
// is false for an entry the index has not recorded yet; its record is then
// zero, and a caller treats its tracking as starting now.
type EntryUsage struct {
	Tier Tier   `json:"tier"`
	Name string `json:"name"`
	Path string `json:"path"`
	UsageRecord
	Tracked bool `json:"tracked"`
}

// IsReservedPath reports whether a store-relative path names the usage index
// or one of the files kept beside it while it is updated: its lock and its
// temp files. These are excluded from every enumeration and search, and
// refused as an entry address.
func IsReservedPath(p string) bool {
	clean := path.Clean(strings.TrimPrefix(filepath.ToSlash(p), "./"))
	if strings.Contains(clean, "/") {
		return false
	}
	return clean == UsageIndexFile ||
		clean == UsageIndexFile+store.LockSuffix ||
		strings.HasPrefix(clean, UsageIndexFile+store.TempInfix)
}

// refuseReserved builds the refusal for an entry address naming the usage
// index or its sidecar files.
func refuseReserved(addr Address, p string) error {
	return output.NewError(
		ErrCodeIndexReserved,
		fmt.Sprintf("%q is the store's usage index, written only by Spektacular", p),
	).WithResource(p).WithNextAction(fmt.Sprintf(
		`run "knowledge usage --tier %s --filter %s" to read it; it cannot be written or deleted directly`,
		addr.Tier, addr.Name,
	))
}

// SetCreatedDater installs the history lookup used to date an entry the index
// has not seen before. Nil means no history: such an entry is dated from the
// store's modification time for it instead.
func (s *Set) SetCreatedDater(d CreatedDater) {
	s.dater = d
}

// RecordDiscoveries records one discovery, at the given time, for each search
// hit, with one index update per store the hits came from. It is a separate
// call a command makes after it has its result, never part of Search itself,
// so an internal search can never count by accident.
func (s *Set) RecordDiscoveries(hits []store.Hit, at time.Time) error {
	groups := make(map[int][]string)
	for _, hit := range hits {
		i, ok := s.sourceIndex(Tier(hit.Tier), hit.Name)
		if !ok || IsReservedPath(hit.Path) {
			continue
		}
		groups[i] = append(groups[i], filepath.ToSlash(hit.Path))
	}
	return s.recordEach(groups, at, func(rec *UsageRecord) *UsageLog { return &rec.Discoveries }, nil)
}

// RecordRetrievals records one retrieval, at the given time, for each entry,
// naming the workflow when one is given, with one index update per store. Like
// RecordDiscoveries it is called by the command layer after the read it
// records, so the set's own reads stay untracked.
func (s *Set) RecordRetrievals(entries []Entry, at time.Time, wf *WorkflowRef) error {
	groups := make(map[int][]string)
	for _, e := range entries {
		i, ok := s.sourceIndex(e.Tier, e.Name)
		if !ok || IsReservedPath(e.Path) {
			continue
		}
		groups[i] = append(groups[i], filepath.ToSlash(e.Path))
	}
	return s.recordEach(groups, at, func(rec *UsageRecord) *UsageLog { return &rec.Retrievals }, wf)
}

// recordEach applies one event per listed path to the chosen log, store by
// store in configured order. The first failure is returned; stores updated
// before it keep their update.
func (s *Set) recordEach(groups map[int][]string, at time.Time, log func(*UsageRecord) *UsageLog, wf *WorkflowRef) error {
	at = at.UTC()
	event := UsageEvent{At: at}
	if wf != nil {
		event.Workflow, event.Spec = wf.Kind, wf.Spec
	}
	for i, src := range s.sources {
		paths := groups[i]
		if len(paths) == 0 {
			continue
		}
		err := s.update(src, at, func(idx *UsageIndex, _ map[string]bool) {
			for _, p := range paths {
				rec := idx.Entries[p]
				if rec == nil {
					rec = s.newRecord(src, p, at)
					idx.Entries[p] = rec
				}
				l := log(rec)
				l.Count++
				last := at
				l.Last = &last
				l.Recent = append(l.Recent, event)
				if len(l.Recent) > usageRecentCap {
					l.Recent = l.Recent[len(l.Recent)-usageRecentCap:]
				}
			}
		})
		if err != nil {
			return fmt.Errorf("recording usage in knowledge store %q: %w", src.name, err)
		}
	}
	return nil
}

// EnsureRecord makes sure the entry at path has a record, after a write. An
// existing record is left untouched, so an entry's creation date and its
// counts survive every later rewrite. A path seen for the first time is
// created now, unless the store's history dates it earlier.
func (s *Set) EnsureRecord(addr Address, p string, at time.Time) error {
	if IsReservedPath(p) {
		return refuseReserved(addr, p)
	}
	src, err := s.resolve(addr)
	if err != nil {
		return err
	}
	at = at.UTC()
	p = filepath.ToSlash(p)
	return s.update(src, at, func(idx *UsageIndex, fromHistory map[string]bool) {
		rec, ok := idx.Entries[p]
		if !ok {
			idx.Entries[p] = &UsageRecord{Created: at, TrackedSince: at}
			return
		}
		if dated, fresh := fromHistory[p]; fresh && !dated {
			// Reconcile has just met this path, and its history had no date
			// for it: it is new with this write, so it was created now rather
			// than at whatever modification time the write itself produced.
			rec.Created = at
		}
	})
}

// ForgetRecords drops the records for the named paths, after they were
// deleted. Reconcile drops the record of any vanished file as well; naming the
// paths makes the drop explicit, so it happens even for a store whose listing
// cannot tell.
func (s *Set) ForgetRecords(addr Address, paths []string) error {
	src, err := s.resolve(addr)
	if err != nil {
		return err
	}
	return s.update(src, time.Now().UTC(), func(idx *UsageIndex, _ map[string]bool) {
		for _, p := range paths {
			delete(idx.Entries, filepath.ToSlash(p))
		}
	})
}

// Usage reports the usage of every entry across the stores the selector
// covers, in configured store order then listing order. It is read-only: it
// reads each index as it stands, writes nothing and reconciles nothing. An
// entry the index has not recorded yet is reported with Tracked false.
// Category descriptions are not entries and are left out.
func (s *Set) Usage(sel Selector) ([]EntryUsage, error) {
	if err := s.validateSelector(sel); err != nil {
		return nil, err
	}
	var usage []EntryUsage
	for _, src := range s.sources {
		if !sel.covers(src) {
			continue
		}
		raw, err := src.store.Read(UsageIndexFile)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("reading the usage index of knowledge store %q: %w", src.name, err)
		}
		idx, err := decodeUsageIndex(raw)
		if err != nil {
			return nil, fmt.Errorf("reading the usage index of knowledge store %q: %w", src.name, err)
		}
		files, err := listFiles(src.store, "")
		if err != nil {
			return nil, fmt.Errorf("listing knowledge store %q: %w", src.name, err)
		}
		for _, f := range files {
			if IsCategoryDescription(f) {
				continue
			}
			eu := EntryUsage{Tier: src.tier, Name: src.name, Path: f}
			if rec, ok := idx.Entries[f]; ok {
				eu.UsageRecord = *rec
				eu.Tracked = true
			}
			usage = append(usage, eu)
		}
	}
	return usage, nil
}

// update is the single path every index change takes: one atomic
// read-modify-write of the store's index through store.Writer.Update, so
// parallel processes never lose one another's changes. Before mutate runs the
// index is reconciled with the store's entries; mutate is told which paths
// reconcile has just added, and whether each was dated from history.
func (s *Set) update(src scopedStore, at time.Time, mutate func(idx *UsageIndex, fresh map[string]bool)) error {
	return src.store.Update(UsageIndexFile, func(current []byte, _ bool) ([]byte, error) {
		idx, err := decodeUsageIndex(current)
		if err != nil {
			return nil, err
		}
		fresh, err := s.reconcile(src, idx, at)
		if err != nil {
			return nil, err
		}
		mutate(idx, fresh)
		return encodeUsageIndex(idx)
	})
}

// reconcile brings an index in line with the store: every entry the store
// lists gains a record if it has none, and the record of any path that no
// longer exists is dropped. That one rule covers entries that predate the
// index, entries written by hand, and entries removed outside Spektacular, so
// there is no migration step. It returns the paths it added, each mapped to
// whether its creation date came from history.
func (s *Set) reconcile(src scopedStore, idx *UsageIndex, at time.Time) (map[string]bool, error) {
	files, err := listFiles(src.store, "")
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("listing knowledge store %q: %w", src.name, err)
	}
	fresh := make(map[string]bool)
	for _, f := range files {
		if IsCategoryDescription(f) {
			continue
		}
		if _, ok := idx.Entries[f]; ok {
			continue
		}
		rec, dated := s.datedRecord(src, f, at)
		idx.Entries[f] = rec
		fresh[f] = dated
	}
	for p := range idx.Entries {
		if !src.store.Exists(p) {
			delete(idx.Entries, p)
		}
	}
	return fresh, nil
}

// newRecord makes the record for a path the index has never seen, outside a
// reconcile: an entry a listing does not show, such as one an ignore rule
// hides, but which was still read or written by its exact path.
func (s *Set) newRecord(src scopedStore, p string, at time.Time) *UsageRecord {
	rec, _ := s.datedRecord(src, p, at)
	return rec
}

// datedRecord makes a fresh record whose tracking starts at at, dated from the
// store's history when the dater has a date, else from the store's
// modification time for the path, else at. It reports whether the date came
// from history.
func (s *Set) datedRecord(src scopedStore, p string, at time.Time) (*UsageRecord, bool) {
	rec := &UsageRecord{Created: at, TrackedSince: at}
	if s.dater != nil {
		if created, ok := s.dater(src.location, p); ok {
			rec.Created = created.UTC()
			return rec, true
		}
	}
	if info, err := src.store.Stat(p); err == nil && !info.ModTime.IsZero() {
		rec.Created = info.ModTime.UTC()
	}
	return rec, false
}

// sourceIndex finds the configured store a tier and name address.
func (s *Set) sourceIndex(tier Tier, name string) (int, bool) {
	for i, src := range s.sources {
		if src.tier == tier && src.name == name {
			return i, true
		}
	}
	return 0, false
}

// decodeUsageIndex parses an index document; empty input is a fresh index.
func decodeUsageIndex(raw []byte) (*UsageIndex, error) {
	idx := &UsageIndex{Version: usageIndexVersion}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, idx); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", UsageIndexFile, err)
		}
	}
	if idx.Entries == nil {
		idx.Entries = make(map[string]*UsageRecord)
	}
	return idx, nil
}

// encodeUsageIndex renders an index as indented JSON with a trailing newline.
// Map keys are sorted by the encoder, so diffs of the index stay line-stable.
func encodeUsageIndex(idx *UsageIndex) ([]byte, error) {
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// PruneRemoval is one entry the age rule removes, with the usage evidence that
// put it there: when it was created, how long it has been tracked, when it was
// last retrieved (nil for never), and how often searches surfaced it.
type PruneRemoval struct {
	Tier          Tier       `json:"tier"`
	Name          string     `json:"name"`
	Path          string     `json:"path"`
	Created       time.Time  `json:"created"`
	TrackedSince  time.Time  `json:"tracked_since"`
	LastRetrieval *time.Time `json:"last_retrieval,omitempty"`
	Discoveries   int        `json:"discoveries"`
}

// PruneCandidates applies prune's age rule to a usage report. An entry is
// removed when it was created more than maxAge ago, has been tracked for at
// least maxAge, and has not been retrieved within maxAge. A discovery does not
// keep an entry: an entry searches keep surfacing but nobody reads is still
// unused. An entry the index has not tracked yet is never removed, since its
// tracking starts now; that is what keeps the first prune after the index
// arrives from removing every entry that predates it. Category descriptions
// are not entries and are never candidates.
func PruneCandidates(usage []EntryUsage, maxAge time.Duration, now time.Time) []PruneRemoval {
	var removals []PruneRemoval
	for _, eu := range usage {
		if !eu.Tracked || IsCategoryDescription(eu.Path) || IsReservedPath(eu.Path) {
			continue
		}
		if now.Sub(eu.Created) <= maxAge || now.Sub(eu.TrackedSince) < maxAge {
			continue
		}
		if last := eu.Retrievals.Last; last != nil && now.Sub(*last) <= maxAge {
			continue
		}
		removals = append(removals, PruneRemoval{
			Tier:          eu.Tier,
			Name:          eu.Name,
			Path:          eu.Path,
			Created:       eu.Created,
			TrackedSince:  eu.TrackedSince,
			LastRetrieval: eu.Retrievals.Last,
			Discoveries:   eu.Discoveries.Count,
		})
	}
	return removals
}
