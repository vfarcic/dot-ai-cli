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

```
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

```
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
