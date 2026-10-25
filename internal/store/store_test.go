package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hivecommons/spektacular/internal/filelock"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	return NewFileStore(t.TempDir(), "project")
}

// withoutModTime strips the timestamp from each entry so a test about names
// and kinds can compare against literals without pinning the clock.
func withoutModTime(entries []DirEntry) []DirEntry {
	out := make([]DirEntry, len(entries))
	for i, e := range entries {
		out[i] = DirEntry{Name: e.Name, IsDir: e.IsDir}
	}
	return out
}

func TestWrite_CreatesFileAndParentDirs(t *testing.T) {
	st := newTestStore(t)
	err := st.Write("subdir/file.txt", []byte("hello"))
	require.NoError(t, err)
	data, err := st.Read("subdir/file.txt")
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestWrite_OverwritesExisting(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("file.txt", []byte("v1")))
	require.NoError(t, st.Write("file.txt", []byte("v2")))
	data, err := st.Read("file.txt")
	require.NoError(t, err)
	require.Equal(t, []byte("v2"), data)
}

func TestRead_ReturnsErrNotFoundForMissing(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Read("missing.txt")
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestDelete_RemovesFile(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("file.txt", []byte("data")))
	require.NoError(t, st.Delete("file.txt"))
	require.False(t, st.Exists("file.txt"))
}

func TestDelete_IdempotentOnMissing(t *testing.T) {
	st := newTestStore(t)
	err := st.Delete("nonexistent.txt")
	require.NoError(t, err)
}

func TestList_DistinguishesFilesFromDirectories(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("dir/a.txt", []byte("a")))
	require.NoError(t, st.Write("dir/b.txt", []byte("b")))
	require.NoError(t, st.Write("dir/sub/c.txt", []byte("c")))
	entries, err := st.List("dir")
	require.NoError(t, err)
	require.ElementsMatch(t, []DirEntry{
		{Name: "a.txt", IsDir: false},
		{Name: "b.txt", IsDir: false},
		{Name: "sub", IsDir: true},
	}, withoutModTime(entries))
}

// List carries each entry's modification time, so a caller listing N
// artifacts learns when each last changed without N further Stat calls.
func TestList_CarriesModTime(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("dir/a.txt", []byte("a")))
	require.NoError(t, st.Write("dir/sub/c.txt", []byte("c")))
	fileTime := time.Date(2026, time.March, 4, 5, 6, 7, 0, time.UTC)
	dirTime := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(st.Root(), "dir", "a.txt"), fileTime, fileTime))
	require.NoError(t, os.Chtimes(filepath.Join(st.Root(), "dir", "sub"), dirTime, dirTime))

	entries, err := st.List("dir")
	require.NoError(t, err)
	byName := map[string]DirEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	require.True(t, fileTime.Equal(byName["a.txt"].ModTime), "file entry carries its mtime")
	require.True(t, dirTime.Equal(byName["sub"].ModTime), "directory entry carries its mtime")
}

func TestList_ReturnsErrNotFoundForMissingDir(t *testing.T) {
	st := newTestStore(t)
	_, err := st.List("nodir")
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestExists_TrueForFile(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("file.txt", []byte("x")))
	require.True(t, st.Exists("file.txt"))
}

func TestExists_FalseForMissing(t *testing.T) {
	st := newTestStore(t)
	require.False(t, st.Exists("missing.txt"))
}

func TestStat_ReportsModTimeAndLeavesCreatedAtZero(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("file.txt", []byte("x")))
	modTime := time.Date(2026, time.January, 4, 5, 6, 7, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(st.Root(), "file.txt"), modTime, modTime))

	info, err := st.Stat("file.txt")
	require.NoError(t, err)
	require.True(t, modTime.Equal(info.ModTime), "ModTime must be the filesystem mtime, got %s", info.ModTime)
	require.True(t, info.CreatedAt.IsZero(), "FileStore does not report a portable birth time")
}

func TestStat_ReturnsErrNotFoundForMissing(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Stat("missing.txt")
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestStat_RejectsPathTraversal(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Stat("../outside.txt")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNotFound), "traversal is a refusal, not a missing file")
}

func TestRoot_ReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	st := NewFileStore(dir, "project")
	require.Equal(t, filepath.Clean(dir), st.Root())
}

func TestNewFileStore_RecordsLabel(t *testing.T) {
	st := NewFileStore(t.TempDir(), "project:project")
	require.Equal(t, "project:project", st.Label())
}

var _ Store = (*FileStore)(nil)

func TestPathTraversal_Rejected(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Read("../escape.txt")
	require.Error(t, err)
	err = st.Write("../escape.txt", []byte("x"))
	require.Error(t, err)
	err = st.Delete("../escape.txt")
	require.Error(t, err)
	_, err = st.List("../escape")
	require.Error(t, err)
}

// TestPathTraversal_ErrorNamesAttemptedPath asserts that abs()'s rejection
// message names the specific offending path (not just the generic "path
// escapes store root" phrase), across every Store method that resolves a
// path — so an agent handed the error can see which path it passed.
func TestPathTraversal_ErrorNamesAttemptedPath(t *testing.T) {
	st := newTestStore(t)
	const escapePath = "../../etc/passwd"
	const wantMsg = `path "../../etc/passwd" escapes store root`

	_, err := st.Read(escapePath)
	require.EqualError(t, err, wantMsg)

	err = st.Write(escapePath, []byte("x"))
	require.EqualError(t, err, wantMsg)

	err = st.Delete(escapePath)
	require.EqualError(t, err, wantMsg)

	_, err = st.List(escapePath)
	require.EqualError(t, err, wantMsg)
}

