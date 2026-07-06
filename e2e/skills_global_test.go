//go:build integration

package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PRD #19: `--global` — user-level (~/.claude) hook + custom --path ---
//
// `--global` targets the user-level Claude Code layout instead of the
// project-local one: the SessionStart hook lands in ~/.claude/settings.json and,
// without --path, skills default to ~/.claude/skills (the shared "global
// catalog"). It also lifts the --install-hook/--path conflict so a global hook
// can pair with a custom --path, and it round-trips through the stored hook
// command so every session-start regenerates to the same place.
//
// Every test isolates HOME (HOME=t.TempDir()) so a --global run can never touch
// the developer's real ~/.claude, and asserts the user-level target — never the
// project CWD — receives the hook/skills.

// globalEnv returns the base env for a --global run: an isolated HOME plus a
// cleared skills-filter set so the emitted/stored hook command is deterministic
// (runCLIInDir already strips ambient DOT_AI_* via hermeticEnviron, but a config
// file under a real HOME could still inject filters — the fresh HOME rules that
// out too).
func globalEnv(home string) []string {
	return []string{
		"HOME=" + home,
		"DOT_AI_SKILLS_INCLUDE=",
		"DOT_AI_SKILLS_EXCLUDE=",
		"DOT_AI_SKILLS_CUSTOM_ONLY=",
	}
}

// 1. --install-hook --global writes the SessionStart hook to
// $HOME/.claude/settings.json (NOT the project CWD), generates skills into
// $HOME/.claude/skills, reports the actual settings path, and stores a command
// carrying --global but no --path.
func TestSkillsGenerate_Global_InstallHook_TargetsHomeSettings(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir() // CWD — must NOT receive a .claude/ tree in global mode.

	stdout, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only")
	if code != 0 {
		t.Fatalf("--install-hook --global must succeed, got exit %d; stderr: %s", code, stderr)
	}

	homeSettings := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(homeSettings); err != nil {
		t.Fatalf("expected global hook at %s: %v", homeSettings, err)
	}
	// The project CWD must be untouched — the whole point of --global.
	if _, err := os.Stat(filepath.Join(project, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("project-local .claude/settings.json must NOT be written in global mode (stat err: %v)", err)
	}

	// Success message names the ACTUAL path written (not a hard-coded string).
	if !strings.Contains(stdout, "SessionStart hook installed in "+homeSettings) {
		t.Errorf("expected success message to name %q, got: %s", homeSettings, stdout)
	}

	// Skills default to $HOME/.claude/skills (routing skill always written there).
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "dot-ai", "SKILL.md")); err != nil {
		t.Errorf("expected default skills under $HOME/.claude/skills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills")); !os.IsNotExist(err) {
		t.Errorf("project-local .claude/skills must NOT be written in global mode (stat err: %v)", err)
	}

	// Stored command round-trips --global; a no-path global run stores NO --path.
	command, _ := readHookCommand(t, home)
	if !strings.Contains(command, "--global") {
		t.Errorf("expected stored hook to carry --global, got: %s", command)
	}
	if strings.Contains(command, "--path") {
		t.Errorf("a no-path global run must NOT store --path, got: %s", command)
	}
}

// 2. M1 decision: --global WITHOUT --install-hook is meaningful — it defaults the
// skills output to $HOME/.claude/skills (the "write to the global catalog" mode).
// No hook is installed and no settings.json is written.
func TestSkillsGenerate_Global_NoInstallHook_DefaultsToHomeSkills(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	stdout, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--global", "--custom-only")
	if code != 0 {
		t.Fatalf("--global without --install-hook must succeed, got exit %d; stderr: %s", code, stderr)
	}

	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
		t.Errorf("expected --global (no hook) to write skills into $HOME/.claude/skills: %v", err)
	}
	// No hook: no settings.json anywhere, and no hook message.
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Errorf("--global without --install-hook must NOT write settings.json (stat err: %v)", err)
	}
	if strings.Contains(stdout, "SessionStart hook installed") {
		t.Errorf("--global without --install-hook must not report a hook, got: %s", stdout)
	}
	// Success message names the resolved global dir.
	if !strings.Contains(stdout, filepath.Join(home, ".claude", "skills")) {
		t.Errorf("expected success message to name $HOME/.claude/skills, got: %s", stdout)
	}
}

