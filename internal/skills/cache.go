package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/vfarcic/dot-ai-cli/internal/client"
)

// PRD #13 M4b — `skills cache prune --older-than` GC of the clone cache.
//
// The persistent --repo-fetch clone cache (<cacheBase>/repos/<hash>/) and the
// upload-state store (<cacheBase>/uploads/<hash>) accumulate over time. prune
// reaps entries whose LAST USE is older than a duration. Last use is the dir
// mtime, which CloneRepoFetchCached bumps on every successful sync (touchCache),
// so an actively-fetched cache is never reaped — only genuinely idle ones.

// PruneResult summarises one prune sweep. All fields are counts (plus the basis
// dir existence) so the caller can print a tidy, credential-free summary — the
// only identifiers involved are sha256 basenames, which carry no secret.
type PruneResult struct {
	// ReposMissing is true when no clone-cache directory exists at all (a
	// never-used cache) — the "nothing to prune" fast path.
	ReposMissing bool
	// ReposScanned counts the clone-cache entries examined.
	ReposScanned int
	// ReposPruned counts the aged clone-cache entries removed.
	ReposPruned int
	// ReposKept counts the entries left because they are still in use (last-use
	// newer than the cutoff).
	ReposKept int
	// ReposLocked counts the aged entries SKIPPED because a concurrent
	// --repo-fetch holds their per-URL flock (or they could not be removed).
	ReposLocked int
	// UploadsPruned counts the aged upload-state records removed.
	UploadsPruned int
}

// Removed reports whether the sweep deleted anything.
func (r PruneResult) Removed() bool { return r.ReposPruned > 0 || r.UploadsPruned > 0 }

// PruneRepoCache removes clone-cache entries (and aged upload-state records)
// whose last-use time is older than maxAge. It honours the per-URL flock: an
// aged entry a concurrent --repo-fetch is using (lock held) is skipped, never
// deleted out from under the live run. A missing cache dir is not an error —
// the result reports ReposMissing so the caller can say "nothing to prune".
func PruneRepoCache(maxAge time.Duration) (PruneResult, error) {
	base, err := cacheBaseDir()
	if err != nil {
		return PruneResult{}, err
	}
	cutoff := time.Now().Add(-maxAge)
	var res PruneResult

	reposDir := filepath.Join(base, "repos")
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return res, &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to read the --repo-fetch cache directory: %v", err),
				ExitCode: client.ExitToolError,
			}
		}
		res.ReposMissing = true
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cacheDir := filepath.Join(reposDir, e.Name())
		info, statErr := os.Stat(cacheDir)
		if statErr != nil {
			continue
		}
		res.ReposScanned++
		// Keep anything used at or after the cutoff — an active cache.
		if !info.ModTime().Before(cutoff) {
			res.ReposKept++
			continue
		}
		// Aged. Take the SAME per-URL flock a --repo-fetch run takes, with a
		// single non-blocking try: if a concurrent run holds it, skip this entry
		// rather than deleting a cache in active use.
		lock := flock.New(cacheDir + repoCacheLockSuffix)
		ok, lerr := lock.TryLock()
		if lerr != nil || !ok {
			res.ReposLocked++
			continue
		}
		// Remove the cache dir while holding the lock (mirrors freshClone's
		// remove-under-lock), then release. The now-orphan .lock file is left in
		// place ON PURPOSE: removing it would swap the inode out from under any
		// concurrent --repo-fetch about to flock the same path, breaking mutual
		// exclusion in that window (see lock.go's repoCacheLockSuffix comment). The
		// orphan is tiny and sha256-named, so leaving it costs nothing.
		rmErr := os.RemoveAll(cacheDir)
		_ = lock.Unlock()
		if rmErr != nil {
			res.ReposLocked++
			continue
		}
		res.ReposPruned++
	}

	res.UploadsPruned = pruneUploadState(base, cutoff)
	return res, nil
}

// pruneUploadState removes upload-state records last written before cutoff. A
// missing uploads dir yields zero. Best-effort: an unremovable entry is skipped.
func pruneUploadState(base string, cutoff time.Time) int {
	dir := filepath.Join(base, "uploads")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	pruned := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, infoErr := e.Info()
		if infoErr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			pruned++
		}
	}
	return pruned
}
