# dot-ai CLI

[![CI](https://github.com/vfarcic/dot-ai-cli/actions/workflows/ci.yaml/badge.svg)](https://github.com/vfarcic/dot-ai-cli/actions/workflows/ci.yaml)
[![Release](https://github.com/vfarcic/dot-ai-cli/actions/workflows/release.yaml/badge.svg)](https://github.com/vfarcic/dot-ai-cli/actions/workflows/release.yaml)
[![License](https://img.shields.io/github/license/vfarcic/dot-ai-cli)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vfarcic/dot-ai-cli)](go.mod)

**Auto-generated Go CLI for [dot-ai](https://github.com/vfarcic/dot-ai) — AI-powered Kubernetes operations via command line**

→ **[Read the Documentation](https://devopstoolkit.ai/docs/cli/)**

## Overview

The dot-ai CLI provides command-line access to all dot-ai capabilities with ~33% better token efficiency than MCP. Generated from the server's OpenAPI spec, it exposes the full REST API as self-contained binaries for all major platforms.

Unlike MCP (limited to 8 high-level tools to minimize context window usage), the CLI exposes all REST API endpoints since there's no token cost per command — making it ideal for both AI agents and human operators.

## Quick Install

```bash
# Homebrew (macOS/Linux)
brew install vfarcic/tap/dot-ai

# Scoop (Windows)
scoop bucket add dot-ai https://github.com/vfarcic/scoop-dot-ai
scoop install dot-ai

# Binary download (replace OS and arch as needed)
curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-darwin-arm64 \
  -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai
```

## Quick Start

```bash
# Verify installation
dot-ai version

# AI-powered cluster operations
dot-ai query "what pods are running?"
dot-ai remediate "nginx pod crashlooping"
dot-ai recommend "deploy postgres database"

# Direct resource access (CLI exclusive)
dot-ai resources --kind Deployment --namespace default
dot-ai logs --name nginx-pod --namespace default --tailLines 50

# Generate agent skills
dot-ai skills generate --agent claude-code
```

See the [Quick Start Guide](https://devopstoolkit.ai/docs/cli/quick-start) for more.

## Documentation

- **[Getting Started](https://devopstoolkit.ai/docs/cli/)** — Overview and introduction
- **[Installation Guide](https://devopstoolkit.ai/docs/cli/setup/installation)** — All installation methods
- **[Configuration](https://devopstoolkit.ai/docs/cli/setup/configuration)** — Server URL, authentication, output formats
- **[Shell Completion](https://devopstoolkit.ai/docs/cli/setup/shell-completion)** — Bash, Zsh, Fish setup
- **[Commands Reference](https://devopstoolkit.ai/docs/cli/guides/cli-commands-overview)** — All available commands
- **[Skills Generation](https://devopstoolkit.ai/docs/cli/guides/skills-generation)** — AI agent integration

## Building from Source

Requires Go 1.22+ and [Task](https://taskfile.dev).

```bash
task build          # Build for current OS/arch
task build-all      # Cross-compile for all platforms
task test           # Run integration tests
```

## Architecture

The CLI is a thin HTTP client over the dot-ai REST API:

```
dot-ai CLI  →  HTTP (GET/POST/DELETE)  →  dot-ai REST API Server
```

All commands are auto-generated from the server's OpenAPI spec — zero manual code changes needed for new endpoints.

## Related Projects

- **[dot-ai](https://github.com/vfarcic/dot-ai)** — Main server (MCP + REST API)
- **[dot-ai-ui](https://github.com/vfarcic/dot-ai-ui)** — Web UI for visualizations
- **[dot-ai-stack](https://github.com/vfarcic/dot-ai-stack)** — Kubernetes deployment stack

## Support

- **[Documentation](https://devopstoolkit.ai/docs/cli/)** — Complete guides and references
- **[Issues](https://github.com/vfarcic/dot-ai-cli/issues)** — Bug reports and feature requests
- **[Discussions](https://github.com/vfarcic/dot-ai-cli/discussions)** — Questions and community support

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Please note that this project follows a [Code of Conduct](CODE_OF_CONDUCT.md).

## License

MIT License - see [LICENSE](LICENSE) for details.
