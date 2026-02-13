# Installation

Install the DevOps AI Toolkit CLI on your preferred platform.

## Prerequisites

- Access to a running [DevOps AI Toolkit server](https://devopstoolkit.ai/docs/mcp/setup/mcp-setup)

## Homebrew (macOS/Linux)

```bash
brew install vfarcic/tap/dot-ai
```

## Scoop (Windows)

```bash
# Add the bucket
scoop bucket add dot-ai https://github.com/vfarcic/scoop-dot-ai

# Install
scoop install dot-ai
```

## Binary Download

Download the latest release for your platform:

**macOS (Apple Silicon):**
```bash
curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-darwin-arm64 \
  -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai
```

**macOS (Intel):**
```bash
curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-darwin-amd64 \
  -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai
```

**Linux (x86_64):**
```bash
curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-linux-amd64 \
  -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai
```

**Linux (ARM64):**
```bash
curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-linux-arm64 \
  -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai
```

**Windows:**

Download from [GitHub Releases](https://github.com/vfarcic/dot-ai-cli/releases/latest) and add to PATH.

## Configuration

Configure the server URL and authentication:

```bash
export DOT_AI_URL="https://your-server-url"
export DOT_AI_AUTH_TOKEN="your-token"
```

See [Configuration](configuration.md) for more options.

## Verification

Verify the CLI can connect to your server:

```bash
dot-ai version
```

You should see version and diagnostic information from the server.

## Next Steps

- **[Configuration](configuration.md)** — Detailed configuration options
- **[Shell Completion](shell-completion.md)** — Enable command autocompletion
- **[Commands Overview](../guides/cli-commands-overview.md)** — See all available commands