// 3. --global --path <dir>: the conflict guard is lifted, skills land in <dir>,
// and the stored command carries BOTH --global and --path <dir> (the shell
// expanded any ~ before the CLI saw it, so an absolute path is stored). Replaying
// the stored command under a DIFFERENT HOME still writes to <dir> — proving the
// absolute --path round-trips independently of $HOME.
func TestSkillsGenerate_Global_Path_HonoredAndRoundTripped(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	customDir := t.TempDir() // an absolute path, as the shell would hand the CLI.

	_, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global",
		"--path", customDir, "--custom-only")
	if code != 0 {
		t.Fatalf("--install-hook --global --path must succeed (guard lifted), got exit %d; stderr: %s", code, stderr)
	}

	if _, err := os.Stat(filepath.Join(customDir, "dot-ai", "SKILL.md")); err != nil {
		t.Errorf("expected skills generated into the custom --path %s: %v", customDir, err)
	}
	// Hook still lands in the global settings, not the custom dir.
	if _, err := os.Stat(filepath.Join(home, ".claude", "settings.json")); err != nil {
		t.Errorf("expected global hook at $HOME/.claude/settings.json: %v", err)
	}

	command, _ := readHookCommand(t, home)
	for _, want := range []string{"--global", "--path", customDir} {
		if !strings.Contains(command, want) {
			t.Errorf("expected stored hook to carry %q, got: %s", want, command)
		}
	}

	// Replay under a DIFFERENT HOME: the absolute --path wins, so skills land in
	// customDir regardless of $HOME (and the replay's fresh HOME stays untouched).
	otherHome := t.TempDir()
	_, rout, rerr, rcode := rerunHookCommand(t, globalEnv(otherHome), command)
	if rcode != 0 {
		t.Fatalf("replay of --global --path hook must succeed, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	if _, err := os.Stat(filepath.Join(customDir, "dot-ai", "SKILL.md")); err != nil {
		t.Errorf("replay must regenerate into the round-tripped --path %s: %v", customDir, err)
	}
	if _, err := os.Stat(filepath.Join(otherHome, ".claude", "skills")); !os.IsNotExist(err) {
		t.Errorf("replay of a --path hook must NOT fall back to $HOME/.claude/skills (stat err: %v)", err)
	}
}

