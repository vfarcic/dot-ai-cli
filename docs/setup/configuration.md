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

## Configuration Precedence

Settings are applied in this order (highest to lowest priority):

| Setting | Flag | Env var | Config file | Default |
|---------|------|---------|-------------|---------|
| Server URL | `--server-url` | `DOT_AI_URL` | `settings.json` `server_url` | `http://localhost:3456` |
| Auth token | `--token` | `DOT_AI_AUTH_TOKEN` | `credentials.json` `auth_token` / `access_token` | none |
| Output format | `--output` | `DOT_AI_OUTPUT_FORMAT` | `settings.json` `output_format` | `yaml` |

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

**Using config files (no env vars needed):**
```bash
# Write once
mkdir -p ~/.config/dot-ai && chmod 700 ~/.config/dot-ai
echo '{"server_url":"https://dot-ai.example.com","output_format":"json"}' > ~/.config/dot-ai/settings.json
echo '{"auth_token":"your-token"}' > ~/.config/dot-ai/credentials.json
chmod 600 ~/.config/dot-ai/*.json

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
