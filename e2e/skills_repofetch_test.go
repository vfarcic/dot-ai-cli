//go:build integration

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// Guard: --install-hook with --repo-dir is refused until M5. BuildHookCommand
// does not yet emit the source flag, so an installed hook would regenerate
// without the source — silently broken. The error must name PRD #13 M5.
func TestSkillsGenerate_RepoDir_InstallHookRejected(t *testing.T) {
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--agent", "claude-code",
		"--install-hook", "--repo-dir", "/some/skills/dir", "--source-label", "foo")
	if exitCode == 0 {
		t.Fatalf("expected non-zero exit for --install-hook with --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "--install-hook") || !strings.Contains(combined, "M5") {
		t.Errorf("expected an --install-hook M5 not-supported error, got: %s", combined)
	}
}

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
	if !strings.Contains(stdout, "Uploaded local source as "+url) {
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
// proof UploadLocalSource was handed <clone>/<repo-path>, not the whole clone.
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