// 4. Round-trip form (M3 decision): the stored command embeds --global, NOT the
// resolved ~/.claude/skills path. Replaying under a DIFFERENT HOME therefore
// re-resolves ~/.claude/skills against that new $HOME — host-portable, surviving
// a dotfile sync to a machine with a different home.
func TestSkillsGenerate_Global_RoundTrip_ReResolvesAgainstHome(t *testing.T) {
	installHome := t.TempDir()
	project := t.TempDir()

	_, stderr, code := runCLIInDir(t, project, globalEnv(installHome),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only")
	if code != 0 {
		t.Fatalf("global install must succeed, got exit %d; stderr: %s", code, stderr)
	}
	command, _ := readHookCommand(t, installHome)

	// A different HOME on replay -> skills must land in THAT home's ~/.claude/skills.
	replayHome := t.TempDir()
	_, rout, rerr, rcode := rerunHookCommand(t, globalEnv(replayHome), command)
	if rcode != 0 {
		t.Fatalf("replay must succeed, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	if _, err := os.Stat(filepath.Join(replayHome, ".claude", "skills", "dot-ai", "SKILL.md")); err != nil {
		t.Errorf("replay must re-resolve ~/.claude/skills against the new $HOME (%s): %v", replayHome, err)
	}
}

// 5. --global composes with a source flag (--repo-fetch): the source is
// round-tripped as usual AND the hook lands in $HOME/.claude/settings.json.
// Replaying regenerates the source into $HOME/.claude/skills, tagged with the URL.
func TestSkillsGenerate_Global_ComposesWithRepoFetch(t *testing.T) {
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)

	home := t.TempDir()
	project := t.TempDir()
	env := append(globalEnv(home), "XDG_CACHE_HOME="+t.TempDir())

	_, stderr, code := runCLIInDir(t, project, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only", "--repo-fetch", url)
	if code != 0 {
		t.Fatalf("--global + --repo-fetch must succeed, got exit %d; stderr: %s", code, stderr)
	}

	// Hook lands in the user-level settings, carrying BOTH --global and --repo-fetch.
	homeSettings := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(homeSettings); err != nil {
		t.Fatalf("expected global hook at %s: %v", homeSettings, err)
	}
	command, _ := readHookCommand(t, home)
	for _, want := range []string{"--global", "--repo-fetch", url} {
		if !strings.Contains(command, want) {
			t.Errorf("expected stored hook to carry %q, got: %s", want, command)
		}
	}

	// Replay (same HOME) regenerates the source into $HOME/.claude/skills.
	replayHome := t.TempDir()
	_, rout, rerr, rcode := rerunHookCommand(t, append(globalEnv(replayHome), "XDG_CACHE_HOME="+t.TempDir()), command)
	if rcode != 0 {
		t.Fatalf("replay of --global + --repo-fetch hook must regenerate, got exit %d; stdout: %s stderr: %s", rcode, rout, rerr)
	}
	got := readSkillSource(t, filepath.Join(replayHome, ".claude", "skills", "dot-ai-wip-fetched-skill", "SKILL.md"))
	if got != url {
		t.Errorf("replay expected dot-ai-wip-fetched-skill tagged source %q, got %q", url, got)
	}
}

// 5b. Credential scrubbing under --global (Success Criterion #5 / Verification
// Checklist "credential scrubbing on the stored URL is unchanged"): the compose
// test above uses a bare file:// URL, which has no credential to scrub. Here a
// TOKEN-BEARING --repo-fetch URL is paired WITH --global and we assert the token
// appears NOWHERE in the user-level settings.json and the stored command carries
// the SCRUBBED URL — proving RedactURL still runs on the hook path in global mode
// (mirrors the project-mode TestSkillsGenerate_M5_InstallHook_RepoFetch_CredentialScrubbed).
func TestSkillsGenerate_Global_ComposesWithRepoFetch_CredentialScrubbed(t *testing.T) {
	const token = "global-hook-s3cr3t-xyz"
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	scrubbed := fileURL(repo)
	credURL := "file://user:" + token + "@" + repo

	home := t.TempDir()
	project := t.TempDir()
	env := append(globalEnv(home), "XDG_CACHE_HOME="+t.TempDir())

	_, stderr, code := runCLIInDir(t, project, env,
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only", "--repo-fetch", credURL)
	if code != 0 {
		t.Fatalf("--global + credentialed --repo-fetch must succeed, got exit %d; stderr: %s", code, stderr)
	}

	// The hook lands in the user-level settings (the --global target).
	homeSettings := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(homeSettings); err != nil {
		t.Fatalf("expected global hook at %s: %v", homeSettings, err)
	}

	command, raw := readHookCommand(t, home)
	// The credential must never reach the global settings.json, in any form.
	if strings.Contains(raw, token) {
		t.Errorf("credential token leaked into global settings.json: %s", raw)
	}
	if strings.Contains(raw, "user:"+token) {
		t.Errorf("raw userinfo leaked into global settings.json: %s", raw)
	}
	// The stored command carries the SCRUBBED URL, identical to the no-cred form,
	// alongside --global.
	if !strings.Contains(command, "--global") {
		t.Errorf("expected stored hook to carry --global, got: %s", command)
	}
	if !strings.Contains(command, scrubbed) {
		t.Errorf("expected stored hook to carry the scrubbed URL %q, got: %s", scrubbed, command)
	}
}

// 6. Installing a global hook PRESERVES unrelated content already in
// $HOME/.claude/settings.json (the merge only touches dot-ai SessionStart entries).
func TestSkillsGenerate_Global_PreservesUnrelatedSettings(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("mkdir ~/.claude: %v", err)
	}

	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git status:*)"},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": ".*",
					"hooks": []any{
						map[string]any{"type": "command", "command": "echo pre-tool"},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	_, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only")
	if code != 0 {
		t.Fatalf("global install must succeed, got exit %d; stderr: %s", code, stderr)
	}

	raw, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read merged settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("merged settings not valid JSON: %v", err)
	}
	if settings["permissions"] == nil {
		t.Error("expected unrelated 'permissions' to be preserved")
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks["PreToolUse"] == nil {
		t.Error("expected unrelated PreToolUse hook to be preserved")
	}
	sessionStart, _ := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Errorf("expected exactly 1 SessionStart entry, got %d", len(sessionStart))
	}
}

// 7. Re-running a global install is idempotent — exactly one dot-ai SessionStart
// entry in $HOME/.claude/settings.json after two runs.
func TestSkillsGenerate_Global_InstallHook_Idempotent(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	for i := 0; i < 2; i++ {
		_, stderr, code := runCLIInDir(t, project, globalEnv(home),
			"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--custom-only")
		if code != 0 {
			t.Fatalf("run %d: expected exit 0; stderr: %s", i+1, stderr)
		}
	}

	// readHookCommand fatals unless there is EXACTLY one SessionStart entry.
	readHookCommand(t, home)
}

// 8. The --install-hook/--path conflict guard is lifted ONLY with --global:
// without it the combination still errors; with it, it succeeds.
func TestSkillsGenerate_Global_LiftsPathGuardOnlyWhenSet(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	customDir := t.TempDir()

	// Without --global: the guard still rejects --install-hook + --path.
	_, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--path", customDir, "--custom-only")
	if code == 0 {
		t.Fatalf("--install-hook --path WITHOUT --global must still error")
	}
	if !strings.Contains(stderr, "--install-hook cannot be used with --path") {
		t.Errorf("expected the project-mode conflict error, got: %s", stderr)
	}

	// With --global: the same combination succeeds.
	_, stderr, code = runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--global", "--path", customDir, "--custom-only")
	if code != 0 {
		t.Fatalf("--install-hook --global --path must succeed (guard lifted), got exit %d; stderr: %s", code, stderr)
	}
}

// 9. --global requires --agent claude-code (the ~/.claude layout is Claude Code's
// home; a cursor/windsurf --global would be a silent no-op).
func TestSkillsGenerate_Global_RequiresClaudeCode(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	_, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "cursor", "--global")
	if code == 0 {
		t.Fatalf("--global with a non-claude-code agent must error")
	}
	if !strings.Contains(stderr, "--global requires --agent claude-code") {
		t.Errorf("expected the --global agent-requirement error, got: %s", stderr)
	}
}

