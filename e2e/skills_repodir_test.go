//go:build integration

package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- PRD #13 M2: --repo-dir local source end-to-end ---
//
// These exercise the real --repo-dir path against the pinned mock (:3001), which
// now exposes the ingestion endpoint (POST /api/v1/prompts/sources) and ?source=
// render. The CLI reads a local directory, uploads it, and drives list+render
// through ?source=<identifier> — no git, no clone.
//
// IMPORTANT: the security posture refuses a --repo-dir under /tmp, so the SUCCESS
// tests must NOT use t.TempDir() for the source (it is /tmp-based). repoDirSource
// creates the source under the test's working directory (the e2e/ package dir),
// which is neither /tmp nor world-writable. The output dir (--path) is
// unrestricted, so t.TempDir() is fine there.

// repoDirSource creates a source directory (NOT under /tmp) populated with the
// given path->content files and returns its path. Cleaned up after the test.
func repoDirSource(t *testing.T, files map[string]string) string {
	t.Helper()
	base, err := os.Getwd() // the e2e/ package dir: writable, not /tmp
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir, err := os.MkdirTemp(base, "m2src-")
	if err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

// argTakingPromptFile is an argument-taking prompt named after a built-in prompt
// (troubleshoot-pod) so it appears in the mock's list response, with a
// distinctive body marker that only the UPLOADED source carries. podName is
// optional so the generator's empty-body render succeeds (the generator renders
// each prompt without arguments at generate time).
const argTakingPromptFile = `---
name: troubleshoot-pod
description: Custom troubleshoot from the uploaded local source
arguments:
  - name: podName
    required: false
---
UPLOADED-LOCAL-MARKER troubleshoot {{podName}} via repo-dir.`

// 1. Opt-in gate: without DOT_AI_ALLOW_REPO_DIR=1, --repo-dir is refused with a
// clear, non-zero-exit error naming the env var.
func TestSkillsGenerate_RepoDir_RequiresOptIn(t *testing.T) {
	src := repoDirSource(t, map[string]string{"troubleshoot-pod/SKILL.md": argTakingPromptFile})
	out := t.TempDir()
	// USER is set, but the opt-in gate must fire first regardless.
	stdout, stderr, code := runCLIWithEnv(t, []string{"USER=tester"},
		"skills", "generate", "--path", out, "--repo-dir", src, "--source-label", "foo")
	if code == 0 {
		t.Fatalf("expected non-zero exit without DOT_AI_ALLOW_REPO_DIR=1; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "DOT_AI_ALLOW_REPO_DIR") {
		t.Errorf("expected opt-in error naming DOT_AI_ALLOW_REPO_DIR, got: %s", combined)
	}
	// Nothing should have been generated.
	if _, err := os.Stat(filepath.Join(out, "dot-ai-troubleshoot-pod")); !os.IsNotExist(err) {
		t.Errorf("expected no skills generated when --repo-dir is refused")
	}
}

// 2. End-to-end success: with the opt-in set and a non-/tmp source, the run reads
// the local dir (zero git/network for the fetch), uploads it, and tags every
// generated skill with the auto-prefixed identifier local:<user>-<label>.
func TestSkillsGenerate_RepoDir_EndToEnd_SourceFrontmatter(t *testing.T) {
	src := repoDirSource(t, map[string]string{"troubleshoot-pod/SKILL.md": argTakingPromptFile})
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Uploaded local source as local:tester-foo") {
		t.Errorf("expected upload confirmation for local:tester-foo, got: %s", stdout)
	}
	// Every generated prompt skill is tagged with the CLI-computed identifier,
	// NOT the server-echoed list source ("built-in").
	got := readSkillSource(t, filepath.Join(out, "dot-ai-troubleshoot-pod", "SKILL.md"))
	if got != "local:tester-foo" {
		t.Errorf("expected source frontmatter local:tester-foo, got %q", got)
	}
}

// 3. LOAD-BEARING ?source= check: an argument-taking skill loaded via --repo-dir
// renders through the server's ingested-source path. The generated body must come
// from the UPLOADED source (the marker), proving the render carried ?source= and
// resolved the uploaded source — not the server's built-in default. The {{podName}}
// template survives, so the skill is genuinely argument-taking (the server does
// full substitution when later invoked with arguments — see the mock contract).
func TestSkillsGenerate_RepoDir_ArgTakingSkill_RendersViaSource(t *testing.T) {
	src := repoDirSource(t, map[string]string{"troubleshoot-pod/SKILL.md": argTakingPromptFile})
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--include", "troubleshoot-pod",
		"--repo-dir", src, "--source-label", "foo")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}

	content, err := os.ReadFile(filepath.Join(out, "dot-ai-troubleshoot-pod", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected dot-ai-troubleshoot-pod generated: %v", err)
	}
	s := string(content)
	// The body came from the uploaded source via ?source= (load-bearing proof).
	if !strings.Contains(s, "UPLOADED-LOCAL-MARKER") {
		t.Errorf("expected the uploaded source body (proves ?source= render), got:\n%s", s)
	}
	// The argument template is preserved — this is an argument-taking skill.
	if !strings.Contains(s, "{{podName}}") {
		t.Errorf("expected the {{podName}} argument template in the rendered skill, got:\n%s", s)
	}
	// It must NOT be the server's built-in fixture render (that body mentions the
	// fixture pod name); that would mean ?source= was ignored and a clone/default
	// served instead.
	if strings.Contains(s, "nginx-deployment-7d9c67b5f-abc12") {
		t.Errorf("rendered the built-in default source, not the uploaded one (?source= not honored):\n%s", s)
	}
	// And it carries the source identifier.
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-troubleshoot-pod", "SKILL.md")); got != "local:tester-foo" {
		t.Errorf("expected source frontmatter local:tester-foo, got %q", got)
	}
}

