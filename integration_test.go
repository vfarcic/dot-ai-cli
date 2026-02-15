//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	abs, err := filepath.Abs("dot-ai-test")
	if err != nil {
		panic("failed to resolve binary path: " + err.Error())
	}
	binaryPath = abs

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
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
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
	for _, val := range []string{"pattern", "policy", "capabilities"} {
		if !strings.Contains(stdout, val) {
			t.Errorf("expected completion to include %q, got: %s", val, stdout)
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

// --- Skills generation ---

func TestSkillsGenerate_CreatesToolSkills(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Skills generated successfully in") {
		t.Errorf("expected success message with path, got: %s", stdout)
	}
	if !strings.Contains(stdout, dir) {
		t.Errorf("expected output to include target directory %s, got: %s", dir, stdout)
	}

	// Verify tool skills were created (fixture has: query, recommend, remediate, kubectl_get).
	for _, tool := range []string{"query", "recommend", "remediate", "kubectl_get"} {
		skillPath := filepath.Join(dir, "dot-ai-"+tool, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("expected skill file %s to exist: %v", skillPath, err)
			continue
		}
		s := string(content)
		if !strings.Contains(s, "name: dot-ai-"+tool) {
			t.Errorf("skill %s missing name in frontmatter", tool)
		}
		if !strings.Contains(s, "user-invocable: true") {
			t.Errorf("skill %s missing user-invocable flag", tool)
		}
		if !strings.Contains(s, "dot-ai "+tool) {
			t.Errorf("skill %s missing usage reference", tool)
		}
	}
}

func TestSkillsGenerate_CreatesPromptSkills(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Verify prompt skills were created (fixture has: troubleshoot-pod, explain-resource, security-review, optimize-resources).
	for _, prompt := range []string{"troubleshoot-pod", "explain-resource", "security-review", "optimize-resources"} {
		skillPath := filepath.Join(dir, "dot-ai-"+prompt, "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("expected skill file %s to exist: %v", skillPath, err)
			continue
		}
		s := string(content)
		if !strings.Contains(s, "name: dot-ai-"+prompt) {
			t.Errorf("prompt skill %s missing name in frontmatter", prompt)
		}
		if !strings.Contains(s, "user-invocable: true") {
			t.Errorf("prompt skill %s missing user-invocable flag", prompt)
		}
	}
}

func TestSkillsGenerate_PromptRenderedContent(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// The mock returns the same rendered content for all prompts (troubleshoot-pod fixture).
	skillPath := filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("expected skill file to exist: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "troubleshoot") {
		t.Errorf("expected rendered prompt content with troubleshoot text, got: %s", s)
	}
}

func TestSkillsGenerate_ToolSkillHasParameters(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// The query tool has an "intent" parameter.
	content, err := os.ReadFile(filepath.Join(dir, "dot-ai-query", "SKILL.md"))
	if err != nil {
		t.Fatalf("expected skill file to exist: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "intent") {
		t.Errorf("expected query skill to document intent parameter, got: %s", s)
	}
	if !strings.Contains(s, "required") {
		t.Errorf("expected query skill to indicate required parameter, got: %s", s)
	}
}

func TestSkillsGenerate_AgentClaudeCode(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the parent so the test verifies the command creates the skills dir.
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001", "skills", "generate", "--agent", "claude-code")
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, errBuf.String())
	}

	// Verify at least one skill was created in the claude-code skills dir.
	outDir := filepath.Join(dir, ".claude", "skills")
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "dot-ai-") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected dot-ai-* skill directories in output")
	}
}

func TestSkillsGenerate_CreatesRoutingSkill(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	content, err := os.ReadFile(filepath.Join(dir, "dot-ai", "SKILL.md"))
	if err != nil {
		t.Fatal("expected routing skill dot-ai/SKILL.md to exist")
	}
	s := string(content)
	if !strings.Contains(s, "name: dot-ai") {
		t.Error("routing skill missing name in frontmatter")
	}
	if strings.Contains(s, "user-invocable: true") {
		t.Error("routing skill should NOT be user-invocable")
	}
	if !strings.Contains(s, "dot-ai --help") {
		t.Error("routing skill should reference dot-ai --help")
	}
	if !strings.Contains(s, "Kubernetes") {
		t.Error("routing skill should mention Kubernetes for intent matching")
	}
}

func TestSkillsGenerate_CleansExistingOnRerun(t *testing.T) {
	dir := t.TempDir()

	// Create stale skills that should be cleaned up (both prefixed and routing).
	staleDir := filepath.Join(dir, "dot-ai-stale-skill")
	os.MkdirAll(staleDir, 0o755)
	os.WriteFile(filepath.Join(staleDir, "SKILL.md"), []byte("old"), 0o644)
	staleRouting := filepath.Join(dir, "dot-ai")
	os.MkdirAll(staleRouting, 0o755)
	os.WriteFile(filepath.Join(staleRouting, "SKILL.md"), []byte("old routing"), 0o644)

	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Stale skill should be gone.
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Error("expected stale dot-ai-stale-skill to be removed on re-run")
	}

	// Routing skill should be regenerated (not stale content).
	content, err := os.ReadFile(filepath.Join(dir, "dot-ai", "SKILL.md"))
	if err != nil {
		t.Fatal("expected routing skill to be regenerated")
	}
	if string(content) == "old routing" {
		t.Error("expected routing skill to have fresh content, not stale")
	}

	// Fresh skills should exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); err != nil {
		t.Error("expected dot-ai-query skill to be regenerated")
	}
}

