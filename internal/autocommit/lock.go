package autocommit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LockFile is the project-level commit lock, relative to the project's
// .spektacular directory. Orchestrated plans run side by side in one working
// copy, and each makes its own path-scoped commit when it finishes; the lock
// serialises those commits so two never contend for git's index.
const LockFile = "workflows/.commit.lock"

// Lock timing. A commit is small, so a waiter gives up after LockTimeout
// rather than hang; a lock older than LockStale was left by a process that
// died holding it, and is broken.
var (
	LockTimeout = 30 * time.Second
	LockStale   = 10 * time.Minute
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
// removes the lock. A lock older than LockStale is broken and taken over.
func AcquireLock(dataDir string) (release func(), err error) {
	path := filepath.Join(dataDir, LockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(LockTimeout)
	delay := lockBackoff
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid %d at %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
			_ = f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > LockStale {
			_ = os.Remove(path)
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

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
