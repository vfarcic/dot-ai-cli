//go:build integration

package e2e_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// --- PRD #13 M1: --repo-fetch / --repo-dir / --source-label flag surface ---
//
// M1 is pure CLI surface + validation (no network, clone, or upload). Every
// assertion below is decided in PreRunE — or, for the allowed-but-unimplemented
// path, in the honest M2/M3 RunE stub — so these tests never depend on a server
// call and pass regardless of the pinned mock. runCLI still drives the real
// binary over --server-url, matching the project's integration-test convention.

// Mutual exclusion: at most one of --repo / --repo-fetch / --repo-dir may be
// supplied; combining two names both conflicting flags and exits non-zero.
func TestSkillsGenerate_SourceFlags_MutualExclusion(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantInErr []string
	}{
		{
			"repo+repo-fetch",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-fetch", "https://github.com/orgB/skills"},
			[]string{"--repo", "--repo-fetch", "mutually exclusive"},
		},
		{
			"repo+repo-dir",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo", "--repo-dir", "mutually exclusive"},
		},
		{
			"repo-fetch+repo-dir",
			[]string{"--repo-fetch", "https://github.com/orgB/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo-fetch", "--repo-dir", "mutually exclusive"},
		},
		{
			"repo+repo-fetch+repo-dir",
			[]string{"--repo", "https://github.com/orgA/skills", "--repo-fetch", "https://github.com/orgB/skills", "--repo-dir", "/some/skills/dir", "--source-label", "foo"},
			[]string{"--repo", "--repo-fetch", "--repo-dir", "mutually exclusive"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for mutually exclusive source flags; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			for _, want := range tc.wantInErr {
				if !strings.Contains(combined, want) {
					t.Errorf("expected mutual-exclusion error to contain %q, got: %s", want, combined)
				}
			}
			// Plain (non-RequestError) failures must also render with exactly one
			// "Error:" prefix now that cobra's own error printing is silenced.
			if strings.Contains(combined, "Error: Error:") {
				t.Errorf("expected a single \"Error:\" prefix on the usage error, got: %s", combined)
			}
		})
	}
}

// --repo-dir requires --source-label: a local path is not a stable
// cross-machine identifier, so the error must name the missing flag.
func TestSkillsGenerate_RepoDir_RequiresSourceLabel(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--repo-dir", "/some/skills/dir")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when --repo-dir lacks --source-label; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--source-label") {
		t.Errorf("expected error to name the missing --source-label flag, got: %s", combined)
	}
}

// --source-label requires --repo-dir: a label is meaningless without a local
// directory to apply it to (symmetric with --repo-path/--repo-branch).
func TestSkillsGenerate_SourceLabel_RequiresRepoDir(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit when --source-label lacks --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--source-label") || !strings.Contains(combined, "--repo-dir") {
		t.Errorf("expected error naming both --source-label and --repo-dir, got: %s", combined)
	}
}

// --repo-path / --repo-branch are valid qualifiers for --repo-fetch (a
// repo-bearing flag). Validation must NOT reject them; M3 is now implemented, so
// the run proceeds into the REAL clone path (no "not yet implemented" stub). A
// bogus file:// URL keeps this offline and fast — the clone fails, but never with
// a qualifier or stub error. (Successful branch/path clones are covered by the
// M3 end-to-end tests below.)
func TestSkillsGenerate_RepoFetch_AllowsPathBranch(t *testing.T) {
	const bogus = "file:///nonexistent/dot-ai-repofetch-allows-qualifiers"
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-fetch", bogus, "--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-fetch", bogus, "--repo-branch", "team-skills"}},
		{"both", []string{"--repo-fetch", bogus, "--repo-path", "skills", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			combined := stdout + stderr
			// The qualifier rule must accept --repo-fetch, so no qualifier error.
			if strings.Contains(combined, "require --repo") {
				t.Errorf("--repo-path/--repo-branch must be allowed with --repo-fetch; got qualifier error: %s", combined)
			}
			// M3 is implemented: the not-yet-implemented stub is gone.
			if strings.Contains(combined, "not yet implemented") {
				t.Errorf("the --repo-fetch M3 stub must be gone, got: %s", combined)
			}
			// The clone of a nonexistent repo fails, so the run exits non-zero.
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit from a bogus --repo-fetch clone; stdout: %s stderr: %s", stdout, stderr)
			}
		})
	}
}

// --repo-dir end-to-end behavior (read, upload, ?source= render, security
// posture) is covered in skills_repodir_test.go now that M2 is implemented; the
// "not yet implemented" stub it used to hit is gone.

// --repo-path / --repo-branch must be rejected with --repo-dir: a local
// directory takes no subdir/branch qualifier. The qualifier error (not the M2
// stub) must fire.
func TestSkillsGenerate_RepoDir_RejectsPathBranch(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-dir", "/some/skills/dir", "--source-label", "foo", "--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-dir", "/some/skills/dir", "--source-label", "foo", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for --repo-path/--repo-branch with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "require --repo or --repo-fetch") {
				t.Errorf("expected qualifier error (a dir takes no path/branch), got: %s", combined)
			}
		})
	}
}

// PRD #13 M5: --install-hook with --repo-dir/--repo-fetch is no longer rejected —
// BuildHookCommand now round-trips the source flags. The round-trip is covered by
// the hook tests in skills_hook_roundtrip_test.go.

