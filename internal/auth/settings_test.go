package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	origFunc := configDirFunc
	configDirFunc = func() string { return dir }
	defer func() { configDirFunc = origFunc }()

	s := Settings{ServerURL: "https://example.com", OutputFormat: "json"}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify file permissions.
	info, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("permissions = %o, want 0600", perm)
	}

	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.ServerURL != "https://example.com" {
		t.Errorf("ServerURL = %q, want %q", loaded.ServerURL, "https://example.com")
	}
	if loaded.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", loaded.OutputFormat, "json")
	}
}

func TestLoadSettingsMissingFile(t *testing.T) {
	dir := t.TempDir()
	origFunc := configDirFunc
	configDirFunc = func() string { return dir }
	defer func() { configDirFunc = origFunc }()

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if s.ServerURL != "" || s.OutputFormat != "" {
		t.Errorf("expected zero-value Settings, got %+v", s)
	}
}

func TestLoadSettingsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	origFunc := configDirFunc
	configDirFunc = func() string { return dir }
	defer func() { configDirFunc = origFunc }()

	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSettings()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
