package filelock

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Backoff is a package variable, so these tests shorten it with a restoring
// cleanup and must never run in parallel with one another.

func shortBackoff(t *testing.T) {
	t.Helper()
	old := Backoff
	Backoff = time.Millisecond
	t.Cleanup(func() { Backoff = old })
}

// Goroutines contending for the lock all get it, one at a time: no two are
// ever inside the critical section together.
func TestAcquire_SerialisesConcurrentHolders(t *testing.T) {
	shortBackoff(t)
	path := filepath.Join(t.TempDir(), "nested", "thing.lock")

	const holders = 8
	var inside, maxInside, entered int32
	errs := make([]error, holders)
	var wg sync.WaitGroup
	for i := range holders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := Acquire(path, 10*time.Second, time.Minute)
			if err != nil {
				errs[i] = err
				return
			}
			n := atomic.AddInt32(&inside, 1)
			for {
				m := atomic.LoadInt32(&maxInside)
				if n <= m || atomic.CompareAndSwapInt32(&maxInside, m, n) {
					break
				}
			}
			atomic.AddInt32(&entered, 1)
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&inside, -1)
			release()
		}()
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(holders), entered)
	require.Equal(t, int32(1), maxInside, "two holders were inside the lock at once")
	require.NoFileExists(t, path, "the last release removes the lock")
}

// A waiter that cannot get the lock within its timeout gives up with a
// LockError naming the lock file and what its holder recorded, and leaves the
// holder's lock in place.
func TestAcquire_TimeoutReportsTheHolder(t *testing.T) {
	shortBackoff(t)
	path := filepath.Join(t.TempDir(), "held.lock")
	require.NoError(t, os.WriteFile(path, []byte("pid 4242 at 2026-10-04T10:00:00Z\n"), 0o644))

	release, err := Acquire(path, 100*time.Millisecond, time.Hour)
	require.Nil(t, release)
	var lockErr *LockError
	require.ErrorAs(t, err, &lockErr)
	require.Equal(t, path, lockErr.Path)
	require.Equal(t, "pid 4242 at 2026-10-04T10:00:00Z", lockErr.Holder)
	require.Equal(t,
		"timed out waiting for the lock "+path+" (held by pid 4242 at 2026-10-04T10:00:00Z)",
		err.Error())

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "pid 4242 at 2026-10-04T10:00:00Z\n", string(b))
}

// A lock older than the stale age was left by a process that died holding it:
// it is broken and taken over at once, not waited out.
func TestAcquire_BreaksAStaleLock(t *testing.T) {
	shortBackoff(t)
	path := filepath.Join(t.TempDir(), "stale.lock")
	require.NoError(t, os.WriteFile(path, []byte("pid 4242 at 2026-10-04T10:00:00Z\n"), 0o644))
	old := time.Now().Add(-2 * time.Minute)
	require.NoError(t, os.Chtimes(path, old, old))

	release, err := Acquire(path, 100*time.Millisecond, time.Minute)
	require.NoError(t, err)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(b), "pid 4242", "the stale holder's lock is replaced")
	require.Contains(t, string(b), "pid ", "the new holder records itself")

	release()
	require.NoFileExists(t, path)
}

// A lock released by its holder can be taken again straight away.
func TestAcquire_ReleasedLockIsFreeAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "again.lock")

	release, err := Acquire(path, 100*time.Millisecond, time.Minute)
	require.NoError(t, err)
	require.FileExists(t, path)
	release()

	release, err = AcquireWithBackoff(path, 100*time.Millisecond, time.Minute, time.Millisecond)
	require.NoError(t, err)
	release()
}