// Guard: --pull-latest with --repo-dir is refused — --pull-latest forces a
// server-side git pull, which is meaningless for an uploaded local source.
func TestSkillsGenerate_RepoDir_PullLatestRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir,
		"--pull-latest", "--repo-dir", "/some/skills/dir", "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --pull-latest with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--pull-latest") || !strings.Contains(combined, "--repo-dir") {
		t.Errorf("expected a --pull-latest/--repo-dir incompatibility error, got: %s", combined)
	}
}

// Validation: a --source-label with characters outside [A-Za-z0-9._-] is refused
// with a clear usage error (it becomes a server-stored identifier and feeds the
// local:<user>-<label> prefix).
func TestSkillsGenerate_SourceLabel_InvalidCharset(t *testing.T) {
	for _, label := range []string{"bad/label", "has space", "with:colon"} {
		t.Run(label, func(t *testing.T) {
			dir := t.TempDir()
			stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir,
				"--repo-dir", "/some/skills/dir", "--source-label", label)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for invalid --source-label %q; stdout: %s stderr: %s", label, stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "--source-label") || !strings.Contains(combined, "invalid") {
				t.Errorf("expected an invalid --source-label charset error, got: %s", combined)
			}
		})
	}
}

// Existing behavior preserved: --repo-path / --repo-branch with NONE of the
// repo-bearing flags still errors.
func TestSkillsGenerate_PathBranch_RequireRepoFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"repo-path", []string{"--repo-path", "skills"}},
		{"repo-branch", []string{"--repo-branch", "team-skills"}},
		{"both", []string{"--repo-path", "skills", "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode == 0 {
				t.Fatalf("expected non-zero exit for --repo-path/--repo-branch without a repo flag; stdout: %s stderr: %s", stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "require --repo or --repo-fetch") {
				t.Errorf("expected qualifier error, got: %s", combined)
			}
		})
	}
}

// Sanity: the new flags are documented in the help output.
func TestSkillsGenerate_RepoFetchDir_InHelp(t *testing.T) {
	out, err := exec.Command(binaryPath, "skills", "generate", "--help").Output()
	if err != nil {
		t.Fatalf("expected exit 0 from --help, got error: %v", err)
	}
	s := string(out)
	for _, want := range []string{"--repo-fetch", "--repo-dir", "--source-label"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected help to mention %q", want)
		}
	}
}

// --- PRD #13 M3: --repo-fetch network source end-to-end ---
//
// These exercise the REAL --repo-fetch clone path against the pinned mock
// (:3001). The clone is genuine but LOCAL: each test inits a throwaway git repo,
// commits a SKILL.md under a brand-new (non-built-in) prompt name, and the CLI
// clones it via a file:// URL using the host git stack. The mock knows nothing
// about that repo — that is the whole point: the CLI clones and uploads it; the
// server only renders the uploaded source via ?source=<scrubbed-url>. No network,
// no server-side clone.
//
// Unlike --repo-dir, --repo-fetch never runs AuthorizeRepoDir, so its clone (an
// os.MkdirTemp dir under /tmp) is intentionally NOT subject to the /tmp /
// world-writable refusals — the throwaway temp clone IS the mechanism.

// newFetchedSkillFile is a brand-new prompt that exists ONLY in the cloned repo;
// its name collides with no server built-in. Generating dot-ai-wip-fetched-skill
// proves the full clone -> upload -> list-by-source -> render -> generate chain.
const newFetchedSkillFile = `---
name: wip-fetched-skill
description: A brand-new skill authored only in the fetched repo
---
WIP-FETCHED-MARKER body for a repo the CLI cloned itself.`

// repoFetchGitRepo creates a throwaway LOCAL git repository under the e2e/
// package dir, writes the given path->content files, commits them on the default
// branch, and returns the absolute repo path. Cleaned up after the test. The
// test clones it via fileURL(path) — a real, offline file:// clone.
func repoFetchGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	base, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir, err := os.MkdirTemp(base, "m3repo-")
	if err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	writeRepoFiles(t, dir, files)
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "dot-ai test")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "initial skills")
	return dir
}

// writeRepoFiles writes path->content files under dir, creating parent dirs.
func writeRepoFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// runGit runs a git subcommand in dir, failing the test on a non-zero exit.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// gitCurrentBranch returns the checked-out branch name in dir.
func gitCurrentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse in %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}

// fileURL builds a file:// URL for an absolute local path. The source identifier
// is RedactURL of this string (or of a credentialed variant of it).
func fileURL(absPath string) string { return "file://" + absPath }

// 1. End-to-end success: the CLI clones a local repo via file://, uploads it, and
// GENERATES a skill for a brand-new prompt that exists ONLY in the repo, tagged
// source: <scrubbed-url>. This is the M3 Success Criterion: a source the server
// never touched, rendered from the CLI-uploaded clone.
func TestSkillsGenerate_RepoFetch_EndToEnd_NewSkill(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	out := t.TempDir()
	url := fileURL(repo)

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", url)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	// Upload confirmation carries the scrubbed-URL identifier.
	if !strings.Contains(stdout, "Uploaded source as "+url) {
		t.Errorf("expected upload confirmation for %q, got: %s", url, stdout)
	}
	// The cloned brand-new prompt becomes a skill named after itself — proof the
	// clone -> upload -> list-by-source -> render chain ran end-to-end.
	content, err := os.ReadFile(filepath.Join(out, "dot-ai-wip-fetched-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected dot-ai-wip-fetched-skill generated from the cloned repo: %v", err)
	}
	if !strings.Contains(string(content), "WIP-FETCHED-MARKER") {
		t.Errorf("expected the cloned skill body, got:\n%s", string(content))
	}
	// Tagged with source: <scrubbed-url>.
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-wip-fetched-skill", "SKILL.md")); got != url {
		t.Errorf("expected source frontmatter %q, got %q", url, got)
	}
}

