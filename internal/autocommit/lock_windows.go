//go:build windows

package autocommit

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// lockOffset is where the locked byte sits. Windows locks are mandatory, so
// locking the holder record itself would stop a waiter reading who holds the
// lock; a single byte far past the record locks nothing anyone reads.
const lockOffset = 1 << 30

// tryLock takes an exclusive LockFileEx lock on f without waiting; ok is
// false when another handle holds it.
func tryLock(f *os.File) (ok bool, err error) {
	ol := &windows.Overlapped{Offset: lockOffset}
	err = windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

// releaseLock clears the holder record, drops the lock and closes f, then
// removes the lock file. Go opens files without FILE_SHARE_DELETE, so the
// removal fails while anyone else has the file open, as a waiter does from
// before it locks until it lets go; it can never take the file from under a
// new holder. When it fails, the file stays, unlocked and empty, for the next
// holder.
func releaseLock(f *os.File, path string) {
	_ = f.Truncate(0)
	ol := &windows.Overlapped{Offset: lockOffset}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
	_ = f.Close()
	_ = os.Remove(path)
}
