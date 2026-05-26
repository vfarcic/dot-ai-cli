package skills

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// lockFileName is the name of the per-output-dir lock file. Multiple
// `dot-ai skills generate` invocations targeting the same directory race on
// this lock so they can't corrupt each other (PRD #12: hooks may fire in
// parallel).
const lockFileName = ".dot-ai.lock"

// lockTimeout is the maximum time acquireLock waits before failing.
const lockTimeout = 5 * time.Second

// fileLock wraps an acquired flock for deferred release.
type fileLock struct {
	fl *flock.Flock
}

// acquireLock obtains an exclusive flock on <dir>/.dot-ai.lock. It polls
// until either the lock is acquired or the timeout elapses. The returned
// error is user-facing and intentionally never leaks the lock-file path —
// neither on contention nor on lower-level filesystem errors.
func acquireLock(dir string) (*fileLock, error) {
	fl := flock.New(filepath.Join(dir, lockFileName))
	deadline := time.Now().Add(lockTimeout)
	for {
		ok, err := fl.TryLock()
		if err != nil {
			return nil, fmt.Errorf("could not acquire output-directory lock")
		}
		if ok {
			return &fileLock{fl: fl}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another `dot-ai skills generate` is in progress")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Release frees the lock. Safe to call on a nil receiver.
func (l *fileLock) Release() {
	if l == nil || l.fl == nil {
		return
	}
	_ = l.fl.Unlock()
}