// 2. Credential scrubbing (HARD Success Criterion): a --repo-fetch URL embedding
// user:token@ must never leak the credential — not into the source: frontmatter,
// stdout, stderr, or the upload identifier. The local file:// clone succeeds
// (git ignores file-transport userinfo), so the credential is present on input
// yet must be RedactURL-scrubbed everywhere on output.
func TestSkillsGenerate_RepoFetch_CredentialScrubbing(t *testing.T) {
	const token = "s3cr3t-tok-abc123"
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	out := t.TempDir()
	scrubbed := fileURL(repo)                      // file:///.../m3repo-XXXX  (no creds)
	credURL := "file://user:" + token + "@" + repo // file://user:token@/.../m3repo-XXXX

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", credURL)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	combined := stdout + stderr
	// The credential must appear NOWHERE in any output.
	if strings.Contains(combined, token) {
		t.Errorf("credential token leaked into output: %s", combined)
	}
	if strings.Contains(combined, "user:"+token) {
		t.Errorf("raw userinfo leaked into output: %s", combined)
	}
	// The frontmatter records the SCRUBBED URL, identical to the no-cred form.
	got := readSkillSource(t, filepath.Join(out, "dot-ai-wip-fetched-skill", "SKILL.md"))
	if got != scrubbed {
		t.Errorf("expected scrubbed source frontmatter %q, got %q", scrubbed, got)
	}
	if strings.Contains(got, token) || strings.Contains(got, "user:") {
		t.Errorf("credential leaked into source frontmatter: %q", got)
	}
}

// 3. --repo-branch qualifies --repo-fetch: the team-skills branch carries a
// brand-new skill that the default branch lacks. Cloning with --repo-branch
// team-skills (and the source HEAD left on the default branch) must produce the
// branch-only skill — proof the single-branch clone honored the flag.
func TestSkillsGenerate_RepoFetch_RepoBranch(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	def := gitCurrentBranch(t, repo)

	const branchSkill = `---
name: wip-branch-skill
description: Present only on the team-skills branch
---
WIP-BRANCH-MARKER body present only on team-skills.`
	runGit(t, repo, "checkout", "-q", "-b", "team-skills")
	writeRepoFiles(t, repo, map[string]string{"wip-branch-skill/SKILL.md": branchSkill})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "branch-only skill")
	// Leave the source HEAD on the default branch so a clone WITHOUT --repo-branch
	// would NOT see the branch-only skill — making this test discriminating.
	runGit(t, repo, "checkout", "-q", def)

	out := t.TempDir()
	url := fileURL(repo)
	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only",
		"--repo-fetch", url, "--repo-branch", "team-skills")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	// The branch-only skill is present -> --repo-branch cloned team-skills.
	if _, err := os.ReadFile(filepath.Join(out, "dot-ai-wip-branch-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected dot-ai-wip-branch-skill from the team-skills branch: %v", err)
	}
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-wip-branch-skill", "SKILL.md")); got != url {
		t.Errorf("expected source frontmatter %q, got %q", url, got)
	}
}

// 4. --repo-path qualifies --repo-fetch: only the named subdir is uploaded. The
// sub-dir skill is generated; a root-level skill OUTSIDE the subdir is excluded —
// proof the uploader was handed <clone>/<repo-path>, not the whole clone.
func TestSkillsGenerate_RepoFetch_RepoPath(t *testing.T) {
	const subSkill = `---
name: wip-sub-skill
description: Lives under the team/ subdirectory
---
WIP-SUB-MARKER body under a subdir.`
	const rootSkill = `---
name: wip-root-skill
description: Lives at the repo root, outside the subdir
---
WIP-ROOT-MARKER body at the repo root.`
	repo := repoFetchGitRepo(t, map[string]string{
		"team/wip-sub-skill/SKILL.md": subSkill,
		"wip-root-skill/SKILL.md":     rootSkill,
	})
	out := t.TempDir()
	url := fileURL(repo)

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only",
		"--repo-fetch", url, "--repo-path", "team")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	// The subdir skill is present...
	if _, err := os.ReadFile(filepath.Join(out, "dot-ai-wip-sub-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected dot-ai-wip-sub-skill from --repo-path team: %v", err)
	}
	// ...and the root-level skill (outside the subdir) is NOT uploaded/generated.
	if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-root-skill")); !os.IsNotExist(err) {
		t.Errorf("expected the root skill to be EXCLUDED by --repo-path team")
	}
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-wip-sub-skill", "SKILL.md")); got != url {
		t.Errorf("expected source frontmatter %q, got %q", url, got)
	}
}

