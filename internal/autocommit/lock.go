package autocommit

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hivecommons/spektacular/internal/filelock"
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
// removes the lock. A lock older than LockStale is broken and taken over. The
// lock file logic itself is the shared filelock package's; this keeps the
// commit lock's own timing and error.
func AcquireLock(dataDir string) (release func(), err error) {
	path := filepath.Join(dataDir, LockFile)
	release, err = filelock.AcquireWithBackoff(path, LockTimeout, LockStale, lockBackoff)
	var held *filelock.LockError
	if errors.As(err, &held) {
		return nil, &LockError{Path: held.Path, Holder: held.Holder}
	}
	return release, err
}
