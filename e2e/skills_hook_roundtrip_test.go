//go:build integration

package e2e_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// --- PRD #13 M5 Part B: --install-hook round-trips the new source flags ---
//
// BuildHookCommand now emits --repo-fetch (credential-scrubbed) / --repo-dir /
// --source-label / --no-cache so an installed SessionStart hook regenerates the
// SAME source on every firing. These tests install a hook, read the stored
// command from .claude/settings.json, assert it carries the expected flags, and
// then re-run that command THROUGH A SHELL (the way Claude Code actually runs a
// SessionStart hook) to prove it regenerates the same source. Replaying via a
// shell — not exec.Command argv — is essential: it is the only path that would
// surface a shell-injection in the stored command (see the ShellInjection test),
// which an argv replay structurally cannot catch.
//
// Three credential/opt-in/safety properties are also proven: a credentialed
// --repo-fetch (and --repo) URL is stored SCRUBBED, DOT_AI_ALLOW_REPO_DIR is NOT
// embedded (a --repo-dir hook reads it from the env at hook-run time), and a
// shell metacharacter in a stored value is treated as inert DATA, never executed.

// hermeticEnviron returns os.Environ() with every DOT_AI_*-prefixed entry
// removed. The hook round-trip tests assert behavior that DOT_AI_* env vars
// (DOT_AI_ALLOW_REPO_DIR, DOT_AI_SKILLS_*, DOT_AI_GIT_TOKEN, …) directly affect,
// so an ambient one leaking in from the dev/CI shell could flip a result. The
// only DOT_AI_* the subprocess should see are the test-specific ones the caller
// appends on top of this base.
func hermeticEnviron() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DOT_AI_") {
			continue
		}
		env = append(env, kv)
	}
	return env
}

// runCLIInDir runs the CLI in a specific working directory (so a relative
// --agent claude-code writes .claude/ there) with the given extra env.
func runCLIInDir(t *testing.T, dir string, env []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	full := append([]string{"--server-url", "http://localhost:3001"}, args...)
	cmd := exec.Command(binaryPath, full...)
	cmd.Dir = dir
	cmd.Env = append(hermeticEnviron(), env...)
	var o, e strings.Builder
	cmd.Stdout = &o
	cmd.Stderr = &e
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run CLI in %s: %v", dir, err)
		}
	}
	return o.String(), e.String(), code
}

// readHookCommand returns the single stored SessionStart hook command from
// <dir>/.claude/settings.json. It also returns the raw settings bytes so callers
// can assert on the whole file (e.g. no credential anywhere).
func readHookCommand(t *testing.T, dir string) (command, raw string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	hooks, _ := settings["hooks"].(map[string]any)
	ss, _ := hooks["SessionStart"].([]any)
	if len(ss) != 1 {
		t.Fatalf("expected exactly 1 SessionStart entry, got %d (raw: %s)", len(ss), string(data))
	}
	entry, _ := ss[0].(map[string]any)
	inner, _ := entry["hooks"].([]any)
	if len(inner) != 1 {
		t.Fatalf("expected exactly 1 inner hook, got %d", len(inner))
	}
	hook, _ := inner[0].(map[string]any)
	cmd, _ := hook["command"].(string)
	if cmd == "" {
		t.Fatalf("stored hook command is empty (raw: %s)", string(data))
	}
	return cmd, string(data)
}

