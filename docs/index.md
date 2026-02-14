# CLI Documentation

**Command-line interface for AI-powered Kubernetes operations**

## What is the CLI?

The CLI provides command-line access to all [DevOps AI Toolkit](https://devopstoolkit.ai/docs/ai-engine/) capabilities. It's a lightweight HTTP client designed for both AI agents and human operators who prefer terminal-based workflows.

Unlike MCP (limited to 8 high-level tools to minimize context window usage), the CLI exposes **all REST API endpoints** since there's no token cost per command. This means you get access to direct resource queries, logs, events, and more—all from a single binary with zero runtime dependencies.

**Key benefits:**

- **Single binary** — No installation dependencies, just download and run
- **Cross-platform** — Linux, macOS, Windows (amd64 + arm64)
- **Token efficient** — Lower token overhead than MCP for AI agents
- **Complete API access** — All 26 REST API endpoints (MCP exposes 8 tools)
- **Composable** — Shell piping, scripting, and CI/CD integration

## When to Use the CLI

The CLI is ideal for:

- **Scripting and automation** — Shell scripts, CI/CD pipelines, scheduled jobs
- **AI agent integration** — Lower token overhead than MCP protocol
- **Direct API access** — Commands for resources, logs, events, namespaces not available via MCP
- **Composability** — Pipe output between commands, combine with other CLI tools

For details on DevOps AI Toolkit features (query, recommend, remediate, etc.), see the [main documentation](https://devopstoolkit.ai/docs/ai-engine/).

## Getting Started

**[Quick Start](quick-start.md)** — Set up your AI agent to use the CLI

## Documentation

### Setup

- **[Installation](setup/installation.md)** — Homebrew, Scoop, binary download
- **[Configuration](setup/configuration.md)** — Server URL, authentication, output format
- **[Shell Completion](setup/shell-completion.md)** — Bash, Zsh, Fish autocompletion

### Guides

- **[Commands Overview](guides/cli-commands-overview.md)** — All available commands
- **[Skills Generation](guides/skills-generation.md)** — Enable AI agents to discover and use the CLI
- **[Output Formats](guides/output-formats.md)** — YAML vs JSON
- **[Automation](guides/automation.md)** — Scripting and CI/CD integration

## Architecture

```
┌─────────────┐
│     CLI     │
└──────┬──────┘
       │ HTTP (GET/POST/DELETE)
       │ Bearer auth, JSON body
       ▼
┌─────────────────────┐
│ DevOps AI Toolkit   │
│ REST API Server     │
└─────────────────────┘
```

The CLI is a stateless HTTP client that reads the embedded OpenAPI spec and generates commands dynamically. All commands map directly to REST API endpoints.

## Related Projects

- **[DevOps AI Toolkit](https://devopstoolkit.ai/docs/ai-engine/)** — Main server (MCP + REST API)
- **[Web UI](https://devopstoolkit.ai/docs/ui/)** — Visualizations and dashboards
- **[Stack](https://devopstoolkit.ai/docs/stack/)** — Kubernetes deployment
