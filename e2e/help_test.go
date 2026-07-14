//go:build integration

package e2e_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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
	for _, excluded := range []string{"tools-post", "openapi"} {
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

func TestHelp_ShowsSkillsCommand(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "skills") {
		t.Error("expected root help to list skills command")
	}
}

func TestHelp_ShowsAuthCommand(t *testing.T) {
	cmd := exec.Command(binaryPath, "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "auth") {
		t.Error("expected root help to list auth command")
	}
}

// --- Shell completion ---

func TestCompletion_Bash_GeneratesScript(t *testing.T) {
	cmd := exec.Command(binaryPath, "completion", "bash")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "bash completion") {
		t.Errorf("expected bash completion script, got: %s", string(out))
	}
}

func TestCompletion_Zsh_GeneratesScript(t *testing.T) {
	cmd := exec.Command(binaryPath, "completion", "zsh")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "compdef") {
		t.Errorf("expected zsh compdef in script, got: %s", string(out))
	}
}

func TestCompletion_Fish_GeneratesScript(t *testing.T) {
	cmd := exec.Command(binaryPath, "completion", "fish")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "complete") {
		t.Errorf("expected fish complete commands, got: %s", string(out))
	}
}

func TestCompletion_EnumFlag_DataType(t *testing.T) {
	cmd := exec.Command(binaryPath, "__complete", "manageOrgData", "--dataType", "")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	// As of dot-ai v1.24.0 (PRD #375, unified knowledge base), manageOrgData
	// is capabilities-only; patterns/policies moved to the manageKnowledge tool.
	if !strings.Contains(stdout, "capabilities") {
		t.Errorf("expected completion to include %q, got: %s", "capabilities", stdout)
	}
	for _, val := range []string{"pattern", "policy"} {
		if strings.Contains(stdout, val) {
			t.Errorf("expected completion to no longer include %q (moved to manageKnowledge), got: %s", val, stdout)
		}
	}
	// Directive :4 means ShellCompDirectiveNoFileComp.
	if !strings.Contains(stdout, ":4") {
		t.Errorf("expected NoFileComp directive (:4), got: %s", stdout)
	}
}

func TestCompletion_EnumFlag_OutputGlobal(t *testing.T) {
	cmd := exec.Command(binaryPath, "__complete", "namespaces", "--output", "")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, val := range []string{"json", "yaml"} {
		if !strings.Contains(stdout, val) {
			t.Errorf("expected completion to include %q, got: %s", val, stdout)
		}
	}
}

func TestCompletion_EnumFlag_RemediateMode(t *testing.T) {
	cmd := exec.Command(binaryPath, "__complete", "remediate", "--mode", "")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, val := range []string{"manual", "automatic"} {
		if !strings.Contains(stdout, val) {
			t.Errorf("expected completion to include %q, got: %s", val, stdout)
		}
	}
}