// 5. A clone that fails (bogus URL) exits non-zero with a CLEAN, URL-scrubbed
// error and writes no skill. A credentialed bogus file:// path proves the error
// path also scrubs: the token must not appear, and GIT_TERMINAL_PROMPT=0 keeps an
// un-authable clone from hanging (the test would otherwise time out).
func TestSkillsGenerate_RepoFetch_CloneFailure_ScrubbedNoSkill(t *testing.T) {
	const token = "s3cr3t-tok-fail789"
	out := t.TempDir()
	// Nonexistent local path -> git fails fast, offline, no credential prompt.
	bogus := "file://user:" + token + "@/nonexistent/dot-ai-repofetch-clone-failure"

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", bogus)
	if code == 0 {
		t.Fatalf("expected non-zero exit for a failing --repo-fetch clone; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	// Scrubbed: the credential must not leak into the error output.
	if strings.Contains(combined, token) {
		t.Errorf("credential token leaked into clone-failure error: %s", combined)
	}
	if strings.Contains(combined, "user:"+token) {
		t.Errorf("raw userinfo leaked into clone-failure error: %s", combined)
	}
	// The error names --repo-fetch so the failure is attributable.
	if !strings.Contains(combined, "--repo-fetch") {
		t.Errorf("expected the clone-failure error to name --repo-fetch, got: %s", combined)
	}
	// Nothing should have been generated.
	if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-fetched-skill")); !os.IsNotExist(err) {
		t.Errorf("expected no skill generated when the clone fails")
	}
}

// 5b. [PRD #13 M3 review fix C] Clone failure over HTTPS, offline. A credentialed
// https URL pointed at a closed local port (127.0.0.1:1) fails fast with
// "Connection refused" — no network, no DNS, no hang — and exercises the https
// stderr path, where git echoes the remote URL. The credential must not leak.
// (Modern git also redacts userinfo from its own diagnostic, so on the host git
// this is belt-and-suspenders; the deterministic proof that
// client.RedactCredentials(stderr) scrubs an echoed credential is the white-box
// TestCloneFailureMessage_ScrubsCredentialedStderr in internal/skills.)
func TestSkillsGenerate_RepoFetch_CloneFailure_HTTPS_ScrubbedNoSkill(t *testing.T) {
	const token = "s3cr3t-tok-https456"
	out := t.TempDir()
	// 127.0.0.1:1 has no listener -> connection refused, instant, fully offline.
	bogus := "https://user:" + token + "@127.0.0.1:1/nonexistent/dot-ai-repofetch.git"

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", bogus)
	if code == 0 {
		t.Fatalf("expected non-zero exit for a failing https --repo-fetch clone; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if strings.Contains(combined, token) {
		t.Errorf("credential token leaked into https clone-failure error: %s", combined)
	}
	if strings.Contains(combined, "user:"+token) {
		t.Errorf("raw userinfo leaked into https clone-failure error: %s", combined)
	}
	if !strings.Contains(combined, "--repo-fetch") {
		t.Errorf("expected the clone-failure error to name --repo-fetch, got: %s", combined)
	}
	if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-fetched-skill")); !os.IsNotExist(err) {
		t.Errorf("expected no skill generated when the https clone fails")
	}
}

// 6. [PRD #13 M3 review fix B] --pull-latest is rejected with --repo-fetch: it
// forces a server-side git pull, meaningless for a CLI-uploaded source (mirrors
// the existing --repo-dir guard). Decided in PreRunE, so offline and fast.
func TestSkillsGenerate_RepoFetch_PullLatestRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir,
		"--pull-latest", "--repo-fetch", "https://github.com/orgB/skills")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --pull-latest with --repo-fetch; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--pull-latest") || !strings.Contains(combined, "--repo-fetch") {
		t.Errorf("expected a --pull-latest/--repo-fetch incompatibility error, got: %s", combined)
	}
}

// 7. [PRD #13 M3 review fix E] --repo-path rejection paths. Each clones a real
// (offline file://) repo successfully, then repoFetchSubdir must reject the
// subdir — exit non-zero with a clean error and write NO skill. Covers a
// ".."-traversal, an absolute path, and a missing subdir; the symlink-escape
// case is its own test below because it needs a committed symlink.
func TestSkillsGenerate_RepoFetch_RepoPath_Rejections(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)
	cases := []struct {
		name     string
		repoPath string
		wantErr  string
	}{
		{"dotdot-traversal", "../escape", "must be a relative path inside the repo"},
		{"absolute", "/etc", "must be a relative path inside the repo"},
		{"missing", "does-not-exist", "not found in the cloned repo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only",
				"--repo-fetch", url, "--repo-path", tc.repoPath)
			if code == 0 {
				t.Fatalf("expected non-zero exit for --repo-path %q; stdout: %s stderr: %s", tc.repoPath, stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, tc.wantErr) {
				t.Errorf("expected error %q for --repo-path %q, got: %s", tc.wantErr, tc.repoPath, combined)
			}
			if !strings.Contains(combined, "--repo-path") {
				t.Errorf("expected the error to name --repo-path, got: %s", combined)
			}
			if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-fetched-skill")); !os.IsNotExist(err) {
				t.Errorf("expected no skill generated when --repo-path is rejected")
			}
		})
	}
}

// 8. [PRD #13 M3 review fix A+E] The priority fix: a committed symlink whose
// resolution escapes the clone must be refused. A lexical --repo-path check
// passes ("escape" / "escape/etc" are clean relative names), but EvalSymlinks
// resolves the committed "escape -> /" symlink OUTSIDE the clone, so
// repoFetchSubdir must reject before WalkDir can upload host files. Covers both a
// terminal symlink (--repo-path escape) and an INTERMEDIATE one (escape/etc).
func TestSkillsGenerate_RepoFetch_RepoPath_SymlinkEscape(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	// Commit a symlink that points at the filesystem root: always exists, always
	// outside the throwaway clone, so resolution is a genuine escape (not a
	// dangling/not-found link). git preserves symlinks across a file:// clone.
	if err := os.Symlink("/", filepath.Join(repo, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "add escaping symlink")
	url := fileURL(repo)

	cases := []struct {
		name     string
		repoPath string
	}{
		{"terminal-symlink", "escape"},         // escape -> /
		{"intermediate-symlink", "escape/etc"}, // escape/etc -> /etc (outside clone)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only",
				"--repo-fetch", url, "--repo-path", tc.repoPath)
			if code == 0 {
				t.Fatalf("expected non-zero exit for symlink-escape --repo-path %q; stdout: %s stderr: %s", tc.repoPath, stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "resolves outside the cloned repo") {
				t.Errorf("expected an escape rejection for --repo-path %q, got: %s", tc.repoPath, combined)
			}
			if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-fetched-skill")); !os.IsNotExist(err) {
				t.Errorf("expected no skill generated when a symlink --repo-path escapes the clone")
			}
		})
	}
}

