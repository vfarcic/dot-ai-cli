package skills

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vfarcic/dot-ai-cli/internal/client"
	"github.com/vfarcic/dot-ai-cli/internal/config"
)

// PRD #13 M2 — local-directory ("--repo-dir") source ingestion.
//
// The CLI reads skill source from a local directory (no network, no git), uploads
// it to the server's ingestion endpoint (POST /api/v1/prompts/sources), then lists
// and renders it through ?source=<identifier>. The server renders the uploaded
// source exactly as it would a server-cloned repo — the only difference is how the
// source reached the server's cache.

const (
	// repoDirAllowEnv is the opt-in gate. --repo-dir accepts an arbitrary
	// filesystem path (a side-loading vector for arbitrary skill code), so it is
	// default-off: the user must explicitly set this to "1".
	repoDirAllowEnv = "DOT_AI_ALLOW_REPO_DIR"
	// repoDirAllowlistEnv is an optional colon-separated list of base directories
	// under which a --repo-dir path must live. When unset, no path restriction is
	// applied beyond the /tmp + world-writable refusals below.
	repoDirAllowlistEnv = "DOT_AI_REPO_DIR_ALLOW"

	// Ingestion limits mirrored from the frozen server contract. Pre-checking
	// them yields a clear CLI error instead of relying solely on the server 413.
	maxSourceFiles      = 100
	maxSourceTotalBytes = 256 * 1024 // 256 KiB total *decoded* bytes
)

// Sentinel errors returned from the readLocalSource walk the moment a contract
// limit would be exceeded. They let us bail BEFORE os.ReadFile pulls an
// oversized/over-count file into memory (the side-loading OOM guard the PRD
// names) rather than enforcing the limits only after the whole walk.
var (
	errSourceTooManyFiles = errors.New("source exceeds file limit")
	errSourceTooManyBytes = errors.New("source exceeds byte limit")
)

// sourceLabelPattern is the charset allowed in a --source-label. The label
// becomes a server-stored identifier and feeds the local:<user>-<label> prefix,
// so restrict it to identifier/URL-safe characters: no path separators,
// whitespace, or control characters.
var sourceLabelPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ValidSourceLabel reports whether label is a safe --source-label (non-empty and
// limited to [A-Za-z0-9._-]).
func ValidSourceLabel(label string) bool {
	return sourceLabelPattern.MatchString(label)
}

// sourceFile is one file in an uploaded source: a forward-slash relative path,
// base64-encoded content, and an octal permission string.
type sourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"` // base64-encoded
	Mode    string `json:"mode"`
}

// sourceUploadRequest is the POST /api/v1/prompts/sources body.
type sourceUploadRequest struct {
	Source      string       `json:"source"`
	ContentHash string       `json:"contentHash"`
	Files       []sourceFile `json:"files"`
}

// sourceUploadResponse is the success envelope returned by the ingestion endpoint.
type sourceUploadResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Source      string `json:"source"`
		ContentHash string `json:"contentHash"`
		FileCount   int    `json:"fileCount"`
		Status      string `json:"status"` // "ingested" | "unchanged"
	} `json:"data"`
}

// SourceIdentifier turns a --source-label into a globally-unique-per-server
// identifier by auto-prefixing the host identity, since the server stores
// local:<label> verbatim with no per-principal namespacing (two hosts uploading
// local:foo would otherwise overwrite each other). The scheme is, in order:
//
//	local:<user>-<label>   ($USER, else the OS user)
//	local:<host>-<label>   ($HOSTNAME, else os.Hostname() — when no user is known)
//
// If neither a user nor a host can be determined it returns an error rather than
// silently uploading a bare local:<label> that could collide. The same returned
// string is used for the upload source field, the source: frontmatter tag, and
// the ?source= render/list param.
func SourceIdentifier(label string) (string, error) {
	if u := sanitizeIdentityPart(os.Getenv("USER")); u != "" {
		return "local:" + u + "-" + label, nil
	}
	if cur, err := user.Current(); err == nil {
		if u := sanitizeIdentityPart(cur.Username); u != "" {
			return "local:" + u + "-" + label, nil
		}
	}
	if h := sanitizeIdentityPart(os.Getenv("HOSTNAME")); h != "" {
		return "local:" + h + "-" + label, nil
	}
	if h, err := os.Hostname(); err == nil {
		if h := sanitizeIdentityPart(h); h != "" {
			return "local:" + h + "-" + label, nil
		}
	}
	return "", &client.RequestError{
		Message: fmt.Sprintf("Error: cannot determine a host identity to namespace --source-label %q; "+
			"set $USER or $HOSTNAME so the source identifier (local:<user>-%s) is unique per server", label, label),
		ExitCode: client.ExitUsageError,
	}
}

