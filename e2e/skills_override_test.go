//go:build integration

package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- PRD #16: per-request path, branch, and credential on the prompts override ---
//
// These tests split across two backends:
//
//   - The pinned mock at :3001 (via runCLI) verifies the contract behaviorally:
//     path+branch resolve the `skill-on-branch` marker, `source` stability,
//     request-scoped 400s, and no secret leakage. These would fail against an
//     older mock that ignores the new params.
//   - A capturing httptest backend (captureServer) verifies wire-format details
//     the stateless mock cannot expose: that repo/path/branch land in the query
//     (list + each render) vs the body (refresh), and that the credential
//     travels ONLY as the X-Dot-AI-Git-Token header (never query/body) and is
//     gated on --repo + DOT_AI_GIT_TOKEN. The real CLI binary still drives it
//     over HTTP via --server-url, so these remain binary-subprocess tests.

const gitTokenHeaderName = "X-Dot-AI-Git-Token"

// capturedRequest records what the capturing backend received for one request.
type capturedRequest struct {
	Method string
	Path   string
	Query  map[string][]string
	Token  string // X-Dot-AI-Git-Token header value ("" if absent)
	Body   string
}

// captureServer is a minimal stand-in for the dot-ai server that records every
// request and returns just enough fixture JSON for a generate run to complete.
type captureServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

func newCaptureServer(t *testing.T) *captureServer {
	t.Helper()
	cs := &captureServer{}
	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		cs.mu.Lock()
		cs.requests = append(cs.requests, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Token:  r.Header.Get(gitTokenHeaderName),
			Body:   string(bodyBytes),
		})
		cs.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tools":
			io.WriteString(w, `{"success":true,"data":{"tools":[]}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/prompts":
			// Echo the override repo back as the source, like the real server.
			io.WriteString(w, `{"success":true,"data":{"prompts":[{"name":"p1","description":"p1 desc"}],"source":`+jsonString(r.URL.Query().Get("repo"))+`}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/prompts/refresh":
			io.WriteString(w, `{"success":true,"data":{"refreshed":true,"promptsLoaded":1,"source":"x"}}`)
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

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (cs *captureServer) snapshot() []capturedRequest {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]capturedRequest, len(cs.requests))
	copy(out, cs.requests)
	return out
}

