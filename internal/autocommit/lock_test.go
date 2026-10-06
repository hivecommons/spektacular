package autocommit

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The lock's timing lives in package variables, so these tests shorten them
// with a restoring cleanup and must never run in parallel with one another.

// setLockTimeout sets LockTimeout for the test, restoring it afterwards.
func setLockTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := LockTimeout
	LockTimeout = d
	t.Cleanup(func() { LockTimeout = old })
}

// lockPath is where AcquireLock puts the lock under dataDir, hand-copied.
func lockPath(dataDir string) string {
	return filepath.Join(dataDir, "workflows", ".commit.lock")
}

// Goroutines contending for the lock all get it, one at a time: no two are
// ever inside the critical section together.
func TestAcquireLock_SerialisesConcurrentHolders(t *testing.T) {
	// A short backoff keeps eight waiters from sleeping their way up to the
	// one-second cap.
	oldBackoff := lockBackoff
	lockBackoff = time.Millisecond
	t.Cleanup(func() { lockBackoff = oldBackoff })
	dataDir := t.TempDir()

	const holders = 8
	var inside, maxInside, entered int32
	errs := make([]error, holders)
	var wg sync.WaitGroup
	for i := range holders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := AcquireLock(dataDir)
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
	require.NoFileExists(t, lockPath(dataDir), "the last release removes the lock")
}

// A waiter that cannot get the lock within LockTimeout gives up with a
// LockError naming the lock file and what its holder recorded, and leaves the
// holder's lock in place.
func TestAcquireLock_TimeoutReportsTheHolder(t *testing.T) {
	setLockTimeout(t, 200*time.Millisecond)
	dataDir := t.TempDir()
	path := lockPath(dataDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("pid 4242 at 2026-10-04T10:00:00Z\n"), 0o644))

	release, err := AcquireLock(dataDir)
	require.Nil(t, release)
	var lockErr *LockError
	require.ErrorAs(t, err, &lockErr)
	require.Equal(t, path, lockErr.Path)
	require.Equal(t, "pid 4242 at 2026-10-04T10:00:00Z", lockErr.Holder)
	require.Equal(t,
		"timed out waiting for the commit lock "+path+" (held by pid 4242 at 2026-10-04T10:00:00Z)",
		err.Error())

	require.Equal(t, "pid 4242 at 2026-10-04T10:00:00Z\n", string(readLock(t, path)))
}

// A lock older than LockStale was left by a process that died holding it: it
// is broken and taken over at once, not waited out.
func TestAcquireLock_BreaksAStaleLock(t *testing.T) {
	setLockTimeout(t, 200*time.Millisecond)
	dataDir := t.TempDir()
	path := lockPath(dataDir)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("pid 4242 at 2026-10-04T10:00:00Z\n"), 0o644))
	old := time.Now().Add(-LockStale - time.Minute)
	require.NoError(t, os.Chtimes(path, old, old))

	release, err := AcquireLock(dataDir)
	require.NoError(t, err)
	require.NotContains(t, string(readLock(t, path)), "pid 4242", "the stale holder's lock is replaced")

	release()
	require.NoFileExists(t, path)
}

// A lock released by its holder can be taken again straight away.
func TestAcquireLock_ReleasedLockIsFreeAgain(t *testing.T) {
	setLockTimeout(t, 200*time.Millisecond)
	dataDir := t.TempDir()

	release, err := AcquireLock(dataDir)
	require.NoError(t, err)
	require.FileExists(t, lockPath(dataDir))
	release()

	release, err = AcquireLock(dataDir)
	require.NoError(t, err)
	release()
}

func readLock(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}
