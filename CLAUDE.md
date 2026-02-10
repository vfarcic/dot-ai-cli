# CLAUDE.md

## Testing

Always redirect test output to `./tmp/test-output.txt` so results can be examined without consuming context. Create the `tmp` directory if it doesn't exist.

```bash
mkdir -p tmp && task test > tmp/test-output.txt 2>&1
```

- Check only the **last 30 lines** (`tail -30 tmp/test-output.txt`) to see if tests passed.
- Read the **full file** only if tests failed and you need to diagnose.
