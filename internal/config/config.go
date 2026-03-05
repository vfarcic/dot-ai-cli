package config

import (
	"os"
	"time"

	"github.com/vfarcic/dot-ai-cli/internal/auth"
)

const (
	DefaultServerURL    = "http://localhost:3456"
	DefaultOutputFormat = "yaml"
)

type Config struct {
	ServerURL    string
	Token        string
	OutputFormat string
}

// Resolve applies configuration precedence:
// flags > env vars > settings.json/credentials.json > defaults.
//
// Flag values are already set on the struct by cobra. If a flag was not
// provided (empty string), we fall back to env, then file, then default.
func (c *Config) Resolve() {
	settings, _ := auth.LoadSettings()
	creds, _ := auth.LoadCredentials()

	// Server URL: flag > env > settings.json > default
	if c.ServerURL == "" {
		if v := os.Getenv("DOT_AI_URL"); v != "" {
			c.ServerURL = v
		} else if settings.ServerURL != "" {
			c.ServerURL = settings.ServerURL
		} else {
			c.ServerURL = DefaultServerURL
		}
	}

	// Token: flag > env > credentials.json auth_token > credentials.json access_token (if valid) > none
	if c.Token == "" {
		if v := os.Getenv("DOT_AI_AUTH_TOKEN"); v != "" {
			c.Token = v
		} else if creds.AuthToken != "" {
			c.Token = creds.AuthToken
		} else if creds.AccessToken != "" && !isExpired(creds.ExpiresAt) {
			c.Token = creds.AccessToken
		}
	}

	// Output format: flag > env > settings.json > default
	if c.OutputFormat == "" {
		if v := os.Getenv("DOT_AI_OUTPUT_FORMAT"); v != "" {
			c.OutputFormat = v
		} else if settings.OutputFormat != "" {
			c.OutputFormat = settings.OutputFormat
		} else {
			c.OutputFormat = DefaultOutputFormat
		}
	}
}

// isExpired checks whether the given RFC 3339 timestamp is in the past.
// Returns true (expired) if the value is empty or unparseable, so that
// callers skip unusable tokens.
func isExpired(expiresAt string) bool {
	if expiresAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return true
	}
	return time.Now().After(t)
}
