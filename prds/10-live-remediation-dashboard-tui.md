# PRD #10: Live Remediation Dashboard TUI

**Status:** Not Started
**Priority:** Medium
**GitHub Issue:** [#10](https://github.com/vfarcic/dot-ai-cli/issues/10)
**Created:** 2026-03-29
**Triggered by:** Feature request from `dot-ai-prd-425-session-list-api-and-sse-streaming-for-remediation`

## Problem Statement

The dot-ai server is adding two new endpoints for remediation session monitoring: `GET /api/v1/sessions` (list with filtering/pagination) and `GET /api/v1/events/remediations` (SSE stream for real-time session lifecycle events). These endpoints were built specifically to enable a consumer that can discover and act on remediation sessions.

A CLI that dumps raw SSE events to stdout is not useful. Users need an interactive interface to:
- See all remediation sessions at a glance
- Watch sessions update in real-time as the server investigates and analyzes issues
- Accept remediation suggestions when analysis is complete
- Copy session details for manual remediation via agents like Claude Code or Cursor

## Solution Overview

A `dot-ai dashboard` command that launches a terminal UI (TUI) using charmbracelet/bubbletea. On startup it fetches existing sessions and subscribes to the SSE stream for live updates. Sessions are displayed in an interactive table with keyboard-driven actions: accept remediation, copy session info, filter by status.

```bash
# Launch the dashboard
dot-ai dashboard

# With explicit server URL
dot-ai dashboard --server-url https://dot-ai.example.com
```

## User Experience

### Dashboard Layout

```
 dot-ai dashboard                                        Connected
 ─────────────────────────────────────────────────────────────────
 Session ID    Status              Issue                  Updated
 ─────────────────────────────────────────────────────────────────
 a1b2c3d4...   analysis_complete   Pod CrashLoopBack...   2m ago
 e5f6g7h8...   investigating       OOMKilled in prod...   5m ago
 i9j0k1l2...   executed_success.   Disk pressure on...   12m ago
 m3n4o5p6...   failed              Network policy bl...   1h ago
 ─────────────────────────────────────────────────────────────────
 a accept  c copy  r refresh  ? help  q quit
```

### Key Interactions

- **Navigate**: Up/Down or j/k to move between sessions
- **Accept (a)**: For `analysis_complete` sessions, prompts to choose execution method:
  - `1` = Execute via MCP
  - `2` = Execute via agent
  - `Esc` = Cancel
- **Copy (c)**: Copies session details (ID, status, issue, tool, timestamps) to system clipboard for use in external agents. Falls back to displaying in TUI if clipboard is unavailable.
- **Refresh (r)**: Force re-fetch all sessions from server
- **Quit (q or Ctrl+C)**: Clean exit, closes SSE connection

### Real-Time Updates

Sessions update in-place as SSE events arrive. New sessions appear at the top (sorted by updatedAt descending). Status transitions are reflected immediately without user action. A "Connected" indicator in the header shows SSE stream health; "Reconnecting..." appears on disconnect with automatic retry.

### Error States

- **Server unreachable**: Error message in TUI with `r` to retry
- **Authentication failed**: "Run 'dot-ai auth login'" message
- **SSE disconnect**: Auto-reconnect after 3 seconds
- **No sessions**: Empty state with helpful message
- **Accept fails**: Flash error, session remains in table

## Technical Approach

### New Dependencies

- `github.com/charmbracelet/bubbletea` - TUI framework (elm architecture for Go)
- `github.com/charmbracelet/bubbles` - table, spinner, help key components
- `github.com/charmbracelet/lipgloss` - terminal styling

### Package Structure

```
cmd/dashboard.go                     -- cobra command (hardcoded, like auth/config/skills)
internal/dashboard/
    model.go                         -- bubbletea Model: Init, Update, View
    session.go                       -- Session type, status constants, sorting
    api.go                           -- FetchSessions(), AcceptRemediation()
    sse.go                           -- SSE client, event parsing, tea.Cmd integration
    keymap.go                        -- key bindings
    styles.go                        -- lipgloss styles (status colors, layout)
```

### API Endpoints Consumed

1. **`GET /api/v1/sessions`** - List sessions
   - Query params: `status` (optional enum), `limit` (default 50, max 200), `offset` (default 0)
   - Response: array of `{sessionId, status, issue, mode, toolName, createdAt, updatedAt}`
   - Statuses: `investigating`, `analysis_complete`, `failed`, `executed_successfully`, `executed_with_errors`

2. **`GET /api/v1/events/remediations`** - SSE stream
   - Content-Type: text/event-stream
   - Events: `session-created`, `session-updated`
   - Data: `{"sessionId":"...","toolName":"remediate","status":"...","issue":"...","timestamp":"..."}`
   - 30-second heartbeat (`: heartbeat`)

3. **`POST /api/v1/tools/remediate`** - Accept/execute remediation
   - Body: `{"sessionId": "...", "executeChoice": 1|2}` (1=MCP, 2=agent)
   - Used when user accepts an `analysis_complete` session

### Key Design Decisions

- **Custom HTTP client for SSE**: The existing `client.Do()` calls `io.ReadAll` which blocks forever on SSE streams. The dashboard uses direct `net/http` with `bufio.Scanner` for line-by-line streaming.
- **Hardcoded command**: TUI is fundamentally different from request/response commands and cannot be auto-generated from the OpenAPI spec.
- **Map + sorted slice for sessions**: `map[string]Session` for O(1) upsert from SSE events, sorted slice rebuilt for table display.
- **bubbletea tea.Cmd chain for SSE**: Each SSE event is delivered as a `tea.Msg`. After processing, a new `tea.Cmd` is issued to read the next event. This is the idiomatic bubbletea pattern for streaming data.

### Integration with Existing CLI

- Registered as a hardcoded cobra command in `cmd/dashboard.go`, following the same pattern as `auth`, `config`, and `skills`
- Uses the shared `config.Config` for server URL, token, and output format
- Respects `--server-url` and `--token` persistent flags
- Bearer token authentication from config (same as all other commands)

## Success Criteria

- Dashboard launches, connects to SSE, and displays sessions within 2 seconds
- New SSE events update the table in real-time without user action
- Accept action successfully triggers remediation execution on the server
- Copy action places usable session info on the system clipboard
- Graceful handling of all error states (connection loss, auth failure, empty state)
- Clean exit on Ctrl+C with SSE connection cleanup

## Milestones

- [ ] Session data types and helpers implemented with unit tests
- [ ] HTTP client for sessions list and accept endpoints working with unit tests
- [ ] SSE streaming client parsing events correctly with unit tests
- [ ] TUI model with table view displaying sessions from initial fetch
- [ ] Live SSE updates reflected in TUI table in real-time
- [ ] Accept remediation flow (keyboard shortcut -> choice prompt -> POST -> flash result)
- [ ] Copy session info to clipboard with platform detection and fallback
- [ ] Error handling: connection loss, auth failure, empty state, reconnection
- [ ] Integration tests for dashboard help and basic functionality
- [ ] Feature response written back to requesting project

## Dependencies

- Server-side: `GET /api/v1/sessions` and `GET /api/v1/events/remediations` endpoints must be deployed (PRD #425 in dot-ai server)
- Mock server: May need SSE fixture support added to `ghcr.io/vfarcic/dot-ai-mock-server` for full integration testing
