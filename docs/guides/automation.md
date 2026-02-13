# Automation

Use the CLI in scripts, CI/CD pipelines, and automated workflows.

## Exit Codes

The CLI uses standard exit codes for automation:

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | Tool execution error (server returned error) |
| `2` | Connection error (server unreachable) |
| `3` | Usage error (invalid arguments, missing required params) |

## Error Handling in Scripts

**Check exit code:**
```bash
#!/bin/bash
if dot-ai <command>; then
  echo "Success"
else
  echo "Failed with exit code $?"
  exit 1
fi
```

**Handle specific errors:**
```bash
#!/bin/bash
dot-ai <command>
EXIT_CODE=$?

case $EXIT_CODE in
  0) echo "Success" ;;
  1) echo "Server error" ;;
  2) echo "Connection failed" ;;
  3) echo "Invalid usage" ;;
esac
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Deploy
on: [push]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Install CLI
        run: |
          curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-linux-amd64 \
            -o /usr/local/bin/dot-ai && chmod +x /usr/local/bin/dot-ai

      - name: Deploy application
        env:
          DOT_AI_URL: ${{ secrets.DOT_AI_URL }}
          DOT_AI_AUTH_TOKEN: ${{ secrets.DOT_AI_AUTH_TOKEN }}
        run: |
          dot-ai <command> --output json
```

### GitLab CI

```yaml
deploy:
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y curl
    - curl -sL https://github.com/vfarcic/dot-ai-cli/releases/latest/download/dot-ai-linux-amd64 -o /usr/local/bin/dot-ai
    - chmod +x /usr/local/bin/dot-ai
  script:
    - dot-ai <command> --output json
  variables:
    DOT_AI_URL: $DOT_AI_URL
    DOT_AI_AUTH_TOKEN: $DOT_AI_AUTH_TOKEN
```

## Processing Output

**Extract values with jq:**
```bash
#!/bin/bash
RESULT=$(dot-ai <command> --output json | jq -r '.result')
echo "Result: $RESULT"
```

**Loop over array results:**
```bash
#!/bin/bash
dot-ai resources --kind Deployment --output json | \
  jq -r '.items[].metadata.name' | \
  while read name; do
    echo "Processing: $name"
  done
```

## Configuration Best Practices

**Use environment variables in CI/CD:**
```bash
export DOT_AI_URL="https://dot-ai.example.com"
export DOT_AI_AUTH_TOKEN="${SECRET_TOKEN}"
export DOT_AI_OUTPUT_FORMAT="json"
```

**Don't hardcode credentials:**
```bash
# Bad
dot-ai <command> --token hardcoded-token

# Good
dot-ai <command> --token "${DOT_AI_AUTH_TOKEN}"
```

## Scripting Examples

**Conditional execution:**
```bash
#!/bin/bash
if dot-ai <command> --output json | jq -e '.healthy' > /dev/null; then
  echo "System healthy, proceeding..."
  # Continue with workflow
else
  echo "System unhealthy, aborting"
  exit 1
fi
```

**Retry logic:**
```bash
#!/bin/bash
MAX_RETRIES=3
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  if dot-ai <command>; then
    echo "Success"
    exit 0
  fi

  RETRY_COUNT=$((RETRY_COUNT + 1))
  echo "Retry $RETRY_COUNT/$MAX_RETRIES"
  sleep 5
done

echo "Failed after $MAX_RETRIES attempts"
exit 1
```

## Next Steps

- **[Output Formats](output-formats.md)** — Control output for parsing
- **[Configuration](../setup/configuration.md)** — Environment variables and flags
- **[Commands Overview](cli-commands-overview.md)** — Available commands