// runCLIAtServer runs the CLI binary against an arbitrary server URL with a
// controlled environment. Any ambient DOT_AI_GIT_TOKEN is stripped first so
// token-gating assertions are deterministic, then extraEnv is layered on top.
func runCLIAtServer(t *testing.T, serverURL string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	fullArgs := append([]string{"--server-url", serverURL}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "DOT_AI_GIT_TOKEN=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, extraEnv...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// findRequest returns the first captured request matching method+path.
func findRequest(t *testing.T, reqs []capturedRequest, method, path string) capturedRequest {
	t.Helper()
	for _, r := range reqs {
		if r.Method == method && r.Path == path {
			return r
		}
	}
	t.Fatalf("no %s %s request captured; got: %+v", method, path, reqs)
	return capturedRequest{}
}

// Verification: param mapping on every request + token transport via header.
// With --repo/--repo-path/--repo-branch and DOT_AI_GIT_TOKEN set, both the list
// call and each render call must carry repo/path/branch in the query AND the
// X-Dot-AI-Git-Token header, and the token must never appear in query or body.
func TestSkillsGenerate_Override_ParamsAndTokenOnEveryRequest(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	const (
		repo   = "https://github.com/orgA/skills"
		path   = "skills"
		branch = "team-skills"
		token  = "secret-tok-abc123"
	)
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, []string{"DOT_AI_GIT_TOKEN=" + token},
		"skills", "generate", "--path", dir, "--custom-only",
		"--repo", repo, "--repo-path", path, "--repo-branch", branch)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	reqs := cs.snapshot()
	list := findRequest(t, reqs, http.MethodGet, "/api/v1/prompts")
	render := findRequest(t, reqs, http.MethodPost, "/api/v1/prompts/p1")

	for _, r := range []capturedRequest{list, render} {
		if got := r.Query["repo"]; len(got) != 1 || got[0] != repo {
			t.Errorf("%s %s: expected ?repo=%q, got %v", r.Method, r.Path, repo, got)
		}
		if got := r.Query["path"]; len(got) != 1 || got[0] != path {
			t.Errorf("%s %s: expected ?path=%q, got %v", r.Method, r.Path, path, got)
		}
		if got := r.Query["branch"]; len(got) != 1 || got[0] != branch {
			t.Errorf("%s %s: expected ?branch=%q, got %v", r.Method, r.Path, branch, got)
		}
		if r.Token != token {
			t.Errorf("%s %s: expected X-Dot-AI-Git-Token header %q, got %q", r.Method, r.Path, token, r.Token)
		}
	}

	// The credential must NEVER leak into the query or body of any request.
	for _, r := range reqs {
		for k, vals := range r.Query {
			for _, v := range vals {
				if strings.Contains(v, token) {
					t.Errorf("token leaked into query %s=%q on %s %s", k, v, r.Method, r.Path)
				}
			}
		}
		if strings.Contains(r.Body, token) {
			t.Errorf("token leaked into body on %s %s: %s", r.Method, r.Path, r.Body)
		}
	}
}

// Verification: token gating — DOT_AI_GIT_TOKEN unset => no header is sent,
// even though the override params are still present.
func TestSkillsGenerate_Override_TokenUnset_NoHeader(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	const repo = "https://github.com/orgA/skills"
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, nil,
		"skills", "generate", "--path", dir, "--custom-only",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	reqs := cs.snapshot()
	if len(reqs) == 0 {
		t.Fatal("expected at least one captured request")
	}
	for _, r := range reqs {
		if r.Token != "" {
			t.Errorf("expected no X-Dot-AI-Git-Token header when DOT_AI_GIT_TOKEN unset; got %q on %s %s", r.Token, r.Method, r.Path)
		}
	}
	// Override params still flow even without a credential.
	list := findRequest(t, reqs, http.MethodGet, "/api/v1/prompts")
	if got := list.Query["repo"]; len(got) != 1 || got[0] != repo {
		t.Errorf("expected ?repo=%q on list, got %v", repo, got)
	}
}

// Verification: token gating — DOT_AI_GIT_TOKEN set but NO --repo => the header
// is inert server-side, so the CLI must not send it (nor any override params)
// on the non-override requests.
func TestSkillsGenerate_NoRepo_TokenSet_NoHeaderNoParams(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, []string{"DOT_AI_GIT_TOKEN=secret-should-not-be-sent"},
		"skills", "generate", "--path", dir, "--custom-only")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	reqs := cs.snapshot()
	if len(reqs) == 0 {
		t.Fatal("expected at least one captured request")
	}
	for _, r := range reqs {
		if r.Token != "" {
			t.Errorf("expected no token header on non-override run; got %q on %s %s", r.Token, r.Method, r.Path)
		}
		for _, key := range []string{"repo", "path", "branch"} {
			if vals, ok := r.Query[key]; ok {
				t.Errorf("expected no ?%s on non-override run; got %v on %s %s", key, vals, r.Method, r.Path)
			}
		}
		if strings.Contains(r.Body, "secret-should-not-be-sent") {
			t.Errorf("token leaked into body on %s %s: %s", r.Method, r.Path, r.Body)
		}
	}
}

// C3 + Verification: refresh override travels as JSON BODY fields (not query),
// with the credential as the header.
func TestSkillsGenerate_PullLatest_Refresh_OverrideInBody(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	const (
		repo   = "https://github.com/orgA/skills"
		path   = "skills"
		branch = "team-skills"
		token  = "secret-tok-refresh"
	)
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, []string{"DOT_AI_GIT_TOKEN=" + token},
		"skills", "generate", "--path", dir, "--custom-only", "--pull-latest",
		"--repo", repo, "--repo-path", path, "--repo-branch", branch)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	refresh := findRequest(t, cs.snapshot(), http.MethodPost, "/api/v1/prompts/refresh")

	var body map[string]string
	if err := json.Unmarshal([]byte(refresh.Body), &body); err != nil {
		t.Fatalf("refresh body is not a JSON object: %v; raw: %s", err, refresh.Body)
	}
	if body["repo"] != repo {
		t.Errorf("expected refresh body repo=%q, got %q", repo, body["repo"])
	}
	if body["path"] != path {
		t.Errorf("expected refresh body path=%q, got %q", path, body["path"])
	}
	if body["branch"] != branch {
		t.Errorf("expected refresh body branch=%q, got %q", branch, body["branch"])
	}
	// repo/path/branch must NOT be in the query for refresh.
	for _, key := range []string{"repo", "path", "branch"} {
		if vals, ok := refresh.Query[key]; ok {
			t.Errorf("expected refresh override in body not query; found ?%s=%v", key, vals)
		}
	}
	// Credential travels as the header, never in the body.
	if refresh.Token != token {
		t.Errorf("expected refresh X-Dot-AI-Git-Token header %q, got %q", token, refresh.Token)
	}
	if strings.Contains(refresh.Body, token) {
		t.Errorf("token leaked into refresh body: %s", refresh.Body)
	}
}

