package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vfarcic/dot-ai-cli/internal/client"
)

// repoFetchCloneTimeout bounds each host git invocation (clone OR fetch) so a
// wedged transport (an askpass GUI that never returns, a half-open TCP
// connection, a hung credential helper) can never hang the CLI forever. Two
// minutes is generous for a --depth 1 clone over a working transport yet short
// enough to fail visibly.
const repoFetchCloneTimeout = 2 * time.Minute

// repoFetchKillGrace bounds how long cmd.Run() may keep blocking AFTER the
// context deadline fires and the cancel hook runs. Even if a grandchild git
// spawned ignores the process-group kill, the inherited stderr pipe is
// force-closed after this grace so Run() returns instead of hanging on the pipe
// past the deadline (PRD #13 M4 carry-over from M3).
const repoFetchKillGrace = 5 * time.Second

// hardenedGitEnv returns the host environment locked down for one git
// invocation: every interactive credential prompt is disabled so an un-authable
// operation fails fast instead of blocking on a terminal/GUI prompt, and the
// allowed fetch transports are pinned so a permissive host gitconfig (e.g.
// protocol.ext.allow=always) cannot turn a hostile URL into command execution.
// file is retained for the file:// e2e clones. Shared by clone AND fetch so the
// cache path can never drift from the M3 posture.
func hardenedGitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GIT_ALLOW_PROTOCOL=file:git:http:https:ssh",
	)
}

// runGit is the single hardened git runner shared by the --repo-fetch clone and
// the cache fetch/checkout. It runs `git <args...>` under a fresh
// repoFetchCloneTimeout-bounded context with the locked-down environment, in
// its own process group, with a WaitDelay backstop and a group-kill cancel hook
// (see configureGitProcessGroup). It captures stderr (surfaced only on failure,
// after credential scrubbing by the caller) and reports whether the bounded
// context deadline fired so the caller can render a clean timeout message
// instead of git's opaque "signal: killed".
func runGit(args ...string) (stderr string, timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), repoFetchCloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = hardenedGitEnv()
	configureGitProcessGroup(cmd)
	cmd.WaitDelay = repoFetchKillGrace
	var buf strings.Builder
	cmd.Stderr = &buf // captured; surfaced only on failure, after scrubbing
	runErr := cmd.Run()
	return buf.String(), ctx.Err() == context.DeadlineExceeded, runErr
}

// PRD #13 M3 — network source ("--repo-fetch") clone via the host git stack.
//
// The CLI clones the requested repo using the host's local git stack (SSH agent,
// git credential helper, ~/.gitconfig, GIT_SSH_COMMAND, GIT_CONFIG_GLOBAL, …) and
// then feeds the clone into the SAME upload/list/render chain that --repo-dir
// uses (NewLocalSourceUploader → ?source=<identifier>). This serves sources the
// SERVER cannot reach but the CLI host can (SSO / device-attested VPNs, etc.).
//
// The temp-clone path below ("clone to a temp dir, use it, delete it") is the M3
// behavior; under M4 it is reached via --no-cache. The DEFAULT --repo-fetch path
// is now CloneRepoFetchCached, a persistent, incremental, concurrency-safe clone
// cache that shares the same hardened runGit runner, .git strip, and
// repoFetchSubdir containment as this one. The source identifier (used for the
// upload source field, the source: frontmatter tag, and the ?source= param) is
// the credential-scrubbed URL (RedactURL), computed by the caller; neither path
// ever lets a raw, possibly-credentialed URL or raw git stderr reach output.

