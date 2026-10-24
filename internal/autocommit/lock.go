package autocommit

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile is the project-level commit lock, relative to the project's
// .spektacular directory. Orchestrated plans run side by side in one working
// copy, and each makes its own path-scoped commit when it finishes; the lock
// serialises those commits so two never contend for git's index.
//
// The lock is an operating-system lock on the file (flock on Unix,
// LockFileEx on Windows), not the file's existence. The operating system
// drops it when its holder exits, however it exits, so a holder that is
// killed never leaves the lock behind, and a holder that is slow is never
// mistaken for a dead one. While the lock is held the file records who holds
// it; a released lock's file is removed.
const LockFile = "workflows/.commit.lock"

// Lock timing. A commit is small, so a waiter gives up after LockTimeout
// rather than hang.
var (
	LockTimeout = 30 * time.Second
	lockBackoff = 50 * time.Millisecond
)

// LockError reports a commit lock that could not be taken in time, naming
// the lock file and what its holder recorded, so the user can see who holds
// it.
type LockError struct {
	Path   string
	Holder string
}

func (e *LockError) Error() string {
	return fmt.Sprintf("timed out waiting for the commit lock %s (held by %s)", e.Path, e.Holder)
}

// AcquireLock takes the commit lock under dataDir (the project's .spektacular
// directory), waiting up to LockTimeout. It returns a release function that
// drops the lock and removes the lock file.
func AcquireLock(dataDir string) (release func(), err error) {
	path := filepath.Join(dataDir, LockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(LockTimeout)
	delay := lockBackoff
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
		if err != nil {
			return nil, err
		}
		ok, err := tryLock(f)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if ok && isLockFile(f, path) {
			// A holder that died left its record behind; replace it with ours.
			_ = f.Truncate(0)
			_, _ = f.WriteAt(fmt.Appendf(nil, "pid %d at %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339)), 0)
			return func() { releaseLock(f, path) }, nil
		}
		_ = f.Close()
		if ok {
			// The holder removed the file between our open and our lock, so
			// what we locked is no longer the lock file: try again at once.
			continue
		}
		if time.Now().After(deadline) {
			holder, _ := os.ReadFile(path)
			return nil, &LockError{Path: path, Holder: string(trimNewline(holder))}
		}
		time.Sleep(delay)
		if delay < time.Second {
			delay *= 2
		}
	}
}

// isLockFile reports whether f is still the file at path.
func isLockFile(f *os.File, path string) bool {
	held, err := f.Stat()
	if err != nil {
		return false
	}
	current, err := os.Stat(path)
	return err == nil && os.SameFile(held, current)
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