// Backward compat: a --repo-only run must produce exactly the #15 wire format —
// a single ?repo= query param, no path/branch, and (with no token) no header.
func TestSkillsGenerate_RepoOnly_WireFormatUnchanged(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	const repo = "https://github.com/orgA/skills"
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, nil,
		"skills", "generate", "--path", dir, "--custom-only", "--repo", repo)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	for _, r := range cs.snapshot() {
		if r.Path == "/api/v1/prompts" || strings.HasPrefix(r.Path, "/api/v1/prompts/") {
			if got := r.Query["repo"]; len(got) != 1 || got[0] != repo {
				t.Errorf("%s %s: expected ?repo=%q, got %v", r.Method, r.Path, repo, got)
			}
			if _, ok := r.Query["path"]; ok {
				t.Errorf("%s %s: repo-only run must not send ?path=", r.Method, r.Path)
			}
			if _, ok := r.Query["branch"]; ok {
				t.Errorf("%s %s: repo-only run must not send ?branch=", r.Method, r.Path)
			}
			if r.Token != "" {
				t.Errorf("%s %s: repo-only run with no token must send no header, got %q", r.Method, r.Path, r.Token)
			}
		}
	}
}

// --- Behavioral tests against the pinned mock (:3001) ---

const branchMarkerText = "This prompt resolves ONLY when both"

// Verification: path + branch actually resolve. With BOTH flags the mock
// surfaces the `skill-on-branch` marker, and its rendered body proves the
// render (POST) call — not just the list call — carried path+branch.
func TestSkillsGenerate_PathBranch_ResolvesSkillOnBranch(t *testing.T) {
	dir := t.TempDir()
	repo := "https://github.com/orgA/skills"
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--custom-only",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	content, err := os.ReadFile(filepath.Join(dir, "dot-ai-skill-on-branch", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected dot-ai-skill-on-branch to be generated (path+branch resolved): %v", err)
	}
	if !strings.Contains(string(content), branchMarkerText) {
		t.Errorf("expected rendered branch-marker body (proves render carried path+branch), got:\n%s", content)
	}

	// The default prompt set must NOT appear — the override resolved the
	// distinct branch set, not the root/main set.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("expected default prompts to be absent when path+branch resolve skill-on-branch")
	}
}

// Verification: path or branch ALONE (with a repo) does not reach the branch
// set — the mock returns the default prompts, no skill-on-branch, no 400.
func TestSkillsGenerate_OnlyPathOrOnlyBranch_NoSkillOnBranch(t *testing.T) {
	repo := "https://github.com/orgA/skills"
	cases := []struct {
		name string
		args []string
	}{
		{"only-path", []string{"--repo", repo, "--repo-path", "skills"}},
		{"only-branch", []string{"--repo", repo, "--repo-branch", "team-skills"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir, "--custom-only"}, tc.args...)
			_, stderr, exitCode := runCLI(t, args...)
			if exitCode != 0 {
				t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
			}
			if _, err := os.Stat(filepath.Join(dir, "dot-ai-skill-on-branch", "SKILL.md")); !os.IsNotExist(err) {
				t.Errorf("expected skill-on-branch to be ABSENT with only one of path/branch")
			}
			if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
				t.Errorf("expected the default prompt set when only one of path/branch is supplied: %v", err)
			}
		})
	}
}

