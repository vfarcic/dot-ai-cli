# dot-ai CLI

Auto-generated Go CLI for [dot-ai](https://github.com/vfarcic/dot-ai), the AI-powered Kubernetes operations platform.

The CLI is generated from the server's OpenAPI spec — all commands, flags, and help text are derived automatically. It compiles to self-contained binaries for all major OS/arch combinations.

## Status

Under development. See [prds/1-go-cli.md](prds/1-go-cli.md) for the full product requirements document.

## Architecture

The CLI is a thin HTTP client over the dot-ai REST API:

```
dot-ai CLI  →  HTTP (GET/POST/DELETE)  →  dot-ai REST API Server
```

Unlike MCP (limited to 8 high-level tools), the CLI exposes all REST API endpoints since there's no token cost per command.

## Usage (Planned)

```bash
# AI-powered tools
dot-ai query "what pods are running?"
dot-ai remediate "nginx pod crashlooping"
dot-ai recommend "deploy postgres database"
dot-ai operate "scale nginx to 3 replicas"

# Direct resource access (CLI exclusive)
dot-ai resources --kind Deployment --namespace default
dot-ai logs --name nginx-pod --namespace default --tailLines 50

# Global options
dot-ai query "test" --server-url http://remote:3456 --output json
```

## Building

Requires Go 1.22+.

```bash
make build          # Build for current OS/arch
make build-all      # Cross-compile for all platforms
```
