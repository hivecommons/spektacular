package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	return NewFileStore(t.TempDir(), "project")
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
	}, entries)
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
