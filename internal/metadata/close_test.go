package metadata

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeStore is a tiny in-memory StoreReadWriter used to exercise Close without
// depending on the concrete FileStore implementation. readErr, when non-nil,
// short-circuits Read so callers can assert error propagation.
type fakeStore struct {
	data    map[string][]byte
	readErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[string][]byte{}}
}

func (f *fakeStore) Read(path string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	b, ok := f.data[path]
	if !ok {
		return nil, errors.New("fake store: not found")
	}
	return b, nil
}

func (f *fakeStore) Write(path string, content []byte) error {
	f.data[path] = append([]byte{}, content...)
	return nil
}

// today returns time.Now().UTC() truncated to day, matching the value Merge
// (and therefore Close) uses when UpdateOptions.Today is unset.
func today() time.Time {
	return time.Now().UTC().Truncate(24 * time.Hour)
}

// TestClose_TransitionsDraftToFinal seeds the fake store with an
// draft artifact and asserts Close rewrites its frontmatter to final
// with today's date stamped as closed_date, preserving created_date.
func TestClose_TransitionsDraftToFinal(t *testing.T) {
	fake := newFakeStore()
	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	seed, err := Render(Metadata{
		CreatedDate:    created,
		DocumentStatus: StatusDraft,
	}, []byte("# Body\n"))
	require.NoError(t, err)
	require.NoError(t, fake.Write("doc.md", seed))

	require.NoError(t, Close(fake, "doc.md", StatusFinal))

	got := fake.data["doc.md"]
	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.True(t, meta.CreatedDate.Equal(created), "created_date must be preserved")
	require.Equal(t, StatusFinal, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(today()), "closed_date must be stamped to today")
	require.Equal(t, "# Body\n", string(body), "body must be preserved byte-for-byte")
}

// TestClose_OnBareArtifactAttachesFrontmatter seeds the fake store with raw
// markdown (no frontmatter block) and asserts Close attaches a fresh
// frontmatter block: created_date=today, the caller-provided status, and
// closed_date=today because the status is a closed one.
func TestClose_OnBareArtifactAttachesFrontmatter(t *testing.T) {
	fake := newFakeStore()
	require.NoError(t, fake.Write("bare.md", []byte("# Just a heading\n")))

	require.NoError(t, Close(fake, "bare.md", StatusArchived))

	got := fake.data["bare.md"]
	meta, body, err := Split(got)
	require.NoError(t, err)
	require.NotNil(t, meta, "Close must attach frontmatter to a bare artifact")
	require.True(t, meta.CreatedDate.Equal(today()), "created_date must be today")
	require.Equal(t, StatusArchived, meta.DocumentStatus)
	require.True(t, meta.ClosedDate.Equal(today()), "closed_date must be stamped to today")
	require.Equal(t, "# Just a heading\n", string(body), "body must be preserved")
}

// TestClose_Idempotent seeds the fake store with an already-final artifact
// and asserts calling Close(..., StatusFinal) reproduces the same bytes
// byte-for-byte — the first-transition closed_date is preserved.
func TestClose_Idempotent(t *testing.T) {
	fake := newFakeStore()
	created := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	closed := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	seed, err := Render(Metadata{
		CreatedDate:    created,
		DocumentStatus: StatusFinal,
		ClosedDate:     closed,
	}, []byte("# Body\n"))
	require.NoError(t, err)
	require.NoError(t, fake.Write("doc.md", seed))

	require.NoError(t, Close(fake, "doc.md", StatusFinal))

	require.Equal(t, string(seed), string(fake.data["doc.md"]),
		"a second Close on an already-final artifact must reproduce the same bytes")

	// closed_date must specifically be preserved from the seed, not re-stamped
	// to today.
	meta, _, err := Split(fake.data["doc.md"])
	require.NoError(t, err)
	require.NotNil(t, meta)
	require.True(t, meta.ClosedDate.Equal(closed), "closed_date must remain the first-transition date")
}

// TestClose_PropagatesReadError arranges the fake store to return a sentinel
// error from Read and asserts Close returns it unchanged, so callers can filter
// via errors.Is against their store's not-found sentinel.
func TestClose_PropagatesReadError(t *testing.T) {
	sentinel := errors.New("fake read failure")
	fake := &fakeStore{data: map[string][]byte{}, readErr: sentinel}

	err := Close(fake, "any.md", StatusFinal)
	require.ErrorIs(t, err, sentinel, "Close must return the store's Read error unchanged")
	require.Empty(t, fake.data, "Close must not write anything when Read fails")
}