// shQuote mirrors internal/skills.shellQuote: wrap a value in POSIX single quotes,
// escaping any embedded single quote via the '\'' idiom. Used to splice the real
// test-binary path into a stored hook command before handing the whole line to a
// shell — the binary is "dot-ai" in the stored command but not on PATH in tests.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellArgv returns the argument vector a POSIX shell parses the stored hook
// command into (excluding the leading program name). It runs the command through
// `sh` with the program replaced by a probe that prints each positional arg on
// its own line, so the REAL shell parser decides word-splitting and quote
// handling. This lets the backward-compat test assert ARGV-identity across a
// quoting-style change (Go %q double quotes vs POSIX single quotes parse to the
// SAME argv) without re-implementing a shell tokenizer. The stored values here
// (flags, URLs, paths, regex filters) never contain newlines, so a newline
// separator is unambiguous.
func shellArgv(t *testing.T, command string) []string {
	t.Helper()
	rest := strings.TrimPrefix(command, "dot-ai")
	if rest == command {
		t.Fatalf("hook command must start with 'dot-ai', got: %s", command)
	}
	script := `argv() { for a in "$@"; do printf '%s\n' "$a"; done; }; argv` + rest
	out, err := exec.Command("sh", "-c", script).Output()
	if err != nil {
		t.Fatalf("shell-tokenize %q: %v", command, err)
	}
	s := strings.TrimSuffix(string(out), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// rerunHookCommand replays the stored hook command THROUGH A SHELL in a fresh
// working directory — exactly how Claude Code runs a SessionStart hook (the
// stored command is a single string handed to a shell, not an argv). The stored
// command names the program "dot-ai" (not on PATH in tests), so only the leading
// "dot-ai" token is swapped for the shell-quoted test binary; everything after it
// is passed to the shell VERBATIM, so the shell performs the same quoting and
// word-splitting Claude Code's shell would. The mock is reached via DOT_AI_URL
// (the env equivalent of --server-url). Returns the working dir (where
// .claude/skills was written, and where any injected command would run) plus
// stdout/stderr/exit.
func rerunHookCommand(t *testing.T, env []string, command string) (workdir, stdout, stderr string, code int) {
	t.Helper()
	rest := strings.TrimPrefix(command, "dot-ai")
	if rest == command {
		t.Fatalf("hook command must start with 'dot-ai', got: %s", command)
	}
	workdir = t.TempDir()
	cmd := exec.Command("sh", "-c", shQuote(binaryPath)+rest)
	cmd.Dir = workdir
	cmd.Env = append(hermeticEnviron(), append([]string{"DOT_AI_URL=http://localhost:3001"}, env...)...)
	var o, e strings.Builder
	cmd.Stdout = &o
	cmd.Stderr = &e
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("rerun hook command via shell: %v", err)
		}
	}
	return workdir, o.String(), e.String(), code
}