// --- PRD #13 M4a: persistent clone cache, --no-cache, per-URL concurrency ---
//
// These exercise the DEFAULT (cached) --repo-fetch path. Each sets its OWN
// XDG_CACHE_HOME to a t.TempDir() so the cache layout is deterministic and never
// touches the real ~/.cache. The cache dir for a plain (credential-free) file://
// url is <root>/dot-ai-cli/repos/<sha256(url)>/, matching repoCacheDir's keying
// (RedactURL is identity for a url with no embedded credential).

// repoFetchCacheDir computes the persistent cache dir the CLI uses for url under
// cacheRoot. For a credential-free file:// url, RedactURL(url) == url, so the
// sha256 is taken over the url verbatim — the same key the implementation uses.
func repoFetchCacheDir(cacheRoot, url string) string {
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(cacheRoot, "dot-ai-cli", "repos", hex.EncodeToString(sum[:]))
}

// runCLIRaw runs the CLI with a custom environment WITHOUT calling t.Fatalf, so
// it is safe to invoke from a goroutine (the concurrency test). It returns the
// combined stdout+stderr and the exit code; err is non-nil only for a failure
// to start/await the process (not a non-zero CLI exit).
func runCLIRaw(env []string, args ...string) (combined string, exitCode int, err error) {
	fullArgs := append([]string{"--server-url", "http://localhost:3001"}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	cmd.Env = append(os.Environ(), env...)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return string(out), exitErr.ExitCode(), nil
		}
		return string(out), -1, runErr
	}
	return string(out), 0, nil
}

// M4a-1. Re-run REUSES the cache (fetch, not re-clone). Run 1 creates the cache
// dir with a real .git. We then drop an unrelated marker INSIDE .git/ and commit
// a NEW skill to the source. Run 2 must: (a) leave the marker intact — proving
// the dir was NOT blown away and re-cloned (a re-clone os.RemoveAll's the whole
// dir, marker included) — and (b) pick up the new commit via the incremental
// fetch, generating the new skill.
func TestSkillsGenerate_RepoFetch_Cache_ReuseNotReclone(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)

	// Run 1: first fetch → clone into the cache.
	out1 := t.TempDir()
	combined1, code1, err := runCLIRaw(env, "skills", "generate", "--path", out1, "--custom-only", "--repo-fetch", url)
	if err != nil {
		t.Fatalf("run 1 failed to execute: %v", err)
	}
	if code1 != 0 {
		t.Fatalf("run 1 expected exit 0, got %d; output: %s", code1, combined1)
	}
	if _, err := os.Stat(filepath.Join(out1, "dot-ai-wip-fetched-skill", "SKILL.md")); err != nil {
		t.Fatalf("run 1 expected dot-ai-wip-fetched-skill: %v", err)
	}

	// The cache dir exists with a real .git — the persistent clone.
	cacheDir := repoFetchCacheDir(cacheRoot, url)
	gitDir := filepath.Join(cacheDir, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		t.Fatalf("expected a persistent cache clone at %s/.git after run 1 (err=%v)", cacheDir, err)
	}

	// Drop a marker inside .git that only a re-clone (which removes the whole
	// cache dir) would destroy; an incremental fetch leaves it untouched.
	marker := filepath.Join(gitDir, "dot-ai-reuse-marker")
	if err := os.WriteFile(marker, []byte("survives a fetch, dies in a re-clone"), 0o600); err != nil {
		t.Fatalf("write reuse marker: %v", err)
	}

	// Add a brand-new skill to the SOURCE so run 2 must fetch to see it.
	const secondSkill = `---
name: wip-second-skill
description: Added to the source between run 1 and run 2
---
WIP-SECOND-MARKER body fetched incrementally on the second run.`
	writeRepoFiles(t, repo, map[string]string{"wip-second-skill/SKILL.md": secondSkill})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "second skill")

	// Run 2: must reuse the cache (fetch), not re-clone.
	out2 := t.TempDir()
	combined2, code2, err := runCLIRaw(env, "skills", "generate", "--path", out2, "--custom-only", "--repo-fetch", url)
	if err != nil {
		t.Fatalf("run 2 failed to execute: %v", err)
	}
	if code2 != 0 {
		t.Fatalf("run 2 expected exit 0, got %d; output: %s", code2, combined2)
	}

	// (a) The marker survived → the cache was REUSED, not blown away & re-cloned.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("expected the .git marker to survive run 2 (proof of reuse, not re-clone): %v", err)
	}
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		t.Errorf("expected the cache .git to persist after run 2 (err=%v)", err)
	}
	// (b) The incremental fetch picked up the new commit.
	if _, err := os.Stat(filepath.Join(out2, "dot-ai-wip-second-skill", "SKILL.md")); err != nil {
		t.Errorf("expected run 2 to fetch and generate the newly-committed dot-ai-wip-second-skill: %v", err)
	}
}

