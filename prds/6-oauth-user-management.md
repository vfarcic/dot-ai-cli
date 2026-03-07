# PRD #6: OAuth Authentication & User Management Commands

**Status:** Complete
**Priority:** High
**GitHub Issue:** [#6](https://github.com/vfarcic/dot-ai-cli/issues/6)
**Created:** 2026-03-03
**Related:** [dot-ai PRD #380](https://github.com/vfarcic/dot-ai/issues/380) — Gateway Auth RBAC (parent)

## Problem Statement

The CLI currently only supports static bearer tokens (`--token` / `DOT_AI_AUTH_TOKEN`). The dot-ai server now supports OAuth via Dex (PRD #380), providing individual user identity, audit trails, and enterprise IdP integration. Without OAuth support in the CLI:

- CLI users cannot authenticate with individual identity — they share one token
- No way to use enterprise SSO (Google, GitHub, LDAP) from the CLI
- User management commands exist in the REST API but lack CLI documentation and workflow guidance
- The CLI falls behind MCP clients (Claude Code, ChatGPT) that already support OAuth

## Solution Overview

Add OAuth browser-based authentication to the CLI alongside the existing static token auth:

```
dot-ai auth login
  ↓
Opens browser → Dex login page → User authenticates
  ↓
Local callback server receives token → Stored in ~/.config/dot-ai/credentials.json
  ↓
Subsequent commands use stored OAuth token automatically
```

User management commands (`dot-ai users create`, `dot-ai users list`, `dot-ai users delete`) are already auto-generated from the OpenAPI spec. This PRD adds the auth flow and documentation.

## User Experience

```bash
# OAuth login (opens browser for Dex login)
dot-ai auth login
# → Opening browser for authentication...
# → Authenticated as alice@example.com

# Check current auth status
dot-ai auth status
# → Authenticated via: OAuth
# → User: alice@example.com
# → Token expires: 2026-03-03T15:00:00Z

# Logout (clear stored tokens)
dot-ai auth logout
# → Logged out. Stored credentials removed.

# User management (auto-generated from OpenAPI — already works with --token)
dot-ai users create --email bob@example.com --password "..."
dot-ai users list
dot-ai users delete --email bob@example.com

# Auth precedence: --token flag > DOT_AI_AUTH_TOKEN env > credentials.json auth_token > OAuth token
dot-ai query "what pods are running?"          # Uses stored OAuth or static token
dot-ai query "what pods are running?" --token x # Overrides with static token
```

## Architecture

### Auth Flow

The CLI implements the OAuth Authorization Code flow with PKCE (same flow MCP clients use):

1. CLI starts a temporary local HTTP server on a random port (e.g., `http://localhost:PORT/callback`)
2. CLI opens the browser to `{server-url}/authorize?response_type=code&client_id=...&redirect_uri=http://localhost:PORT/callback&code_challenge=...&code_challenge_method=S256`
3. Server redirects to Dex → user logs in → Dex redirects back to server `/callback`
4. Server redirects to CLI's local callback with authorization code
5. CLI exchanges code for access token at `{server-url}/token`
6. CLI stores token in `~/.config/dot-ai/credentials.json`
7. Local HTTP server shuts down

**Note:** The CLI registers as a dynamic OAuth client via the server's `/register` endpoint (RFC 7591) before starting the flow.

### Persistent Configuration: Two-File Split

The CLI uses two files under `~/.config/dot-ai/` to separate durable settings from auth state:

#### `~/.config/dot-ai/settings.json` — Durable Configuration

```json
{
  "server_url": "https://dot-ai.example.com",
  "output_format": "yaml"
}
```

- Persistent alternative to env vars and flags for `server_url` and `output_format`
- File permissions: `0600` (owner read/write only)
- Never modified by `auth login` or `auth logout`

#### `~/.config/dot-ai/credentials.json` — All Auth State

```json
{
  "auth_token": "static-bearer-token",
  "access_token": "eyJhbG...",
  "token_type": "Bearer",
  "expires_at": "2026-03-03T15:00:00Z",
  "client_id": "cli-abc123",
  "client_secret": "..."
}
```

- `auth_token`: static bearer token (alternative to `--token` / `DOT_AI_AUTH_TOKEN`)
- `access_token`, `token_type`, `expires_at`, `client_id`, `client_secret`: OAuth session state written by `auth login`
- File permissions: `0600` (owner read/write only)
- `auth logout` clears only OAuth fields (`access_token`, `token_type`, `expires_at`, `client_id`, `client_secret`) — leaves `auth_token` intact
- Token refresh: if expired, prompt user to re-authenticate (`dot-ai auth login`)
- `server_url` is NOT stored here — it lives only in `settings.json`

### Configuration Precedence

The CLI resolves each configuration value independently:

**Server URL:**
1. `--server-url` flag (highest priority)
2. `DOT_AI_URL` environment variable
3. `settings.json` → `server_url`
4. Default: `http://localhost:3456`

**Authentication Token:**
1. `--token` flag (highest priority)
2. `DOT_AI_AUTH_TOKEN` environment variable
3. `credentials.json` → `auth_token` (static token)
4. `credentials.json` → `access_token` (OAuth, if valid and not expired)
5. No authentication

**Output Format:**
1. `--output` flag (highest priority)
2. `DOT_AI_OUTPUT_FORMAT` environment variable
3. `settings.json` → `output_format`
4. Default: `yaml`

### Auto-Generated User Management Commands

The server's OpenAPI spec includes user management endpoints. The CLI auto-generates these commands:

| Endpoint | CLI Command | Notes |
|----------|------------|-------|
| `POST /api/v1/users` | `dot-ai users create --email X --password Y` | Create Dex static user |
| `GET /api/v1/users` | `dot-ai users list` | List user emails |
| `DELETE /api/v1/users/:email` | `dot-ai users delete --email X` | Delete user |

These commands work with both OAuth and static token auth.

## Milestones

### Milestone 1: Persistent Configuration (settings.json + credentials.json)

- [x] Implement `internal/auth/settings.go` — Load/Save `~/.config/dot-ai/settings.json` (`server_url`, `output_format`) with `0600` permissions
- [x] Implement `internal/auth/credentials.go` — Load/Save/ClearOAuth `~/.config/dot-ai/credentials.json` (`auth_token`, OAuth fields) with `0600` permissions
- [x] Update `internal/config/config.go` — new precedence: flags > env > settings.json/credentials.json > defaults
- [x] Unit tests for settings load/save, credentials load/save/clear, and updated precedence

### Milestone 2: Auth Commands (login/logout/status)

- [x] Implement `dot-ai auth login` — dynamic client registration, PKCE flow, browser open, local callback server, token storage to credentials.json
- [x] Implement `dot-ai auth logout` — clear only OAuth fields from credentials.json (leave `auth_token` intact)
- [x] Implement `dot-ai auth status` — show current auth mode, user identity, token expiry
- [x] Integration tests for auth commands

### Milestone 3: Generic REST Subcommand Grouping

- [x] Update `cmd/dynamic.go` `registerCommands` — detect top-level name collisions and group them as subcommands with friendly method-derived names
- [x] Method mapping: GET→`list` (or `get` with path param), POST→`create`, DELETE→`delete`, PUT→`update`, PATCH→`patch`
- [x] For DELETE with path param (e.g., `/users/{email}`), register as `users delete <email>` subcommand
- [x] Integration tests for `users list`, `users create`, `users delete`

### Milestone 4: Manual Testing

- [x] Deploy dot-ai server with Dex enabled to a test cluster
- [x] Verify `settings.json` precedence: set `server_url` in settings.json, confirm CLI uses it without `--server-url` or `DOT_AI_URL`
- [x] Verify `credentials.json` static token: set `auth_token`, confirm CLI authenticates without `--token` or `DOT_AI_AUTH_TOKEN`
- [x] Verify flag/env overrides: confirm `--token` and `DOT_AI_AUTH_TOKEN` take priority over file-based values
- [x] Run `dot-ai auth login` — confirm browser opens, Dex login completes, token stored in `credentials.json`
- [x] Run `dot-ai auth status` — confirm it shows OAuth user identity and token expiry
- [x] Run authenticated commands (`dot-ai query`, `dot-ai users list`) using stored OAuth token
- [x] Run `dot-ai auth logout` — confirm OAuth fields cleared, `auth_token` preserved if set
- [x] Verify expired token handling: confirm CLI prompts to re-authenticate

### Milestone 5: Documentation

- [x] New `docs/setup/authentication.md` — OAuth login flow, static token, auth precedence, troubleshooting
- [x] New `docs/guides/user-management.md` — create/list/delete users via CLI, when to use static users vs IdP connectors
- [x] Update `docs/setup/configuration.md` — add settings.json, credentials.json, auth precedence section

### Milestone 6: Feature Request to dot-ai

- [x] Send feature request to `dot-ai` project: update `docs/ai-engine/setup/authentication.md` to link to CLI-specific user management page (`https://devopstoolkit.ai/docs/cli/guides/user-management`)

## Design Decisions

| # | Decision | Date | Rationale |
|---|----------|------|-----------|
| 1 | Split config into `settings.json` (durable config) and `credentials.json` (auth state) | 2026-03-03 | OAuth session state is ephemeral (login creates, logout clears) while server URL and output format are persistent user preferences. Mixing them in one file creates awkward logout semantics. |
| 2 | `server_url` lives only in `settings.json`, not in `credentials.json` | 2026-03-03 | The CLI resolves server URL from the standard precedence chain. OAuth tokens are validated against the resolved URL at use time. If the user changes servers, they re-authenticate — correct behavior. |
| 3 | Static `auth_token` lives in `credentials.json`, not `settings.json` | 2026-03-03 | A bearer token is a credential, not a setting. All auth state belongs together in `credentials.json`. |
| 4 | `auth logout` clears only OAuth fields, not `auth_token` | 2026-03-03 | The user sets `auth_token` deliberately as a static credential. Logout should only clear the OAuth session, not destroy unrelated auth config. |
| 5 | New precedence: flags > env vars > settings.json/credentials.json > defaults | 2026-03-03 | `settings.json` and `credentials.json` provide a persistent alternative to env vars, reducing the need for shell configuration. Existing flag and env var behavior is unchanged. |
| 6 | Generic REST method-to-subcommand grouping for multi-method paths | 2026-03-06 | When multiple HTTP methods exist on the same path (e.g., GET+POST `/users`), group them as subcommands (`users list`, `users create`) instead of appending method names (`users-get`, `users-post`). Applies generically to any resource. |

## Dependencies

| Dependency | Status | Notes |
|------------|--------|-------|
| dot-ai OAuth endpoints (PRD #380) | Complete | Server serves OAuth metadata, authorize, token, register |
| User management in OpenAPI spec | Complete | `POST/GET /api/v1/users`, `DELETE /api/v1/users/:email` |
| Dex as OIDC provider | Complete | Ships as Helm subchart with dot-ai |

## Success Criteria

- `dot-ai auth login` opens browser, completes OAuth flow, stores token
- Subsequent commands use stored OAuth token automatically (no `--token` flag needed)
- `dot-ai auth status` shows user identity (email, groups) for OAuth users
- `dot-ai users list/create/delete` work for managing Dex static users
- Static token auth (`--token`, `DOT_AI_AUTH_TOKEN`) continues to work unchanged
- Documentation covers both auth modes and user management workflows
