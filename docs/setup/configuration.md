# Configuration

Configure the CLI to connect to your DevOps AI Toolkit server.

## Server URL

Specify the server address:

**Environment variable:**
```bash
export DOT_AI_URL="https://your-server-url"
```

**Command-line flag:**
```bash
dot-ai query "test" --server-url https://your-server-url
```

**Default:** `http://localhost:3456`

## Authentication

Set the authentication token:

**Environment variable:**
```bash
export DOT_AI_AUTH_TOKEN="your-token-here"
```

**Command-line flag:**
```bash
dot-ai query "test" --token your-token-here
```

**Default:** No authentication (for local development)

## Output Format

Choose the output format:

**Environment variable:**
```bash
export DOT_AI_OUTPUT_FORMAT="json"  # or "yaml"
```

**Command-line flag:**
```bash
dot-ai query "test" --output json
```

**Default:** `yaml`

**Options:**
- `yaml` — Human-readable, structured output (default)
- `json` — Machine-parseable, raw API response

## Persistent Configuration Files

The CLI stores settings and credentials in `~/.config/dot-ai/` with restricted permissions (owner-only access).

**`settings.json`** — user preferences:
```json
{
  "server_url": "https://dot-ai.example.com",
  "output_format": "json"
}
```

**`credentials.json`** — authentication state:
```json
{
  "auth_token": "your-static-token"
}
```

OAuth fields (`access_token`, `token_type`, `expires_at`, `client_id`, `client_secret`) are managed automatically by `dot-ai auth login` and `dot-ai auth logout`. See [Authentication](authentication.md) for details.

## Config Command

Manage persistent settings with the `config` command instead of editing JSON files:

```bash
# Set a value
dot-ai config set server-url https://dot-ai.example.com
dot-ai config set output-format json
dot-ai config set skills.include "query|recommend|remediate"
dot-ai config set skills.exclude "debug-.*"

# Get a value
dot-ai config get server-url

# List all settings (always shows all known keys)
dot-ai config list

# Reset a value to its default
dot-ai config reset server-url
```

**Supported keys:**

| Key | Description | Default |
|-----|-------------|---------|
| `server-url` | Server URL | (not set) |
| `output-format` | Output format (json, yaml) | `yaml` |
| `skills.include` | Regex for skills to include | (not set) |
| `skills.exclude` | Regex for skills to exclude | (not set) |
| `skills.custom_only` | Only generate custom skills, skip MCP tools (true/false) | (not set) |

Unknown keys are rejected with an error listing all valid keys.

## Configuration Precedence

Settings are applied in this order (highest to lowest priority):

| Setting | Flag | Env var | Config file | Default |
|---------|------|---------|-------------|---------|
| Server URL | `--server-url` | `DOT_AI_URL` | `settings.json` `server_url` | `http://localhost:3456` |
| Auth token | `--token` | `DOT_AI_AUTH_TOKEN` | `credentials.json` `auth_token` / `access_token` | none |
| Output format | `--output` | `DOT_AI_OUTPUT_FORMAT` | `settings.json` `output_format` | `yaml` |
| Skills include | `--include` | `DOT_AI_SKILLS_INCLUDE` | `settings.json` `skills_include` | none |
| Skills exclude | `--exclude` | `DOT_AI_SKILLS_EXCLUDE` | `settings.json` `skills_exclude` | none |
| Skills custom only | `--custom-only` | `DOT_AI_SKILLS_CUSTOM_ONLY` | `settings.json` `skills_custom_only` | none |

For auth tokens specifically, `auth_token` (static) takes priority over `access_token` (OAuth) in the credentials file. Expired OAuth tokens are skipped.

## Example Configuration

**For local development:**
```bash
# No configuration needed - defaults work
dot-ai version
```

**For remote server:**
```bash
# Set once in your shell profile
export DOT_AI_URL="https://dot-ai.example.com"
export DOT_AI_AUTH_TOKEN="your-token"

# Then use normally
dot-ai query "what pods are running?"
```

**Using the config command:**
```bash
# Set persistent values
dot-ai config set server-url https://dot-ai.example.com
dot-ai config set output-format json

# Then use normally
dot-ai query "what pods are running?"
```

**For multiple environments:**
```bash
# Development
DOT_AI_URL="https://dev.example.com" dot-ai query "test"

# Production
DOT_AI_URL="https://prod.example.com" DOT_AI_AUTH_TOKEN="prod-token" dot-ai query "test"
```

## Next Steps

- **[Authentication](authentication.md)** — OAuth login flow and token management
- **[Shell Completion](shell-completion.md)** — Enable command autocompletion
- **[Commands Overview](../guides/cli-commands-overview.md)** — See all available commands
- **[Automation](../guides/automation.md)** — Use in scripts and CI/CD
