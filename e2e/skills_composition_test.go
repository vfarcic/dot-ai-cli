//go:build integration

package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PRD #13 M5 Part A: composition across ALL FOUR source types ---
//
// One output dir is populated by four distinct sources, each with its own
// `source:` identifier, then the source-tagged wipe-own-slice and
// first-source-wins mechanics (PRD #12, uniform across sources) are proven to
// hold for the new --repo-fetch / --repo-dir source types alongside the existing
// env-var/default and --repo ones:
//
//   - env-var/default (no source flag) -> source "built-in"
//   - --repo <url>                     -> source <url>            (server "clone")
//   - --repo-fetch file://<repo>       -> source <scrubbed-url>   (CLI clone+upload)
//   - --repo-dir <dir> --source-label  -> source local:<user>-<label> (CLI upload)
//
// To give env-var and --repo (which both return the SAME built-in prompt set
// from the mock) distinct, non-clobbering skills, each is scoped with --include
// to a different built-in prompt. --repo-fetch and --repo-dir each carry a
// brand-new prompt that exists ONLY in their uploaded source. XDG_CACHE_HOME is a
// fresh temp dir so the clone cache and upload-state store are isolated.

// assertSkillSource reads dot-ai-<name>/SKILL.md under out and asserts its
// source: frontmatter equals want.
func assertSkillSource(t *testing.T, out, name, want string) {
	t.Helper()
	got := readSkillSource(t, filepath.Join(out, "dot-ai-"+name, "SKILL.md"))
	if got != want {
		t.Errorf("skill %q: expected source %q, got %q", name, want, got)
	}
}

// readSkillFull returns the verbatim bytes of dot-ai-<name>/SKILL.md so an
// untouched-by-another-source assertion can compare byte-for-byte.
func readSkillFull(t *testing.T, out, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "dot-ai-"+name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill %q: %v", name, err)
	}
	return string(data)
}

func TestSkillsGenerate_M5_Composition_AllFourSources(t *testing.T) {
	out := t.TempDir()
	xdg := t.TempDir()
	// Shared across every run: opt-in + identity for --repo-dir, isolated cache.
	env := []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + xdg}

	// Source identifiers, one per source type — all distinct.
	const (
		builtinID = "built-in"
		repoURL   = "https://github.com/orgA/compose-skills"
		repoDirID = "local:tester-compose"
	)
	repoF := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	fetchURL := fileURL(repoF) // RedactURL is identity here (no embedded credential)
	repoDirSrc := repoDirSource(t, map[string]string{"wip-new-skill/SKILL.md": newWipSkillFile})

	// --- Phase 1: compose all four into one output dir ---

	// 1) env-var/default, scoped to one built-in prompt.
	if _, stderr, code := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--include", "^troubleshoot-pod$"); code != 0 {
		t.Fatalf("env-var/default run: expected exit 0, got %d; stderr: %s", code, stderr)
	}
	// 2) --repo, scoped to a DIFFERENT built-in prompt (no clobber of the above).
	if _, stderr, code := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--include", "^explain-resource$", "--repo", repoURL); code != 0 {
		t.Fatalf("--repo run: expected exit 0, got %d; stderr: %s", code, stderr)
	}
	// 3) --repo-fetch: clones a local file:// repo carrying a brand-new prompt.
	if _, stderr, code := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--repo-fetch", fetchURL); code != 0 {
		t.Fatalf("--repo-fetch run: expected exit 0, got %d; stderr: %s", code, stderr)
	}
	// 4) --repo-dir: uploads a local dir carrying a brand-new prompt.
	if _, stderr, code := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--repo-dir", repoDirSrc, "--source-label", "compose"); code != 0 {
		t.Fatalf("--repo-dir run: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	// Every source's skill is present, tagged with its OWN distinct identifier —
	// no clobbering across sources.
	assertSkillSource(t, out, "troubleshoot-pod", builtinID)
	assertSkillSource(t, out, "explain-resource", repoURL)
	assertSkillSource(t, out, "wip-fetched-skill", fetchURL)
	assertSkillSource(t, out, "wip-new-skill", repoDirID)

	// --- Phase 2: cross-source collision is first-source-wins (skip + warn) ---
	// Re-run --repo, now also requesting troubleshoot-pod, which the env-var/default
	// source already owns (source "built-in"). It must be skipped with a warning and
	// must NOT be re-tagged to repoURL; explain-resource (this source's own) is
	// regenerated; the other sources are untouched.
	_, stderr2, code2 := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--include", "^(troubleshoot-pod|explain-resource)$", "--repo", repoURL)
	if code2 != 0 {
		t.Fatalf("collision re-run: expected exit 0, got %d; stderr: %s", code2, stderr2)
	}
	for _, want := range []string{"first-source-wins", builtinID, "troubleshoot-pod"} {
		if !strings.Contains(stderr2, want) {
			t.Errorf("expected collision warning to contain %q; got stderr:\n%s", want, stderr2)
		}
	}
	// First-source-wins: the colliding skill keeps the FIRST source.
	assertSkillSource(t, out, "troubleshoot-pod", builtinID)
	assertSkillSource(t, out, "explain-resource", repoURL)
	assertSkillSource(t, out, "wip-fetched-skill", fetchURL)
	assertSkillSource(t, out, "wip-new-skill", repoDirID)

	// --- Phase 3: re-running ONE source wipes ONLY its own slice ---
	// Pre-seed a stale skill tagged with the --repo-dir source, snapshot the OTHER
	// sources' skills, then re-run --repo-dir. The stale same-source skill must be
	// wiped, the source's real skill regenerated, and every other source left
	// byte-for-byte intact.
	staleDir := filepath.Join(out, "dot-ai-stale-compose")
	if err := os.MkdirAll(staleDir, 0o755); err != nil {
		t.Fatalf("mkdir stale: %v", err)
	}
	staleContent := "---\nname: dot-ai-stale-compose\ndescription: stale\nuser-invocable: true\nsource: \"" + repoDirID + "\"\n---\n\nstale body\n"
	if err := os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte(staleContent), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	before := map[string]string{
		"troubleshoot-pod":  readSkillFull(t, out, "troubleshoot-pod"),
		"explain-resource":  readSkillFull(t, out, "explain-resource"),
		"wip-fetched-skill": readSkillFull(t, out, "wip-fetched-skill"),
	}

	if _, stderr3, code3 := runCLIWithEnv(t, env, "skills", "generate", "--path", out,
		"--custom-only", "--repo-dir", repoDirSrc, "--source-label", "compose"); code3 != 0 {
		t.Fatalf("--repo-dir wipe-own-slice re-run: expected exit 0, got %d; stderr: %s", code3, stderr3)
	}

	// The stale same-source skill is gone (own slice wiped)...
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("expected the stale --repo-dir skill to be wiped on re-run, but it still exists")
	}
	// ...the source's real skill is regenerated and still correctly tagged...
	assertSkillSource(t, out, "wip-new-skill", repoDirID)
	// ...and every OTHER source's skill is byte-for-byte untouched.
	for name, want := range before {
		if got := readSkillFull(t, out, name); got != want {
			t.Errorf("skill %q from another source was modified by the --repo-dir re-run\nwant:\n%s\ngot:\n%s", name, want, got)
		}
	}
}
