# Output Formats

Control how the CLI formats command output.

## Available Formats

### YAML (Default)

Human-readable structured output.

**When to use:**
- Interactive terminal use
- Reading output directly
- Debugging and development

**Example:**
```bash
dot-ai version
```

**Output:**
```yaml
version: 1.2.1
server:
  version: 1.2.1
  healthy: true
```

### JSON

Raw API response, machine-parseable.

**When to use:**
- Scripting and automation
- Piping to other tools (jq, etc.)
- AI agents processing output
- CI/CD pipelines

**Example:**
```bash
dot-ai version --output json
```

**Output:**
```json
{
  "version": "1.2.1",
  "server": {
    "version": "1.2.1",
    "healthy": true
  }
}
```

## Setting Output Format

**Command-line flag:**
```bash
dot-ai <command> --output json
dot-ai <command> --output yaml
```

**Environment variable:**
```bash
export DOT_AI_OUTPUT_FORMAT="json"
dot-ai <command>
```

**Default:** `yaml`

## Processing Output

**Extract fields with jq:**
```bash
dot-ai version --output json | jq '.server.version'
```

**Filter arrays:**
```bash
dot-ai resources --kind Deployment --output json | jq '.items[] | .metadata.name'
```

**Combine with other tools:**
```bash
dot-ai resources --kind Pod --output json | jq -r '.items[].metadata.name' | xargs -I {} echo "Pod: {}"
```

## For AI Agents

AI agents should use JSON output for structured parsing:

```bash
dot-ai <command> --output json
```

This ensures consistent, parseable responses without YAML formatting ambiguities.

## Next Steps

- **[Automation](automation.md)** — Use output in scripts and CI/CD
- **[Commands Overview](cli-commands-overview.md)** — See all available commands
- **[Configuration](../setup/configuration.md)** — Set default output format