// M4a-2. --no-cache leaves NO persistent cache entry: it clones to a throwaway
// temp dir, uses it, and deletes it — and still generates correctly.
func TestSkillsGenerate_RepoFetch_NoCache_NoPersistentEntry(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)

	out := t.TempDir()
	combined, code, err := runCLIRaw(env, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", url, "--no-cache")
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0 with --no-cache, got %d; output: %s", code, combined)
	}
	// The skill is still generated...
	if _, err := os.Stat(filepath.Join(out, "dot-ai-wip-fetched-skill", "SKILL.md")); err != nil {
		t.Fatalf("expected dot-ai-wip-fetched-skill with --no-cache: %v", err)
	}
	// ...but no persistent cache entry is left behind.
	cacheDir := repoFetchCacheDir(cacheRoot, url)
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Errorf("expected NO persistent cache dir with --no-cache, but %s exists (err=%v)", cacheDir, err)
	}
}

// M4a-3. Two concurrent --repo-fetch of the SAME url serialize via the per-URL
// flock: both succeed, the cache is not corrupted, and the second sees a valid
// checkout (no race/partial). Each run writes to its OWN output dir so the
// separate output-dir lock never contends — only the per-URL cache lock does.
//
// To make the contention DETERMINISTIC (rather than hoping the two subprocesses
// happen to overlap), the test pre-acquires the very per-URL flock the CLI uses
// (<cacheDir>.lock, beside the cache dir) and holds it while BOTH subprocesses
// start. Both block in acquireRepoCacheLock until the gate is released; one then
// wins, clones, releases, and the other proceeds over the populated cache. Any
// interleaving still leaves both succeeding, so forcing contention adds no flake.
func TestSkillsGenerate_RepoFetch_Cache_ConcurrentSerialization(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)

	// Pre-create the cache parent and grab the per-URL gate lock the CLI will
	// contend on (repoCacheLockSuffix == ".lock", a sibling of the cache dir), so
	// both runs below are guaranteed to block on it rather than racing past.
	cacheDir := repoFetchCacheDir(cacheRoot, url)
	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o700); err != nil {
		t.Fatalf("mkdir cache parent: %v", err)
	}
	gate := flock.New(cacheDir + ".lock")
	if ok, err := gate.TryLock(); err != nil || !ok {
		t.Fatalf("test failed to pre-acquire the per-URL gate lock (ok=%v err=%v)", ok, err)
	}
	gateReleased := false
	releaseGate := func() {
		if !gateReleased {
			gateReleased = true
			_ = gate.Unlock()
		}
	}
	// Belt-and-suspenders: never wedge the lock for the full 5-min CLI timeout if
	// the test fails before the explicit release below.
	defer releaseGate()

	out1, out2 := t.TempDir(), t.TempDir()
	type result struct {
		combined string
		code     int
		err      error
	}
	results := make([]result, 2)
	outs := []string{out1, out2}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c, code, err := runCLIRaw(env, "skills", "generate", "--path", outs[idx], "--custom-only", "--repo-fetch", url)
			results[idx] = result{c, code, err}
		}(i)
	}
	// Give both subprocesses time to reach acquireRepoCacheLock and block on the
	// gate, then open it so they contend on the real per-URL lock simultaneously.
	time.Sleep(1 * time.Second)
	releaseGate()
	wg.Wait()

	// Both runs succeed — the loser of the flock waited and then proceeded.
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("concurrent run %d failed to execute: %v", i, r.err)
		}
		if r.code != 0 {
			t.Fatalf("concurrent run %d expected exit 0, got %d; output: %s", i, r.code, r.combined)
		}
		if _, err := os.Stat(filepath.Join(outs[i], "dot-ai-wip-fetched-skill", "SKILL.md")); err != nil {
			t.Errorf("concurrent run %d expected dot-ai-wip-fetched-skill: %v", i, err)
		}
	}

	// The shared cache is intact and has a valid HEAD (no partial/corrupt clone).
	if fi, err := os.Stat(filepath.Join(cacheDir, ".git")); err != nil || !fi.IsDir() {
		t.Fatalf("expected an intact cache .git after concurrent runs (err=%v)", err)
	}
	head := exec.Command("git", "-C", cacheDir, "rev-parse", "--verify", "HEAD")
	if out, err := head.CombinedOutput(); err != nil {
		t.Errorf("expected a valid HEAD in the shared cache after concurrent runs: %v\n%s", err, out)
	}
}

// M4a-5. [PRD #13 M4a review fix] The persistent per-URL cache dir holds fetched,
// possibly-private source, so its mode must be 0700. `git clone` creates the leaf
// dir under the process umask (measured 0755/0775 — group/world-readable), so the
// CLI chmods it to 0700; the 0700 parent alone would not protect the leaf's own
// mode. Assert the <root>/dot-ai-cli/repos/<hash> dir is exactly 0700 after a run.
func TestSkillsGenerate_RepoFetch_Cache_DirMode0700(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)

	out := t.TempDir()
	combined, code, err := runCLIRaw(env, "skills", "generate", "--path", out, "--custom-only", "--repo-fetch", url)
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, combined)
	}
	cacheDir := repoFetchCacheDir(cacheRoot, url)
	fi, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("expected the persistent cache dir at %s: %v", cacheDir, err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected cache dir mode 0700 (private — holds fetched source), got %#o", perm)
	}
}

