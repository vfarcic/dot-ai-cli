//go:build integration

package e2e_test

import (
	"strings"
	"testing"
)

func TestConfigSet_SetsValue(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	// Set a value.
	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "set", "server-url", "https://example.com")
	if exitCode != 0 {
		t.Fatalf("set: exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "server-url: https://example.com") {
		t.Errorf("set output = %q, want confirmation", stdout)
	}

	// Get it back.
	stdout, stderr, exitCode = runCLIWithEnv(t, env, "config", "get", "server-url")
	if exitCode != 0 {
		t.Fatalf("get: exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "https://example.com") {
		t.Errorf("get output = %q, want value", stdout)
	}
	if strings.Contains(stdout, "(default)") {
		t.Errorf("get output should not say (default) for an explicitly set value")
	}
}

func TestConfigGet_ReturnsDefault(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "get", "output-format")
	if exitCode != 0 {
		t.Fatalf("exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "yaml") || !strings.Contains(stdout, "(default)") {
		t.Errorf("output = %q, want 'yaml (default)'", stdout)
	}
}

func TestConfigGet_ReturnsNotSet(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "get", "skills.include")
	if exitCode != 0 {
		t.Fatalf("exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "(not set)") {
		t.Errorf("output = %q, want '(not set)'", stdout)
	}
}

func TestConfigList_ShowsAllKeys(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "list")
	if exitCode != 0 {
		t.Fatalf("exit %d; stderr: %s", exitCode, stderr)
	}
	for _, key := range []string{"server-url", "output-format", "skills.include", "skills.exclude", "skills.custom_only"} {
		if !strings.Contains(stdout, key) {
			t.Errorf("list output missing key %q; got: %s", key, stdout)
		}
	}
}

func TestConfigList_ShowsSetAndDefaultValues(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	// Set one key.
	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "server-url", "https://custom.example.com")
	if exitCode != 0 {
		t.Fatal("set failed")
	}

	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "list")
	if exitCode != 0 {
		t.Fatalf("exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "server-url: https://custom.example.com") {
		t.Errorf("expected set value in list; got: %s", stdout)
	}
	if !strings.Contains(stdout, "output-format: yaml (default)") {
		t.Errorf("expected default annotation in list; got: %s", stdout)
	}
}

func TestConfigReset_ClearsValue(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	// Set then reset.
	_, setStderr, setExit := runCLIWithEnv(t, env, "config", "set", "server-url", "https://example.com")
	if setExit != 0 {
		t.Fatalf("set before reset failed: %s", setStderr)
	}
	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "reset", "server-url")
	if exitCode != 0 {
		t.Fatalf("reset: exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "reset") {
		t.Errorf("reset output = %q, want confirmation", stdout)
	}

	// Verify it's gone.
	stdout, _, exitCode = runCLIWithEnv(t, env, "config", "get", "server-url")
	if exitCode != 0 {
		t.Fatal("get after reset failed")
	}
	if !strings.Contains(stdout, "(not set)") {
		t.Errorf("get after reset = %q, want '(not set)'", stdout)
	}
}

func TestConfigReset_WithDefault(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, setStderr, setExit := runCLIWithEnv(t, env, "config", "set", "output-format", "json")
	if setExit != 0 {
		t.Fatalf("set before reset failed: %s", setStderr)
	}
	stdout, stderr, exitCode := runCLIWithEnv(t, env, "config", "reset", "output-format")
	if exitCode != 0 {
		t.Fatalf("reset: exit %d; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "reset to default (yaml)") {
		t.Errorf("reset output = %q, want default mentioned", stdout)
	}

	stdout, _, _ = runCLIWithEnv(t, env, "config", "get", "output-format")
	if !strings.Contains(stdout, "yaml (default)") {
		t.Errorf("get after reset = %q, want 'yaml (default)'", stdout)
	}
}

func TestConfigSet_UnknownKey_Error(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, stderr, exitCode := runCLIWithEnv(t, env, "config", "set", "foo", "bar")
	if exitCode == 0 {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(stderr, "unknown key") {
		t.Errorf("stderr = %q, want 'unknown key'", stderr)
	}
	if !strings.Contains(stderr, "server-url") {
		t.Errorf("stderr should list valid keys; got: %s", stderr)
	}
}

func TestConfigGet_UnknownKey_Error(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, stderr, exitCode := runCLIWithEnv(t, env, "config", "get", "foo")
	if exitCode == 0 {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(stderr, "unknown key") {
		t.Errorf("stderr = %q, want 'unknown key'", stderr)
	}
}

func TestConfigReset_UnknownKey_Error(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, stderr, exitCode := runCLIWithEnv(t, env, "config", "reset", "foo")
	if exitCode == 0 {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(stderr, "unknown key") {
		t.Errorf("stderr = %q, want 'unknown key'", stderr)
	}
}

func TestConfigSet_SkillsInclude(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.include", "query|recommend")
	if exitCode != 0 {
		t.Fatal("set skills.include failed")
	}

	stdout, _, exitCode := runCLIWithEnv(t, env, "config", "get", "skills.include")
	if exitCode != 0 {
		t.Fatal("get skills.include failed")
	}
	if !strings.Contains(stdout, "query|recommend") {
		t.Errorf("get skills.include = %q, want 'query|recommend'", stdout)
	}
}

func TestConfigSet_SkillsExclude(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.exclude", "debug-.*")
	if exitCode != 0 {
		t.Fatal("set skills.exclude failed")
	}

	stdout, _, exitCode := runCLIWithEnv(t, env, "config", "get", "skills.exclude")
	if exitCode != 0 {
		t.Fatal("get skills.exclude failed")
	}
	if !strings.Contains(stdout, "debug-.*") {
		t.Errorf("get skills.exclude = %q, want 'debug-.*'", stdout)
	}
}

func TestConfigSet_SkillsCustomOnly(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, _, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.custom_only", "true")
	if exitCode != 0 {
		t.Fatal("set skills.custom_only failed")
	}

	stdout, _, exitCode := runCLIWithEnv(t, env, "config", "get", "skills.custom_only")
	if exitCode != 0 {
		t.Fatal("get skills.custom_only failed")
	}
	if !strings.Contains(stdout, "true") {
		t.Errorf("get skills.custom_only = %q, want 'true'", stdout)
	}
}

func TestConfigSet_SkillsCustomOnly_InvalidValue(t *testing.T) {
	home := t.TempDir()
	env := []string{"HOME=" + home, "XDG_CONFIG_HOME=", "DOT_AI_URL=", "DOT_AI_OUTPUT_FORMAT=", "DOT_AI_SKILLS_INCLUDE=", "DOT_AI_SKILLS_EXCLUDE=", "DOT_AI_SKILLS_CUSTOM_ONLY=", "DOT_AI_AUTH_TOKEN="}

	_, stderr, exitCode := runCLIWithEnv(t, env, "config", "set", "skills.custom_only", "maybe")
	if exitCode == 0 {
		t.Fatal("expected error for invalid skills.custom_only value")
	}
	if !strings.Contains(stderr, "must be") {
		t.Errorf("expected validation error, got: %s", stderr)
	}
}

func TestConfigHelp_NoServer(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "config", "--help")
	if exitCode != 0 {
		t.Fatal("config --help failed")
	}
	if !strings.Contains(stdout, "set") || !strings.Contains(stdout, "get") || !strings.Contains(stdout, "list") || !strings.Contains(stdout, "reset") {
		t.Errorf("help should mention all subcommands; got: %s", stdout)
	}
}
