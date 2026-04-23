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

	// Verify tool skills were created (fixture has all server tools).
	for _, tool := range []string{"query", "recommend", "remediate", "operate", "manageOrgData", "manageKnowledge", "version", "projectSetup", "users"} {
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

func TestSkillsGenerate_WritesSupportingFiles(t *testing.T) {
	dir := t.TempDir()
	_, stderr, exitCode := runCLI(t, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// The mock returns files[] alongside messages for all prompts.
	// Use troubleshoot-pod as the representative prompt skill.
	skillDir := filepath.Join(dir, "dot-ai-troubleshoot-pod")

	// Flat supporting file must exist with correct decoded content.
	scriptPath := filepath.Join(skillDir, "troubleshoot.sh")
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("expected supporting file troubleshoot.sh to exist: %v", err)
	}
	if !strings.Contains(string(scriptContent), "#!/bin/bash") {
		t.Errorf("expected script to contain shebang, got: %s", string(scriptContent))
	}
	if !strings.Contains(string(scriptContent), "kubectl get pod") {
		t.Errorf("expected script to contain kubectl command, got: %s", string(scriptContent))
	}

	// Supporting file must have executable permissions.
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755, got %o", info.Mode().Perm())
	}

	// Nested path must create intermediate directories.
	nestedPath := filepath.Join(skillDir, "templates", "pod-debug.yaml")
	nestedContent, err := os.ReadFile(nestedPath)
	if err != nil {
		t.Fatalf("expected nested file templates/pod-debug.yaml to exist: %v", err)
	}
	if !strings.Contains(string(nestedContent), "kind: Pod") {
		t.Errorf("expected YAML to contain 'kind: Pod', got: %s", string(nestedContent))
	}

	// Nested file must also have executable permissions.
	ninfo, err := os.Stat(nestedPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if ninfo.Mode().Perm() != 0o755 {
		t.Errorf("expected permissions 0755 on nested file, got %o", ninfo.Mode().Perm())
	}

	// SKILL.md must still exist alongside supporting files.
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("expected SKILL.md to still exist: %v", err)
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

func TestSkillsGenerate_IncludeFlag_FiltersSkills(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--include", "query|recommend")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	for _, name := range []string{"query", "recommend"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); err != nil {
			t.Errorf("expected skill %s to exist", name)
		}
	}

	for _, name := range []string{"remediate", "manageOrgData", "troubleshoot-pod"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("expected %s to be filtered out by include", name)
		}
	}
}

func TestSkillsGenerate_ExcludeFlag_FiltersSkills(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--exclude", "manage.*")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); err != nil {
		t.Error("expected query to exist")
	}

	for _, name := range []string{"manageOrgData", "manageKnowledge"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded", name)
		}
	}
}

func TestSkillsGenerate_IncludeAndExclude_Combined(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--include", ".*", "--exclude", "manage.*")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	for _, name := range []string{"query", "recommend", "remediate"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); err != nil {
			t.Errorf("expected skill %s to exist", name)
		}
	}

	for _, name := range []string{"manageOrgData", "manageKnowledge"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("expected %s to be excluded", name)
		}
	}
}

func TestSkillsGenerate_PersistedSettings_Respected(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.include", "query|recommend")
	if exitCode != 0 {
		t.Fatal("failed to set skills.include")
	}

	dir := t.TempDir()
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	for _, name := range []string{"query", "recommend"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+name, "SKILL.md")); err != nil {
			t.Errorf("expected skill %s to exist", name)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-remediate", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected remediate to be filtered out by persisted settings")
	}
}

func TestSkillsGenerate_FlagsOverrideSettings(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.include", "query")
	if exitCode != 0 {
		t.Fatal("failed to set skills.include")
	}

	dir := t.TempDir()
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--include", "recommend")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-recommend", "SKILL.md")); err != nil {
		t.Error("expected recommend to exist (flag override)")
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected query to be filtered out (flag overrides settings)")
	}
}

func TestSkillsGenerate_FiltersApplyToPrompts(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--include", "troubleshoot-pod")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
		t.Error("expected troubleshoot-pod prompt to exist")
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-explain-resource", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected explain-resource to be filtered out")
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected query tool to be filtered out")
	}
}

func TestSkillsGenerate_InvalidRegex_Error(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--include", "[invalid")
	if exitCode == 0 {
		t.Fatal("expected error for invalid regex")
	}
	if !strings.Contains(stderr, "invalid include pattern") {
		t.Errorf("expected error about invalid pattern; got: %s", stderr)
	}
}

func TestSkillsGenerate_Help_ShowsFilterFlags(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	if !strings.Contains(stdout, "--include") {
		t.Error("expected help to mention --include flag")
	}
	if !strings.Contains(stdout, "--exclude") {
		t.Error("expected help to mention --exclude flag")
	}
	if !strings.Contains(stdout, "DOT_AI_SKILLS_INCLUDE") {
		t.Error("expected help to mention DOT_AI_SKILLS_INCLUDE env var")
	}
	if !strings.Contains(stdout, "DOT_AI_SKILLS_EXCLUDE") {
		t.Error("expected help to mention DOT_AI_SKILLS_EXCLUDE env var")
	}
}

func TestSkillsGenerate_PullLatest_InHelp(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	if !strings.Contains(string(out), "--pull-latest") {
		t.Error("expected help to mention --pull-latest flag")
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

func TestSkillsGenerate_CustomOnlyFlag_SkipsToolSkills(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir, "--custom-only")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Prompt skills should exist.
	for _, prompt := range []string{"troubleshoot-pod", "explain-resource", "security-review", "optimize-resources"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+prompt, "SKILL.md")); err != nil {
			t.Errorf("expected prompt skill %s to exist", prompt)
		}
	}

	// Tool skills should NOT exist.
	for _, tool := range []string{"query", "recommend", "remediate", "operate", "manageOrgData", "manageKnowledge", "version", "projectSetup", "users"} {
		if _, err := os.Stat(filepath.Join(dir, "dot-ai-"+tool, "SKILL.md")); !os.IsNotExist(err) {
			t.Errorf("expected tool skill %s to be absent with --custom-only", tool)
		}
	}

	// Routing skill should still exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai", "SKILL.md")); err != nil {
		t.Error("expected routing skill to exist even with --custom-only")
	}
}

