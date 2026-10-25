package knowledge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/filelock"
	"github.com/hivecommons/spektacular/internal/output"
	"github.com/hivecommons/spektacular/internal/store"
	"github.com/stretchr/testify/require"
)

// projectAddr is the address singleSourceSet's one store answers to.
var projectAddr = Address{Tier: TierProject, Name: "project"}

// readIndex decodes the usage index as it stands on disk in dir, so a test can
// see records Usage would not report (such as one for a file that is gone).
func readIndex(t *testing.T, dir string) *UsageIndex {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, UsageIndexFile))
	require.NoError(t, err)
	idx, err := decodeUsageIndex(raw)
	require.NoError(t, err)
	return idx
}

// usageOf returns the reported usage of the entry at p in the set's stores.
func usageOf(t *testing.T, set *Set, p string) EntryUsage {
	t.Helper()
	usage, err := set.Usage(Selector{Tier: TierAll})
	require.NoError(t, err)
	for _, u := range usage {
		if u.Path == p {
			return u
		}
	}
	require.Failf(t, "entry not reported", "no usage reported for %q", p)
	return EntryUsage{}
}

// Criterion 1: a discovery and a retrieval each add exactly one to their own
// count, stamped with the time given, and a retrieval keeps the workflow and
// spec it was given; one recorded without a workflow carries neither.
func TestSet_UsageRecordsAddOneWithTheGivenTimeAndWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/compass.md", "# Compass\n\nthe compass points north\n")
	set := singleSourceSet(t, dir)

	found := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	read := time.Date(2026, 3, 1, 9, 5, 0, 0, time.UTC)
	readAgain := time.Date(2026, 3, 2, 14, 30, 0, 0, time.UTC)

	hit := store.Hit{Tier: "project", Name: "project", Path: "learnings/compass.md"}
	require.NoError(t, set.RecordDiscoveries([]store.Hit{hit}, found))

	u := usageOf(t, set, "learnings/compass.md")
	require.True(t, u.Tracked)
	require.Equal(t, 1, u.Discoveries.Count)
	require.NotNil(t, u.Discoveries.Last)
	require.True(t, found.Equal(*u.Discoveries.Last))
	require.Len(t, u.Discoveries.Recent, 1)
	require.True(t, found.Equal(u.Discoveries.Recent[0].At))
	require.Empty(t, u.Discoveries.Recent[0].Workflow)
	require.Empty(t, u.Discoveries.Recent[0].Spec)
	require.Equal(t, 0, u.Retrievals.Count, "a discovery is not a retrieval")
	require.Nil(t, u.Retrievals.Last)

	entry := Entry{Tier: TierProject, Name: "project", Path: "learnings/compass.md"}
	require.NoError(t, set.RecordRetrievals([]Entry{entry}, read,
		&WorkflowRef{Kind: "implement", Spec: "000072_knowledge-access-index-and-prune"}))
	require.NoError(t, set.RecordRetrievals([]Entry{entry}, readAgain, nil))

	u = usageOf(t, set, "learnings/compass.md")
	require.Equal(t, 1, u.Discoveries.Count, "a retrieval is not a discovery")
	require.Equal(t, 2, u.Retrievals.Count)
	require.True(t, readAgain.Equal(*u.Retrievals.Last))
	require.Len(t, u.Retrievals.Recent, 2)
	require.True(t, read.Equal(u.Retrievals.Recent[0].At))
	require.Equal(t, "implement", u.Retrievals.Recent[0].Workflow)
	require.Equal(t, "000072_knowledge-access-index-and-prune", u.Retrievals.Recent[0].Spec)
	require.True(t, readAgain.Equal(u.Retrievals.Recent[1].At))
	require.Empty(t, u.Retrievals.Recent[1].Workflow, "no workflow given, none recorded")
	require.Empty(t, u.Retrievals.Recent[1].Spec)
}

// Criterion 1: the recent list is capped at 20 events, keeping the newest,
// while the count keeps rising past it.
func TestSet_UsageRecentListIsCappedWhileTheCountKeepsGrowing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/busy.md", "# Busy\n\nread often\n")
	set := singleSourceSet(t, dir)

	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	entry := Entry{Tier: TierProject, Name: "project", Path: "learnings/busy.md"}
	for i := 0; i < 25; i++ {
		require.NoError(t, set.RecordRetrievals([]Entry{entry}, base.Add(time.Duration(i)*time.Minute), nil))
	}

	u := usageOf(t, set, "learnings/busy.md")
	require.Equal(t, 25, u.Retrievals.Count)
	require.Len(t, u.Retrievals.Recent, 20)
	require.True(t, base.Add(5*time.Minute).Equal(u.Retrievals.Recent[0].At), "the oldest five are dropped")
	require.True(t, base.Add(24*time.Minute).Equal(u.Retrievals.Recent[19].At))
	require.True(t, base.Add(24*time.Minute).Equal(*u.Retrievals.Last))
}

