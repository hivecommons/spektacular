//go:build unix

package autocommit

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryLock takes an exclusive flock on f without waiting; ok is false when
// another open file holds it. flock locks belong to the open file, so two
// opens in one process exclude each other just as two processes do.
func tryLock(f *os.File) (ok bool, err error) {
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}

// releaseLock removes the lock file while still holding its lock, then
// drops the lock by closing f. Removing it first means nobody can lock this
// file and take it for the lock file: a waiter that locks it after the close
// finds path gone or a different file, and tries again.
func releaseLock(f *os.File, path string) {
	_ = os.Remove(path)
	_ = f.Close()
}