// sanitizeIdentityPart keeps an identity fragment safe to embed in a source
// identifier: it drops any leading domain (Windows DOMAIN\user), trims spaces,
// and strips characters outside [A-Za-z0-9._-] so the identifier stays stable
// and URL-clean. Returns "" when nothing usable remains.
func sanitizeIdentityPart(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexAny(s, `\/`); i >= 0 {
		s = s[i+1:]
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// AuthorizeRepoDir enforces the --repo-dir security posture and returns the
// resolved (absolute, symlink-evaluated) directory to read. It is default-off
// and opt-in:
//   - requires DOT_AI_ALLOW_REPO_DIR=1;
//   - refuses a path under /tmp or $TMPDIR (shared, world-writable temp space is
//     a side-loading vector);
//   - refuses a world-writable directory;
//   - if DOT_AI_REPO_DIR_ALLOW is set, requires the path to live under one of
//     its colon-separated base directories.
//
// Every refusal returns a non-zero-exit *client.RequestError with an actionable
// message and never reads or uploads anything.
func AuthorizeRepoDir(dir string) (string, error) {
	if os.Getenv(repoDirAllowEnv) != "1" {
		return "", &client.RequestError{
			Message: fmt.Sprintf("Error: --repo-dir is opt-in: set %s=1 to allow reading skills from a local "+
				"directory. It accepts an arbitrary filesystem path (a side-loading vector for arbitrary skill "+
				"code), so it is disabled by default; prefer --repo with DOT_AI_GIT_TOKEN whenever a static "+
				"credential reaches the source.", repoDirAllowEnv),
			ExitCode: client.ExitUsageError,
		}
	}

	resolved, err := filepath.Abs(dir)
	if err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: cannot resolve --repo-dir %q: %v", dir, err),
			ExitCode: client.ExitUsageError,
		}
	}
	// Resolve symlinks before any security check, and fail CLOSED on error: if
	// the path cannot be resolved (dangling link, missing component, permission
	// error on a path element), refuse rather than continuing on the unresolved
	// filepath.Abs path. Proceeding would run every check below against a path
	// that differs from what we'd actually read — a fail-open in a trust
	// boundary. A normal directory and a valid symlink both resolve cleanly, so
	// only a genuine resolution error refuses here.
	real, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: cannot resolve --repo-dir path %q: %v", dir, err),
			ExitCode: client.ExitUsageError,
		}
	}
	resolved = real

	info, err := os.Stat(resolved)
	if err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-dir %q is not readable: %v", dir, err),
			ExitCode: client.ExitUsageError,
		}
	}
	if !info.IsDir() {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-dir %q is not a directory", dir),
			ExitCode: client.ExitUsageError,
		}
	}

	if root := tempRootContaining(resolved); root != "" {
		return "", &client.RequestError{
			Message: fmt.Sprintf("Error: --repo-dir %q is under the shared temp directory %q; refusing to read "+
				"skills from world-writable temp space. Point --repo-dir at a directory you control.", dir, root),
			ExitCode: client.ExitUsageError,
		}
	}

	// Optional operator allowlist, normalized once. It is reused below as the
	// upward stop boundary for the world-writable ancestor walk: above an
	// allowlist base is the operator-declared trust root, out of our scope.
	var allowBases []string
	allowRaw := strings.TrimSpace(os.Getenv(repoDirAllowlistEnv))
	if allowRaw != "" {
		allowBases = normalizeBases(strings.Split(allowRaw, ":"))
	}

	// Refuse a world-writable target OR any world-writable ancestor: a writable
	// parent lets another user swap the directory out from under us, defeating the
	// leaf-only check. The walk stops at the filesystem root, or at an allowlist
	// base when one is configured.
	if ww := worldWritableAncestor(resolved, allowBases); ww != "" {
		var perm os.FileMode
		if wwInfo, statErr := os.Stat(ww); statErr == nil {
			perm = wwInfo.Mode().Perm()
		}
		what := fmt.Sprintf("--repo-dir %q is world-writable (mode %o)", dir, perm)
		if ww != resolved {
			what = fmt.Sprintf("--repo-dir %q has a world-writable ancestor %q (mode %o)", dir, ww, perm)
		}
		return "", &client.RequestError{
			Message: fmt.Sprintf("Error: %s; refusing to read skills from a directory any user can modify. "+
				"Tighten its permissions (e.g. chmod o-w).", what),
			ExitCode: client.ExitUsageError,
		}
	}

	if allowRaw != "" {
		if !containedIn(resolved, allowBases) {
			return "", &client.RequestError{
				Message: fmt.Sprintf("Error: --repo-dir %q is not under any base directory in %s (%s)",
					dir, repoDirAllowlistEnv, allowRaw),
				ExitCode: client.ExitUsageError,
			}
		}
	}

	return resolved, nil
}

