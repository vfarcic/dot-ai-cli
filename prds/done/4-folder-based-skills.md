# PRD #4: Folder-Based Skills — Write Supporting Files

**Status:** Complete
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

Extend `writePromptSkill` to decode and write supporting files from the `files[]` response field. All supporting files get `0o755` permissions.

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
   c. Write with `os.WriteFile` using `0o755`

### File Permissions

All supporting files are written with `0o755` permissions. Supporting files exist to be invoked by `SKILL.md` (shell scripts, executables), so executable permission is the safe and correct default. No heuristic detection is needed.

## Technical Decisions

### Why `0o755` for all supporting files instead of detecting permissions?

Supporting files exist to be invoked by `SKILL.md` — they're shell scripts, executables, or templates that skills reference directly. Making all of them executable is the safe default: a non-script file with execute permission is harmless, but a script without it breaks at runtime. This eliminates the need for heuristic detection (extension matching, shebang parsing) and the edge cases that come with it.

### Why base64 instead of raw text?

Decided in the server-side PRD (#387). Supporting files may contain binary content (images, compiled templates) or characters that don't serialize well in JSON. Base64 is safe and universally supported. Go's `encoding/base64` handles this with zero dependencies.

## Success Criteria

1. `skills generate` writes supporting files alongside `SKILL.md` for folder-based skills
2. Base64 content is correctly decoded to original file bytes
3. All supporting files get `0o755` permissions
4. Nested paths (e.g., `templates/deployment.yaml`) create intermediate directories
5. Prompts without `files[]` (flat prompts, tool skills) continue to work identically
6. Integration tests validate file writing, permissions, and nested paths

## Milestones

- [x] **M1: Unmarshal files from response** — Add `promptFile` struct and `Files` field to `promptRenderResponse`. No behavior change yet, just parse the field from JSON. Verify existing tests still pass (the field is optional, so backward-compatible)
- [x] **M2: Write supporting files to disk** — After writing `SKILL.md` in `writePromptSkill`, iterate over `Files`, base64-decode each, create subdirectories for nested paths, and write to the skill folder. All files get `0o644` initially
- [x] **M3: Executable permissions for supporting files** — All supporting files are written with `0o755` permissions. No heuristic detection needed — supporting files exist to be invoked by SKILL.md, so executable permission is the correct default
- [x] **M4: Update PRD to reflect simplified permission model** — Replace heuristic-based Technical Decisions (extension detection, shebang checking) and Success Criteria (items 3-4) with the actual approach: all supporting files get `0o755`. Remove the permission detection false positives risk
- [x] **M5: Integration tests** — Unit tests for `writePromptSkill` covering flat files, nested paths, permissions, backward compatibility, multiple files, and invalid base64. Integration test `TestSkillsGenerate_WritesSupportingFiles` validates end-to-end flow against mock server with `files[]` fixture

> **Implementation order**: M1 → M2 → M3 → M4 → M5. All milestones complete.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Mock server doesn't return `files[]` for test prompts | Add test fixtures to `dot-ai-mock-server` with folder-based skill responses. Publish updated mock image |
| Large base64 files cause memory issues | Server already enforces 5 MB per-file limit. CLI can trust this — no additional limit needed |
| Unnecessary execute permission on non-script files | Harmless — execute permission on a YAML template or text file has no side effects, while missing execute permission on a script breaks it at runtime |

## Dependencies

- `dot-ai` server PRD #387 — complete. REST API already returns `files[]`
- `dot-ai-mock-server` — complete. Fixture updated with `files[]` field, image published to `ghcr.io/vfarcic/dot-ai-mock-server:latest`
