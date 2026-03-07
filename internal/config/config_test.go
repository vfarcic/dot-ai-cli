package config

import (
	"testing"

	"github.com/vfarcic/dot-ai-cli/internal/auth"
)

// setConfigDir overrides the auth package's config directory for tests.
func setConfigDir(t *testing.T, dir string) {
	t.Helper()
	auth.SetConfigDirForTest(dir)
	t.Cleanup(func() { auth.ResetConfigDir() })
}

func TestResolveDefaults(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	// Clear env vars.
	for _, key := range []string{"DOT_AI_URL", "DOT_AI_AUTH_TOKEN", "DOT_AI_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.ServerURL != DefaultServerURL {
		t.Errorf("ServerURL = %q, want %q", c.ServerURL, DefaultServerURL)
	}
	if c.Token != "" {
		t.Errorf("Token = %q, want empty", c.Token)
	}
	if c.OutputFormat != DefaultOutputFormat {
		t.Errorf("OutputFormat = %q, want %q", c.OutputFormat, DefaultOutputFormat)
	}
}

func TestResolveFromFiles(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	for _, key := range []string{"DOT_AI_URL", "DOT_AI_AUTH_TOKEN", "DOT_AI_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}

	s := auth.Settings{ServerURL: "https://file.example.com", OutputFormat: "json"}
	if err := s.Save(); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
	cr := auth.Credentials{AuthToken: "file-token"}
	if err := cr.Save(); err != nil {
		t.Fatalf("Save credentials: %v", err)
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.ServerURL != "https://file.example.com" {
		t.Errorf("ServerURL = %q, want %q", c.ServerURL, "https://file.example.com")
	}
	if c.Token != "file-token" {
		t.Errorf("Token = %q, want %q", c.Token, "file-token")
	}
	if c.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", c.OutputFormat, "json")
	}
}

func TestResolveEnvOverridesFiles(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	t.Setenv("DOT_AI_URL", "https://env.example.com")
	t.Setenv("DOT_AI_AUTH_TOKEN", "env-token")
	t.Setenv("DOT_AI_OUTPUT_FORMAT", "json")

	// Write file values that should be overridden.
	s := auth.Settings{ServerURL: "https://file.example.com", OutputFormat: "yaml"}
	if err := s.Save(); err != nil {
		t.Fatalf("Save settings: %v", err)
	}
	cr := auth.Credentials{AuthToken: "file-token"}
	if err := cr.Save(); err != nil {
		t.Fatalf("Save credentials: %v", err)
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.ServerURL != "https://env.example.com" {
		t.Errorf("ServerURL = %q, want %q", c.ServerURL, "https://env.example.com")
	}
	if c.Token != "env-token" {
		t.Errorf("Token = %q, want %q", c.Token, "env-token")
	}
	if c.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", c.OutputFormat, "json")
	}
}

func TestResolveFlagsOverrideAll(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	t.Setenv("DOT_AI_URL", "https://env.example.com")
	t.Setenv("DOT_AI_AUTH_TOKEN", "env-token")
	t.Setenv("DOT_AI_OUTPUT_FORMAT", "json")

	c := Config{
		ServerURL:    "https://flag.example.com",
		Token:        "flag-token",
		OutputFormat: "yaml",
	}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.ServerURL != "https://flag.example.com" {
		t.Errorf("ServerURL = %q, want %q", c.ServerURL, "https://flag.example.com")
	}
	if c.Token != "flag-token" {
		t.Errorf("Token = %q, want %q", c.Token, "flag-token")
	}
	if c.OutputFormat != "yaml" {
		t.Errorf("OutputFormat = %q, want %q", c.OutputFormat, "yaml")
	}
}

func TestResolveOAuthTokenFallback(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	for _, key := range []string{"DOT_AI_URL", "DOT_AI_AUTH_TOKEN", "DOT_AI_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}

	// No auth_token, but valid OAuth token.
	cr := auth.Credentials{
		AccessToken: "oauth-token",
		ExpiresAt:   "2099-01-01T00:00:00Z",
	}
	if err := cr.Save(); err != nil {
		t.Fatalf("Save credentials: %v", err)
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.Token != "oauth-token" {
		t.Errorf("Token = %q, want %q", c.Token, "oauth-token")
	}
}

func TestResolveExpiredOAuthTokenSkipped(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	for _, key := range []string{"DOT_AI_URL", "DOT_AI_AUTH_TOKEN", "DOT_AI_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}

	cr := auth.Credentials{
		AccessToken: "expired-token",
		ExpiresAt:   "2020-01-01T00:00:00Z",
	}
	if err := cr.Save(); err != nil {
		t.Fatalf("Save credentials: %v", err)
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.Token != "" {
		t.Errorf("Token = %q, want empty (expired)", c.Token)
	}
}

func TestResolveAuthTokenTakesPriorityOverOAuth(t *testing.T) {
	dir := t.TempDir()
	setConfigDir(t, dir)

	for _, key := range []string{"DOT_AI_URL", "DOT_AI_AUTH_TOKEN", "DOT_AI_OUTPUT_FORMAT"} {
		t.Setenv(key, "")
	}

	cr := auth.Credentials{
		AuthToken:   "static-token",
		AccessToken: "oauth-token",
		ExpiresAt:   "2099-01-01T00:00:00Z",
	}
	if err := cr.Save(); err != nil {
		t.Fatalf("Save credentials: %v", err)
	}

	c := Config{}
	if err := c.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if c.Token != "static-token" {
		t.Errorf("Token = %q, want %q", c.Token, "static-token")
	}
}

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt string
		want      bool
	}{
		{"empty", "", true},
		{"invalid", "not-a-date", true},
		{"past", "2020-01-01T00:00:00Z", true},
		{"future", "2099-01-01T00:00:00Z", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isExpired(tt.expiresAt); got != tt.want {
				t.Errorf("isExpired(%q) = %v, want %v", tt.expiresAt, got, tt.want)
			}
		})
	}
}