func TestSkillsGenerate_CustomOnlyEnvVar(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=true"}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Tool skills should NOT exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected tool skills to be absent when DOT_AI_SKILLS_CUSTOM_ONLY=true")
	}

	// Prompt skills should exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
		t.Error("expected prompt skills to exist")
	}
}

func TestSkillsGenerate_CustomOnlyPersistedSetting(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.custom_only", "true")
	if exitCode != 0 {
		t.Fatal("failed to set skills.custom_only")
	}

	dir := t.TempDir()
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected tool skills to be absent with persisted custom_only setting")
	}
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
		t.Error("expected prompt skills to exist")
	}
}

func TestSkillsGenerate_CustomOnlyWithIncludeExclude(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	env := []string{"HOME=" + home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY="}
	_, stderr, exitCode := runCLIWithEnv(t, env, "skills", "generate", "--path", dir,
		"--custom-only", "--include", "troubleshoot-pod")
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, stderr)
	}

	// Only troubleshoot-pod should exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-troubleshoot-pod", "SKILL.md")); err != nil {
		t.Error("expected troubleshoot-pod to exist")
	}

	// Other prompts should be filtered out.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-explain-resource", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected explain-resource to be filtered out by --include")
	}

	// Tool skills should not exist.
	if _, err := os.Stat(filepath.Join(dir, "dot-ai-query", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("expected tool skills to be absent with --custom-only")
	}
}

func TestSkillsGenerate_InstallHook_ForwardsCustomOnly(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only")
	cmd.Dir = dir
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, errBuf.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json: %v", err)
	}
	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	entry := sessionStart[0].(map[string]any)
	innerHooks := entry["hooks"].([]any)
	hook := innerHooks[0].(map[string]any)
	command := hook["command"].(string)
	if !strings.Contains(command, "--custom-only") {
		t.Errorf("expected hook command to contain --custom-only, got: %s", command)
	}
}

func TestSkillsGenerate_InstallHook_ForwardsIncludeExclude(t *testing.T) {
	home := t.TempDir()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook",
		"--include", "query|recommend", "--exclude", "manage.*")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+home, "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=")
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		t.Fatalf("expected exit 0, got %d; stderr: %s", exitCode, errBuf.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("expected settings.json: %v", err)
	}
	var settings map[string]any
	json.Unmarshal(data, &settings)
	hooks := settings["hooks"].(map[string]any)
	sessionStart := hooks["SessionStart"].([]any)
	entry := sessionStart[0].(map[string]any)
	innerHooks := entry["hooks"].([]any)
	hook := innerHooks[0].(map[string]any)
	command := hook["command"].(string)
	if !strings.Contains(command, "--include") {
		t.Errorf("expected hook command to contain --include, got: %s", command)
	}
	if !strings.Contains(command, "--exclude") {
		t.Errorf("expected hook command to contain --exclude, got: %s", command)
	}
}

func TestSkillsGenerate_InstallHook_ReplacesOnRerun(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)

	// First run without --custom-only.
	cmd := exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook")
	cmd.Dir = dir
	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run 1 failed; stderr: %s", errBuf.String())
	}

	// Second run with --custom-only should replace the hook.
	cmd = exec.Command(binaryPath, "--server-url", "http://localhost:3001",
		"skills", "generate", "--agent", "claude-code", "--install-hook", "--custom-only")
	cmd.Dir = dir
	errBuf.Reset()
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run 2 failed; stderr: %s", errBuf.String())
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
		t.Fatalf("expected exactly 1 SessionStart entry after replace, got %d", len(sessionStart))
	}
	entry := sessionStart[0].(map[string]any)
	innerHooks := entry["hooks"].([]any)
	if len(innerHooks) != 1 {
		t.Fatalf("expected exactly 1 inner hook after replace, got %d", len(innerHooks))
	}
	hook := innerHooks[0].(map[string]any)
	command := hook["command"].(string)
	if !strings.Contains(command, "--custom-only") {
		t.Errorf("expected replaced hook to contain --custom-only, got: %s", command)
	}
}

func TestSkillsGenerate_Help_ShowsCustomOnlyFlag(t *testing.T) {
	cmd := exec.Command(binaryPath, "skills", "generate", "--help")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected exit 0, got error: %v", err)
	}
	stdout := string(out)
	if !strings.Contains(stdout, "--custom-only") {
		t.Error("expected help to mention --custom-only flag")
	}
	if !strings.Contains(stdout, "DOT_AI_SKILLS_CUSTOM_ONLY") {
		t.Error("expected help to mention DOT_AI_SKILLS_CUSTOM_ONLY env var")
	}
}
