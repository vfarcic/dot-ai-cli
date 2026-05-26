# Skills Generation

Enable AI agents to use the DevOps AI Toolkit CLI and access server prompts as native skills.

## What Are Skills?

Skills are agent capabilities that make AI coding assistants (Claude Code, Cursor, Windsurf) aware of available tools and workflows. The CLI can generate skills from server capabilities.

## What Gets Generated

Skills generation serves two purposes:

### 1. CLI Awareness (Routing Skill)

Creates a `dot-ai` routing skill that makes agents aware of the CLI:
- Triggers on Kubernetes and DevOps operations
- Directs agents to use CLI instead of MCP
- Teaches agents to use `dot-ai --help` for command discovery
- Lower token overhead than MCP protocol

### 2. Server Prompts (Prompt Skills)

Exposes server prompts as native agent skills:
- Each server prompt becomes an agent skill (e.g., `dot-ai-projectSetup`, `dot-ai-query`)
- Users can invoke them as native skills in their agent
- Prefixed with `dot-ai-` to avoid naming conflicts
- Skills can include supporting files (shell scripts, templates, manifests) alongside `SKILL.md`

## Supported Agents

- **Claude Code** — `.claude/skills/`
- **Cursor** — `.cursor/skills/`
- **Windsurf** — `.windsurf/skills/`

Note: Cursor also auto-discovers skills from `.claude/skills/`, so Claude Code skills work in Cursor without duplication.

## Generate Skills

The server caches skills for performance (default: 24 hours). Use `--pull-latest` to force the server to pull the latest from the git repository before generating:

```bash
dot-ai skills generate --agent claude-code --pull-latest
```

**For Claude Code:**
```bash
dot-ai skills generate --agent claude-code
```

**For Cursor:**
```bash
dot-ai skills generate --agent cursor
```

**For Windsurf:**
```bash
dot-ai skills generate --agent windsurf
```

**Custom path (unsupported agents):**
```bash
dot-ai skills generate --path ./custom/skills/
```

## Custom-Only Mode

By default, `skills generate` creates skills for both MCP tools (query, recommend, remediate, etc.) and custom prompts (troubleshoot-pod, explain-resource, etc.). Use `--custom-only` to skip MCP tool skills and generate only custom prompt skills:

```bash
dot-ai skills generate --agent claude-code --custom-only
```

Persist it so all future generations respect it:

```bash
dot-ai config set skills.custom_only true
dot-ai skills generate --agent claude-code

# Clear it
dot-ai config reset skills.custom_only
```

