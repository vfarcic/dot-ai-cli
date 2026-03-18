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

1. `--include` / `--exclude` flags (highest priority)
2. `DOT_AI_SKILLS_INCLUDE` / `DOT_AI_SKILLS_EXCLUDE` environment variables
3. `settings.json` → `skills_include` / `skills_exclude`
4. Default: empty (no filtering — generate all skills)

### Filter Logic

- Patterns are regular expressions matched against skill names (without the `dot-ai-` prefix)
- If `--include` is set, only skills matching the pattern are kept
- If `--exclude` is set, skills matching the pattern are removed
- If both are set, include is applied first, then exclude
- Filters apply to both tool skills and prompt skills

## Auto-Update with SessionStart Hook

For Claude Code, you can install a hook that automatically regenerates skills at the start of every session:

```bash
dot-ai skills generate --agent claude-code --install-hook
```

This adds a `SessionStart` hook to `.claude/settings.json` that runs `dot-ai skills generate --agent claude-code` on session startup. The hook is idempotent — running the command again won't create duplicates. It merges with any existing settings.

## Updating Skills

Re-running the command updates all `dot-ai-*` skills:

```bash
dot-ai skills generate --agent claude-code
```

Existing `dot-ai-*` skills are deleted and regenerated with the latest server capabilities.

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