// CloneRepoFetch shallow-clones rawURL into a fresh temp directory using the
// host's git binary and returns the directory to hand to NewLocalSourceUploader plus
// a cleanup func the caller MUST defer. This is the --no-cache path: no
// persistent cache, no incremental fetch, no per-URL flock — clone, use, delete.
// When subPath is set, the returned dir is <cloneDir>/<subPath> (validated to be
// a clean relative path inside the clone); when branch is set, the clone is
// restricted to that single branch.
//
// The URL and every flag are passed to git as separate argv elements via
// exec.Command — never through a shell and never string-interpolated — so a
// hostile URL cannot inject a command or a git flag. The git invocation runs
// through the shared runGit, which locks down the environment (GIT_TERMINAL_PROMPT=0,
// empty GIT_ASKPASS/SSH_ASKPASS, GIT_ALLOW_PROTOCOL pinned) and bounds the run
// with a context timeout + process-group kill.
//
// On any failure the returned error scrubs credentials (the URL via RedactURL,
// any git stderr via client.RedactCredentials) so an embedded user:token@ never
// reaches the message, and the temp dir is removed before returning.
func CloneRepoFetch(rawURL, branch, subPath string) (sourceDir string, cleanup func(), err error) {
	noop := func() {}

	cloneDir, err := os.MkdirTemp("", "dot-ai-repofetch-")
	if err != nil {
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to create temp dir for --repo-fetch clone: %v", err),
			ExitCode: client.ExitToolError,
		}
	}
	cleanup = func() { os.RemoveAll(cloneDir) }

	// Build argv explicitly. "--" terminates option parsing so a URL or path that
	// begins with "-" can never be smuggled in as a git flag.
	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch, "--single-branch")
	}
	args = append(args, "--", rawURL, cloneDir)

	stderr, timedOut, runErr := runGit(args...)
	if runErr != nil {
		cleanup()
		return "", noop, &client.RequestError{
			Message:  cloneFailureMessage(rawURL, stderr, timedOut),
			ExitCode: client.ExitToolError,
		}
	}

	// The clone's .git directory is VCS metadata, not skill source. Uploading it
	// would burn the ingestion budget (100 files / 256 KiB) on git internals — and
	// on any repo with real history it would spuriously exceed those limits — so
	// strip it before readLocalSource walks the tree. (This only prunes our own
	// throwaway temp clone.)
	if rmErr := os.RemoveAll(filepath.Join(cloneDir, ".git")); rmErr != nil {
		cleanup()
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to prepare --repo-fetch clone of %s: %v", RedactURL(rawURL), rmErr),
			ExitCode: client.ExitToolError,
		}
	}

	sourceDir = cloneDir
	if subPath != "" {
		resolved, perr := repoFetchSubdir(cloneDir, subPath)
		if perr != nil {
			cleanup()
			return "", noop, perr
		}
		sourceDir = resolved
	}
	return sourceDir, cleanup, nil
}

// CloneRepoFetchCached is the DEFAULT --repo-fetch path: a persistent,
// incremental, concurrency-safe clone cache. It resolves the per-URL cache dir,
// takes the per-URL flock, clones on first use / fetches incrementally on later
// use, then hands back a throwaway, .git-free copy of the (sub)tree to upload
// plus a cleanup the caller MUST defer. The cache itself PERSISTS across runs;
// only the returned upload copy is temporary.
//
// It shares CloneRepoFetch's security posture exactly: argv-safe git via the
// hardened runGit, the uploaded tree never contains .git, and a subPath is
// resolved through repoFetchSubdir's EvalSymlinks containment before anything is
// uploaded. Credentials never reach the cache path (the dir is keyed by the
// SCRUBBED url's sha256), the persisted remote (scrubbed after clone), the lock
// name, or any error.
func CloneRepoFetchCached(rawURL, branch, subPath string) (sourceDir string, cleanup func(), err error) {
	noop := func() {}

	cacheDir, err := repoCacheDir(RedactURL(rawURL))
	if err != nil {
		return "", noop, err
	}
	// Create the PARENT chain (<root>/dot-ai-cli/repos) at 0700: the cache may
	// hold fetched, possibly-private source. This guards only the parents — the
	// leaf <hash> dir is created by `git clone` under the process umask, so it is
	// chmod'd to 0700 separately in freshClone. The repos parent must also exist
	// before the per-URL lock (a sibling file of cacheDir) can be created.
	if mkErr := os.MkdirAll(filepath.Dir(cacheDir), 0o700); mkErr != nil {
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to create the --repo-fetch cache directory: %v", mkErr),
			ExitCode: client.ExitToolError,
		}
	}

	// Per-URL flock: serialize concurrent --repo-fetch of the SAME url so two
	// runs can't corrupt the shared cache. The second waits, then sees the
	// first's checkout (its own fetch is a fast no-op). Held through the
	// clone/fetch and the upload-copy, then released on return — the upload
	// itself reads the temp copy, not the cache, so it needs no lock.
	lock, lerr := acquireRepoCacheLock(cacheDir)
	if lerr != nil {
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: %v", lerr),
			ExitCode: client.ExitToolError,
		}
	}
	defer lock.Release()

	if serr := syncCache(cacheDir, rawURL, branch); serr != nil {
		return "", noop, serr
	}
	// Record this run as the cache entry's last use so `skills cache prune
	// --older-than` keeps an actively-used cache and only reaps idle ones.
	touchCache(cacheDir)

	// Resolve the subdir on the REAL cached tree (so EvalSymlinks containment
	// runs against actual repo content), then export a throwaway, .git-free copy
	// of exactly that (sub)tree for upload. The cache's .git is preserved for the
	// next incremental fetch.
	srcRoot := cacheDir
	if subPath != "" {
		resolved, perr := repoFetchSubdir(cacheDir, subPath)
		if perr != nil {
			return "", noop, perr
		}
		srcRoot = resolved
	}

	uploadDir, uerr := os.MkdirTemp("", "dot-ai-repofetch-cache-")
	if uerr != nil {
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to create temp dir for --repo-fetch upload: %v", uerr),
			ExitCode: client.ExitToolError,
		}
	}
	cleanup = func() { os.RemoveAll(uploadDir) }
	if cerr := copyTreeExcludingGit(srcRoot, uploadDir); cerr != nil {
		cleanup()
		return "", noop, &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to prepare --repo-fetch upload of %s: %v", RedactURL(rawURL), cerr),
			ExitCode: client.ExitToolError,
		}
	}
	return uploadDir, cleanup, nil
}

