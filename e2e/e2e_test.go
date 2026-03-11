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

var binaryPath string

func TestMain(m *testing.M) {
	// Build from the project root (one level up from e2e/).
	cmd := exec.Command("go", "build", "-o", filepath.Join("e2e", "dot-ai-test"), ".")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build binary: " + string(out))
	}
	abs, err := filepath.Abs("dot-ai-test")
	if err != nil {
		panic("failed to resolve binary path: " + err.Error())
	}
	binaryPath = abs

	code := m.Run()

	os.Remove(binaryPath)
	os.Exit(code)
}

// runCLI executes the CLI binary with the given args and returns stdout, stderr, and exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	fullArgs := append([]string{"--server-url", "http://localhost:3001"}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// runCLIWithEnv executes the CLI binary with custom env vars and returns stdout, stderr, and exit code.
func runCLIWithEnv(t *testing.T, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	fullArgs := append([]string{"--server-url", "http://localhost:3001"}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// writeOAuthCredentials creates a credentials.json with an OAuth access_token
// in the given config directory (~/.config/dot-ai/).
func writeOAuthCredentials(t *testing.T, homeDir string) {
	t.Helper()
	credDir := filepath.Join(homeDir, ".config", "dot-ai")
	os.MkdirAll(credDir, 0700)
	creds := map[string]string{
		"access_token": "test-oauth-token",
		"token_type":   "Bearer",
		"expires_at":   "2099-01-01T00:00:00Z",
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600)
}

// writeStaticCredentials creates a credentials.json with a static auth_token
// in the given config directory.
func writeStaticCredentials(t *testing.T, homeDir string) {
	t.Helper()
	credDir := filepath.Join(homeDir, ".config", "dot-ai")
	os.MkdirAll(credDir, 0700)
	creds := map[string]string{
		"auth_token": "test-static-token",
	}
	data, _ := json.MarshalIndent(creds, "", "  ")
	os.WriteFile(filepath.Join(credDir, "credentials.json"), data, 0600)
}
