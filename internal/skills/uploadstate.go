package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vfarcic/dot-ai-cli/internal/client"
)

// PRD #13 M4b — content-hash upload gating state.
//
// To skip re-uploading an unchanged --repo-dir/--repo-fetch source, the CLI
// records the last-uploaded contentHash per source identifier in a small
// upload-state store under the SAME XDG cache root the clone cache uses
// (<cacheRoot>/dot-ai-cli/uploads/<sha256(identifier)>). The file holds only the
// sha256 contentHash — never the source bytes, the identifier, or any credential
// (the identifier is hashed into the path, and the only identifiers that reach
// here — local:<user>-<label> and a RedactURL-scrubbed URL — carry no secret).

// cacheBaseDir resolves the CLI's per-user cache base, <root>/dot-ai-cli, where
// root = $XDG_CACHE_HOME when set (checked first so it wins on every platform,
// matching repoCacheDir), else os.UserCacheDir() (Linux → ~/.cache). The clone
// cache (repos/) and the upload-state store (uploads/) both live under it, so
// they share one root and one prune sweep.
func cacheBaseDir() (string, error) {
	root := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME"))
	if root == "" {
		var err error
		root, err = os.UserCacheDir()
		if err != nil {
			return "", &client.RequestError{
				Message:  "Error: failed to resolve the dot-ai-cli cache directory: " + err.Error(),
				ExitCode: client.ExitToolError,
			}
		}
	}
	return filepath.Join(root, "dot-ai-cli"), nil
}

// uploadsDir returns the upload-state directory, <cacheBase>/uploads.
func uploadsDir() (string, error) {
	base, err := cacheBaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "uploads"), nil
}

// uploadStatePath returns the per-source upload-state file path,
// <cacheBase>/uploads/<sha256(identifier)>. Keying by the hash of the identifier
// keeps the filename fixed-length, filesystem-safe, and free of any identifier
// text (which, for a credential-scrubbed URL, is already secret-free anyway).
func uploadStatePath(identifier string) (string, error) {
	dir, err := uploadsDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(identifier))
	return filepath.Join(dir, hex.EncodeToString(sum[:])), nil
}

// readUploadedHash returns the last contentHash uploaded for identifier, or ""
// when none is recorded (first-ever run, pruned state, or any read error). A ""
// result always forces an upload — the gate must never skip on a missing/unread
// record, which is the M4b backward-compat guarantee (first run always uploads).
func readUploadedHash(identifier string) string {
	path, err := uploadStatePath(identifier)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// writeUploadedHash records hash as the last-uploaded contentHash for
// identifier. The uploads dir is created 0700 (it may sit beside private cached
// source) and the state file 0600. Errors are returned but the caller treats
// them as non-fatal: a failed write only costs a redundant upload next run.
func writeUploadedHash(identifier, hash string) error {
	dir, err := uploadsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := uploadStatePath(identifier)
	if err != nil {
		return err
	}
	// Write to a temp file in the SAME dir, then os.Rename over the target so a
	// crash mid-write can never leave a torn/partial hash file: the record is
	// all-or-nothing. A same-dir rename is atomic on POSIX, and os.CreateTemp
	// already creates the temp file 0600 — the same posture as the final target.
	tmp, err := os.CreateTemp(dir, ".hash-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, werr := tmp.WriteString(hash); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return werr
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return cerr
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return rerr
	}
	return nil
}

// touchUploadState bumps the upload-state file's mtime to now so a source that
// is actively used but UNCHANGED (and therefore skips its upload) still counts
// as recently used for `skills cache prune --older-than`. Best-effort: a missing
// file or chtimes error is ignored.
func touchUploadState(identifier string) {
	path, err := uploadStatePath(identifier)
	if err != nil {
		return
	}
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}
