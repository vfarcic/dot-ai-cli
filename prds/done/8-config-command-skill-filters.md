# PRD #8: Config Command & Skill Include/Exclude Filters

**Status:** Complete
**Completed:** 2026-03-19
**Priority:** Medium
**GitHub Issue:** [#8](https://github.com/vfarcic/dot-ai-cli/issues/8)
**Created:** 2026-03-18
**Supersedes:** [#3](https://github.com/vfarcic/dot-ai-cli/issues/3) (--include/--exclude regex filters for skills generate)

## Problem Statement

The CLI already persists user settings in `~/.config/dot-ai/settings.json` (server URL, output format) and credentials in `credentials.json`, but there is no CLI command to manage these settings. Users must manually edit JSON files to change persistent configuration — an error-prone and undiscoverable experience.

Additionally, `skills generate` always generates all skills from the server. Users who only need a subset (e.g., only `query` and `recommend`) must regenerate everything and manually delete unwanted skills. There is no way to persistently or temporarily filter which skills are generated.

## Solution Overview

1. **`dot-ai config` command** with `set`, `get`, `list`, and `reset` subcommands to read/write `settings.json` through the CLI
2. **New `skills.include` and `skills.exclude` settings** — persisted regex patterns that control which skills `skills generate` produces
3. **`--include` / `--exclude` flags** on `skills generate` for one-time overrides, following the existing precedence model: flag > env > settings.json > default

```text
# Persist skill filters once
dot-ai config set skills.include "query|recommend|remediate"

# All future generates respect the filter
dot-ai skills generate --agent claude-code

# One-time override to generate everything
dot-ai skills generate --agent claude-code --include ".*"
```

## User Experience

### Config Management

```bash
# Set a value
dot-ai config set server-url https://dot-ai.example.com
dot-ai config set output-format json
dot-ai config set skills.include "query|recommend|remediate"
dot-ai config set skills.exclude "debug-.*"

# Get a value
dot-ai config get server-url
# → https://dot-ai.example.com

# Get an unset value shows the default
dot-ai config get output-format
# → yaml (default)

# List all settings (always shows all known keys with current or default values)
dot-ai config list
# → server-url: https://dot-ai.example.com
# → output-format: yaml (default)
# → skills.include: query|recommend|remediate
# → skills.exclude: debug-.*

# Reset a value to default (removes from settings.json)
dot-ai config reset server-url
dot-ai config reset skills.include

# Unknown key is rejected with guidance
dot-ai config set foo bar
# → Error: unknown key "foo". Valid keys: server-url, output-format, skills.include, skills.exclude
```

### Key Discoverability

Users discover available config keys through three mechanisms:

1. **`config list`** — always shows all known keys with current or default values, even before any are set
2. **`config set` error on unknown key** — rejects invalid keys and lists all valid ones
3. **`config set --help`** — documents all keys and their purpose

### Skill Filtering

```bash
# Only generate skills matching the include regex
dot-ai config set skills.include "query|recommend"
dot-ai skills generate --agent claude-code
# → Generates: dot-ai-query, dot-ai-recommend (and prompts matching the pattern)

# Exclude specific skills
dot-ai config set skills.exclude "debug-.*|experimental-.*"
dot-ai skills generate --agent claude-code
# → Generates all skills except those matching the exclude pattern

# One-time override via flags (ignores persisted settings)
dot-ai skills generate --agent claude-code --include ".*" --exclude ""

# Include and exclude work together: include is applied first, then exclude
dot-ai config set skills.include ".*"
dot-ai config set skills.exclude "debug-.*"
# → Generates everything except debug-* skills
```

## Architecture

### Config Keys

The `config` command manages all fields in `settings.json`. Keys use CLI-friendly names (kebab-case) that map to JSON fields:

| CLI Key | JSON Field | Description |
|---------|-----------|-------------|
| `server-url` | `server_url` | Server URL |
| `output-format` | `output_format` | Output format (json, yaml) |
| `skills.include` | `skills_include` | Regex for skills to include |
| `skills.exclude` | `skills_exclude` | Regex for skills to exclude |

### Settings Struct Changes

```go
type Settings struct {
    ServerURL     string `json:"server_url,omitempty"`
    OutputFormat  string `json:"output_format,omitempty"`
    SkillsInclude string `json:"skills_include,omitempty"`
    SkillsExclude string `json:"skills_exclude,omitempty"`
}
```

### Filter Precedence

For `skills.include` and `skills.exclude`, the standard precedence applies:

1. `--include` / `--exclude` flags (highest priority)
2. `DOT_AI_SKILLS_INCLUDE` / `DOT_AI_SKILLS_EXCLUDE` environment variables
3. `settings.json` → `skills_include` / `skills_exclude`
4. Default: empty (no filtering — generate all skills)

### Filter Logic

Filtering applies to both tools and prompts by matching against the skill name (without the `dot-ai-` prefix):

1. If `include` is set, only skills whose name matches the include regex are kept
2. If `exclude` is set, skills whose name matches the exclude regex are removed
3. If both are set, include is applied first, then exclude
4. Empty or unset means no filtering at that stage

### Scope Boundary

The `config` command manages **only `settings.json`** (non-secret preferences). It does NOT touch `credentials.json` — credential management stays with `auth login/logout/status`. This preserves the clean two-file split established in PRD #6.

## Milestones

### Milestone 1: Config Command (set/get/list/reset)

- [x] Add `cmd/config.go` with `config set <key> <value>`, `config get <key>`, `config list`, `config reset <key>` subcommands
- [x] Implement key-name mapping (CLI kebab-case ↔ JSON snake_case) with validation of known keys
- [x] `config list` shows all known keys with current or default values; unknown key errors list valid keys
- [x] Wire subcommands to `auth.LoadSettings()` / `Settings.Save()` for reading and writing `settings.json`
- [x] Integration tests: set/get/list/reset for all supported keys, unknown key rejection

### Milestone 2: Skill Include/Exclude Settings

- [x] Add `SkillsInclude` and `SkillsExclude` fields to `auth.Settings` struct
- [x] Add `--include` / `--exclude` flags to `skills generate` command
- [x] Add `DOT_AI_SKILLS_INCLUDE` / `DOT_AI_SKILLS_EXCLUDE` env var support
- [x] Resolve filter values using standard 4-tier precedence (flag > env > settings.json > default)
- [x] Apply regex filtering in `skills.Generate()` to both tools and prompts before writing

### Milestone 3: Integration Tests for Skill Filtering

- [x] Test: `--include` flag filters tools and prompts to matching names only
- [x] Test: `--exclude` flag removes matching tools and prompts
- [x] Test: combined include + exclude (include applied first, then exclude)
- [x] Test: persisted `skills.include` / `skills.exclude` in settings.json are respected when flags are absent
- [x] Test: flags override persisted settings

### Milestone 4: Documentation

- [x] Update `docs/setup/configuration.md` with `config` command usage and all supported keys
- [x] Update `docs/guides/skills-generation.md` with include/exclude filter examples
- [x] Update `docs/guides/cli-commands-overview.md` with new `config` command

## Design Decisions

| # | Decision | Date | Rationale |
|---|----------|------|-----------|
| 1 | `config` manages only `settings.json`, not `credentials.json` | 2026-03-18 | Credentials are managed by `auth login/logout/status`. Mixing config and credential management would blur the clean separation established in PRD #6. |
| 2 | CLI keys use kebab-case (e.g., `server-url`), JSON uses snake_case | 2026-03-18 | kebab-case is the CLI convention (matches flag names). The mapping is straightforward and validated. |
| 3 | Include is applied before exclude when both are set | 2026-03-18 | This is the most intuitive model: "start with these, then remove those." Matches how most filter systems work (e.g., firewall rules, .gitignore). |
| 4 | Filters match skill names without `dot-ai-` prefix | 2026-03-18 | Users think in terms of skill names (`query`, `recommend`), not internal prefixed names. The prefix is an implementation detail. |
| 5 | Supersede issue #3 rather than implement it separately | 2026-03-18 | Issue #3 requested only `--include`/`--exclude` flags. A `config` command with persistent filters is a superset that eliminates the tedium of specifying flags every time. |
| 6 | `config list` always shows all known keys | 2026-03-18 | Key discoverability is critical. Showing all keys (with defaults for unset values) lets users learn what's configurable without reading docs. Unknown key errors also list valid keys. |
| 7 | Filter precedence resolved in `cmd/skills.go`, not `config.Config` | 2026-03-18 | Skill filtering is only used by `skills generate`, not globally. Keeping resolution in the command layer avoids adding single-purpose fields to the shared Config struct. |

## Dependencies

| Dependency | Status | Notes |
|------------|--------|-------|
| `internal/auth/settings.go` (Settings struct, Load/Save) | Complete | From PRD #6 — provides the persistence layer |
| `internal/skills/generator.go` (Generate function) | Complete | Needs modification to accept and apply filters |
| Mock server (`dot-ai-mock-server`) | Complete | Already returns tools and prompts for integration tests |

## Success Criteria

- `dot-ai config set/get/list/reset` works for all supported keys
- `config list` shows all known keys with current or default values, even when nothing is configured
- Unknown keys are rejected with a helpful error listing valid keys
- Settings persist across CLI invocations in `~/.config/dot-ai/settings.json`
- `skills generate --include/--exclude` filters generated skills by regex
- Persisted `skills.include`/`skills.exclude` are used when flags are absent
- Flags override persisted settings (standard precedence)
- All existing functionality continues to work unchanged