// worldWritableAncestor returns the first directory at or above path that is
// world-writable (the offending path), or "" if none is. The walk climbs to the
// filesystem root, or stops at a configured allowlist base (its parents are the
// operator-declared trust root). Non-stat-able levels are skipped, not treated as
// failures.
func worldWritableAncestor(path string, allowBases []string) string {
	stop := make(map[string]bool, len(allowBases))
	for _, b := range allowBases {
		stop[b] = true
	}
	for {
		if info, err := os.Stat(path); err == nil && info.IsDir() && info.Mode().Perm()&0o002 != 0 {
			return path
		}
		if stop[path] {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			break // reached the filesystem root
		}
		path = parent
	}
	return ""
}

// tempRootContaining returns the shared temp root (/tmp, os.TempDir(), or
// $TMPDIR) that contains path, or "" if none does. Each root is symlink-resolved
// so the check cannot be bypassed via a symlink (and so it works where the temp
// dir is itself a symlink, e.g. macOS /tmp -> /private/tmp).
func tempRootContaining(path string) string {
	seen := map[string]bool{}
	for _, root := range []string{os.TempDir(), os.Getenv("TMPDIR"), "/tmp"} {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		if real, err := filepath.EvalSymlinks(root); err == nil {
			root = real
		}
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		if path == root || strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return root
		}
	}
	return ""
}

// normalizeBases resolves each base directory (trimmed, made absolute,
// symlink-evaluated, cleaned), dropping empties. The result is comparable against
// an already-resolved path.
func normalizeBases(bases []string) []string {
	out := make([]string, 0, len(bases))
	for _, base := range bases {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		if abs, err := filepath.Abs(base); err == nil {
			base = abs
		}
		if real, err := filepath.EvalSymlinks(base); err == nil {
			base = real
		}
		out = append(out, filepath.Clean(base))
	}
	return out
}

