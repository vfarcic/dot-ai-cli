# PRD #6: OAuth Authentication & User Management Commands

**Status:** Draft
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

# Auth precedence: --token flag > stored OAuth token > DOT_AI_AUTH_TOKEN env
dot-ai query "what pods are running?"          # Uses stored OAuth token
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

### Token Storage

```json
// ~/.config/dot-ai/credentials.json
{
  "server_url": "https://dot-ai.example.com",
  "access_token": "eyJhbG...",
  "token_type": "Bearer",
  "expires_at": "2026-03-03T15:00:00Z",
  "client_id": "cli-abc123",
  "client_secret": "..."
}
```

- File permissions: `0600` (owner read/write only)
- One credential set per server URL (supports multiple servers)
- Token refresh: if expired, prompt user to re-authenticate (`dot-ai auth login`)

### Auth Precedence

The CLI resolves authentication in this order:
1. `--token` flag (static token, highest priority)
2. Stored OAuth token from `~/.config/dot-ai/credentials.json` (if valid and not expired)
3. `DOT_AI_AUTH_TOKEN` environment variable (static token fallback)
4. No authentication (for unauthenticated local development)

### Auto-Generated User Management Commands

The server's OpenAPI spec includes user management endpoints. The CLI auto-generates these commands:

| Endpoint | CLI Command | Notes |
|----------|------------|-------|
| `POST /api/v1/users` | `dot-ai users create --email X --password Y` | Create Dex static user |
| `GET /api/v1/users` | `dot-ai users list` | List user emails |
| `DELETE /api/v1/users/:email` | `dot-ai users delete --email X` | Delete user |

These commands work with both OAuth and static token auth.

## Milestones

### Milestone 1: Auth Commands (login/logout/status)

- [ ] Implement `dot-ai auth login` — dynamic client registration, PKCE flow, browser open, local callback server, token storage
- [ ] Implement `dot-ai auth logout` — clear stored credentials for current server
- [ ] Implement `dot-ai auth status` — show current auth mode, user identity, token expiry
- [ ] Update `internal/client/client.go` — auth precedence (flag > stored OAuth > env var)
- [ ] Token storage in `~/.config/dot-ai/credentials.json` with `0600` permissions
- [ ] Integration tests for auth commands

### Milestone 2: Documentation

- [ ] New `docs/setup/authentication.md` — OAuth login flow, static token, auth precedence, troubleshooting
- [ ] New `docs/guides/user-management.md` — create/list/delete users via CLI, when to use static users vs IdP connectors
- [ ] Update `docs/setup/configuration.md` — add OAuth credential storage, auth precedence section

### Milestone 3: Feature Request to dot-ai

- [ ] Send feature request to `dot-ai` project: update `docs/ai-engine/setup/authentication.md` to link to CLI-specific user management page (`https://devopstoolkit.ai/docs/cli/guides/user-management`)

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
