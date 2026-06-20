//go:build integration

package e2e_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// --- PRD #13 M4b: content-hash upload gating, evict-retry, `skills cache prune` ---
//
// These use a t.TempDir() XDG_CACHE_HOME so the upload-state store
// (<cache>/dot-ai-cli/uploads/) and the clone cache (<cache>/dot-ai-cli/repos/)
// are deterministic and never touch the suite-shared cache. Gating is verified
// against a capturing backend (so the second, skipped upload is provably absent
// from the request log); the evict-retry is verified against a controlled
// backend that 400s the list until a forced re-upload restores the source.

// countCaptured counts captured requests matching method+path.
func countCaptured(reqs []capturedRequest, method, path string) int {
	n := 0
	for _, r := range reqs {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

// 1. Gating (--repo-dir): a second run over an UNCHANGED source skips the upload
// (no second POST /api/v1/prompts/sources, and the CLI reports it skipped);
// changing the content re-uploads; the first run always uploads.
func TestSkillsGenerate_RepoDir_ContentHashGating(t *testing.T) {
	cs := newRepoDirCaptureServer(t)
	src := repoDirSource(t, map[string]string{"p1/SKILL.md": "---\nname: p1\n---\nbody one"})
	cacheRoot := t.TempDir()
	env := []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + cacheRoot}

	// Run 1: first-ever run for this identifier — MUST upload.
	out1 := t.TempDir()
	stdout1, stderr1, code1 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out1, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code1 != 0 {
		t.Fatalf("run 1 expected exit 0, got %d; stdout: %s stderr: %s", code1, stdout1, stderr1)
	}
	if !strings.Contains(stdout1, "Uploaded source as local:tester-foo") {
		t.Errorf("run 1 expected an upload confirmation, got: %s", stdout1)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 1 {
		t.Fatalf("run 1 expected exactly 1 upload, got %d", n)
	}

	// Run 2: identical source — MUST skip the upload (gated on the content hash).
	out2 := t.TempDir()
	stdout2, stderr2, code2 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out2, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code2 != 0 {
		t.Fatalf("run 2 expected exit 0, got %d; stdout: %s stderr: %s", code2, stdout2, stderr2)
	}
	if !strings.Contains(stdout2, "unchanged, skipping upload") {
		t.Errorf("run 2 expected an 'unchanged, skipping upload' report, got: %s", stdout2)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 1 {
		t.Errorf("run 2 must NOT re-upload an unchanged source; expected 1 total upload, got %d", n)
	}

	// Change the source content — the next run MUST re-upload.
	if err := os.WriteFile(filepath.Join(src, "p1", "SKILL.md"), []byte("---\nname: p1\n---\nbody TWO changed"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	out3 := t.TempDir()
	stdout3, stderr3, code3 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out3, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	if code3 != 0 {
		t.Fatalf("run 3 expected exit 0, got %d; stdout: %s stderr: %s", code3, stdout3, stderr3)
	}
	if !strings.Contains(stdout3, "Uploaded source as local:tester-foo") {
		t.Errorf("run 3 expected a re-upload after the content changed, got: %s", stdout3)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 2 {
		t.Errorf("run 3 expected a 2nd upload after the content changed; got %d total", n)
	}
}

// 2. Gating (--repo-fetch): identical to the --repo-dir case but over the cached
// clone path. A re-run of the same url with unchanged content skips the upload;
// committing a new file (picked up by the incremental fetch) re-uploads.
func TestSkillsGenerate_RepoFetch_ContentHashGating(t *testing.T) {
	cs := newRepoDirCaptureServer(t)
	repo := repoFetchGitRepo(t, map[string]string{"wip-fetched-skill/SKILL.md": newFetchedSkillFile})
	url := fileURL(repo)
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	// Run 1: clone + first upload.
	out1 := t.TempDir()
	stdout1, stderr1, code1 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out1, "--custom-only", "--repo-fetch", url)
	if code1 != 0 {
		t.Fatalf("run 1 expected exit 0, got %d; stdout: %s stderr: %s", code1, stdout1, stderr1)
	}
	if !strings.Contains(stdout1, "Uploaded source as "+url) {
		t.Errorf("run 1 expected an upload confirmation, got: %s", stdout1)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 1 {
		t.Fatalf("run 1 expected exactly 1 upload, got %d", n)
	}

	// Run 2: same url, unchanged content — incremental fetch is a no-op and the
	// upload is gated out.
	out2 := t.TempDir()
	stdout2, stderr2, code2 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out2, "--custom-only", "--repo-fetch", url)
	if code2 != 0 {
		t.Fatalf("run 2 expected exit 0, got %d; stdout: %s stderr: %s", code2, stdout2, stderr2)
	}
	if !strings.Contains(stdout2, "unchanged, skipping upload") {
		t.Errorf("run 2 expected an 'unchanged, skipping upload' report, got: %s", stdout2)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 1 {
		t.Errorf("run 2 must NOT re-upload an unchanged fetched source; expected 1 total, got %d", n)
	}

	// Commit a new skill to the source; the next run fetches it and re-uploads.
	const second = `---
name: wip-second-fetched
description: Added between gating runs
---
WIP-SECOND-FETCHED-MARKER body.`
	writeRepoFiles(t, repo, map[string]string{"wip-second-fetched/SKILL.md": second})
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "-m", "second fetched skill")

	out3 := t.TempDir()
	stdout3, stderr3, code3 := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out3, "--custom-only", "--repo-fetch", url)
	if code3 != 0 {
		t.Fatalf("run 3 expected exit 0, got %d; stdout: %s stderr: %s", code3, stdout3, stderr3)
	}
	if !strings.Contains(stdout3, "Uploaded source as "+url) {
		t.Errorf("run 3 expected a re-upload after the fetched content changed, got: %s", stdout3)
	}
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 2 {
		t.Errorf("run 3 expected a 2nd upload after the fetched content changed; got %d total", n)
	}
}

// 3. Evict-retry (load-bearing): a controlled backend 400s the list ?source=
// (the server's re-upload guidance) until the source has been uploaded TWICE —
// modelling an in-memory-LRU eviction between the gated upload and the list. The
// CLI must DETECT the evict 400, force a re-upload, retry the list ONCE, and the
// run must SUCCEED (skills generated) — transparent recovery. Contrast
// TestSkillsGenerate_RepoDir_ListSourceEvicted_CleanError, whose backend 400s
// forever, so the forced re-upload can't help and the clean error stands.
func TestSkillsGenerate_RepoDir_Evicted_ReuploadRetrySucceeds(t *testing.T) {
	cs := newEvictUntilReuploadedServer(t)
	src := repoDirSource(t, map[string]string{"p1/SKILL.md": "---\nname: p1\n---\nbody"})
	out := t.TempDir()
	env := []string{"DOT_AI_ALLOW_REPO_DIR=1", "USER=tester", "XDG_CACHE_HOME=" + t.TempDir()}

	stdout, stderr, code := runCLIAtServer(t, cs.URL, env,
		"skills", "generate", "--path", out, "--custom-only", "--repo-dir", src, "--source-label", "foo")
	combined := stdout + stderr
	// Transparent recovery: the run SUCCEEDS despite the first list being evicted.
	if code != 0 {
		t.Fatalf("expected exit 0 (transparent evict recovery), got %d; output: %s", code, combined)
	}
	// The skill was generated — proof the retried list succeeded after re-upload.
	if _, err := os.Stat(filepath.Join(out, "dot-ai-p1", "SKILL.md")); err != nil {
		t.Errorf("expected dot-ai-p1 generated after the evict-retry recovery: %v", err)
	}
	// The CLI announced the forced re-upload (it did not silently swallow the 400).
	if !strings.Contains(combined, "re-uploading") {
		t.Errorf("expected a 're-uploading' notice on the evict-retry, got: %s", combined)
	}
	// Exactly two uploads: the gated one, then the forced re-upload.
	if n := countCaptured(cs.snapshot(), http.MethodPost, "/api/v1/prompts/sources"); n != 2 {
		t.Errorf("expected 2 uploads (gated + forced re-upload), got %d", n)
	}
	// No internal/irrelevant leakage in the success path.
	for _, leak := range []string{"panic", "goroutine", "runtime error", "nil pointer", "VALIDATION_ERROR"} {
		if strings.Contains(combined, leak) {
			t.Errorf("evict-retry leaked internal detail %q: %s", leak, combined)
		}
	}
}

// newEvictUntilReuploadedServer accepts uploads (200 ingested) and serves the
// list/render normally ONLY after the source has been uploaded at least twice;
// before that it answers GET /api/v1/prompts?source= with the server's 400
// re-upload guidance. This models a source evicted between the first upload and
// the list that is restored by a forced re-upload.
func newEvictUntilReuploadedServer(t *testing.T) *captureServer {
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
		uploads := 0
		for _, rq := range cs.requests {
			if rq.Method == http.MethodPost && rq.Path == "/api/v1/prompts/sources" {
				uploads++
			}
		}
		cs.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/prompts/sources":
			io.WriteString(w, `{"success":true,"data":{"source":`+jsonString(r.URL.Query().Get("source"))+`,"contentHash":"sha256:x","fileCount":1,"status":"ingested"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/prompts":
			if uploads >= 2 {
				io.WriteString(w, `{"success":true,"data":{"prompts":[{"name":"p1","description":"p1 desc"}],"source":`+jsonString(r.URL.Query().Get("source"))+`}}`)
				return
			}
			src := r.URL.Query().Get("source")
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"success":false,"error":{"code":"VALIDATION_ERROR","message":"Ingested source not found: `+src+`. (Re)upload it via POST /api/v1/prompts/sources before rendering."}}`)
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

// --- `skills cache prune --older-than` ---

// makeCacheEntry creates a clone-cache entry dir under cacheRoot keyed exactly as
// the CLI keys it (sha256 of the url), with a sentinel file and the given mtime.
func makeCacheEntry(t *testing.T, cacheRoot, url string, mtime time.Time) string {
	t.Helper()
	dir := repoFetchCacheDir(cacheRoot, url)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatalf("mkdir cache entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	// Set the dir mtime LAST (writing into it bumps mtime to now).
	if err := os.Chtimes(dir, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return dir
}

// 4. prune removes an aged entry and keeps a fresh one.
func TestSkillsCachePrune_RemovesAgedKeepsFresh(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	aged := makeCacheEntry(t, cacheRoot, "https://example.com/aged", time.Now().Add(-48*time.Hour))
	fresh := makeCacheEntry(t, cacheRoot, "https://example.com/fresh", time.Now())

	combined, code, err := runCLIRaw(env, "skills", "cache", "prune", "--older-than", "1h")
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, combined)
	}
	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Errorf("expected the aged cache entry to be pruned, but it still exists (err=%v)", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected the fresh cache entry to be kept: %v", err)
	}
	if !strings.Contains(combined, "removed 1 clone-cache") {
		t.Errorf("expected a prune summary reporting 1 removed entry, got: %s", combined)
	}
}

// 5. prune on a missing/empty cache is a clean no-op (exit 0).
func TestSkillsCachePrune_EmptyCacheNoop(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	combined, code, err := runCLIRaw(env, "skills", "cache", "prune", "--older-than", "720h")
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0 on an empty cache, got %d; output: %s", code, combined)
	}
	if !strings.Contains(combined, "nothing to prune") {
		t.Errorf("expected a 'nothing to prune' report, got: %s", combined)
	}
}

// 6. An invalid --older-than is a clean usage error (non-zero exit, no panic).
func TestSkillsCachePrune_InvalidDuration(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	combined, code, err := runCLIRaw(env, "skills", "cache", "prune", "--older-than", "not-a-duration")
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 3 {
		t.Fatalf("expected exit 3 (the clean usage-error code) for an invalid --older-than, got %d; output: %s", code, combined)
	}
	if !strings.Contains(combined, "invalid") || !strings.Contains(combined, "older-than") {
		t.Errorf("expected a clean invalid-duration usage error, got: %s", combined)
	}
	for _, leak := range []string{"panic", "goroutine", "runtime error"} {
		if strings.Contains(combined, leak) {
			t.Errorf("invalid-duration path leaked %q: %s", leak, combined)
		}
	}
}

// 7. prune SKIPS an aged entry whose per-URL flock is held by a concurrent
// --repo-fetch. The test pre-acquires the very lock the CLI prune contends on
// (held in THIS process; the prune runs in a SUBPROCESS, so the flock genuinely
// conflicts cross-process) and asserts the aged entry survives the prune.
func TestSkillsCachePrune_SkipsLockedEntry(t *testing.T) {
	cacheRoot := t.TempDir()
	env := []string{"XDG_CACHE_HOME=" + cacheRoot}

	const url = "https://example.com/locked"
	aged := makeCacheEntry(t, cacheRoot, url, time.Now().Add(-48*time.Hour))

	// Hold the sibling per-URL lock the prune will try to acquire for this entry.
	lock := flock.New(aged + ".lock")
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("test failed to pre-acquire the entry lock (ok=%v err=%v)", ok, err)
	}
	defer lock.Unlock()

	combined, code, err := runCLIRaw(env, "skills", "cache", "prune", "--older-than", "1h")
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; output: %s", code, combined)
	}
	// The locked (in-use) aged entry must NOT be deleted.
	if _, err := os.Stat(aged); err != nil {
		t.Errorf("expected the flock-held aged entry to be SKIPPED (kept), but it is gone: %v", err)
	}
	if !strings.Contains(combined, "skipped 1 in use") {
		t.Errorf("expected the summary to report 1 skipped-in-use entry, got: %s", combined)
	}
}