// Compile-time interface-satisfaction assertions. Store is split into a read
// half (Reader) and a write half (Writer), and these fail to build if an
// implementation stops satisfying any of the three, naming which one.
//
// What they deliberately do not catch: a method added to Store directly
// rather than to Reader or Writer still compiles here, because both
// implementations would carry it. Keeping the split complete is a review
// obligation, not something the compiler can check — a new method belongs in
// Reader if it only observes the store and in Writer if it mutates it.
var (
	_ Reader = (*FileStore)(nil)
	_ Writer = (*FileStore)(nil)
	_ Store  = (*FileStore)(nil)

	// The decorator returned by NewIgnoreStore and NewSourceStore.
	_ Reader = (*ignoreStore)(nil)
	_ Writer = (*ignoreStore)(nil)
	_ Store  = (*ignoreStore)(nil)
)

// Update hands fn exists=false and no bytes when the file is absent, and
// writes what fn returns, creating parent directories.
func TestUpdate_MissingFilePassesExistsFalse(t *testing.T) {
	st := newTestStore(t)

	var sawExists = true
	var sawBytes []byte
	err := st.Update("sub/counter.json", func(current []byte, exists bool) ([]byte, error) {
		sawExists, sawBytes = exists, current
		return []byte("1\n"), nil
	})
	require.NoError(t, err)
	require.False(t, sawExists)
	require.Nil(t, sawBytes)

	data, err := st.Read("sub/counter.json")
	require.NoError(t, err)
	require.Equal(t, "1\n", string(data))

	err = st.Update("sub/counter.json", func(current []byte, exists bool) ([]byte, error) {
		sawExists, sawBytes = exists, current
		return []byte("2\n"), nil
	})
	require.NoError(t, err)
	require.True(t, sawExists)
	require.Equal(t, "1\n", string(sawBytes))
}

// A callback that fails aborts the update: the file keeps its bytes, the error
// is returned, and neither a temp file nor the lock is left behind.
func TestUpdate_FailingCallbackLeavesTheFileUnchanged(t *testing.T) {
	st := newTestStore(t)
	require.NoError(t, st.Write("doc.json", []byte("original\n")))
	boom := errors.New("boom")

	err := st.Update("doc.json", func([]byte, bool) ([]byte, error) {
		return []byte("half-made"), boom
	})
	require.ErrorIs(t, err, boom)

	data, err := st.Read("doc.json")
	require.NoError(t, err)
	require.Equal(t, "original\n", string(data))
	requireOnlyFiles(t, st.Root(), "doc.json")
}

// A nil result writes nothing: an absent file stays absent, and a present one
// keeps its bytes and its modification time.
func TestUpdate_NilResultIsANoOp(t *testing.T) {
	st := newTestStore(t)

	require.NoError(t, st.Update("absent.json", func([]byte, bool) ([]byte, error) { return nil, nil }))
	require.False(t, st.Exists("absent.json"))

	require.NoError(t, st.Write("present.json", []byte("kept\n")))
	past := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	require.NoError(t, os.Chtimes(filepath.Join(st.Root(), "present.json"), past, past))
	require.NoError(t, st.Update("present.json", func([]byte, bool) ([]byte, error) { return nil, nil }))

	info, err := st.Stat("present.json")
	require.NoError(t, err)
	require.True(t, past.Equal(info.ModTime), "a nil result must not rewrite the file")
	requireOnlyFiles(t, st.Root(), "present.json")
}

// Update refuses a path that escapes the store root, like every other call.
func TestUpdate_RejectsPathTraversal(t *testing.T) {
	st := newTestStore(t)
	err := st.Update("../escape.json", func([]byte, bool) ([]byte, error) { return []byte("x"), nil })
	require.Error(t, err)
}

// Many updaters, each holding its own store instance over the same root as
// separate processes would, increment one counter through Update. None is
// lost: the final value is exactly the number of increments, and the lock is
// gone afterwards.
func TestUpdate_ConcurrentIncrementsFromSeparateInstancesAreAllKept(t *testing.T) {
	// A short first backoff keeps sixteen waiters from sleeping their way up
	// to the one-second cap; the lock path under test is unchanged.
	oldBackoff := filelock.Backoff
	filelock.Backoff = time.Millisecond
	t.Cleanup(func() { filelock.Backoff = oldBackoff })
	root := t.TempDir()
	const workers, perWorker = 16, 25

	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := NewFileStore(root, "project")
			for range perWorker {
				err := st.Update("counter.json", func(current []byte, exists bool) ([]byte, error) {
					var doc struct{ N int }
					if exists {
						if err := json.Unmarshal(current, &doc); err != nil {
							return nil, err
						}
					}
					doc.N++
					return json.Marshal(doc)
				})
				if err != nil {
					errs[i] = err
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	data, err := os.ReadFile(filepath.Join(root, "counter.json"))
	require.NoError(t, err)
	var doc struct{ N int }
	require.NoError(t, json.Unmarshal(data, &doc))
	require.Equal(t, 400, doc.N, "every increment must survive")
	requireOnlyFiles(t, root, "counter.json")
}

// requireOnlyFiles asserts the directory holds exactly the named files: no
// lock file and no temp file left over from an update.
func requireOnlyFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	require.ElementsMatch(t, names, got)
}
