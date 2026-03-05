package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// configDirFunc can be overridden in tests.
var configDirFunc = defaultConfigDir

// Settings holds durable user preferences stored in settings.json.
type Settings struct {
	ServerURL    string `json:"server_url,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

func defaultConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "dot-ai")
}

// ConfigDir returns the path to ~/.config/dot-ai/.
func ConfigDir() string {
	return configDirFunc()
}

// SettingsPath returns the path to the settings file.
func SettingsPath() string {
	return filepath.Join(ConfigDir(), "settings.json")
}

// LoadSettings reads settings from disk. Returns zero-value Settings if the
// file does not exist.
func LoadSettings() (Settings, error) {
	var s Settings
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

// Save writes settings to disk with 0600 permissions, creating the config
// directory if needed.
func (s *Settings) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), data, 0600)
}
