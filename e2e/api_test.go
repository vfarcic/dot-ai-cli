//go:build integration

package e2e_test

import (
	"os/exec"
	"strings"
	"testing"
)

// --- GET endpoints with fixtures ---

func TestResources_GET_ReturnsPods(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "resources", "--kind", "Pod", "--apiVersion", "v1", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "nginx-deployment-7d9c67b5f-abc12") {
		t.Errorf("expected response to contain pod name, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"total": 3`) {
		t.Errorf("expected response to contain total 3, got: %s", stdout)
	}
}

func TestResourcesKinds_GET_ReturnsKinds(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "resources", "kinds", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, `"Pod"`) {
		t.Errorf("expected response to contain Pod kind, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"Deployment"`) {
		t.Errorf("expected response to contain Deployment kind, got: %s", stdout)
	}
}

func TestNamespaces_GET_ReturnsList(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "namespaces", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, `"default"`) {
		t.Errorf("expected response to contain default namespace, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"kube-system"`) {
		t.Errorf("expected response to contain kube-system namespace, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"monitoring"`) {
		t.Errorf("expected response to contain monitoring namespace, got: %s", stdout)
	}
}

func TestVisualize_GET_WithPathParam(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "visualize", "test-session-123", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Cluster Architecture Overview") {
		t.Errorf("expected response to contain visualization title, got: %s", stdout)
	}
	if !strings.Contains(stdout, "mermaid") {
		t.Errorf("expected response to contain mermaid type, got: %s", stdout)
	}
}

// --- DELETE endpoint with fixture ---

func TestKnowledgeSource_DELETE_WithPathParam(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "knowledge", "source", "default/my-docs", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, `"chunksDeleted": 42`) {
		t.Errorf("expected response to contain chunksDeleted 42, got: %s", stdout)
	}
}

// --- POST endpoints with fixtures ---

func TestKnowledgeAsk_POST_WithBody(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "knowledge", "ask", "how to configure RBAC?", "--limit", "20", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "RBAC policies") {
		t.Errorf("expected response to contain RBAC answer, got: %s", stdout)
	}
	if !strings.Contains(stdout, "sources") {
		t.Errorf("expected response to contain sources, got: %s", stdout)
	}
}

func TestManageKnowledge_POST_IngestFixture(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "manageKnowledge",
		"--operation", "ingest",
		"--content", "test content",
		"--uri", "https://example.com/doc.md",
		"--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, `"chunksCreated": 3`) {
		t.Errorf("expected response to contain chunksCreated 3, got: %s", stdout)
	}
}

// --- POST endpoints WITHOUT fixtures (501) ---

func TestQuery_POST_NoFixture_ReturnsError(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "query", "what pods are running?")
	if exitCode != 1 {
		t.Fatalf("expected exit 1 (server error), got %d", exitCode)
	}
	if stderr == "" {
		t.Error("expected error message on stderr")
	}
}

func TestVersion_POST_NoFixture_ReturnsError(t *testing.T) {
	_, stderr, exitCode := runCLI(t, "version")
	if exitCode != 1 {
		t.Fatalf("expected exit 1 (server error), got %d", exitCode)
	}
	if stderr == "" {
		t.Error("expected error message on stderr")
	}
}

// --- Error scenarios ---

func TestConnectionError_ExitCode2(t *testing.T) {
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:19999", "namespaces")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
	if !strings.Contains(errBuf.String(), "cannot connect") {
		t.Errorf("expected connection error message, got: %s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "--server-url") {
		t.Errorf("expected hint about --server-url flag, got: %s", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "DOT_AI_URL") {
		t.Errorf("expected hint about DOT_AI_URL env var, got: %s", errBuf.String())
	}
}

// --- User management commands (method-grouped subcommands) ---

func TestUsers_List(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "users", "list", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "admin@dot-ai.local") {
		t.Errorf("expected admin user in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "alice@example.com") {
		t.Errorf("expected alice user in output, got: %s", stdout)
	}
}

func TestUsers_Create(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "users", "create", "--email", "test@example.com", "--password", "securepass", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "User created successfully") {
		t.Errorf("expected success message, got: %s", stdout)
	}
}

func TestUsers_Delete(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "users", "delete", "bob@example.com", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "User deleted successfully") {
		t.Errorf("expected success message, got: %s", stdout)
	}
}

func TestUsersHelp_ShowsSubcommands(t *testing.T) {
	cmd := exec.Command(binaryPath, "users", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, sub := range []string{"list", "create", "delete"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("expected users help to list %q subcommand, got: %s", sub, stdout)
		}
	}
}
