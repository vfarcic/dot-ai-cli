# Authentication

Authenticate the CLI to access protected DevOps AI Toolkit server endpoints.

Before using CLI authentication, ensure the server has authentication enabled — see the [server authentication setup](https://devopstoolkit.ai/docs/ai-engine/setup/authentication).

## OAuth Browser Login

The recommended authentication method uses OAuth:

```bash
dot-ai auth login
```

Opens your browser to the login page. After you authenticate, the token is stored in `~/.config/dot-ai/credentials.json` and used automatically for subsequent commands.

### Headless / SSH Environments

If a browser is not available, use `--no-browser`:

```bash
dot-ai auth login --no-browser
```

The CLI prints the authorization URL instead of opening a browser. Copy the URL to a machine with a browser and complete the login there.

## Static Token Authentication

For scripts, CI/CD, or environments where interactive login is impractical, use a static bearer token:

**Command-line flag:**
```bash
dot-ai query "test" --token your-token-here
```

**Environment variable:**
```bash
export DOT_AI_AUTH_TOKEN="your-token-here"
dot-ai query "test"
```

**Configuration file** (`~/.config/dot-ai/credentials.json`):
```json
{
  "auth_token": "your-token-here"
}
```

See [Configuration](configuration.md) for the full precedence order.

## Checking Auth Status

View your current authentication state:

```bash
dot-ai auth status
```

**OAuth session:**
```
Authenticated via: OAuth
Token: eyJhbGci...abcd
Token expires: 2026-03-08T12:00:00Z
Status: Valid
```

**Static token:**
```
Authenticated via: Static token
Token: eyJhbGci...wxyz
```

**Not authenticated:**
```
Not authenticated.
Run 'dot-ai auth login' or set --token / DOT_AI_AUTH_TOKEN.
```

## Logging Out

Clear stored OAuth credentials:

```bash
dot-ai auth logout
```

This removes only the OAuth session fields from `credentials.json`. Any static `auth_token` is preserved.

## Token Expiry

OAuth access tokens have a limited lifetime. When a token expires, `auth status` shows:

```
Status: EXPIRED — run 'dot-ai auth login' to re-authenticate
```

Re-run `dot-ai auth login` to obtain a fresh token.

## Token Precedence

When multiple token sources are configured, the CLI uses the first match:

1. `--token` flag
2. `DOT_AI_AUTH_TOKEN` environment variable
3. `auth_token` in `credentials.json` (static token)
4. `access_token` in `credentials.json` (OAuth, if not expired)

## Troubleshooting

**Browser does not open:**
The CLI falls back to printing the URL. Copy it manually or use `--no-browser`. On Linux, ensure `xdg-open` is installed.

**Authentication times out (5 minutes):**
Re-run `dot-ai auth login`. Ensure your browser can reach `http://localhost` on the port shown in the output.

**Firewall blocks the callback:**
The CLI listens on `127.0.0.1` on a random port. Ensure your firewall allows localhost connections. If behind a corporate proxy, try `--no-browser` and complete the flow from a less restricted machine.

**"client registration failed" error:**
Verify the server URL is correct and the server is running.

## Next Steps

- **[Configuration](configuration.md)** — Full configuration precedence and persistent settings
- **[User Management](../guides/user-management.md)** — Create and manage Dex users
- **[Commands Overview](../guides/cli-commands-overview.md)** — See all available commands
