# Commands Overview

The CLI exposes all DevOps AI Toolkit server capabilities as commands. Commands are automatically generated from the server's OpenAPI specification.

## Discovering Commands

To see all available commands:

```bash
dot-ai --help
```

To see help for a specific command:

```bash
dot-ai <command> --help
```

For details on what each feature does, see the [server documentation](https://devopstoolkit.ai/docs/ai-engine/).

## Global Flags

These flags work with all commands:

| Flag | Environment Variable | Description |
|------|---------------------|-------------|
| `--server-url` | `DOT_AI_URL` | Server URL (default: `http://localhost:3456`) |
| `--token` | `DOT_AI_AUTH_TOKEN` | Authentication token |
| `--output` | `DOT_AI_OUTPUT_FORMAT` | Output format: `yaml` or `json` (default: `yaml`) |
| `--help` | - | Show command help |

## Usage Patterns

**Basic command execution:**
```bash
dot-ai <command> [arguments] [flags]
```

**With output format:**
```bash
dot-ai <command> --output json
```

**Remote server:**
```bash
dot-ai <command> --server-url https://remote:3456 --token mytoken
```

**Piping output:**
```bash
dot-ai <command> --output json | jq '.result'
```

## Next Steps

- **[Skills Generation](skills-generation.md)** — Enable AI agents to use the CLI
- **[Output Formats](output-formats.md)** — YAML vs JSON
- **[Automation](automation.md)** — Use in scripts and CI/CD
- **[Server Features](https://devopstoolkit.ai/docs/ai-engine/)** — What each command does