// containedIn reports whether path is one of, or nested under, any of the given
// (already-normalized) base directories.
func containedIn(path string, bases []string) bool {
	for _, base := range bases {
		if path == base || strings.HasPrefix(path, base+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// readLocalSource walks dir and collects every regular file as a sourceFile with
// a forward-slash relative path, base64-encoded content, and octal mode. It also
// returns a stable, order-independent content hash ("sha256:<hex>") over the
// file set. The contract limits (file count, total decoded bytes) are enforced
// here so the user gets a clear error before any upload.
func readLocalSource(dir string) ([]sourceFile, string, error) {
	var files []sourceFile
	var hashes []hashSource
	var fileCount int
	var totalBytes int64

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Skip anything that is not a regular file (symlinks, devices, sockets):
		// we upload concrete bytes, not links into the host filesystem.
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		// Enforce the file-count limit DURING the walk, BEFORE reading bytes into
		// memory. A single hostile/huge file is bounded by the read below.
		fileCount++
		if fileCount > maxSourceFiles {
			return errSourceTooManyFiles
		}

		// Read at most the remaining byte budget (256 KiB total decoded minus what
		// we've already accumulated), +1 byte to detect overflow, straight from the
		// opened file instead of stat-then-os.ReadFile. This closes the stat-vs-read
		// TOCTOU gap and hard-bounds the read: even if the file grew after its size
		// was stat'd, we never pull more than the budget allows. A file larger than
		// the remaining budget bails with the byte-limit sentinel.
		remaining := maxSourceTotalBytes - totalBytes
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(f, remaining+1))
		f.Close()
		if err != nil {
			return err
		}
		if int64(len(data)) > remaining {
			return errSourceTooManyBytes
		}
		totalBytes += int64(len(data))

		mode := fmt.Sprintf("%04o", info.Mode().Perm())
		relSlash := filepath.ToSlash(rel)
		files = append(files, sourceFile{
			Path:    relSlash,
			Content: base64.StdEncoding.EncodeToString(data),
			Mode:    mode,
		})
		hashes = append(hashes, hashSource{path: relSlash, mode: mode, raw: data})
		return nil
	})
	if walkErr != nil {
		switch {
		case errors.Is(walkErr, errSourceTooManyFiles):
			return nil, "", &client.RequestError{
				Message:  fmt.Sprintf("Error: --repo-dir source exceeds the %d-file limit", maxSourceFiles),
				ExitCode: client.ExitUsageError,
			}
		case errors.Is(walkErr, errSourceTooManyBytes):
			return nil, "", &client.RequestError{
				Message:  fmt.Sprintf("Error: --repo-dir source exceeds the %d-byte (256 KiB) limit", maxSourceTotalBytes),
				ExitCode: client.ExitUsageError,
			}
		default:
			return nil, "", &client.RequestError{
				Message:  fmt.Sprintf("Error: failed to read --repo-dir %q: %v", dir, walkErr),
				ExitCode: client.ExitToolError,
			}
		}
	}

	return files, contentHash(hashes), nil
}

// hashSource is the raw, pre-encoding view of one file used only for content
// hashing: forward-slash path, octal mode, and raw (un-encoded) bytes.
type hashSource struct {
	path string
	mode string
	raw  []byte
}

// contentHash computes a stable, order-independent sha256 over the file set by
// sorting on path and hashing each path, mode, and RAW content with explicit
// length framing (so no field boundary can be forged). Hashing the raw bytes
// directly avoids a base64 decode round-trip, and folding in the mode means a
// permission-only change busts the dedup hash. The returned value is
// "sha256:<hex>". It gates server-side re-upload dedup; the CLI-side
// skip-if-unchanged optimization is a later milestone (M4) — M2 always uploads.
func contentHash(items []hashSource) string {
	sorted := make([]hashSource, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].path < sorted[j].path })

	h := sha256.New()
	for _, it := range sorted {
		fmt.Fprintf(h, "%d:%s\n%d:%s\n%d:", len(it.path), it.path, len(it.mode), it.mode, len(it.raw))
		h.Write(it.raw)
		h.Write([]byte("\n"))
	}
	return "sha256:" + fmt.Sprintf("%x", h.Sum(nil))
}

