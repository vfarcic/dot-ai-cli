package config

import "os"

const (
	DefaultServerURL    = "http://localhost:3456"
	DefaultOutputFormat = "yaml"
)

type Config struct {
	ServerURL    string
	Token        string
	OutputFormat string
}

// Resolve applies configuration precedence: flags > env vars > defaults.
// Flag values are already set on the struct by cobra. If a flag was not
// provided (empty string), we fall back to the environment variable,
// then to the default.
func (c *Config) Resolve() {
	if c.ServerURL == "" {
		c.ServerURL = envOrDefault("DOT_AI_URL", DefaultServerURL)
	}
	if c.Token == "" {
		c.Token = os.Getenv("DOT_AI_AUTH_TOKEN")
	}
	if c.OutputFormat == "" {
		c.OutputFormat = envOrDefault("DOT_AI_OUTPUT_FORMAT", DefaultOutputFormat)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