// Verification: `source` stability. For a given repo, `source` is identical
// with and without path/branch/token, and skills tag off it unchanged.
func TestSkillsGenerate_SourceStability_AcrossPathBranchToken(t *testing.T) {
	repo := "https://github.com/orgA/skills"

	// Run 1: repo only.
	dir1 := t.TempDir()
	out1, stderr1, code1 := runCLI(t, "skills", "generate", "--path", dir1, "--custom-only", "--repo", repo)
	if code1 != 0 {
		t.Fatalf("run1 expected exit 0, got %d; stderr: %s", code1, stderr1)
	}
	if !strings.Contains(out1, "Source: "+repo) {
		t.Errorf("run1: expected Source: %s, got: %s", repo, out1)
	}
	if got := readSkillSource(t, filepath.Join(dir1, "dot-ai-troubleshoot-pod", "SKILL.md")); got != repo {
		t.Errorf("run1: expected source frontmatter %q, got %q", repo, got)
	}

	// Run 2: repo + path + branch + token. Source must be unchanged.
	dir2 := t.TempDir()
	out2, stderr2, code2 := runCLIAtServer(t, "http://localhost:3001", []string{"DOT_AI_GIT_TOKEN=tok-xyz"},
		"skills", "generate", "--path", dir2, "--custom-only",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if code2 != 0 {
		t.Fatalf("run2 expected exit 0, got %d; stderr: %s", code2, stderr2)
	}
	if !strings.Contains(out2, "Source: "+repo) {
		t.Errorf("run2: path/branch/token must not change Source; expected %s, got: %s", repo, out2)
	}
	if got := readSkillSource(t, filepath.Join(dir2, "dot-ai-skill-on-branch", "SKILL.md")); got != repo {
		t.Errorf("run2: expected source frontmatter %q (unchanged by path/branch/token), got %q", repo, got)
	}
}

// Verification: error scoping. An invalid --repo-path / --repo-branch yields a
// request-scoped 400 surfaced as an actionable per-source CLI error, with no
// token or credentialed URL in the output.
func TestSkillsGenerate_Override_InvalidPathBranch_PerSourceError(t *testing.T) {
	repo := "https://github.com/orgA/skills"
	cases := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{"invalid-path", []string{"--repo", repo, "--repo-path", "../etc", "--repo-branch", "main"}, "Invalid override subPath"},
		{"invalid-branch", []string{"--repo", repo, "--repo-path", "skills", "--repo-branch", "bad~branch"}, "Invalid override branch name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := append([]string{"skills", "generate", "--path", dir}, tc.args...)
			stdout, stderr, exitCode := runCLI(t, args...)
			if exitCode != 1 {
				t.Fatalf("expected exit 1 (request-scoped error), got %d; stderr: %s", exitCode, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, "skills source") || !strings.Contains(combined, repo) {
				t.Errorf("expected a per-source error naming %q, got: %s", repo, combined)
			}
			if !strings.Contains(combined, tc.wantMsg) {
				t.Errorf("expected validation message %q, got: %s", tc.wantMsg, combined)
			}
		})
	}
}

// Verification: no secret leakage. With a credentialed --repo URL and a header
// token, neither the embedded credential nor the token may appear in any
// generated file or in stdout/stderr — the repo appears only as scrubbed source.
func TestSkillsGenerate_Override_NoSecretLeakage(t *testing.T) {
	dir := t.TempDir()
	const (
		repoSecret   = "REPOSECRET123"
		headerSecret = "HEADERSECRET456"
	)
	repo := "https://x:" + repoSecret + "@github.com/orgA/skills"
	stdout, stderr, exitCode := runCLIAtServer(t, "http://localhost:3001", []string{"DOT_AI_GIT_TOKEN=" + headerSecret},
		"skills", "generate", "--path", dir, "--custom-only",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	for _, secret := range []string{repoSecret, headerSecret} {
		if strings.Contains(stdout, secret) {
			t.Errorf("secret %q leaked into stdout: %s", secret, stdout)
		}
		if strings.Contains(stderr, secret) {
			t.Errorf("secret %q leaked into stderr: %s", secret, stderr)
		}
	}

	// Grep every generated file for both secrets and for the credentialed URL.
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		s := string(data)
		for _, secret := range []string{repoSecret, headerSecret} {
			if strings.Contains(s, secret) {
				t.Errorf("secret %q leaked into generated file %s", secret, p)
			}
		}
		if strings.Contains(s, "x:"+repoSecret+"@") {
			t.Errorf("credentialed URL leaked into generated file %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	// Sanity: the branch set did resolve (proves the credentialed override worked).
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-skill-on-branch", "SKILL.md")); err != nil {
		t.Errorf("expected skill-on-branch generated with credentialed override: %v", err)
	}
}

func TestSkillsGenerate_RepoPathBranch_InHelp(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	s := string(out)
	for _, want := range []string{"--repo-path", "--repo-branch", "DOT_AI_GIT_TOKEN"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected help to mention %q", want)
		}
	}
}

// install-hook must forward --repo-path/--repo-branch (so each hook reproduces
// its source) but must NEVER embed the credential — that stays in the env.
func TestSkillsGenerate_InstallHook_ForwardsRepoPathBranch_NotToken(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	const token = "HOOKSECRET789"
	repo := "https://github.com/orgA/skills"
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "DOT_AI_GIT_TOKEN="+token)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0; stderr: %s", errBuf.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json: %v", err)
	}
	s := string(data)
	for _, want := range []string{"--repo-path", "--repo-branch", "skills", "team-skills"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected hook command to contain %q, got: %s", want, s)
		}
	}
	if strings.Contains(s, token) {
		t.Errorf("credential must never be written to settings.json, but found it: %s", s)
	}
}