func TestSkillsGenerate_PreservesNonDotAISkills(t *testing.T) {
	dir := t.TempDir()

	// Create a user skill that should NOT be deleted.
	userDir := filepath.Join(dir, "my-custom-skill")
	os.MkdirAll(userDir, 0o755)
	os.WriteFile(filepath.Join(userDir, "SKILL.md"), []byte("user skill"), 0o644)

	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// User skill should still exist.
	content, err := os.ReadFile(filepath.Join(userDir, "SKILL.md"))
	if err != nil {
		t.Fatal("expected user skill to be preserved")
	}
	if string(content) != "user skill" {
		t.Error("expected user skill content to be unchanged")
	}
}

func TestSkillsGenerate_NoAgentNoPath_Error(t *testing.T) {
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001", "skills", "generate")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error when neither --agent nor --path is provided")
	}
	if !strings.Contains(errBuf.String(), "--agent") && !strings.Contains(errBuf.String(), "--path") {
		t.Errorf("expected error mentioning --agent or --path, got: %s", errBuf.String())
	}
}

func TestSkillsGenerate_InvalidAgent_Error(t *testing.T) {
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001", "skills", "generate", "--agent", "vscode")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error for invalid agent")
	}
	if !strings.Contains(errBuf.String(), "invalid value") {
		t.Errorf("expected invalid agent error, got: %s", errBuf.String())
	}
}

func TestSkillsGenerate_Help_NoServer(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	if !strings.Contains(stdout, "--agent") {
		t.Error("expected help to mention --agent flag")
	}
	if !strings.Contains(stdout, "--path") {
		t.Error("expected help to mention --path flag")
	}
	if !strings.Contains(stdout, "claude-code") {
		t.Error("expected help to mention claude-code agent")
	}
}

func TestSkillsGenerate_ConnectionError(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:19999", "skills", "generate", "--path", dir)
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d; stderr: %s", exitCode, errBuf.String())
	}
}

func TestSkillsGenerate_AgentCompletion(t *testing.T) {
	cmd := exec.Command(binaryPath, "__complete", "skills", "generate", "--agent", "")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	for _, agent := range []string{"claude-code", "cursor", "windsurf"} {
		if !strings.Contains(stdout, agent) {
			t.Errorf("expected completion to include %q, got: %s", agent, stdout)
		}
	}
}

func TestSkillsGenerate_InstallHook_CreatesSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook")
	cmd.Dir = dir
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, errBuf.String())
	}

	if !strings.Contains(outBuf.String(), "SessionStart hook installed") {
		t.Errorf("expected hook installation message, got: %s", outBuf.String())
	}

	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("expected settings.json to exist: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}

	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart entry, got %d", len(sessionStart))
	}
	entry := sessionStart[0].(map[string]any)
	if entry["matcher"] != "startup" {
		t.Errorf("expected matcher 'startup', got %v", entry["matcher"])
	}
	innerHooks := entry["hooks"].([]any)
	if len(innerHooks) != 1 {
		t.Fatalf("expected 1 inner hook, got %d", len(innerHooks))
	}
	hook := innerHooks[0].(map[string]any)
	if hook["type"] != "command" {
		t.Errorf("expected hook type 'command', got %v", hook["type"])
	}
	if hook["command"] != "dot-ai skills generate --agent claude-code" {
		t.Errorf("expected hook command, got %v", hook["command"])
	}
}

func TestSkillsGenerate_InstallHook_Idempotent(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	for i := 0; i < 2; i++ {
		cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
			"skills", "generate", "--agent", "claude-code", "--install-hook")
		cmd.Dir = dir
		var errBuf strings.Builder
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err != nil {
			t.Fatalf("run %d: expected exit 0; stderr: %s", i+1, errBuf.String())
		}
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json: %v", err)
	}
	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Errorf("expected exactly 1 SessionStart entry after two runs, got %d", len(sessionStart))
	}
}

func TestSkillsGenerate_InstallHook_MergesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []any{"Bash(git status:*)"},
		},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": ".*",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "echo pre-tool",
						},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), data, 0o644)

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook")
	cmd.Dir = dir
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected exit 0; stderr: %s", errBuf.String())
	}

	data, _ = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	var settings map[string]any
	json.Unmarshal(data, &settings)

	if settings["permissions"] == nil {
		t.Error("expected permissions to be preserved")
	}

	hooks := settings["hooks"].(map[string]any)
	if hooks["PreToolUse"] == nil {
		t.Error("expected PreToolUse hook to be preserved")
	}

	sessionStart := hooks["SessionStart"].([]any)
	if len(sessionStart) != 1 {
		t.Errorf("expected 1 SessionStart entry, got %d", len(sessionStart))
	}
}

func TestSkillsGenerate_InstallHook_RequiresClaudeCode(t *testing.T) {
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "cursor", "--install-hook")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error when --install-hook used with non-claude-code agent")
	}
	if !strings.Contains(errBuf.String(), "--install-hook requires --agent claude-code") {
		t.Errorf("expected specific error message, got: %s", errBuf.String())
	}
}

func TestSkillsGenerate_InstallHook_IncompatibleWithPath(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--path", dir, "--install-hook")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error when --install-hook used with --path")
	}
	if !strings.Contains(errBuf.String(), "--install-hook cannot be used with --path") {
		t.Errorf("expected specific error message, got: %s", errBuf.String())
	}
}

func TestSkillsGenerate_InstallHook_InHelp(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "--install-hook") {
		t.Error("expected help to mention --install-hook flag")
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
