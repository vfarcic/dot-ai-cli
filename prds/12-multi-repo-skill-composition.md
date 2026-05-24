# PRD #12: Multi-Repo Skill Composition via `--repo` Flag

**Status:** Draft
**Related Issue:** [vfarcic/dot-ai #575](https://github.com/vfarcic/dot-ai/issues/575) (discussion); [vfarcic/dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581) (server-side companion PRD)

## Problem Statement

`dot-ai skills generate` fetches skills from exactly one git repository — the one the server is configured to load via `DOT_AI_USER_PROMPTS_REPO`. Real-world setups commonly need to compose skills from multiple sources with independent credentials:

- Team-of-teams (org-wide shared skills + per-team private skills)
- Public + private split (OSS skills on GitHub + internal skills on a self-hosted GitLab/Forgejo)
- Trust tiers (production skills in a controlled repo, experimental skills in a sandbox)

Current workarounds fail:

- **Aggregator repo** (subtree/CI mirror) loses credential isolation — one token sees all sources.
- **Running `skills generate` twice** doesn't help because the command wipes all `dot-ai-*` files on each run (`internal/skills/generator.go:cleanExisting`), so the second invocation erases the first.

## Solution Overview

Add a single (non-repeatable) `--repo <url>` flag to `dot-ai skills generate`. Each invocation is **self-contained and source-scoped**:

1. CLI fetches from one repo (default = server's env-var repo, or the URL passed via `--repo`).
2. Server returns the prompts and a stable `source` identifier (PRD: [dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581)).
3. CLI scans the output directory for skills whose frontmatter `source:` matches the current source, deletes them, then writes the fresh set tagged with that same `source:`.
4. Skills tagged with *other* sources are left untouched.

Composition across multiple sources is achieved by running the command multiple times — typically as multiple agent hooks, one per source:

```
Hook A:  dot-ai skills generate --repo https://github.com/orgA/skills
Hook B:  DOT_AI_GIT_TOKEN=$TOKEN_B dot-ai skills generate --repo https://gitlab.corp/team/skills
Hook C:  dot-ai skills generate                                    # uses env-var repo
```

Each hook owns its slice. No accumulation, no in-memory merge, no global wipe. Per-source credentials, branches, and paths come from per-hook env vars (the hook is the scoping unit).

## Scope

**In scope:**

- Single `--repo <url>` flag on `dot-ai skills generate` (not repeatable).
- Per-source wipe-and-replace: scan output dir, remove files whose `source:` frontmatter matches the current source, write the fresh set.
- `source:` frontmatter on every generated skill file. Value comes verbatim from the server's response `source` field.
- File lock on the output directory for the duration of generate (prevents concurrent hook corruption).
- Collision policy across sources: **skip + warn**. If a skill name already exists on disk tagged with a different source, log a warning and do not overwrite. First-arrived-wins by invocation order (typically hook firing order).
- Migration path for existing users whose skills predate `source:` frontmatter.
- Integration tests against `dot-ai-mock-server` covering: env-var-only path (unchanged behavior), `--repo` path, cross-source name collision (skip + warn), removed-upstream skill cleanup, concurrent-invocation safety.

**Out of scope (deferred):**

- Repeatable `--repo`. Replaced by hook-per-source model.
- `--branch` / `--path` flags. Per-hook env vars (`DOT_AI_USER_PROMPTS_BRANCH`, `DOT_AI_USER_PROMPTS_PATH`) handle this naturally if the server adds them later. Not needed for MVP.
- Per-repo token flag. Per-hook env vars (`DOT_AI_GIT_TOKEN`) handle this naturally — each hook scopes its own auth realm. Solves the cross-provider auth case (e.g. private GitHub + private GitLab) without server-side `_TOKEN_N` plumbing.
- CLI-local cloning (the "Axis 2" / `--repo-local` proposal from #575). Server-side fetch only. To be revisited as a separate PRD if a VPN-gated-repo use case crystallizes.
- Configuration file (`.dot-ai/skills.yaml` or similar). Hooks already serve as both config and execution mechanism per source.
- Per-source skill-name prefixing. Preserves the `/dot-ai-query` trigger contract; collisions are handled by skip + warn instead.

## Architecture

### Current behavior (single repo, global wipe)

```go
// generator.go:Generate
prompts, err := fetchPrompts(cfg)               // GET /api/v1/prompts
cleanExisting(outDir)                            // wipes ALL dot-ai-* files
for _, p := range prompts {
    rendered := renderPrompt(cfg, p.Name)        // POST /api/v1/prompts/:name
    writePromptSkill(outDir, p, rendered)
}
```

### Proposed behavior (per-source wipe-and-replace)

```go
// generator.go:Generate (sketch)
lock, err := acquireLock(outDir)                                 // flock on outDir/.dot-ai.lock
if err != nil { return fmt.Errorf("another generate is running: %w", err) }
defer lock.Release()

resp, err := fetchPrompts(cfg, skillsRepo)                       // ?repo=<url> if skillsRepo != ""
if err != nil { return err }
source := resp.Source                                            // verbatim from server, e.g. "https://github.com/orgA/skills"

// Wipe only this source's existing skills.
existing, err := scanExistingSkills(outDir)                       // returns map[name]existingSkill{path, source}
if err != nil { return err }
for _, sk := range existing {
    if sk.source == source {
        os.Remove(sk.path)
    }
}

// Write fresh skills for this source, skipping cross-source collisions.
for _, p := range resp.Prompts {
    if other, exists := existing[p.Name]; exists && other.source != source {
        log.Warnf("skipping %q: already provided by source %q (first-source-wins)", p.Name, other.source)
        continue
    }
    rendered := renderPrompt(cfg, p.Name, skillsRepo)
    writePromptSkill(outDir, p, rendered, source)                // writes `source: <url>` into frontmatter
}
```

### Skill frontmatter

Each generated skill file gains a `source:` field in its YAML frontmatter:

```yaml
---
name: query
description: Natural language cluster queries
source: https://github.com/orgA/skills
---
```

The value is whatever the server returned in its `source` field. The CLI does not normalize it — it must match exactly between writes and subsequent wipes.

### Collision policy

When CLI is about to write skill `X` and a file with that name already exists on disk:

| Existing `source:` | Action |
|---|---|
| Same as current invocation's source | Overwrite (this is the normal wipe-and-replace path) |
| Different source | **Skip and log a warning.** First-arrived wins. |
| Missing (legacy skill from pre-`source:` era) | See **Migration** below |

Rationale: this is the trade-off discussed in #575 — the hook-per-source model loses the deterministic flag-ordering that a single-invocation multi-`--repo` would give us. Skip + warn surfaces the conflict visibly without silently overwriting work from another hook.

### Concurrent invocations

Agent hooks may fire in parallel. To prevent two `dot-ai skills generate` invocations from corrupting each other's output, acquire an exclusive `flock` on `<outDir>/.dot-ai.lock` for the duration of generate. If the lock cannot be acquired within a short timeout (~5s), fail with a clear error: "another `dot-ai skills generate` is in progress."

This is cheap insurance against agent hook racing.

### Migration

Existing installations have skill files generated before this PRD landed — they lack `source:` frontmatter entirely. On a fresh `dot-ai skills generate` invocation:

- **If `--repo` is supplied**: untagged legacy files are *not* wiped (they don't belong to this source) and *not* counted in the collision check (skill is written, untagged file gets overwritten if name matches — same as today's wipe-everything behavior, with a one-time log warning).
- **If `--repo` is omitted** (env-var repo): untagged legacy files are assumed to belong to the env-var repo and are wiped as part of normal per-source cleanup. Same effect as today.

In practice this means: first post-upgrade generate from the env-var repo cleans up legacy files automatically. First post-upgrade generate with `--repo` may leave legacy files behind, which the next env-var-repo generate will clean up.

An optional `dot-ai skills migrate` command could explicitly retag legacy files based on a user-supplied source URL — out of scope for MVP unless users hit pain.

## Technical Decisions

### Why single `--repo` instead of repeatable?

Earlier drafts of this PRD made `--repo` repeatable, with the CLI accumulating sources, deduping, and doing a single wipe-then-write. The hook-per-source model (each agent hook owns one source) collapses that complexity entirely: composition becomes the agent's job, not the CLI's. The CLI gets dumber; the model gets cleaner.

The cost is that per-source ordering is no longer expressed in a single command line — collisions are resolved by hook firing order. Mitigated by skip + warn so collisions are visible.

### Why per-source wipe via frontmatter instead of a side-channel manifest?

A manifest file (`.dot-ai/skills.lock`) introduces state that can drift from reality if a skill file is manually deleted or moved. Frontmatter keeps provenance attached to the file itself — no drift possible. Each generate scans the directory and rebuilds its view of "what came from where" from primary source.

### Why skip + warn instead of last-write-wins on cross-source collision?

Last-write-wins is order-dependent in a way the user can't see (depends on which hook fires last). Skip + warn preserves whoever ran first and surfaces the problem so the user can resolve it (rename a skill, drop a source, etc.). The `/dot-ai-query` trigger contract benefits from stability.

### Why file lock instead of "document that hooks must run sequentially"?

Sequential hook configuration is agent-specific and easy to get wrong. A file lock is one line of Go and makes the worst case ("two hooks fired in parallel, output dir is corrupted") impossible.

### Why no config file?

A config file would let users express per-source modifiers (branch, path, token) declaratively. But agent hooks already serve as both config and execution per source — each hook line is one source with its own env. Adding a config file on top would duplicate the responsibility. Revisit only if a concrete user need surfaces that hooks can't express.

### Why no CLI-local cloning?

[#575 discussion](https://github.com/vfarcic/dot-ai/issues/575) raises VPN-gated repos (server can't reach, laptop can) and local directories as use cases. Both require the CLI to do git operations directly, which has knock-on effects on the render path (server doesn't have the source files). That's a different architecture and deserves a separate PRD if/when a concrete user needs it.

## Success Criteria

1. `dot-ai skills generate` with no flags behaves equivalently to today — env-var repo's skills are written, stale ones from the same source are removed.
2. `dot-ai skills generate --repo X` writes X's skills tagged with `source: X` and leaves skills from other sources untouched.
3. Running multiple invocations (different `--repo` values) in sequence composes their skills correctly without clobbering each other.
4. Cross-source name collisions log a warning and preserve the first-arrived skill.
5. Removing a skill upstream and re-running generate for that source removes it from disk.
6. Concurrent invocations are serialized via file lock; the second invocation fails fast with a clear message.
7. Integration tests against the mock server cover all six cases above.

## Milestones

- [ ] **M1: Server contract** — Confirm [dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581) ships with `?repo=` on `GET /api/v1/prompts` and `POST /api/v1/prompts/:name`, plus a stable `source` field in responses. Update mock server fixtures to support `?repo=` and return `source`.
- [ ] **M2: CLI flag** — Add single `--repo <url>` flag to `skills generate`. Plumb through to `Generate()`.
- [ ] **M3: Per-source wipe + frontmatter** — Implement directory scan with `source:` extraction, per-source removal, and `source:` injection on write. Refactor `cleanExisting` from "wipe all" to "wipe matching source."
- [ ] **M4: Collision policy + file lock** — Implement skip + warn on cross-source collision. Acquire `flock` on output dir; fail fast on contention.
- [ ] **M5: Render path** — Pass `?repo=` through to `POST /api/v1/prompts/:name` when generate ran with `--repo`.
- [ ] **M6: Integration tests** — Cover the seven Success Criteria items.
- [ ] **M7: Documentation** — Update `README.md` and any `docs/` references. Document the hook-per-source model with example agent hook configs. Document the legacy-file migration behavior.

> **Implementation order**: M1 → M2 → M3 → M4 → M5 → M6 → M7.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Mock server doesn't support `?repo=` or stable `source` | Block on M1; fixtures must be updated before downstream milestones can be tested. |
| Users hit cross-source name collisions and don't notice | Warning is logged at default verbosity, not debug. Document in `--repo` help text. |
| Hooks racing on the output dir | File lock (M4). Worst case: one of two parallel hooks fails fast with a clear error. |
| Legacy skill files (pre-`source:`) confuse the wipe logic | Migration heuristic in M3: untagged files belong to env-var repo by default. One-time `dot-ai skills migrate` deferred unless needed. |
| Stale `source:` tags after a repo URL change (user re-points a hook to a fork) | Acceptable for MVP — user can `dot-ai skills generate --repo <old-url>` once with the old URL, or manually delete. Document as a known wart. |

## Dependencies

- **[dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581)** — server-side optional `repo` parameter + stable `source` field in response. M1 blocks on that PRD landing.
- **`dot-ai-mock-server`** — fixture support for `?repo=` and `source` field in responses. Image republish required before M6.