// 10. Backward compatibility: a project-mode run (no --global) is unaffected by a
// set HOME — the hook lands in the project CWD's .claude/settings.json with the
// byte-identical base command, and skills land in the project's .claude/skills.
// $HOME/.claude is never touched.
func TestSkillsGenerate_Global_ProjectModeUnaffected(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()

	_, stderr, code := runCLIInDir(t, project, globalEnv(home),
		"skills", "generate", "--agent", "claude-code", "--install-hook")
	if code != 0 {
		t.Fatalf("project-mode install must succeed, got exit %d; stderr: %s", code, stderr)
	}

	// Hook + skills in the project CWD; $HOME/.claude untouched.
	command, _ := readHookCommand(t, project)
	if command != "dot-ai skills generate --agent claude-code" {
		t.Errorf("project-mode command must be byte-identical to the base, got: %q", command)
	}
	if strings.Contains(command, "--global") || strings.Contains(command, "--path") {
		t.Errorf("project-mode command must carry neither --global nor --path, got: %q", command)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "dot-ai", "SKILL.md")); err != nil {
		t.Errorf("expected project-local skills under ./.claude/skills: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("project mode must not touch $HOME/.claude (stat err: %v)", err)
	}
}

// 11. --global appears in help output.
func TestSkillsGenerate_Global_InHelp(t *testing.T) {
	stdout, _, code := runCLI(t, "skills", "generate", "--help")
	if code != 0 {
		t.Fatalf("help must exit 0, got %d", code)
	}
	if !strings.Contains(stdout, "--global") {
		t.Errorf("expected help to mention --global, got: %s", stdout)
	}
}