// repoCacheDir resolves the persistent clone-cache directory for a source.
// Per-repo dir = <cacheBase>/repos/<sha256(identifier)> where cacheBase is the
// shared XDG-respecting root (see cacheBaseDir) and identifier is the
// credential-SCRUBBED url, so the path is stable across credential rotation and
// never embeds a secret.
func repoCacheDir(identifier string) (string, error) {
	base, err := cacheBaseDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(identifier))
	return filepath.Join(base, "repos", hex.EncodeToString(sum[:])), nil
}

// touchCache bumps cacheDir's mtime to now after a successful sync, so
// `skills cache prune --older-than` measures LAST USE rather than creation: an
// incremental fetch with no on-disk diff would otherwise leave the dir mtime
// frozen at clone time and make an actively-used cache look stale. Best-effort —
// a chtimes failure never fails the run.
func touchCache(cacheDir string) {
	now := time.Now()
	_ = os.Chtimes(cacheDir, now, now)
}

// syncCache brings the cache dir up to date with the requested ref. A usable
// cached repo is updated incrementally (fetch + hard reset — O(diff), not a
// re-clone); a missing, partial, or corrupt cache (or one whose incremental
// update fails) is recovered by blowing the dir away and cloning fresh, rather
// than erroring. Only a failure of that clean re-clone is surfaced.
func syncCache(cacheDir, rawURL, branch string) error {
	if hasUsableRepo(cacheDir) {
		if err := updateCache(cacheDir, rawURL, branch); err == nil {
			return nil
		}
		// The cached repo looked usable but the incremental update failed —
		// e.g. a subtly corrupt object store an interrupted prior run left, or a
		// remote/ref that no longer resolves. Recover with a clean re-clone
		// instead of erroring (PRD #13 M4 corrupt-cache fallback).
	}
	return freshClone(cacheDir, rawURL, branch)
}

// hasUsableRepo reports whether cacheDir holds a clone we can fetch into: a real
// .git directory AND a resolvable HEAD commit. The HEAD check is what
// distinguishes a complete clone from a partial/interrupted one (whose .git
// exists but has no checked-out commit), so a corrupt cache routes to a clean
// re-clone instead of a doomed fetch. It is a local, no-network git call.
func hasUsableRepo(cacheDir string) bool {
	fi, err := os.Stat(filepath.Join(cacheDir, ".git"))
	if err != nil || !fi.IsDir() {
		return false
	}
	_, _, runErr := runGit("-C", cacheDir, "rev-parse", "--verify", "--quiet", "HEAD")
	return runErr == nil
}

