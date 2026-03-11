//go:build integration

package e2e_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRBAC_OAuthUser_AllowedToolsVisible(t *testing.T) {
	configDir := t.TempDir()
	writeOAuthCredentials(t, configDir)

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir},
		"--help")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Mock server returns all tools. OAuth user should see them all.
	for _, tool := range []string{"query", "recommend", "remediate", "operate", "manageKnowledge", "manageOrgData", "projectSetup", "version"} {
		if !strings.Contains(stdout, tool) {
			t.Errorf("expected allowed tool %q to appear in help, got:\n%s", tool, stdout)
		}
	}

	// Non-tool commands should also be visible.
	for _, visible := range []string{"resources", "knowledge", "auth", "skills"} {
		if !strings.Contains(stdout, visible) {
			t.Errorf("expected non-tool command %q to remain visible, got:\n%s", visible, stdout)
		}
	}
}

func TestRBAC_StaticToken_AllCommandsVisible(t *testing.T) {
	configDir := t.TempDir()
	writeStaticCredentials(t, configDir)

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir},
		"--help")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// With a static token, no RBAC filtering happens at all.
	for _, tool := range []string{"query", "recommend", "remediate", "manageKnowledge", "manageOrgData", "operate", "projectSetup", "version"} {
		if !strings.Contains(stdout, tool) {
			t.Errorf("expected tool %q to be visible with static token, got:\n%s", tool, stdout)
		}
	}
}

func TestRBAC_Unauthenticated_AllCommandsVisible(t *testing.T) {
	configDir := t.TempDir()

	stdout, stderr, exitCode := runCLIWithEnv(t,
		[]string{"HOME=" + configDir},
		"--help")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	for _, tool := range []string{"query", "recommend", "remediate", "manageKnowledge", "operate", "version"} {
		if !strings.Contains(stdout, tool) {
			t.Errorf("expected tool %q to be visible when unauthenticated, got:\n%s", tool, stdout)
		}
	}
}

func TestRBAC_OAuthUser_GracefulDegradation(t *testing.T) {
	configDir := t.TempDir()
	writeOAuthCredentials(t, configDir)

	// Point to a non-existent server — tools fetch should fail silently.
	fullArgs := []string{"--server-url", "http://localhost:19999", "--help"}
	cmd := exec.Command(binaryPath, fullArgs...)
	cmd.Env = append(os.Environ(), "HOME="+configDir)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, errBuf.String())
	}

	stdout := outBuf.String()
	// When fetch fails, all commands should remain visible (no filtering).
	for _, tool := range []string{"query", "recommend", "remediate", "manageKnowledge", "operate", "version"} {
		if !strings.Contains(stdout, tool) {
			t.Errorf("expected tool %q to remain visible on fetch failure, got:\n%s", tool, stdout)
		}
	}
}