// Security: a cross-host redirect must NOT carry the git credential to the
// redirect target. net/http strips Authorization on a cross-host redirect but
// leaves custom headers intact, so without a CheckRedirect policy the
// X-Dot-AI-Git-Token header (and, on a 307, the re-sent body) would reach
// whatever host the configured server redirects to. The CLI drives a backend
// that 307-redirects every request to a DIFFERENT host (a capture server on
// another port); the target must never see the token.
func TestSkillsGenerate_Override_CrossHostRedirect_DropsTokenHeader(t *testing.T) {
	target := newCaptureServer(t)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dest := target.URL + r.URL.Path
		if r.URL.RawQuery != "" {
			dest += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, dest, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirector.Close)

	dir := t.TempDir()
	const (
		repo  = "https://github.com/orgA/skills"
		token = "redirect-secret-xyz"
	)
	_, stderr, exitCode := runCLIAtServer(t, redirector.URL, []string{"DOT_AI_GIT_TOKEN=" + token},
		"skills", "generate", "--path", dir, "--custom-only", "--pull-latest",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	reqs := target.snapshot()
	if len(reqs) == 0 {
		t.Fatal("expected the redirect target to receive at least one request")
	}
	for _, r := range reqs {
		if r.Token != "" {
			t.Errorf("git credential leaked across host redirect: %s %s received X-Dot-AI-Git-Token %q", r.Method, r.Path, r.Token)
		}
		if strings.Contains(r.Body, token) {
			t.Errorf("git credential leaked into body across host redirect on %s %s: %s", r.Method, r.Path, r.Body)
		}
	}
}

// Correctness: a numeric (but valid) branch name must be sent as a JSON string
// in the refresh body, not coerced to a JSON number. The override branch
// charset allows all-digit names like "123", so the known-string override
// fields must always be JSON-string encoded.
func TestSkillsGenerate_PullLatest_Refresh_NumericBranchIsJSONString(t *testing.T) {
	cs := newCaptureServer(t)
	dir := t.TempDir()
	const repo = "https://github.com/orgA/skills"
	_, stderr, exitCode := runCLIAtServer(t, cs.URL, nil,
		"skills", "generate", "--path", dir, "--custom-only", "--pull-latest",
		"--repo", repo, "--repo-branch", "123")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	refresh := findRequest(t, cs.snapshot(), http.MethodPost, "/api/v1/prompts/refresh")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(refresh.Body), &raw); err != nil {
		t.Fatalf("refresh body is not a JSON object: %v; raw: %s", err, refresh.Body)
	}
	if got := string(raw["branch"]); got != `"123"` {
		t.Errorf("expected refresh body branch to be the JSON string \"123\", got %s", got)
	}
	if got := string(raw["repo"]); got != jsonString(repo) {
		t.Errorf("expected refresh body repo to be the JSON string %s, got %s", jsonString(repo), got)
	}
}

// Verification: the mock's refresh override fixture reports promptsLoaded:5 for
// the distinct path+branch set, and the CLI surfaces that count.
func TestSkillsGenerate_PullLatest_Refresh_ReportsPromptsLoaded(t *testing.T) {
	dir := t.TempDir()
	repo := "https://github.com/orgA/skills"
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir, "--custom-only", "--pull-latest",
		"--repo", repo, "--repo-path", "skills", "--repo-branch", "team-skills")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "5 prompts loaded") {
		t.Errorf("expected refresh to report 5 prompts loaded for the path+branch set, got: %s", stdout)
	}
}