// uploadSource POSTs an already-read file set + contentHash to the ingestion
// endpoint under identifier and returns the server-reported status ("ingested"
// or "unchanged"). Splitting it out lets NewLocalSourceUploader read+hash the
// tree once and reuse that single read for both the gate decision and the
// upload (so an unchanged source is hashed, not re-walked-then-re-walked).
func uploadSource(cfg *config.Config, identifier string, files []sourceFile, hash string) (string, error) {
	payload, err := json.Marshal(sourceUploadRequest{
		Source:      identifier,
		ContentHash: hash,
		Files:       files,
	})
	if err != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to build source upload for %q: %v", identifier, err),
			ExitCode: client.ExitToolError,
		}
	}

	body, err := client.DoJSON(cfg, "POST", "/api/v1/prompts/sources", payload, nil)
	if err != nil {
		return "", reframeUploadError(err, identifier)
	}

	var resp sourceUploadResponse
	if jerr := json.Unmarshal(body, &resp); jerr != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to parse source upload response for %q: %v", identifier, jerr),
			ExitCode: client.ExitToolError,
		}
	}
	return resp.Data.Status, nil
}

// NewLocalSourceUploader returns an ensureUploaded(force) closure that gates the
// upload of the local source at dir (under identifier) on its content hash. The
// tree is read + hashed at most ONCE (memoized across calls), so an unchanged
// source is never walked twice and the gated call and an evict-retry forced call
// share the same read.
//
//   - force == false (the normal pre-list call): upload only if the source's
//     contentHash differs from the last-uploaded hash recorded in the
//     upload-state store; otherwise SKIP and report "unchanged". A missing
//     record (first-ever run, pruned state) always uploads — the gate never
//     skips an upload the server might be missing.
//   - force == true (the evict-retry): always upload, regardless of the stored
//     hash, because the server has signalled it no longer holds the source.
//
// Each real upload records the new hash; a skip touches the record so an
// actively-used source stays fresh for `cache prune`. Human-facing status lines
// are written to out (the caller's stdout), never to a log. dir must already be
// authorized (--repo-dir) or a throwaway clone copy (--repo-fetch).
func NewLocalSourceUploader(cfg *config.Config, dir, identifier string, out io.Writer) func(force bool) error {
	var (
		files   []sourceFile
		hash    string
		readErr error
		didRead bool
	)
	readOnce := func() error {
		if !didRead {
			files, hash, readErr = readLocalSource(dir)
			didRead = true
		}
		return readErr
	}

	return func(force bool) error {
		if err := readOnce(); err != nil {
			return err
		}
		if !force {
			if stored := readUploadedHash(identifier); stored != "" && stored == hash {
				touchUploadState(identifier)
				fmt.Fprintf(out, "Source %s unchanged, skipping upload\n", identifier)
				return nil
			}
		} else {
			fmt.Fprintf(out, "Server no longer has source %s; re-uploading\n", identifier)
		}

		status, err := uploadSource(cfg, identifier, files, hash)
		if err != nil {
			return err
		}
		// Best-effort: a failed state write only costs a redundant upload next
		// run, so it must not fail an otherwise-successful generate.
		_ = writeUploadedHash(identifier, hash)
		if status != "" {
			fmt.Fprintf(out, "Uploaded source as %s (%s)\n", identifier, status)
		} else {
			fmt.Fprintf(out, "Uploaded source as %s\n", identifier)
		}
		return nil
	}
}

// reframeUploadError turns a request-scoped 4xx from the ingestion endpoint into
// an actionable, per-source CLI error (e.g. the 413/400 limits the contract
// enforces). Non-4xx and API-auth (401) errors pass through unchanged.
func reframeUploadError(err error, identifier string) error {
	re, ok := err.(*client.RequestError)
	if !ok || re.Status < 400 || re.Status >= 500 || re.Status == 401 {
		return err
	}
	msg := re.ServerMessage
	if msg == "" {
		msg = re.Message
	}
	return &client.RequestError{
		Message:  fmt.Sprintf("Error: failed to upload local source %s: %s", identifier, client.RedactCredentials(msg)),
		ExitCode: re.ExitCode,
		Status:   re.Status,
	}
}