// Criterion 2: an entry written and then rewritten keeps its first creation
// date, its tracking start and its counts.
func TestSet_UsageRewriteKeepsTheFirstCreationDateAndCounts(t *testing.T) {
	dir := t.TempDir()
	set := singleSourceSet(t, dir)

	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	used := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)

	require.NoError(t, set.Write(projectAddr, "learnings/new.md", []byte("# New\n\nfirst draft\n")))
	require.NoError(t, set.EnsureRecord(projectAddr, "learnings/new.md", t1))

	u := usageOf(t, set, "learnings/new.md")
	require.True(t, t1.Equal(u.Created), "a brand-new entry is created at the write, not at its mtime")
	require.True(t, t1.Equal(u.TrackedSince))

	entry := Entry{Tier: TierProject, Name: "project", Path: "learnings/new.md"}
	hit := store.Hit{Tier: "project", Name: "project", Path: "learnings/new.md"}
	require.NoError(t, set.RecordRetrievals([]Entry{entry}, used, nil))
	require.NoError(t, set.RecordDiscoveries([]store.Hit{hit, hit}, used))

	require.NoError(t, set.Write(projectAddr, "learnings/new.md", []byte("# New\n\nsecond draft\n")))
	require.NoError(t, set.EnsureRecord(projectAddr, "learnings/new.md", t2))

	u = usageOf(t, set, "learnings/new.md")
	require.True(t, t1.Equal(u.Created), "the rewrite keeps the first creation date")
	require.True(t, t1.Equal(u.TrackedSince))
	require.Equal(t, 1, u.Retrievals.Count)
	require.Equal(t, 2, u.Discoveries.Count)
}

// Criterion 3: entries that predate the index gain a record at the first index
// write, dated from version history when the dater has a date and from the
// file's modification time when it has not, with tracking starting at that
// write.
func TestSet_UsageReconcileBackfillsEntriesThatPredateTheIndex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/historic.md", "# Historic\n\ncommitted long ago\n")
	writeFile(t, dir, "learnings/handmade.md", "# Handmade\n\nnever committed\n")
	writeFile(t, dir, "learnings/trigger.md", "# Trigger\n\nthe entry that is read\n")

	committed := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	modified := time.Date(2025, 2, 20, 12, 0, 0, 0, time.UTC)
	triggerModified := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "learnings/handmade.md"), modified, modified))
	require.NoError(t, os.Chtimes(filepath.Join(dir, "learnings/trigger.md"), triggerModified, triggerModified))

	set := singleSourceSet(t, dir)
	var askedLocation string
	set.SetCreatedDater(func(storeLocation, entryPath string) (time.Time, bool) {
		askedLocation = storeLocation
		if entryPath == "learnings/historic.md" {
			return committed, true
		}
		return time.Time{}, false
	})

	entry := Entry{Tier: TierProject, Name: "project", Path: "learnings/trigger.md"}
	require.NoError(t, set.RecordRetrievals([]Entry{entry}, now, nil))
	require.Equal(t, dir, askedLocation, "the dater is asked about the store's own location")

	historic := usageOf(t, set, "learnings/historic.md")
	require.True(t, historic.Tracked)
	require.True(t, committed.Equal(historic.Created), "dated from version history")
	require.True(t, now.Equal(historic.TrackedSince), "tracking starts at the first index write")
	require.Equal(t, 0, historic.Retrievals.Count)
	require.Equal(t, 0, historic.Discoveries.Count)

	handmade := usageOf(t, set, "learnings/handmade.md")
	require.True(t, handmade.Tracked)
	require.True(t, modified.Equal(handmade.Created), "no history: dated from the file's modification time")
	require.True(t, now.Equal(handmade.TrackedSince))

	trigger := usageOf(t, set, "learnings/trigger.md")
	require.True(t, triggerModified.Equal(trigger.Created))
	require.True(t, now.Equal(trigger.TrackedSince))
	require.Equal(t, 1, trigger.Retrievals.Count)
}

// Criterion 4: a record for an entry deleted on disk, outside Spektacular, is
// dropped at the next index update.
func TestSet_UsageRecordOfAFileDeletedOnDiskIsDroppedAtTheNextUpdate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/keep.md", "# Keep\n\nstays\n")
	writeFile(t, dir, "learnings/gone.md", "# Gone\n\nremoved by hand\n")
	set := singleSourceSet(t, dir)

	at := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	keep := Entry{Tier: TierProject, Name: "project", Path: "learnings/keep.md"}
	gone := Entry{Tier: TierProject, Name: "project", Path: "learnings/gone.md"}
	require.NoError(t, set.RecordRetrievals([]Entry{keep, gone}, at, nil))

	idx := readIndex(t, dir)
	require.Contains(t, idx.Entries, "learnings/gone.md")

	require.NoError(t, os.Remove(filepath.Join(dir, "learnings/gone.md")))
	require.NoError(t, set.RecordRetrievals([]Entry{keep}, at.Add(time.Hour), nil))

	idx = readIndex(t, dir)
	require.NotContains(t, idx.Entries, "learnings/gone.md")
	require.Contains(t, idx.Entries, "learnings/keep.md")
	require.Equal(t, 2, idx.Entries["learnings/keep.md"].Retrievals.Count)
}

