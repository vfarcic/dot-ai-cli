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

## Configuration Precedence

Settings are applied in this order (highest to lowest priority):

1. **Command-line flags** (`--server-url`, `--token`, `--output`)
2. **Environment variables** (`DOT_AI_URL`, `DOT_AI_AUTH_TOKEN`, `DOT_AI_OUTPUT_FORMAT`)
3. **Defaults** (`http://localhost:3456`, no token, `yaml`)

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

**For multiple environments:**
```bash
# Development
DOT_AI_URL="https://dev.example.com" dot-ai query "test"

# Production
DOT_AI_URL="https://prod.example.com" DOT_AI_AUTH_TOKEN="prod-token" dot-ai query "test"
```

## Next Steps

- **[Shell Completion](shell-completion.md)** — Enable command autocompletion
- **[Commands Overview](../guides/cli-commands-overview.md)** — See all available commands
- **[Automation](../guides/automation.md)** — Use in scripts and CI/CD
