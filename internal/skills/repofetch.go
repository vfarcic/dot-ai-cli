package skills

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vfarcic/dot-ai-cli/internal/client"
)

// repoFetchCloneTimeout bounds the host git clone so a wedged transport (an
// askpass GUI that never returns, a half-open TCP connection, a hung credential
// helper) can never hang the CLI forever. Two minutes is generous for a
// --depth 1 clone over a working transport yet short enough to fail visibly.
const repoFetchCloneTimeout = 2 * time.Minute

// PRD #13 M3 — network source ("--repo-fetch") clone via the host git stack.
//
// The CLI clones the requested repo using the host's local git stack (SSH agent,
// git credential helper, ~/.gitconfig, GIT_SSH_COMMAND, GIT_CONFIG_GLOBAL, …) and
// then feeds the clone into the SAME upload/list/render chain that --repo-dir
// uses (UploadLocalSource → ?source=<identifier>). This serves sources the
// SERVER cannot reach but the CLI host can (SSO / device-attested VPNs, etc.).
//
// M3 is "clone to a temp dir, use it, delete it" — there is no persistent cache,
// incremental fetch, or per-URL flock here; those are M4. The source identifier
// (used for the upload source field, the source: frontmatter tag, and the
// ?source= param) is the credential-scrubbed URL (RedactURL), computed by the
// caller; this file never lets a raw, possibly-credentialed URL or raw git
// stderr reach output.

// CloneRepoFetch shallow-clones rawURL into a fresh temp directory using the
// host's git binary and returns the directory to hand to UploadLocalSource plus
// a cleanup func the caller MUST defer. When subPath is set, the returned dir is
// <cloneDir>/<subPath> (validated to be a clean relative path inside the clone);
// when branch is set, the clone is restricted to that single branch.
//
// The URL and every flag are passed to git as separate argv elements via
// exec.Command — never through a shell and never string-interpolated — so a
// hostile URL cannot inject a command or a git flag. The host environment is
// inherited but locked down for this one invocation:
//   - GIT_TERMINAL_PROMPT=0, GIT_ASKPASS=, SSH_ASKPASS= so an un-authable clone
//     fails fast instead of hanging on a terminal/GUI credential prompt;
//   - GIT_ALLOW_PROTOCOL=file:git:http:https:ssh so a permissive host gitconfig
//     (e.g. protocol.ext.allow=always) cannot let a hostile ext::/other URL run
//     an arbitrary command (RCE) — only the expected fetch transports are
//     allowed (file is kept for the file:// e2e clones);
//   - the clone runs under a bounded context timeout so it can never hang
//     indefinitely even if every prompt-suppression above is somehow bypassed.
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

	ctx, cancel := context.WithTimeout(context.Background(), repoFetchCloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	// Inherit the host's git stack (SSH agent, credential helper, gitconfig, …)
	// but lock down this invocation: disable every interactive prompt so an
	// unauthenticated clone errors out immediately rather than blocking on a
	// terminal or GUI credential prompt, and pin the allowed fetch transports so
	// a permissive host gitconfig cannot turn a hostile URL into command
	// execution. file is retained for the file:// e2e clones.
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GIT_ALLOW_PROTOCOL=file:git:http:https:ssh",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr // captured; surfaced only on failure, after scrubbing
	if runErr := cmd.Run(); runErr != nil {
		cleanup()
		return "", noop, &client.RequestError{
			Message:  cloneFailureMessage(rawURL, stderr.String(), ctx.Err() == context.DeadlineExceeded),
			ExitCode: client.ExitToolError,
		}
	}

	// The clone's .git directory is VCS metadata, not skill source. Uploading it
	// would burn the ingestion budget (100 files / 256 KiB) on git internals — and
	// on any repo with real history it would spuriously exceed those limits — so
	// strip it before UploadLocalSource walks the tree. (Reuses UploadLocalSource
	// untouched; this only prunes our own throwaway temp clone.)
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
// returned path is what gets handed to UploadLocalSource.
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