`--custom-only` follows the standard 4-tier precedence (see [Filter Precedence](#filter-precedence) below) and can be combined with `--include`/`--exclude` to further filter within the custom skills.

## Filtering Skills

By default, `skills generate` creates skills for all tools and prompts from the server. You can filter which skills are generated using include/exclude regex patterns.

### One-Time Filtering with Flags

```bash
# Generate only query and recommend skills
dot-ai skills generate --agent claude-code --include "query|recommend"

# Generate all except management skills
dot-ai skills generate --agent claude-code --exclude "manage.*"

# Combine: include all, then exclude specific ones
dot-ai skills generate --agent claude-code --include ".*" --exclude "manage.*"
```

### Persistent Filtering with Config

Set filters once so all future generations respect them:

```bash
# Persist an include filter
dot-ai config set skills.include "query|recommend|remediate"

# All future generates use the filter
dot-ai skills generate --agent claude-code

# One-time override to generate everything
dot-ai skills generate --agent claude-code --include ".*"

# Persist an exclude filter
dot-ai config set skills.exclude "debug-.*|experimental-.*"

# Clear persistent filters
dot-ai config reset skills.include
dot-ai config reset skills.exclude
```

### Filter Precedence

Filters follow the standard 4-tier precedence:

1. `--include` / `--exclude` / `--custom-only` flags (highest priority)
2. `DOT_AI_SKILLS_INCLUDE` / `DOT_AI_SKILLS_EXCLUDE` / `DOT_AI_SKILLS_CUSTOM_ONLY` environment variables
3. `settings.json` → `skills_include` / `skills_exclude` / `skills_custom_only`
4. Default: empty (no filtering — generate all skills)

### Filter Logic

- Patterns are regular expressions matched against skill names (without the `dot-ai-` prefix)
- If `--custom-only` is set, MCP tool skills are skipped entirely
- If `--include` is set, only skills matching the pattern are kept
- If `--exclude` is set, skills matching the pattern are removed
- If both are set, include is applied first, then exclude
- `--include`/`--exclude` filters apply to both tool skills and prompt skills (or just prompt skills when `--custom-only` is active)

## Auto-Update with SessionStart Hook

For Claude Code, you can install a hook that automatically regenerates skills at the start of every session:

```bash
dot-ai skills generate --agent claude-code --install-hook
```

This adds a `SessionStart` hook to `.claude/settings.json` that runs `dot-ai skills generate --agent claude-code` on session startup. The hook captures the flags you pass — `--custom-only`, `--include`, `--exclude`, and `--repo` are forwarded so the hook reproduces the same behavior:

```bash
dot-ai skills generate --agent claude-code --install-hook --custom-only --exclude "debug-.*"
# Hook command: dot-ai skills generate --agent claude-code --custom-only --exclude "debug-.*"
```

The hook is idempotent — running the command again with the same flags won't create duplicates. Re-running with different flags replaces the existing hook. It merges with any existing settings.

To compose skills from multiple repos, install one hook per source:

```bash
dot-ai skills generate --agent claude-code --install-hook                                           # env-var repo
dot-ai skills generate --agent claude-code --install-hook --repo https://github.com/orgA/skills    # org-wide
dot-ai skills generate --agent claude-code --install-hook --repo https://gitlab.corp/team/skills   # team-private
```

Each invocation of `--install-hook` writes one hook scoped to its source. See
[Composing Skills From Multiple Repositories](#composing-skills-from-multiple-repositories) for the full model.

## Updating Skills

Re-running the command updates the `dot-ai-*` skills owned by the current source:

```bash
dot-ai skills generate --agent claude-code
```

Each generated skill carries a `source:` field in its YAML frontmatter
recording which repo it came from (`built-in` for the server's configured
default, or the repo URL passed via `--repo`). On every run, the CLI only
wipes-and-replaces skills tagged with the *current* source — skills from other
sources are left in place. This is what lets you compose skills from multiple
repos via multiple invocations (see [Composing Skills From Multiple Repositories](#composing-skills-from-multiple-repositories)).

## Composing Skills From Multiple Repositories

The `--repo <url>` flag overrides the server's configured default prompts repo
for one invocation:

```bash
dot-ai skills generate --agent claude-code --repo https://github.com/orgA/skills
```

Each invocation is **self-contained and source-scoped** — it fetches from a
single repo, tags every generated skill with that repo's source identifier,
and only manages files from that source. Composition across multiple sources
is achieved by running the command multiple times — typically as one agent
hook per source:

```bash
# Hook A: server's configured default repo (env-var DOT_AI_USER_PROMPTS_REPO on the server)
dot-ai skills generate --agent claude-code

# Hook B: explicit org-wide repo
dot-ai skills generate --agent claude-code --repo https://github.com/orgA/skills

# Hook C: a self-hosted team-private repo, with its own credentials in DOT_AI_GIT_TOKEN
DOT_AI_GIT_TOKEN=$TEAM_TOKEN dot-ai skills generate --agent claude-code --repo https://gitlab.corp/team/skills
```

Each hook owns its slice. Per-source credentials, branches, and paths come
from per-hook env vars — the hook is the scoping unit.

When `--repo` is supplied, the CLI prints the source it received from the
server, redacted of any userinfo for safety:

```text
Skills generated successfully in .claude/skills
Source: https://github.com/orgA/skills
```

The no-flag invocation does not print a `Source:` line (it stays
byte-for-byte equivalent to pre-multi-repo behavior).

### Collision Policy (First-Source-Wins)

If two sources expose a skill with the same name, the first invocation to
write the skill wins; subsequent invocations skip the colliding name and log
a warning to stderr:

```text
warning: skipping "query": already provided by source "https://github.com/orgA/skills" (first-source-wins)
```

The named "other source" is redacted before logging. To resolve a real
collision, rename one of the skills upstream or drop one of the sources.

### Concurrent Invocations

If two hooks fire in parallel and both target the same skills directory, they
serialize on an exclusive lock file (`<outDir>/.dot-ai.lock`). The first
acquirer proceeds; the second waits briefly and, if the lock stays held, fails
fast with:

```text
Error: another `dot-ai skills generate` is in progress
```

Re-run the second hook after the first finishes (or rely on the agent's hook
runner to retry).

### Source Frontmatter

Every generated `SKILL.md` includes a `source:` field:

```yaml
---
name: dot-ai-troubleshoot-pod
description: Generate a troubleshooting guide for a pod
user-invocable: true
source: built-in
---
```

The value comes verbatim from the server's response — `built-in` for the
default repo, or the exact URL you passed to `--repo`. The CLI never modifies
this value; it must match exactly between writes and subsequent wipes.

### Legacy File Migration

Skills generated by earlier versions of the CLI do not have a `source:` field.
The first time you re-run `dot-ai skills generate` after upgrading:

- **No `--repo`** (env-var path): untagged legacy skills are assumed to belong
  to the env-var repo and are wiped as part of the normal per-source cleanup.
  Same effect as the pre-multi-repo behavior.
- **With `--repo X`**: untagged legacy files survive the per-source scan
  (they don't belong to source `X`). If a new skill from source `X` collides
  by name, the legacy file is overwritten. Subsequent runs without `--repo`
  then sweep up any remaining legacy files.

For most users, simply re-running the env-var-path invocation once after
upgrade is enough to retag everything.

## How It Works

1. CLI fetches prompts and tool metadata from the server
2. Generates a routing skill for CLI awareness
3. Creates individual skills for each server prompt, including any supporting files
4. All skills use `dot-ai-` prefix for namespacing

The generated skills include both built-in skills that ship with the server and any user-defined skills you've configured (see [Adding Custom Skills](#adding-custom-skills) below).

Each skill is a directory containing `SKILL.md` and optionally supporting files — shell scripts, templates, manifests, or other resources the skill references:

```text
.claude/skills/dot-ai-worktree-prd/
├── SKILL.md
├── create-worktree.sh
└── templates/
    └── branch-config.yaml
```

Supporting files are written with executable permissions. Nested paths automatically create intermediate directories.

## Adding Custom Skills

You can serve your own skills alongside the built-in ones by organizing them in a git repository.

### Repository Structure

Skills can be defined as single markdown files or as directories with `SKILL.md` and optional supporting files:

```text
my-team-skills/
├── deploy-app.md                    # Single-file skill
├── worktree-prd/                    # Skill with supporting files
│   ├── SKILL.md
│   └── create-worktree.sh
└── k8s-debug/                       # Skill with nested supporting files
    ├── SKILL.md
    ├── debug.sh
    └── templates/
        └── pod-debug.yaml
```

### Skill File Format

Each skill uses YAML frontmatter:

```yaml
---
name: deploy-app
description: Deploy an application to the specified environment
arguments:
  - name: environment
    description: Target environment (dev, staging, prod)
    required: true
---

# Deploy Application

Deploy the application to {{environment}}.
```

### Referencing Supporting Files

When your skill's `SKILL.md` references supporting files, **always use relative paths** — either bare (`analyze.sh`) or dot-prefixed (`./analyze.sh`):

~~~markdown
Run the analysis:
```bash
bash analyze.sh
```
~~~

During generation, the CLI automatically rewrites these to the correct full path based on the target agent and directory structure. For example, with `--agent claude-code`, the above becomes:

~~~markdown
```bash
bash .claude/skills/dot-ai-my-skill/analyze.sh
```
~~~

**Do not hardcode full paths** like `.claude/skills/my-skill/analyze.sh` in your source skill files. The final path depends on:
- The **agent** (`--agent claude-code` → `.claude/skills/`, `--agent cursor` → `.cursor/skills/`)
- The **`dot-ai-` prefix** added during generation

The rewrite applies to all files listed in the skill's supporting files. Nested paths work too — `templates/deploy.yaml` becomes `.claude/skills/dot-ai-my-skill/templates/deploy.yaml`.

### Server Configuration

To point the server at your skills repository, see [User-Defined Prompts](https://devopstoolkit.ai/docs/ai-engine/tools/prompts#user-defined-prompts).

Once configured, running `dot-ai skills generate` pulls your custom skills alongside the built-in ones.

## Agent Behavior

Once skills are generated:

**Routing:**
- Agents become aware of CLI for Kubernetes operations
- Agents prefer CLI over MCP when both are available
- Agents use `dot-ai --help` to discover commands

**Skills:**
- Server prompts appear as native agent skills
- Users can invoke them directly in their coding assistant
- Skills stay in sync with server capabilities

## Next Steps

- **[Automation](automation.md)** — Use CLI in scripts and CI/CD
- **[Output Formats](output-formats.md)** — Control output format
- **[Configuration](../setup/configuration.md)** — Configure server URL
