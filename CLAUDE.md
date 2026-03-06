# CLAUDE.md

## Testing

Always redirect test output to `./tmp/test-output.txt` so results can be examined without consuming context. Create the `tmp` directory if it doesn't exist.

```bash
mkdir -p tmp && task test > tmp/test-output.txt 2>&1
```

- Check only the **last 30 lines** (`tail -30 tmp/test-output.txt`) to see if tests passed.
- Read the **full file** only if tests failed and you need to diagnose.

## Integration Tests

- All tests are integration tests using the `//go:build integration` tag, run via `task test` (which starts/stops the mock server automatically)
- Tests use the binary-subprocess pattern: build the CLI in `TestMain`, then run it via `exec.Command` with `--server-url http://localhost:3001` (see `runCLI` helper in `integration_test.go`)
- The mock server (`ghcr.io/vfarcic/dot-ai-mock-server`) provides fixture responses for all endpoints — **prefer integration tests over unit tests with inline `httptest` servers**
- Do NOT write standalone unit tests with `httptest.NewServer` for functionality the mock server already supports