// updateCache performs the incremental refresh: a shallow fetch of the requested
// ref (default branch when unset) followed by a hard reset of the working tree
// to the fetched commit. Resetting to FETCH_HEAD makes a --repo-branch change
// between runs "just work" (the requested ref is fetched and checked out
// regardless of what was previously checked out) and keeps the run O(diff). The
// live url is passed explicitly on the fetch command line — never via the stored
// origin remote — so no credential is ever read from persisted cache state. The
// returned error is internal (it only decides the re-clone fallback) and never
// surfaced, so it carries no url or git stderr.
func updateCache(cacheDir, rawURL, branch string) error {
	fetchArgs := []string{"-C", cacheDir, "fetch", "--depth", "1", "--", rawURL}
	if branch != "" {
		fetchArgs = append(fetchArgs, branch)
	}
	if _, _, ferr := runGit(fetchArgs...); ferr != nil {
		return fmt.Errorf("repo-fetch cache update: fetch failed")
	}
	if _, _, rerr := runGit("-C", cacheDir, "reset", "--hard", "FETCH_HEAD"); rerr != nil {
		return fmt.Errorf("repo-fetch cache update: reset failed")
	}
	return nil
}

// freshClone (re)creates the cache from scratch: it removes any existing dir
// (clearing a partial/corrupt clone) and shallow-clones the requested ref into
// it. After a successful clone it overwrites the persisted origin URL with the
// SCRUBBED url, so a credential the user embedded in rawURL never lingers in
// .git/config — subsequent fetches pass the live url explicitly instead. On
// failure the partial dir is removed and a credential-scrubbed error is
// surfaced (identical message shaping to the CloneRepoFetch path).
func freshClone(cacheDir, rawURL, branch string) error {
	if rmErr := os.RemoveAll(cacheDir); rmErr != nil {
		return &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to reset the --repo-fetch cache for %s: %v", RedactURL(rawURL), rmErr),
			ExitCode: client.ExitToolError,
		}
	}
	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch, "--single-branch")
	}
	args = append(args, "--", rawURL, cacheDir)

	stderr, timedOut, runErr := runGit(args...)
	if runErr != nil {
		os.RemoveAll(cacheDir) // never leave a partial clone behind
		return &client.RequestError{
			Message:  cloneFailureMessage(rawURL, stderr, timedOut),
			ExitCode: client.ExitToolError,
		}
	}
	// `git clone` creates the leaf cache dir under the process umask (measured
	// 0755/0775), NOT 0700 — so on its own the fetched, possibly-private source
	// would be group/world-readable; only the 0700 parent guards traversal. Chmod
	// the leaf to 0700 so it is self-protecting regardless of umask. The
	// incremental path (updateCache) only reuses this already-0700 dir, so every
	// --repo-fetch run leaves the per-repo cache dir 0700.
	if chErr := os.Chmod(cacheDir, 0o700); chErr != nil {
		os.RemoveAll(cacheDir) // don't leave possibly-private source world-readable
		return &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to secure the --repo-fetch cache for %s: %v", RedactURL(rawURL), chErr),
			ExitCode: client.ExitToolError,
		}
	}
	// Scrub any credential out of the persisted remote. Only needed when the url
	// actually carried one; the live url is still used (passed explicitly) for
	// future fetches, so auth is unaffected.
	if scrubbed := RedactURL(rawURL); scrubbed != rawURL {
		_, _, _ = runGit("-C", cacheDir, "remote", "set-url", "origin", scrubbed)
	}
	return nil
}

// copyTreeExcludingGit copies the working tree at src into dst (created 0700),
// skipping a top-level .git directory so the uploaded copy carries skill source
// only — the cache keeps its .git for the next incremental fetch. Symlinks are
// recreated AS symlinks (never followed): readLocalSource skips non-regular
// files, and repoFetchSubdir's EvalSymlinks check still rejects any link that
// escapes the tree, so the M3 symlink-escape containment is preserved.
func copyTreeExcludingGit(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o700)
		}
		if rel == ".git" && d.IsDir() {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o700)
		case d.Type()&fs.ModeSymlink != 0:
			linkDest, lerr := os.Readlink(p)
			if lerr != nil {
				return lerr
			}
			return os.Symlink(linkDest, target)
		case d.Type().IsRegular():
			return copyRegularFile(p, target)
		default:
			// Devices/sockets/fifos are not uploadable source; skip them.
			return nil
		}
	})
}

