package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	origFunc := configDirFunc
	configDirFunc = func() string { return dir }
	defer func() { configDirFunc = origFunc }()

	c := Credentials{
		AuthToken:    "static-token",
		AccessToken:  "oauth-token",
		TokenType:    "Bearer",
		ExpiresAt:    "2030-01-01T00:00:00Z",
		ClientID:     "cli-123",
		ClientSecret: "secret",
	}
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file permissions.
	info, err := os.Stat(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	loaded, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if loaded.AuthToken != "static-token" {
		t.Errorf("AuthToken = %q, want %q", loaded.AuthToken, "static-token")
	}
	if loaded.AccessToken != "oauth-token" {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, "oauth-token")
	}
}

func TestLoadCredentialsMissingFile(t *testing.T) {
	dir := t.TempDir()
	origFunc := configDirFunc
	configDirFunc = func() string { return dir }
	defer func() { configDirFunc = origFunc }()

	c, err := LoadCredentials()
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if c.AuthToken != "" || c.AccessToken != "" {
		t.Errorf("expected zero-value Credentials, got %+v", c)
	}
}

func TestClearOAuth(t *testing.T) {
	c := Credentials{
		AuthToken:    "keep-this",
		AccessToken:  "oauth-token",
		TokenType:    "Bearer",
		ExpiresAt:    "2030-01-01T00:00:00Z",
		ClientID:     "cli-123",
		ClientSecret: "secret",
	}

	c.ClearOAuth()

	if c.AuthToken != "keep-this" {
		t.Errorf("AuthToken should be preserved, got %q", c.AuthToken)
	}
	if c.AccessToken != "" || c.TokenType != "" || c.ExpiresAt != "" || c.ClientID != "" || c.ClientSecret != "" {
		t.Errorf("OAuth fields should be cleared, got %+v", c)
	}
}