// M4a-6. [PRD #13 M4a review fix] A --repo-branch CHANGE between two cached runs
// of the SAME url must check out the new branch. The cache is keyed by url only
// (not branch), so run 2 REUSES run 1's cache and updateCache's
// `fetch … <branch>` + `reset --hard FETCH_HEAD` must swing the working tree from
// branch alpha to branch beta. Run 1 (alpha) generates the alpha-only skill; run 2
// (beta, same url/cache) must generate the beta-only skill AND drop the alpha one —
// proving the new branch was fetched and checked out over the reused cache, not a
// stale alpha tree. A marker dropped in the cache's .git proves run 2 took the
// incremental reset path (a re-clone would os.RemoveAll the dir, marker included).
func TestSkillsGenerate_RepoFetch_Cache_RepoBranchChange(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	const alphaSkill = `---
name: wip-alpha-skill
description: Present only on the alpha branch
---
WIP-ALPHA-MARKER body present only on alpha.`
	const betaSkill = `---
name: wip-beta-skill
description: Present only on the beta branch
---
WIP-BETA-MARKER body present only on beta.`

	// A repo with two branches, each carrying its OWN branch-only skill, both
	// forked from a default branch that has neither.
	repo := repoFetchGitRepo(t, map[string]string{"README.md": "root"})
	def := gitCurrentBranch(t, repo)
	runGit(t, repo, "checkout", "-q", "-b", "alpha")
	writeRepoFiles(t, repo, map[string]string{"wip-alpha-skill/SKILL.md": alphaSkill})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "alpha skill")
	runGit(t, repo, "checkout", "-q", def)
	runGit(t, repo, "checkout", "-q", "-b", "beta")
	writeRepoFiles(t, repo, map[string]string{"wip-beta-skill/SKILL.md": betaSkill})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "beta skill")
	runGit(t, repo, "checkout", "-q", def)
	url := fileURL(repo)

	// Run 1: clone the repo into the cache on branch alpha.
	out1 := t.TempDir()
	combined1, code1, err := runCLIRaw(env, "skills", "generate", "--path", out1, "--custom-only", "--repo-fetch", url, "--repo-branch", "alpha")
	if err != nil {
		t.Fatalf("run 1 failed to execute: %v", err)
	}
	if code1 != 0 {
		t.Fatalf("run 1 expected exit 0, got %d; output: %s", code1, combined1)
	}
	if _, err := os.Stat(filepath.Join(out1, "dot-ai-wip-alpha-skill", "SKILL.md")); err != nil {
		t.Fatalf("run 1 expected the alpha-branch skill: %v", err)
	}

	// Drop a marker in the cache's .git so we can prove run 2 REUSED the cache
	// (incremental reset path) rather than blowing it away and re-cloning.
	cacheDir := repoFetchCacheDir(cacheRoot, url)
	marker := filepath.Join(cacheDir, ".git", "dot-ai-branchchange-marker")
	if err := os.WriteFile(marker, []byte("survives a reset, dies in a re-clone"), 0o600); err != nil {
		t.Fatalf("write reuse marker: %v", err)
	}

	// Run 2: SAME url (same cache), but --repo-branch beta. The reused cache must
	// fetch beta and reset --hard FETCH_HEAD, swinging the working tree to beta.
	out2 := t.TempDir()
	combined2, code2, err := runCLIRaw(env, "skills", "generate", "--path", out2, "--custom-only", "--repo-fetch", url, "--repo-branch", "beta")
	if err != nil {
		t.Fatalf("run 2 failed to execute: %v", err)
	}
	if code2 != 0 {
		t.Fatalf("run 2 expected exit 0, got %d; output: %s", code2, combined2)
	}
	// The marker survived → run 2 reused the cache via the incremental reset path.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("expected the .git marker to survive run 2 (proof of incremental reset, not re-clone): %v", err)
	}
	// The beta-only skill is present → reset --hard FETCH_HEAD swung to beta...
	if _, err := os.Stat(filepath.Join(out2, "dot-ai-wip-beta-skill", "SKILL.md")); err != nil {
		t.Errorf("run 2 expected the beta-branch skill (proof reset --hard FETCH_HEAD swung to beta): %v", err)
	}
	// ...and the stale alpha-only skill is gone → the tree fully moved to beta.
	if _, err := os.Stat(filepath.Join(out2, "dot-ai-wip-alpha-skill")); !os.IsNotExist(err) {
		t.Errorf("run 2 must NOT carry the stale alpha-branch skill after switching to beta")
	}
}

