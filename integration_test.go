//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var binaryPath string

func TestMain(m *testing.M) {
	cmd := exec.Command("go", "build", "-o", "dot-ai-test", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build binary: " + string(out))
	}
	binaryPath = "./dot-ai-test"

	code := m.Run()

	os.Remove("dot-ai-test")
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
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

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

// --- Output format tests ---

func TestDefaultOutput_IsValidYAML(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "namespaces")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("default output is not valid YAML: %v\nOutput: %s", err, stdout)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
	// Must NOT be valid JSON (proves it was converted).
	if json.Valid([]byte(strings.TrimSpace(stdout))) {
		t.Error("default output should be YAML, not JSON")
	}
}

func TestOutputJSON_IsValidJSON(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "namespaces", "--output", "json")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("--output json is not valid JSON: %v\nOutput: %s", err, stdout)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
	}
}

func TestOutputYAML_Explicit(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "namespaces", "--output", "yaml")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	var result map[string]interface{}
	if err := yaml.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("--output yaml is not valid YAML: %v\nOutput: %s", err, stdout)
	}
	if result["success"] != true {
		t.Errorf("expected success=true, got %v", result["success"])
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

// --- Help works without server ---

func TestHelp_NoServerRequired(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	if !strings.Contains(stdout, "dot-ai") {
		t.Errorf("expected help to contain 'dot-ai', got: %s", stdout)
	}
	if !strings.Contains(stdout, "query") {
		t.Errorf("expected help to list query command, got: %s", stdout)
	}
}

func TestCommandHelp_NoServerRequired(t *testing.T) {
	cmd := exec.Command(binaryPath, "query", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	if !strings.Contains(stdout, "intent") {
		t.Errorf("expected query help to mention intent, got: %s", stdout)
	}
}

func TestHelp_ExcludedCommandsAbsent(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, excluded := range []string{"tools-post", "openapi", "prompts"} {
		if strings.Contains(stdout, excluded) {
			t.Errorf("expected excluded command %q to be absent from help, but found it in:\n%s", excluded, stdout)
		}
	}
}

func TestHelp_RequiredFlagsMarked(t *testing.T) {
	cmd := exec.Command(binaryPath, "resources", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	// Required flags should show "(required)" in their description.
	if !strings.Contains(stdout, "--kind") {
		t.Fatal("expected help to list --kind flag")
	}
	if !strings.Contains(stdout, "(required)") {
		t.Errorf("expected required flags to be marked with '(required)', got:\n%s", stdout)
	}
}
