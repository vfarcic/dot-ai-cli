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

// repoCacheLockSuffix names the per-URL --repo-fetch clone-cache lock. The lock
// lives BESIDE the cache dir ("<cacheDir>.lock"), never inside it, so the
// corrupt-cache recovery (os.RemoveAll(cacheDir) + re-clone) cannot delete the
// lock file out from under a held flock and silently break mutual exclusion.
// The name derives from the cache dir's sha256 basename, so it never contains
// credentials.
const repoCacheLockSuffix = ".lock"

// repoCacheLockTimeout is how long a second concurrent --repo-fetch of the SAME
// url waits for the first to finish populating the per-URL clone cache. It is
// far more generous than lockTimeout because a first clone can take minutes
// (each git call is bounded only by repoFetchCloneTimeout), so a legitimately
// slow first clone must not make the waiter spuriously fail.
const repoCacheLockTimeout = 5 * time.Minute

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

// acquireRepoCacheLock obtains the exclusive per-URL flock guarding the
// --repo-fetch clone cache for one repo. It polls until acquired or the
// (generous) repoCacheLockTimeout elapses. Like acquireLock, the returned error
// is user-facing and intentionally never leaks the lock-file path — neither on
// contention nor on a lower-level filesystem error.
func acquireRepoCacheLock(cacheDir string) (*fileLock, error) {
	fl := flock.New(cacheDir + repoCacheLockSuffix)
	deadline := time.Now().Add(repoCacheLockTimeout)
	for {
		ok, err := fl.TryLock()
		if err != nil {
			return nil, fmt.Errorf("could not acquire the --repo-fetch clone-cache lock")
		}
		if ok {
			return &fileLock{fl: fl}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another `dot-ai skills generate --repo-fetch` of this repo is in progress")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Release frees the lock. Safe to call on a nil receiver.
func (l *fileLock) Release() {
	if l == nil || l.fl == nil {
		return
	}
	_ = l.fl.Unlock()
}
