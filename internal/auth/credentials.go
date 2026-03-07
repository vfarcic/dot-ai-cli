package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Credentials holds all authentication state stored in credentials.json.
type Credentials struct {
	// Static bearer token (alternative to --token / DOT_AI_AUTH_TOKEN).
	AuthToken string `json:"auth_token,omitempty"`

	// OAuth session state (written by auth login, cleared by auth logout).
	AccessToken  string `json:"access_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// CredentialsPath returns the path to the credentials file.
func CredentialsPath() string {
	return filepath.Join(ConfigDir(), "credentials.json")
}

// LoadCredentials reads credentials from disk. Returns zero-value Credentials
// if the file does not exist.
func LoadCredentials() (Credentials, error) {
	var c Credentials
	data, err := os.ReadFile(CredentialsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}

// Save writes credentials to disk with 0600 permissions, creating the config
// directory if needed.
func (c *Credentials) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	path := CredentialsPath()
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return os.Chmod(path, 0600)
}

// ClearOAuth removes only OAuth session fields, leaving auth_token intact.
func (c *Credentials) ClearOAuth() {
	c.AccessToken = ""
	c.TokenType = ""
	c.ExpiresAt = ""
	c.ClientID = ""
	c.ClientSecret = ""
}