// copyRegularFile copies a single regular file from src to dst preserving its
// permission bits.
func copyRegularFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// cloneFailureMessage builds the user-facing --repo-fetch clone error with
// credentials scrubbed from BOTH sources that could carry them: the URL itself
// (via RedactURL) and any git stderr (via client.RedactCredentials). git stderr
// is the subtle one — an older git echoes the credentialed remote verbatim in
// its "unable to access '<url>'" diagnostic; a modern git (2.51+) redacts it
// itself, so RedactCredentials here is the second line of defense that keeps the
// scrub correct regardless of the host git version. When the clone hit the
// bounded timeout, git's stderr is just an opaque "signal: killed", so report
// the timeout plainly instead of surfacing it.
func cloneFailureMessage(rawURL, stderr string, timedOut bool) string {
	msg := fmt.Sprintf("Error: failed to clone --repo-fetch %s", RedactURL(rawURL))
	if timedOut {
		return msg + fmt.Sprintf(": clone timed out after %s", repoFetchCloneTimeout)
	}
	if detail := client.RedactCredentials(strings.TrimSpace(stderr)); detail != "" {
		msg += ": " + detail
	}
	return msg
}

// repoFetchSubdir resolves a --repo-path subdirectory inside the clone, refusing
// anything that is not a clean relative path contained in cloneDir (absolute
// paths or any "../" escape) and requiring it to exist as a directory. The
// returned path is what gets handed to the source uploader.
//
// A LEXICAL check alone is not enough: the clone is attacker-controlled content,
// so a committed symlink — possibly an INTERMEDIATE component of --repo-path
// (e.g. a repo committing "link -> /etc" and a caller passing
// "--repo-path link/sub") — can make filepath.WalkDir descend OUTSIDE the clone
// at resolution time and upload host files. To close that, the candidate subdir
// and the clone root are BOTH resolved through filepath.EvalSymlinks and the
// resolved subdir must stay contained in the resolved root (the same
// EvalSymlinks-then-containedIn posture AuthorizeRepoDir uses in repodir.go).
func repoFetchSubdir(cloneDir, subPath string) (string, error) {
	clean := filepath.Clean(subPath)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", &client.RequestError{
			Message: fmt.Sprintf("Error: --repo-path %q must be a relative path inside the repo "+
				"(no absolute paths or '..')", subPath),
			ExitCode: client.ExitUsageError,
		}
	}
	full := filepath.Join(cloneDir, clean)
	// Defense in depth: confirm the joined path did not lexically escape the root.
	rel, relErr := filepath.Rel(cloneDir, full)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-path %q resolves outside the cloned repo", subPath),
			ExitCode: client.ExitUsageError,
		}
	}

	// Resolve the clone root once. cloneDir is our own os.MkdirTemp directory and
	// always exists; a failure here is an environment fault, not user input.
	resolvedRoot, rootErr := filepath.EvalSymlinks(cloneDir)
	if rootErr != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: failed to resolve the --repo-fetch clone directory: %v", rootErr),
			ExitCode: client.ExitToolError,
		}
	}
	// Resolve the candidate subdir THROUGH any symlinks (including intermediate
	// ones). EvalSymlinks also requires every component to exist, so a missing
	// subdir (or a dangling link) surfaces here as a clean not-found error.
	resolvedFull, evalErr := filepath.EvalSymlinks(full)
	if evalErr != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-path %q not found in the cloned repo", subPath),
			ExitCode: client.ExitUsageError,
		}
	}
	// The resolved subdir must still live inside the resolved clone root; if a
	// symlink pointed it elsewhere, reject before WalkDir can read host files.
	if !containedIn(resolvedFull, []string{resolvedRoot}) {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-path %q resolves outside the cloned repo", subPath),
			ExitCode: client.ExitUsageError,
		}
	}
	info, statErr := os.Stat(resolvedFull)
	if statErr != nil {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-path %q not found in the cloned repo", subPath),
			ExitCode: client.ExitUsageError,
		}
	}
	if !info.IsDir() {
		return "", &client.RequestError{
			Message:  fmt.Sprintf("Error: --repo-path %q is not a directory in the cloned repo", subPath),
			ExitCode: client.ExitUsageError,
		}
	}
	return resolvedFull, nil
}