// newWipSkillFile is a brand-new prompt that exists ONLY in the uploaded local
// source — its name does NOT collide with any server built-in. This is the real
// "author a WIP skill locally" case the --repo-dir feature targets.
const newWipSkillFile = `---
name: wip-new-skill
description: A brand-new skill authored only in the local source
---
WIP-NEW-SKILL-MARKER body for an entirely new skill.`

// 3b. PENDING (list-by-source gap): a --repo-dir source whose prompt name does
// NOT collide with a server built-in. This is the genuine new-WIP-skill case:
// the skill exists ONLY in the uploaded source, so it can appear ONLY if the
// server honors GET /api/v1/prompts?source= for LISTING (returns the uploaded
// source's prompts, not the built-ins). The currently pinned mock ignores
// ?source= on the list call, so the new prompt is never listed and no skill is
// generated — hence this is skipped, not faked. It becomes a live test the
// moment the republished mock honors ?source=. (The render path is already
// covered today by TestSkillsGenerate_RepoDir_ArgTakingSkill_RendersViaSource.)
func TestSkillsGenerate_RepoDir_NonBuiltinPrompt_PendingListBySource(t *testing.T) {
	t.Skip("pending server list-by-source endpoint — requested from PRD #647; un-skip when the mock is republished to honor GET /api/v1/prompts?source=")

	src := repoDirSource(t, map[string]string{"wip-new-skill/SKILL.md": newWipSkillFile})
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}
	// The uploaded WIP prompt becomes a skill named after itself — proof the list
	// came from the uploaded source, not the server built-ins.
	content, err := os.ReadFile(filepath.Join(out, "dot-ai-wip-new-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected dot-ai-wip-new-skill generated from the uploaded source: %v", err)
	}
	if !strings.Contains(string(content), "WIP-NEW-SKILL-MARKER") {
		t.Errorf("expected the uploaded WIP skill body, got:\n%s", string(content))
	}
	if got := readSkillSource(t, filepath.Join(out, "dot-ai-wip-new-skill", "SKILL.md")); got != "local:tester-foo" {
		t.Errorf("expected source frontmatter local:tester-foo, got %q", got)
	}
}