// Criterion 4: ForgetRecords drops the named path's record and leaves the
// others in place, even when the store's listing still shows the file.
func TestSet_UsageForgetRecordsDropsTheNamedPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/a.md", "# A\n\nfirst\n")
	writeFile(t, dir, "learnings/b.md", "# B\n\nsecond\n")
	set := singleSourceSet(t, dir)

	at := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, set.EnsureRecord(projectAddr, "learnings/a.md", at))
	idx := readIndex(t, dir)
	require.Contains(t, idx.Entries, "learnings/a.md")
	require.Contains(t, idx.Entries, "learnings/b.md")

	// The file is left on disk, standing in for a store whose listing cannot
	// tell it is gone, so the drop seen below is ForgetRecords' own and not
	// reconcile's.
	require.NoError(t, set.ForgetRecords(projectAddr, []string{"learnings/a.md"}))

	idx = readIndex(t, dir)
	require.NotContains(t, idx.Entries, "learnings/a.md")
	require.Contains(t, idx.Entries, "learnings/b.md")
}

// Criterion 4: the index, its lock and its temp files sitting in a store root
// never appear in a listing, a search, the tag vocabulary or the always-applied
// payload. Every reserved file mentions the query term and carries a tag, so a
// walk that did not exclude them would report them.
func TestSet_UsageIndexAndSidecarsNeverAppearAsEntries(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/compass.md", "---\ntags: [navigation]\n---\n# Compass\n\nthe compass points north\n")
	writeFile(t, dir, "conventions/style.md", "use tabs\n")
	writeFile(t, dir, UsageIndexFile,
		`{"version": 1, "entries": {"learnings/compass.md": {"created": "2026-01-01T00:00:00Z", "tracked_since": "2026-01-01T00:00:00Z", "discoveries": {"count": 0}, "retrievals": {"count": 0}}}}`+"\n")
	writeFile(t, dir, UsageIndexFile+".lock", "---\ntags: [bookkeeping]\n---\ncompass compass compass\n")
	writeFile(t, dir, UsageIndexFile+".tmp-123", "---\ntags: [bookkeeping]\n---\ncompass compass compass\n")
	set := singleSourceSet(t, dir)

	entries, err := set.List(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.ElementsMatch(t, []Entry{
		{Tier: TierProject, Name: "project", Path: "learnings/compass.md"},
		{Tier: TierProject, Name: "project", Path: "conventions/style.md"},
	}, entries)

	hits, err := set.Search("compass", Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []string{"learnings/compass.md"}, hitPaths(hits))

	uses, err := set.Tags(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []TagUse{{Tag: "navigation", Count: 1}}, uses)

	applied, err := set.AlwaysAppliedEntries(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []AlwaysAppliedEntry{
		{Tier: TierProject, Name: "project", Path: "conventions/style.md", Content: "use tabs\n", Category: "conventions"},
	}, applied)

	usage, err := set.Usage(Selector{Tier: TierAll})
	require.NoError(t, err)
	var usagePaths []string
	for _, u := range usage {
		usagePaths = append(usagePaths, u.Path)
	}
	require.ElementsMatch(t, []string{"learnings/compass.md", "conventions/style.md"}, usagePaths)
}

// Criterion 4: reading, writing or deleting the index or a sidecar as if it
// were an entry is refused, and the refusal points at "knowledge usage".
func TestSet_UsageIndexIsRefusedAsAnEntryAddress(t *testing.T) {
	dir := t.TempDir()
	original := `{"version": 1, "entries": {}}` + "\n"
	writeFile(t, dir, UsageIndexFile, original)
	set := singleSourceSet(t, dir)

	requireRefused := func(t *testing.T, err error) {
		t.Helper()
		var resp *output.ErrorResponse
		require.ErrorAs(t, err, &resp)
		require.Equal(t, ErrCodeIndexReserved, resp.Code)
		require.Contains(t, resp.NextAction, "knowledge usage")
	}

	for _, name := range []string{UsageIndexFile, UsageIndexFile + ".lock", UsageIndexFile + ".tmp-123"} {
		t.Run(name, func(t *testing.T) {
			_, err := set.Read(projectAddr, name)
			requireRefused(t, err)
			requireRefused(t, set.Write(projectAddr, name, []byte("overwritten\n")))
			requireRefused(t, set.Delete(projectAddr, name))
		})
	}

	raw, err := os.ReadFile(filepath.Join(dir, UsageIndexFile))
	require.NoError(t, err)
	require.Equal(t, original, string(raw), "neither the write nor the delete touched the index")
	require.NoFileExists(t, filepath.Join(dir, UsageIndexFile+".lock"))
	require.NoFileExists(t, filepath.Join(dir, UsageIndexFile+".tmp-123"))
}

func TestIsReservedPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{".usage-index.json", true},
		{".usage-index.json.lock", true},
		{".usage-index.json.tmp-123", true},
		{"./.usage-index.json", true},
		{"conventions/.usage-index.json", false},
		{"learnings/.usage-index.json.lock", false},
		{"usage-index.json", false},
		{".usage-index.json.bak", false},
		{"learnings/compass.md", false},
	}
	for _, c := range cases {
		require.Equal(t, c.want, IsReservedPath(c.path), "IsReservedPath(%q)", c.path)
	}
}

