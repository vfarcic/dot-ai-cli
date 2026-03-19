//go:build integration

package e2e_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthLogin_FullFlow(t *testing.T) {
	// Use a temp config dir so we don't pollute the real one.
	configDir := t.TempDir()

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001", "auth", "login", "--no-browser")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configDir)
	// Override the config dir via HOME so ~/.config/dot-ai lands in temp.
	cmd.Env = append(cmd.Env, "HOME="+configDir)

	// We need to read stdout line-by-line to get the URL, then simulate the browser.
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Read stdout lines to find the authorize URL.
	scanner := bufio.NewScanner(stdoutPipe)
	var authURL string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "http") {
			authURL = line
			break
		}
	}

	if authURL == "" {
		cmd.Process.Kill()
		t.Fatal("did not find authorize URL in stdout")
	}

	// Simulate the browser: follow the /authorize redirect to the CLI callback.
	// Use a client that does NOT follow redirects so we can get the Location header.
	noRedirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirectClient.Get(authURL)
	if err != nil {
		cmd.Process.Kill()
		t.Fatalf("GET authorize: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		cmd.Process.Kill()
		t.Fatalf("expected 302 from /authorize, got %d", resp.StatusCode)
	}

	// The mock server redirects to the CLI's local callback with ?code=...
	callbackURL := resp.Header.Get("Location")
	if callbackURL == "" {
		cmd.Process.Kill()
		t.Fatal("no Location header from /authorize")
	}

	// Hit the CLI's callback server (retry to handle startup race).
	var cbResp *http.Response
	for i := 0; i < 10; i++ {
		cbResp, err = http.Get(callbackURL)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		cmd.Process.Kill()
		t.Fatalf("GET callback after retries: %v", err)
	}
	cbResp.Body.Close()

	// Wait for the CLI to finish.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CLI exited with error: %v; stderr: %s", err, stderrBuf.String())
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("CLI did not exit within 10 seconds")
	}

	// Verify credentials were stored.
	credPath := filepath.Join(configDir, ".config", "dot-ai", "credentials.json")
	data, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatalf("reading credentials: %v", err)
	}
	var creds map[string]any
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("parsing credentials: %v", err)
	}
	if creds["access_token"] == nil || creds["access_token"] == "" {
		t.Error("expected access_token to be set in credentials.json")
	}
	if creds["client_id"] == nil || creds["client_id"] == "" {
		t.Error("expected client_id to be set in credentials.json")
	}
}

func TestAuthLogout_ClearsOAuthKeepsStaticToken(t *testing.T) {
	configDir := t.TempDir()

	// Pre-populate credentials with both static and OAuth tokens.
	credDir := filepath.Join(configDir, ".config", "dot-ai")
	os.MkdirAll(credDir, 0700)
	creds := map[string]string{
		"auth_token":    "my-static-token",
		"access_token":  "my-oauth-token",
		"token_type":    "Bearer",
		"expires_at":    "2099-01-01T00:00:00Z",
		"client_id":     "cli-123",
		"client_secret": "secret",
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600)

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir},
		"auth", "logout")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Logged out") {
		t.Errorf("expected logout message, got: %s", stdout)
	}

	// Verify OAuth fields cleared, static token preserved.
	afterData, err := os.ReadFile(filepath.Join(credDir, "credentials.json"))
	if err != nil {
		t.Fatalf("reading credentials: %v", err)
	}
	var afterCreds map[string]string
	json.Unmarshal(afterData, &afterCreds)
	if afterCreds["auth_token"] != "my-static-token" {
		t.Errorf("auth_token should be preserved, got %q", afterCreds["auth_token"])
	}
	if afterCreds["access_token"] != "" {
		t.Errorf("access_token should be cleared, got %q", afterCreds["access_token"])
	}
	if afterCreds["client_id"] != "" {
		t.Errorf("client_id should be cleared, got %q", afterCreds["client_id"])
	}
}

func TestAuthStatus_ShowsOAuth(t *testing.T) {
	configDir := t.TempDir()
	credDir := filepath.Join(configDir, ".config", "dot-ai")
	os.MkdirAll(credDir, 0700)
	creds := map[string]string{
		"access_token": "a-long-oauth-access-token-value",
		"token_type":   "Bearer",
		"expires_at":   "2099-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600)

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir, "DOT_AI_AUTH_TOKEN="},
		"auth", "status")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "OAuth") {
		t.Errorf("expected 'OAuth' in status output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Valid") {
		t.Errorf("expected 'Valid' in status output, got: %s", stdout)
	}
}

func TestAuthStatus_ShowsStaticToken(t *testing.T) {
	configDir := t.TempDir()
	credDir := filepath.Join(configDir, ".config", "dot-ai")
	os.MkdirAll(credDir, 0700)
	creds := map[string]string{
		"auth_token": "a-long-static-token-value",
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600)

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir},
		"auth", "status")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Static token") {
		t.Errorf("expected 'Static token' in status output, got: %s", stdout)
	}
}

func TestAuthStatus_ShowsNotAuthenticated(t *testing.T) {
	configDir := t.TempDir()

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir, "DOT_AI_AUTH_TOKEN="},
		"auth", "status")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Not authenticated") {
		t.Errorf("expected 'Not authenticated' in status output, got: %s", stdout)
	}
}

func TestAuthHelp_ShowsSubcommands(t *testing.T) {
	cmd := exec.Command(binaryPath, "auth", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, sub := range []string{"login", "logout", "status"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected auth help to list %q subcommand, got: %s", sub, stdout)
		}
	}
}