// 4. Security: a --repo-dir under /tmp is refused even with the opt-in set —
// shared, world-writable temp space is a side-loading vector. t.TempDir() is
// /tmp-based here deliberately to exercise the rule.
func TestSkillsGenerate_RepoDir_RefusesTmpPath(t *testing.T) {
	src := t.TempDir() // /tmp-based
	if err := os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code == 0 {
		t.Fatalf("expected non-zero exit for a /tmp --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "temp") {
		t.Errorf("expected a temp-dir refusal, got: %s", combined)
	}
}

// 5. Security: a world-writable --repo-dir is refused even outside /tmp.
func TestSkillsGenerate_RepoDir_RefusesWorldWritable(t *testing.T) {
	src := repoDirSource(t, map[string]string{"SKILL.md": "x"})
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code == 0 {
		t.Fatalf("expected non-zero exit for a world-writable --repo-dir; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "world-writable") {
		t.Errorf("expected a world-writable refusal, got: %s", combined)
	}
}

// 6. Limits: a source exceeding the 100-file ceiling fails with a clear CLI error
// (a pre-check, before any upload — so it does not depend on the server 413).
func TestSkillsGenerate_RepoDir_TooManyFiles(t *testing.T) {
	files := map[string]string{}
	for i := 0; i < 101; i++ {
		files["f"+itoa(i)+".md"] = "x"
	}
	src := repoDirSource(t, files)
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t, []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code == 0 {
		t.Fatalf("expected non-zero exit for >100 files; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "100-file limit") {
		t.Errorf("expected a file-count limit error, got: %s", combined)
	}
}

// 7. Optional allowlist: when DOT_AI_REPO_DIR_ALLOW is set, a path outside every
// listed base directory is refused.
func TestSkillsGenerate_RepoDir_AllowlistRefusal(t *testing.T) {
	src := repoDirSource(t, map[string]string{"SKILL.md": "x"})
	out := t.TempDir()
	stdout, stderr, code := runCLIWithEnv(t,
		[]string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "DOT_AI_REPO_DIR_ALLOW=/opt/skills:/srv/skills"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code == 0 {
		t.Fatalf("expected non-zero exit for a path outside the allowlist; stdout: %s stderr: %s", stdout, stderr)
	}
	combined := stdout + stderr
	if !strings.Contains(combined, "DOT_AI_REPO_DIR_ALLOW") {
		t.Errorf("expected an allowlist refusal naming DOT_AI_REPO_DIR_ALLOW, got: %s", combined)
	}
}

// 8. Wire format: the upload body and the ?source= plumbing, verified against a
// capturing backend the stateless mock cannot expose. The upload carries the
// identifier + a sha256 contentHash + a files array; the list and EACH render
// carry ?source=<identifier> and NEVER ?repo=.
func TestSkillsGenerate_RepoDir_WireFormat_SourceParamNotRepo(t *testing.T) {
	cs := newRepoDirCaptureServer(t)
	src := repoDirSource(t, map[string]string{"p1/SKILL.md": "---\nname: p1\n---\nbody"})
	out := t.TempDir()

	const identifier = "local:tester-foo"
	stdout, stderr, code := runCLIAtServer(t, cs.URL,
		[]string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester"},
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s stderr: %s", code, stdout, stderr)
	}

	reqs := cs.snapshot()

	// The upload happened with the correct nested JSON body.
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
	if body.Source != identifier {
		t.Errorf("expected upload source %q, got %q", identifier, body.Source)
	}
	if !strings.HasPrefix(body.ContentHash, "sha256:") {
		t.Errorf("expected a sha256: contentHash, got %q", body.ContentHash)
	}
	if len(body.Files) != 1 || body.Files[0].Path != "p1/SKILL.md" || body.Files[0].Content == "" {
		t.Errorf("expected one file p1/SKILL.md with base64 content, got %+v", body.Files)
	}

	// The list + render carry ?source= and never ?repo=.
	list := findRequest(t, reqs, http.MethodGet, "/api/v1/prompts")
	render := findRequest(t, reqs, http.MethodPost, "/api/v1/prompts/p1")
	for _, r := range []capturedRequest{list, render} {
		if got := r.Query["source"]; len(got) != 1 || got[0] != identifier {
			t.Errorf("%s %s: expected ?source=%q, got %v", r.Method, r.Path, identifier, got)
		}
		if _, ok := r.Query["repo"]; ok {
			t.Errorf("%s %s: --repo-dir run must never send ?repo=, got %v", r.Method, r.Path, r.Query["repo"])
		}
	}
}

// newRepoDirCaptureServer records every request and returns enough fixture JSON
// for a --repo-dir generate run to complete, including the ingestion endpoint.
func newRepoDirCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	cs := &captureServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		cs.mu.Lock()
		cs.requests = append(cs.requests, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Body:   string(bodyBytes),
		})
		cs.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/prompts/sources":
			io.WriteString(w, `{"success":true,"data":{"source":`+jsonString(r.URL.Query().Get("source"))+`,"contentHash":"sha256:x","fileCount":1,"status":"ingested"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/prompts":
			io.WriteString(w, `{"success":true,"data":{"prompts":[{"name":"p1","description":"p1 desc"}],"source":`+jsonString(r.URL.Query().Get("source"))+`}}`)
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/prompts/"):
			io.WriteString(w, `{"success":true,"data":{"description":"p1 desc","messages":[{"role":"user","content":{"type":"text","text":"body of p1"}}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"success":false,"error":{"code":"NOT_FOUND","message":"no route"}}`)
		}
	}))
	t.Cleanup(cs.Close)
	return cs
}

// itoa is a tiny strconv.Itoa stand-in to avoid an extra import for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