// Usage is read-only: it neither creates an index nor rewrites one, even when
// an entry on disk has no record, and it reports such an entry untracked.
func TestSet_UsageIsReadOnlyAndReportsAnUnrecordedEntryUntracked(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/a.md", "# A\n\nfirst\n")
	set := singleSourceSet(t, dir)

	usage, err := set.Usage(Selector{Tier: TierAll})
	require.NoError(t, err)
	require.Equal(t, []EntryUsage{{Tier: TierProject, Name: "project", Path: "learnings/a.md"}}, usage)
	require.NoFileExists(t, filepath.Join(dir, UsageIndexFile), "Usage must not create the index")

	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, set.EnsureRecord(projectAddr, "learnings/a.md", at))
	before, err := os.ReadFile(filepath.Join(dir, UsageIndexFile))
	require.NoError(t, err)

	writeFile(t, dir, "learnings/b.md", "# B\n\nadded by hand\n")
	b := usageOf(t, set, "learnings/b.md")
	require.False(t, b.Tracked)
	require.True(t, b.Created.IsZero())
	require.True(t, b.TrackedSince.IsZero())
	require.True(t, usageOf(t, set, "learnings/a.md").Tracked)

	after, err := os.ReadFile(filepath.Join(dir, UsageIndexFile))
	require.NoError(t, err)
	require.Equal(t, string(before), string(after), "Usage must not reconcile or rewrite the index")
}

// The index on disk is valid JSON carrying its format version and an entries
// map keyed by store-relative path.
func TestSet_UsageIndexIsVersionedJSONKeyedByPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "learnings/a.md", "# A\n\nfirst\n")
	writeFile(t, dir, "gotchas/b.md", "# B\n\nsecond\n")
	set := singleSourceSet(t, dir)

	at := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	require.NoError(t, set.EnsureRecord(projectAddr, "learnings/a.md", at))

	raw, err := os.ReadFile(filepath.Join(dir, UsageIndexFile))
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Equal(t, float64(1), doc["version"])
	entries, ok := doc["entries"].(map[string]any)
	require.True(t, ok, "entries is an object")
	require.Len(t, entries, 2)
	require.Contains(t, entries, "learnings/a.md")
	require.Contains(t, entries, "gotchas/b.md")
}

// Criterion 5: concurrent searches and reads of one entry, recorded by several
// independent Set instances over the same store, are all counted.
func TestSet_UsageConcurrentRecordsFromSeparateInstancesAreAllCounted(t *testing.T) {
	old := filelock.Backoff
	filelock.Backoff = time.Millisecond
	t.Cleanup(func() { filelock.Backoff = old })

	dir := t.TempDir()
	writeFile(t, dir, "learnings/shared.md", "# Shared\n\neveryone reads this\n")

	const workers = 8
	sets := make([]*Set, workers)
	for i := range sets {
		sets[i] = singleSourceSet(t, dir)
	}

	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	hit := store.Hit{Tier: "project", Name: "project", Path: "learnings/shared.md"}
	entry := Entry{Tier: TierProject, Name: "project", Path: "learnings/shared.md"}

	var wg sync.WaitGroup
	errs := make(chan error, 2*workers)
	for _, s := range sets {
		wg.Add(1)
		go func(s *Set) {
			defer wg.Done()
			errs <- s.RecordDiscoveries([]store.Hit{hit}, at)
			errs <- s.RecordRetrievals([]Entry{entry}, at, nil)
		}(s)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	u := usageOf(t, sets[0], "learnings/shared.md")
	require.Equal(t, workers, u.Discoveries.Count)
	require.Equal(t, workers, u.Retrievals.Count)

	matches, err := filepath.Glob(filepath.Join(dir, UsageIndexFile+"*"))
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(dir, UsageIndexFile)}, matches, "no lock or temp file is left behind")
}