// M4a-4. --no-cache without --repo-fetch is a clean usage error (non-zero exit).
// It only controls the --repo-fetch clone cache, so it is meaningless alone.
// Decided in PreRunE, so offline and fast.
func TestSkillsGenerate_NoCache_RequiresRepoFetch(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", dir, "--no-cache")
	if code == 0 {
		t.Fatalf("expected non-zero exit for --no-cache without --repo-fetch; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--no-cache") || !strings.Contains(combined, "--repo-fetch") {
		t.Errorf("expected a --no-cache/--repo-fetch requirement error, got: %s", combined)
	}
}

// --- PRD #13 M6: integration-coverage gaps for --repo-fetch ---

// SC#6 for --repo-fetch (the HARD gap): an ARGUMENT-TAKING skill loaded via
// --repo-fetch renders through the server's ingested-source path — the --repo-dir
// analogue (TestSkillsGenerate_RepoDir_ArgTakingSkill_RendersViaSource) only proves
// this for a local directory. Here the CLI clones a local git repo (file://) whose
// troubleshoot-pod/SKILL.md SHADOWS the built-in troubleshoot-pod with the shared
// argTakingPromptFile fixture (arguments: podName + a {{podName}} template and the
// distinctive UPLOADED-LOCAL-MARKER body), uploads the .git-free clone, and renders
// troubleshoot-pod via ?source=<scrubbed-url>. The generated body must come from the
// UPLOADED clone (the marker) — proving the render resolved ?source= against the
// CLI-uploaded source, NOT a server built-in — the {{podName}} template survives (so
// the skill is genuinely argument-taking), and the built-in default pod name is
// absent. No DOT_AI_ALLOW_REPO_DIR: --repo-fetch is not gated by the local-dir opt-in.
func TestSkillsGenerate_RepoFetch_ArgTakingSkill_RendersViaSource(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"troubleshoot-pod/SKILL.md": argTakingPromptFile})
	out := t.TempDir()
	url := fileURL(repo)

	stdout, stderr, code := runCLI(t, "skills", "generate", "--path", out, "--custom-only",
		"--include", "troubleshoot-pod", "--repo-fetch", url)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}

	content, err := os.ReadFile(filepath.Join(out, "dot-ai-troubleshoot-pod", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected dot-ai-troubleshoot-pod generated from the cloned repo: %v", err)
	}
	s := string(content)
	// The body came from the uploaded clone via ?source= (load-bearing proof).
	if !strings.Contains(s, "UPLOADED-LOCAL-MARKER") {
		t.Errorf("expected the uploaded clone body (proves ?source= render of the CLI-uploaded clone), got:\n%s", s)
	}
	// The argument template is preserved — this is an argument-taking skill.
	if !strings.Contains(s, "{{podName}}") {
		t.Errorf("expected the {{podName}} argument template in the rendered skill, got:\n%s", s)
	}
	// It must NOT be the server's built-in fixture render (that body names the
	// fixture pod); that would mean ?source= was ignored and a built-in served.
	if strings.Contains(s, "nginx-deployment-7d9c67b5f-abc12") {
		t.Errorf("rendered the built-in default source, not the uploaded clone (?source= not honored):\n%s", s)
	}
	// And it carries the scrubbed-URL source identifier.
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-troubleshoot-pod", "SKILL.md")); got != url {
		t.Errorf("expected source frontmatter %q, got %q", url, got)
	}
}

// [PRD #13 M6 hardening] --repo-fetch wire format: the list and EACH render carry
// ?source=<scrubbed-url> and NEVER ?repo= — making SC#1 (the server never clones for
// --repo-fetch; the CLI clones and uploads) explicit on the wire. Mirrors the
// --repo-dir analogue TestSkillsGenerate_RepoDir_WireFormat_SourceParamNotRepo against
// a capturing backend (the stateless mock cannot expose request params). The upload
// body carries the scrubbed URL as source + a sha256 contentHash + the .git-free file
// list (.git is stripped before upload, so a one-SKILL.md repo uploads exactly one file).
func TestSkillsGenerate_RepoFetch_WireFormat_SourceParamNotRepo(t *testing.T) {
	cs := newRepoDirCaptureServer(t)
	repo := repoFetchGitRepo(t, map[string]string{"p1/SKILL.md": "---\nname: p1\n---\nbody"})
	out := t.TempDir()
	url := fileURL(repo)

	// Isolate the clone cache + upload-state store so the M4b content-hash gate never
	// skips the upload this test asserts on (an unrelated prior run could otherwise
	// have recorded this URL's hash in the shared cache).
	stdout, stderr, code := runCLIAtServer(t, cs.URL,
		[]string{"XDG_CACHE_HOME=" + t.TempDir()},
		"skills", "generate", "--path", out, "--custom-only", "--repo-fetch", url)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}

	reqs := cs.snapshot()

	// The upload happened with the scrubbed URL + the correct nested JSON body.
	upload := findRequest(t, reqs, http.MethodPost, "/api/v1/prompts/sources")
	var body struct {
		Source      string `json:"source"`
		ContentHash string `json:"contentHash"`
		Files       []struct {
			Path    string `json:"path"`
			Content string `json:"content"`
			Mode    string `json:"mode"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(upload.Body), &body); err != nil {
		t.Fatalf("upload body is not valid JSON: %v; raw: %s", err, upload.Body)
	}
	if body.Source != url {
		t.Errorf("expected upload source %q (the scrubbed URL), got %q", url, body.Source)
	}
	if !strings.HasPrefix(body.ContentHash, "sha256:") {
		t.Errorf("expected a sha256: contentHash, got %q", body.ContentHash)
	}
	if len(body.Files) != 1 || body.Files[0].Path != "p1/SKILL.md" || body.Files[0].Content == "" {
		t.Errorf("expected one .git-free file p1/SKILL.md with base64 content, got %+v", body.Files)
	}

	// The list + render carry ?source=<scrubbed-url> and never ?repo= — the server is
	// never asked to clone.
	list := findRequest(t, reqs, http.MethodGet, "/api/v1/prompts")
	render := findRequest(t, reqs, http.MethodPost, "/api/v1/prompts/p1")
	for _, r := range []capturedRequest{list, render} {
		if got := r.Query["source"]; len(got) != 1 || got[0] != url {
			t.Errorf("%s %s: expected ?source=%q, got %v", r.Method, r.Path, url, got)
		}
		if _, ok := r.Query["repo"]; ok {
			t.Errorf("%s %s: --repo-fetch run must never send ?repo=, got %v", r.Method, r.Path, r.Query["repo"])
		}
	}
}
