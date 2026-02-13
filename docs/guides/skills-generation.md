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

## Supported Agents

- **Claude Code** — `.claude/skills/`
- **Cursor** — `.cursor/skills/`
- **Windsurf** — `.windsurf/skills/`

Note: Cursor also auto-discovers skills from `.claude/skills/`, so Claude Code skills work in Cursor without duplication.

## Generate Skills

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

## Updating Skills

Re-running the command updates all `dot-ai-*` skills:

```bash
dot-ai skills generate --agent claude-code
```

Existing `dot-ai-*` skills are deleted and regenerated with the latest server capabilities.

## How It Works

1. CLI fetches prompts and tool metadata from the server
2. Generates a routing skill for CLI awareness
3. Creates individual skills for each server prompt
4. All skills use `dot-ai-` prefix for namespacing

## Agent Behavior

Once skills are generated:

**Routing:**
- Agents become aware of CLI for Kubernetes operations
- Agents prefer CLI over MCP when both are available
- Agents use `dot-ai --help` to discover commands

**Prompts:**
- Server prompts appear as native agent skills
- Users can invoke them directly in their coding assistant
- Skills stay in sync with server capabilities

## Next Steps

- **[Automation](automation.md)** — Use CLI in scripts and CI/CD
- **[Output Formats](output-formats.md)** — Control output format
- **[Configuration](../setup/configuration.md)** — Configure server URL