// 1. --repo-fetch round-trip: installing the hook now SUCCEEDS (the old M2/M3
// guard is gone), the stored command carries --repo-fetch <url>, and re-running
// it regenerates the same source.
func TestSkillsGenerate_M5_InstallHook_RepoFetch_RoundTrip(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)
	env := []string{"XDG_CACHE_HOME=" + t.TempDir()}

	installDir := t.TempDir()
	_, stderr, code := runCLIInDir(t, installDir, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only", "--repo-fetch", url)
	if code != 0 {
		t.Fatalf("install --install-hook + --repo-fetch must now SUCCEED, got exit %d; stderr: %s", code, stderr)
	}

	command, _ := readHookCommand(t, installDir)
	if !strings.Contains(command, "--repo-fetch") || !strings.Contains(command, url) {
		t.Errorf("expected stored hook to contain --repo-fetch %q, got: %s", url, command)
	}

	rerunDir, rout, rerr, rcode := rerunHookCommand(t, env, command)
	if rcode != 0 {
		t.Fatalf("re-running the stored hook must regenerate the source, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	got := readSkillSource(t, filepath.Join(rerunDir, ".claude", "skills", "dot-ai-wip-fetched-skill", "SKILL.md"))
	if got != url {
		t.Errorf("re-run expected dot-ai-wip-fetched-skill tagged source %q, got %q", url, got)
	}
}

// 2. --repo-fetch with --repo-path/--repo-branch/--no-cache: all three qualifiers
// round-trip. The hook is installed for a subdir on a non-default branch with the
// cache bypassed; the stored command carries every flag, and re-running it clones
// that branch's subdir and regenerates the branch+subdir-only skill.
func TestSkillsGenerate_M5_InstallHook_RepoFetch_PathBranchNoCache(t *testing.T) {
	const subSkill = `---
name: wip-sub-skill
description: Lives under team/ on the team-skills branch only
---
WIP-SUB-MARKER body under a subdir on a branch.`
	// Default branch carries only a root skill, so a clone without --repo-branch /
	// --repo-path would NOT find wip-sub-skill — making the round-trip discriminating.
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	def := gitCurrentBranch(t, repo)
	runGit(t, repo, "checkout", "-q", "-b", "team-skills")
	writeRepoFiles(t, repo, map[string]string{"team/wip-sub-skill/SKILL.md": subSkill})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "branch subdir skill")
	runGit(t, repo, "checkout", "-q", def)
	url := fileURL(repo)
	env := []string{"XDG_CACHE_HOME=" + t.TempDir()}

	installDir := t.TempDir()
	_, stderr, code := runCLIInDir(t, installDir, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only",
		"--repo-fetch", url, "--repo-path", "team", "--repo-branch", "team-skills", "--no-cache")
	if code != 0 {
		t.Fatalf("install with --repo-fetch path/branch/no-cache must succeed, got exit %d; stderr: %s", code, stderr)
	}

	command, _ := readHookCommand(t, installDir)
	for _, want := range []string{"--repo-fetch", "--repo-path", "team", "--repo-branch", "team-skills", "--no-cache"} {
		if !strings.Contains(command, want) {
			t.Errorf("expected stored hook to contain %q, got: %s", want, command)
		}
	}

	rerunDir, rout, rerr, rcode := rerunHookCommand(t, env, command)
	if rcode != 0 {
		t.Fatalf("re-running the path/branch/no-cache hook must succeed, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	// The branch+subdir-only skill is regenerated -> --repo-branch and --repo-path
	// both round-tripped through the hook.
	got := readSkillSource(t, filepath.Join(rerunDir, ".claude", "skills", "dot-ai-wip-sub-skill", "SKILL.md"))
	if got != url {
		t.Errorf("re-run expected dot-ai-wip-sub-skill tagged source %q, got %q", url, got)
	}
}

// 3. Credential safety (HARD criterion): a credentialed --repo-fetch URL is
// stored SCRUBBED — the token must appear NOWHERE in settings.json — and the
// stored (scrubbed) command still round-trips.
func TestSkillsGenerate_M5_InstallHook_RepoFetch_CredentialScrubbed(t *testing.T) {
	const token = "hook-s3cr3t-tok-xyz"
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	scrubbed := fileURL(repo)
	credURL := "file://user:" + token + "@" + repo
	env := []string{"XDG_CACHE_HOME=" + t.TempDir()}

	installDir := t.TempDir()
	_, stderr, code := runCLIInDir(t, installDir, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only", "--repo-fetch", credURL)
	if code != 0 {
		t.Fatalf("install with credentialed --repo-fetch must succeed, got exit %d; stderr: %s", code, stderr)
	}

	command, raw := readHookCommand(t, installDir)
	// The credential must never reach settings.json, in any form.
	if strings.Contains(raw, token) {
		t.Errorf("credential token leaked into settings.json: %s", raw)
	}
	if strings.Contains(raw, "user:"+token) {
		t.Errorf("raw userinfo leaked into settings.json: %s", raw)
	}
	// The stored command carries the SCRUBBED URL, identical to the no-cred form.
	if !strings.Contains(command, scrubbed) {
		t.Errorf("expected stored hook to contain the scrubbed URL %q, got: %s", scrubbed, command)
	}

	rerunDir, rout, rerr, rcode := rerunHookCommand(t, env, command)
	if rcode != 0 {
		t.Fatalf("re-running the scrubbed hook must regenerate the source, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	if got := readSkillSource(t, filepath.Join(rerunDir, ".claude", "skills", "dot-ai-wip-fetched-skill", "SKILL.md")); got != scrubbed {
		t.Errorf("re-run expected source %q, got %q", scrubbed, got)
	}
}

// 4. --repo-dir round-trip: installing now SUCCEEDS, the stored command carries
// --repo-dir <path> --source-label <label>, and re-running WITH the opt-in set
// regenerates the same source. Crucially, DOT_AI_ALLOW_REPO_DIR is NOT embedded —
// the stored command never mentions it, and re-running WITHOUT the opt-in in the
// env FAILS (the hook reads the opt-in from the env at hook-run time).
func TestSkillsGenerate_M5_InstallHook_RepoDir_RoundTrip(t *testing.T) {
	src := repoDirSource(t, map[string]string{"wip-new-skill/SKILL.md": newWipSkillFile})
	env := []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + t.TempDir()}

	installDir := t.TempDir()
	_, stderr, code := runCLIInDir(t, installDir, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only",
		"--repo-dir", src, "--source-label", "foo")
	if code != 0 {
		t.Fatalf("install --install-hook + --repo-dir must now SUCCEED, got exit %d; stderr: %s", code, stderr)
	}

	command, raw := readHookCommand(t, installDir)
	for _, want := range []string{"--repo-dir", src, "--source-label", "foo"} {
		if !strings.Contains(command, want) {
			t.Errorf("expected stored hook to contain %q, got: %s", want, command)
		}
	}
	// The opt-in env var must NOT be baked into settings.json (shared/committed
	// files must not let a clone side-load without consent).
	if strings.Contains(raw, "DOT_AI_ALLOW_REPO_DIR") {
		t.Errorf("DOT_AI_ALLOW_REPO_DIR must NOT be embedded in the hook, got: %s", raw)
	}

	// Re-run WITH the opt-in: regenerates the same source.
	rerunDir, rout, rerr, rcode := rerunHookCommand(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + t.TempDir()}, command)
	if rcode != 0 {
		t.Fatalf("re-running the --repo-dir hook (opt-in set) must regenerate, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	if got := readSkillSource(t, filepath.Join(rerunDir, ".claude", "skills", "dot-ai-wip-new-skill", "SKILL.md")); got != "local:tester-foo" {
		t.Errorf("re-run expected dot-ai-wip-new-skill tagged local:tester-foo, got %q", got)
	}

	// Re-run WITHOUT the opt-in: must FAIL, proving the opt-in is read from the env
	// at hook-run time and was never embedded.
	_, _, nerr, ncode := rerunHookCommand(t, []string{"USER=tester", "XDG_CACHE_HOME=" + t.TempDir()}, command)
	if ncode == 0 {
		t.Fatalf("re-running the --repo-dir hook WITHOUT DOT_AI_ALLOW_REPO_DIR must fail")
	}
	if !strings.Contains(nerr, "DOT_AI_ALLOW_REPO_DIR") {
		t.Errorf("expected the opt-in error to name DOT_AI_ALLOW_REPO_DIR, got: %s", nerr)
	}
}

// 5. Backward compatibility (ARGV-identical): a hook installed with NO new source
// flags parses — through a shell — to the SAME argument vector as the pre-PRD-13
// command string. The M5 fix changes the quoting BYTES (Go %q double quotes ->
// POSIX single quotes, to close the shell-injection) but must not change the
// parsed argv, reorder flags, or drop/alter any value. The credentialless --repo
// case is unaffected by Fix 2's RedactURL (no userinfo to strip), so its argv is
// unchanged too.
func TestSkillsGenerate_M5_InstallHook_LegacyCommands_ArgvIdentical(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// want is the canonical PRE-PRD-13 (Go %q) command string. The emitted
		// command now uses single quotes, so the raw bytes differ for quoted
		// values — but both must parse to the same argv (proven via shellArgv).
		want string
	}{
		{
			"no-flag",
			[]string{"skills", "generate", "--agent", "claude-code", "--install-hook"},
			"dot-ai skills generate --agent claude-code",
		},
		{
			"custom-only-include-exclude",
			[]string{"skills", "generate", "--agent", "claude-code", "--install-hook",
				"--custom-only", "--include", "foo", "--exclude", "bar"},
			`dot-ai skills generate --agent claude-code --custom-only --include "foo" --exclude "bar"`,
		},
		{
			"repo-path-branch",
			[]string{"skills", "generate", "--agent", "claude-code", "--install-hook",
				"--repo", "https://github.com/orgA/skills", "--repo-path", "skills", "--repo-branch", "team-skills"},
			`dot-ai skills generate --agent claude-code --repo "https://github.com/orgA/skills" --repo-path "skills" --repo-branch "team-skills"`,
		},
	}
	// Isolate HOME (so no real ~/.config/dot-ai/settings.json injects filters) and
	// clear the skills filter env vars, so the emitted command is deterministic.
	cleanEnv := []string{
		"HOME=" + t.TempDir(),
		"DOT_AI_SKILLS_INCLUDE=",
		"DOT_AI_SKILLS_EXCLUDE=",
		"DOT_AI_SKILLS_CUSTOM_ONLY=",
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installDir := t.TempDir()
			_, stderr, code := runCLIInDir(t, installDir, cleanEnv, tc.args...)
			if code != 0 {
				t.Fatalf("install must succeed, got exit %d; stderr: %s", code, stderr)
			}
			command, _ := readHookCommand(t, installDir)
			got, want := shellArgv(t, command), shellArgv(t, tc.want)
			if !equalStrings(got, want) {
				t.Errorf("hook command not argv-identical\nwant argv: %q (from %s)\ngot argv:  %q (from %s)", want, tc.want, got, command)
			}
		})
	}
}

// equalStrings reports whether two string slices are element-wise equal.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 6. Credential safety for --repo (Fix 2): a credentialed --repo URL is stored
// SCRUBBED in settings.json (the token appears nowhere), closing a pre-existing
// leak — the hook path emitted ov.Repo RAW before this fix, while only the
// --repo-fetch path scrubbed. The stored command carries the scrubbed URL,
// identical to the no-credential form (RedactURL on the hook path).
func TestSkillsGenerate_M5_InstallHook_Repo_CredentialScrubbed(t *testing.T) {
	const token = "repo-hook-s3cr3t-abc"
	const scrubbed = "https://github.com/orgA/skills"
	credURL := "https://user:" + token + "@github.com/orgA/skills"
	// Isolate HOME / clear filter env so the emitted command is deterministic.
	cleanEnv := []string{
		"HOME=" + t.TempDir(),
		"DOT_AI_SKILLS_INCLUDE=",
		"DOT_AI_SKILLS_EXCLUDE=",
		"DOT_AI_SKILLS_CUSTOM_ONLY=",
	}

	installDir := t.TempDir()
	_, stderr, code := runCLIInDir(t, installDir, cleanEnv,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--repo", credURL)
	if code != 0 {
		t.Fatalf("install with credentialed --repo must succeed, got exit %d; stderr: %s", code, stderr)
	}

	command, raw := readHookCommand(t, installDir)
	// The credential must never reach settings.json, in any form.
	if strings.Contains(raw, token) {
		t.Errorf("--repo credential token leaked into settings.json: %s", raw)
	}
	if strings.Contains(raw, "user:"+token) {
		t.Errorf("raw --repo userinfo leaked into settings.json: %s", raw)
	}
	// The stored command carries the SCRUBBED URL, identical to the no-cred form.
	if !strings.Contains(command, scrubbed) {
		t.Errorf("expected stored hook to contain the scrubbed --repo URL %q, got: %s", scrubbed, command)
	}
}

// 7. Shell-injection (Fix 1, BLOCKING): a stored value carrying a shell command-
// substitution payload must be treated as inert DATA when Claude Code runs the
// hook THROUGH A SHELL, never executed. Each subtest installs a hook whose source
// value embeds a real, working payload component `evil$(touch${IFS}<marker>)`
// (the directory/repo truly exists under that name, so install — which uses the
// literal Go string with NO shell — succeeds), then replays the stored command
// VIA A SHELL and asserts (a) the marker file was NOT created (no execution) and
// (b) the literal value round-tripped as data (the source regenerated from the
// payload-named source). With the old Go %q (double-quote) emission the shell
// would expand `$(...)`, create the marker, and mangle the value — so this test
// FAILS pre-fix and PASSES after shellQuote (single quotes).
func TestSkillsGenerate_M5_InstallHook_ShellInjection_NotExecuted(t *testing.T) {
	// Authorized parent for the payload-named source: under the e2e/ package dir
	// (writable, NOT /tmp, NOT world-writable) so --repo-dir's AuthorizeRepoDir
	// accepts it. --repo-fetch has no such restriction but reuses the same parent.
	base, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	// assertNoMarker fails if the payload executed anywhere we can observe. The
	// shell replay runs with CWD = workdir, so a `touch <relative-marker>` would
	// land there; parent/installDir are checked for defense in depth.
	assertNoMarker := func(t *testing.T, marker string, dirs ...string) {
		t.Helper()
		for _, d := range dirs {
			if _, err := os.Stat(filepath.Join(d, marker)); err == nil {
				t.Fatalf("SHELL-INJECTION EXECUTED: marker %q was created under %s", marker, d)
			}
		}
	}

	t.Run("repo-dir", func(t *testing.T) {
		const marker = "PWNED_repo_dir"
		parent, err := os.MkdirTemp(base, "m5inj-dir-")
		if err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(parent) })
		// A REAL, authorized directory whose final path component embeds the
		// payload. No '/' in the payload (a filename can't contain one), so the
		// touched marker is a RELATIVE name created in the replay's CWD on expansion.
		evil := filepath.Join(parent, "evil$(touch${IFS}"+marker+")")
		if err := os.MkdirAll(filepath.Join(evil, "wip-new-skill"), 0o755); err != nil {
			t.Fatalf("mkdir evil src: %v", err)
		}
		if err := os.WriteFile(filepath.Join(evil, "wip-new-skill", "SKILL.md"), []byte(newWipSkillFile), 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}

		env := []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + t.TempDir()}
		installDir := t.TempDir()
		_, stderr, code := runCLIInDir(t, installDir, env,
			"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only",
			"--repo-dir", evil, "--source-label", "evil")
		if code != 0 {
			t.Fatalf("install with a payload-named --repo-dir must succeed (the path is a real authorized dir), got exit %d; stderr: %s", code, stderr)
		}

		command, raw := readHookCommand(t, installDir)
		// Stored as DATA: the literal payload survives verbatim into settings.json
		// (it was not executed at build time, and is single-quoted for the shell).
		if !strings.Contains(raw, "evil$(touch${IFS}"+marker+")") {
			t.Fatalf("expected the literal payload path stored verbatim in settings.json, got: %s", raw)
		}

		workdir, rout, rerr, rcode := rerunHookCommand(t, env, command)
		assertNoMarker(t, marker, workdir, parent, installDir, base)
		if rcode != 0 {
			t.Fatalf("re-running the payload-named --repo-dir hook must regenerate (the value is data), got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
		}
		// Round-tripped as data: the source regenerated from the real, payload-named
		// directory under its label (the $(...) reached the CLI as a literal path).
		if got := readSkillSource(t, filepath.Join(workdir, ".claude", "skills", "dot-ai-wip-new-skill", "SKILL.md")); got != "local:tester-evil" {
			t.Errorf("re-run expected dot-ai-wip-new-skill tagged local:tester-evil, got %q", got)
		}
	})

	t.Run("repo-fetch", func(t *testing.T) {
		const marker = "PWNED_repo_fetch"
		parent, err := os.MkdirTemp(base, "m5inj-fetch-")
		if err != nil {
			t.Fatalf("mkdir parent: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(parent) })
		// A REAL git repo whose directory NAME embeds the payload; its file:// URL
		// therefore carries the payload. RedactURL leaves it verbatim (no userinfo
		// to strip), so the literal payload reaches settings.json as the source URL.
		evil := filepath.Join(parent, "evil$(touch${IFS}"+marker+")")
		if err := os.MkdirAll(filepath.Join(evil, "wip-fetched-skill"), 0o755); err != nil {
			t.Fatalf("mkdir evil repo: %v", err)
		}
		if err := os.WriteFile(filepath.Join(evil, "wip-fetched-skill", "SKILL.md"), []byte(newFetchedSkillFile), 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}
		runGit(t, evil, "init", "-q")
		runGit(t, evil, "config", "user.email", "test@example.com")
		runGit(t, evil, "config", "user.name", "dot-ai test")
		runGit(t, evil, "add", "-A")
		runGit(t, evil, "commit", "-q", "-m", "payload-named repo")
		url := fileURL(evil)

		env := []string{"XDG_CACHE_HOME=" + t.TempDir()}
		installDir := t.TempDir()
		_, stderr, code := runCLIInDir(t, installDir, env,
			"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only", "--repo-fetch", url)
		if code != 0 {
			t.Fatalf("install with a payload-named --repo-fetch URL must succeed (real repo), got exit %d; stderr: %s", code, stderr)
		}

		command, raw := readHookCommand(t, installDir)
		if !strings.Contains(raw, "evil$(touch${IFS}"+marker+")") {
			t.Fatalf("expected the literal payload URL stored verbatim in settings.json, got: %s", raw)
		}

		workdir, rout, rerr, rcode := rerunHookCommand(t, env, command)
		assertNoMarker(t, marker, workdir, parent, installDir, base)
		if rcode != 0 {
			t.Fatalf("re-running the payload-named --repo-fetch hook must regenerate (the value is data), got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
		}
		// Round-tripped as data: the literal URL reached host git, cloned the real
		// repo, and regenerated the source tagged with that URL.
		if got := readSkillSource(t, filepath.Join(workdir, ".claude", "skills", "dot-ai-wip-fetched-skill", "SKILL.md")); got != url {
			t.Errorf("re-run expected dot-ai-wip-fetched-skill tagged source %q, got %q", url, got)
		}
	})

	// CodeRabbit fix [5]: BuildHookCommand also shellQuotes the legacy/qualifier
	// flags (--repo, --repo-path, --repo-branch, --include, --exclude), so a
	// regression in any of THOSE branches would reopen the injection. Cover a
	// representative one (--include) with the same guarantee: a payload-bearing
	// value is stored verbatim as DATA and is inert when the hook runs through a
	// shell. The payload is a valid Go regexp (it just matches nothing), so the
	// CLI accepts it as a filter at both install and replay.
	t.Run("include", func(t *testing.T) {
		const marker = "PWNED_include"
		// Payload doubles as a no-match regexp filter. With the old Go %q (double-
		// quote) emission a shell would expand $(touch${IFS}PWNED_include) on
		// replay; single-quoting keeps it inert.
		payload := "evil$(touch${IFS}" + marker + ")"

		// Clear ambient skills-filter env so the emitted command is deterministic
		// (only the --include we pass should appear).
		env := []string{
			"DOT_AI_SKILLS_INCLUDE=",
			"DOT_AI_SKILLS_EXCLUDE=",
			"DOT_AI_SKILLS_CUSTOM_ONLY=",
		}
		installDir := t.TempDir()
		_, stderr, code := runCLIInDir(t, installDir, env,
			"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only", "--include", payload)
		if code != 0 {
			t.Fatalf("install with a payload-bearing --include must succeed (valid regexp, no shell at build), got exit %d; stderr: %s", code, stderr)
		}

		command, raw := readHookCommand(t, installDir)
		// Stored as DATA: the literal payload survives verbatim into settings.json.
		if !strings.Contains(raw, payload) {
			t.Fatalf("expected the literal --include payload stored verbatim in settings.json, got: %s", raw)
		}

		workdir, rout, rerr, rcode := rerunHookCommand(t, env, command)
		// No execution: the $(...) must never have run, anywhere we can observe.
		assertNoMarker(t, marker, workdir, installDir, base)
		// Inert data: replaying the single-quoted --include through a shell still
		// succeeds (the value reached the CLI's regexp filter as one literal arg).
		if rcode != 0 {
			t.Fatalf("re-running the payload-bearing --include hook must succeed (the value is data), got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
		}
	})
}
