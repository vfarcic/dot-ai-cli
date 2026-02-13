# Quick Start

Get your AI agent using the DevOps AI Toolkit CLI.

## Prerequisites

- Running [DevOps AI Toolkit server](https://devopstoolkit.ai/docs/mcp/setup/mcp-setup)
- AI coding assistant: Claude Code, Cursor, or Windsurf

## Install the CLI

**macOS/Linux:**
```bash
brew install vfarcic/tap/dot-ai
```

For other platforms, see [Installation Guide](setup/installation.md).

## Configure Server Connection

Point the CLI to your server:

```bash
export DOT_AI_URL="http://dot-ai.127.0.0.1.nip.io"  # your server URL
export DOT_AI_AUTH_TOKEN="your-token"               # if authentication is enabled
```

See [Configuration Guide](setup/configuration.md) for details.

## Generate Agent Skills

Enable your AI agent to discover and use the CLI:

```bash
# For Claude Code
dot-ai skills generate --agent claude-code

# For Cursor
dot-ai skills generate --agent cursor

# For Windsurf
dot-ai skills generate --agent windsurf
```

See [Skills Generation](guides/skills-generation.md) for what this does.

## Verify It Works

Ask your agent to use the CLI:

```
"Use the CLI to check the server version"
```

Your agent should execute `dot-ai version` and show you the results. If this works, your agent is successfully using the CLI!

## What's Next

Your agent can now use all DevOps AI Toolkit capabilities via CLI. For details on what you can do, see the [server documentation](https://devopstoolkit.ai/docs/mcp/).

**CLI-specific topics:**
- **[Commands Overview](guides/cli-commands-overview.md)** — How to discover and use commands
- **[Output Formats](guides/output-formats.md)** — Control CLI output format
- **[Automation](guides/automation.md)** — Use CLI in scripts and CI/CD
