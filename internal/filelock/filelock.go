// Package filelock is a small, portable cross-process lock taken by creating a
// lock file exclusively. Each CLI call is its own process, and several can run
// side by side (an epic's orchestrated runs share one working copy), so a lock
// that only lives in memory would serialise nothing. O_CREATE|O_EXCL is atomic
// on every platform Spektacular ships for, where an advisory flock is not
// portable to Windows.
//
// A lock file records which process took it and when. A waiter backs off
// exponentially, gives up after a timeout with a LockError naming the holder,
// and breaks a lock older than the stale age: such a lock was left by a process
// that died holding it.
package filelock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Backoff is the first wait between attempts to take a held lock. The wait
// doubles after each attempt, up to a second. Tests shorten it.
var Backoff = 50 * time.Millisecond

// maxBackoff caps the doubling wait between attempts.
const maxBackoff = time.Second

// LockError reports a lock that could not be taken in time, naming the lock
// file and what its holder recorded, so the user can see who holds it.
type LockError struct {
	Path   string
	Holder string
}

func (e *LockError) Error() string {
	return fmt.Sprintf("timed out waiting for the lock %s (held by %s)", e.Path, e.Holder)
}

// Acquire takes the lock at path, waiting up to timeout, starting from the
// package's Backoff. It returns a release function that removes the lock. A
// lock file older than stale is broken and taken over. The lock's parent
// directory is created when missing.
func Acquire(path string, timeout, stale time.Duration) (release func(), err error) {
	return AcquireWithBackoff(path, timeout, stale, Backoff)
}

// AcquireWithBackoff is Acquire with an explicit first wait between attempts,
// for a caller that keeps its own timing knobs.
func AcquireWithBackoff(path string, timeout, stale, backoff time.Duration) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	delay := backoff
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
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > stale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			holder, _ := os.ReadFile(path)
			return nil, &LockError{Path: path, Holder: string(trimNewline(holder))}
		}
		time.Sleep(delay)
		if delay < maxBackoff {
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
