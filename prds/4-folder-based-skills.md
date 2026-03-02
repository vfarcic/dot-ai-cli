# PRD #4: Folder-Based Skills — Write Supporting Files

**Status:** Pending
**Related Issue:** [dot-ai #387](https://github.com/vfarcic/dot-ai/issues/387) (server-side, complete)

## Problem Statement

The `skills generate` command fetches prompts from the server and writes a `SKILL.md` file for each one. The server now supports folder-based skills — a directory containing `SKILL.md` plus supporting files (shell scripts, manifests, templates). The `POST /api/v1/prompts/:name` response includes an optional `files[]` field with base64-encoded supporting files. The CLI ignores this field entirely, so folder-based skills are generated without the files they depend on.

**Current behavior** (`internal/skills/generator.go:276-321`):
```go
func writePromptSkill(dir string, p promptDef, rendered *promptRenderResponse) error {
    // ... writes only SKILL.md, ignores rendered.Data.Files
    return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(b.String()), 0o644)
}
```

Supporting files like `create-worktree.sh` are silently dropped. The generated skill folder contains only `SKILL.md`, so skills that reference supporting files (e.g., `bash ./create-worktree.sh`) fail at runtime.

## Solution Overview

Extend `writePromptSkill` to decode and write supporting files from the `files[]` response field. Detect executable scripts and set appropriate file permissions.

```
Server: POST /api/v1/prompts/worktree-prd
  → response.data.files = [
      { path: "create-worktree.sh", content: "IyEvYmluL2Jhc2g..." }
    ]

CLI: skills generate --agent claude-code
  → .claude/skills/dot-ai-worktree-prd/
      ├── SKILL.md                   (existing behavior)
      └── create-worktree.sh         (NEW: decoded from base64, chmod 0755)
```

## Architecture

### Response Struct Changes

Add `Files` to the existing `promptRenderResponse` in `generator.go`:

```go
type promptFile struct {
    Path    string `json:"path"`
    Content string `json:"content"` // base64-encoded
}

type promptRenderResponse struct {
    Success bool `json:"success"`
    Data    struct {
        Description string       `json:"description"`
        Messages    []promptMsg  `json:"messages"`
        Files       []promptFile `json:"files"`  // NEW
    } `json:"data"`
}
```

No new API calls needed — the `files` field is already present in the response; the CLI just needs to unmarshal and act on it.

### File Writing Flow

In `writePromptSkill`, after writing `SKILL.md`:

1. Iterate over `rendered.Data.Files`
2. For each file:
   a. Base64-decode `Content`
   b. Create subdirectories if `Path` contains `/` (e.g., `templates/deployment.yaml`)
   c. Detect if file is executable (see permission rules below)
   d. Write with `os.WriteFile` using `0o755` (executable) or `0o644` (regular)

### Executable Permission Detection

A file gets `0o755` permissions if **any** of these are true:
- **Extension**: `.sh`, `.bash`
- **Shebang**: Decoded content starts with `#!` (first two bytes)

This covers shell scripts regardless of extension (e.g., `bootstrap` with `#!/bin/bash`) and standard script extensions even without shebangs. Detection happens after base64 decoding on the raw bytes.

## Technical Decisions

### Why detect permissions client-side instead of server-side?

The server serves raw file content without tracking permissions. This is the right boundary — the server doesn't know what OS or filesystem the client runs on. The CLI is the first consumer that writes files to disk, so it owns permission semantics. If a future Web UI needs different behavior, it makes its own decisions.

### Why only `.sh` and `.bash` extensions?

These are the only extensions called out in the parent PRD (#387). Python, Ruby, and other script extensions are best handled by their interpreters (`python script.py` works without execute permission). Shell scripts are unique in that they're typically invoked directly (`./script.sh`) and require execute permission.

### Why check shebang in addition to extension?

Skill authors may use extensionless scripts (e.g., `bootstrap`, `setup`) which are common in the Unix ecosystem. Checking for `#!` as the first two bytes catches these reliably with zero false positives — no non-script file starts with `#!`.

### Why base64 instead of raw text?

Decided in the server-side PRD (#387). Supporting files may contain binary content (images, compiled templates) or characters that don't serialize well in JSON. Base64 is safe and universally supported. Go's `encoding/base64` handles this with zero dependencies.

## Success Criteria

1. `skills generate` writes supporting files alongside `SKILL.md` for folder-based skills
2. Base64 content is correctly decoded to original file bytes
3. Shell scripts (`.sh`, `.bash`, or shebang `#!`) get `0o755` permissions
4. Non-script files get `0o644` permissions
5. Nested paths (e.g., `templates/deployment.yaml`) create intermediate directories
6. Prompts without `files[]` (flat prompts, tool skills) continue to work identically
7. Integration tests validate file writing, permissions, and nested paths

## Milestones

- [x] **M1: Unmarshal files from response** — Add `promptFile` struct and `Files` field to `promptRenderResponse`. No behavior change yet, just parse the field from JSON. Verify existing tests still pass (the field is optional, so backward-compatible)
- [x] **M2: Write supporting files to disk** — After writing `SKILL.md` in `writePromptSkill`, iterate over `Files`, base64-decode each, create subdirectories for nested paths, and write to the skill folder. All files get `0o644` initially
- [ ] **M3: Executable permission detection** — After writing each file, check extension (`.sh`, `.bash`) and shebang (`#!` prefix on decoded bytes). Set `0o755` for matches. Add `isExecutable(path string, content []byte) bool` helper
- [ ] **M4: Integration tests** — Add test fixtures to the mock server for a folder-based skill with supporting files (including a `.sh` script and a nested path). Test that `skills generate` writes all files with correct content and permissions

> **Implementation order**: M1 → M2 → M3 → M4. Each milestone builds on the previous. M1 is pure struct change with no risk. M2 adds the core file writing. M3 layers on permission detection. M4 validates the full flow against the mock server.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Mock server doesn't return `files[]` for test prompts | Add test fixtures to `dot-ai-mock-server` with folder-based skill responses. Publish updated mock image |
| Large base64 files cause memory issues | Server already enforces 5 MB per-file limit. CLI can trust this — no additional limit needed |
| Permission detection false positives | Shebang check is `#!` as first 2 bytes — virtually zero false positives. Extension check is an exact match on `.sh`/`.bash` only |

## Dependencies

- `dot-ai` server PRD #387 — complete. REST API already returns `files[]`
- `dot-ai-mock-server` — needs updated fixtures for folder-based skill responses (can be done as part of M4)
