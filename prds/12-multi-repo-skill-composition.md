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

Add a repeatable `--repo <url>` flag to `dot-ai skills generate`. Each `--repo` instructs the CLI to additionally fetch from that repository via the server's new optional `repo` parameter (PRD-side: [dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581)). The CLI does the orchestration: N+1 fetches, accumulate in memory, dedupe on name with first-wins, single wipe, single write.

```
dot-ai skills generate --agent claude-code \
    --repo https://github.com/orgA/skills \
    --repo https://github.com/orgB/skills

  ├── GET  /api/v1/prompts                         (server's env-var repo + built-ins)
  ├── GET  /api/v1/prompts?repo=...orgA/skills    (orgA's skills)
  ├── GET  /api/v1/prompts?repo=...orgB/skills    (orgB's skills)
  │
  └── merge → dedupe by name (first-wins) → cleanExisting() once → write all
```

## Scope

**In scope:**
- Repeatable `--repo <url>` flag on `dot-ai skills generate`.
- CLI orchestration: call `fetchPrompts` once per source (default + each `--repo`), merge results, dedupe by name (first repo wins, defaults to env-var repo).
- Render path: when rendering a prompt that came from a `--repo` source, pass that `repo` URL through to `POST /api/v1/prompts/:name?repo=<url>` so the server fetches the rendered content from the right place.
- Single `cleanExisting()` call before writing — guarantees no clobbering across sources.
- Partial-failure handling: if any source fails (network error, invalid URL, etc.), abort the generate with a clear error message listing which source failed. Don't write partial results.
- Integration tests against `dot-ai-mock-server` covering: single-repo (no override, unchanged behavior); two-repo composition; collision (same name from two repos, first wins); one source failing.

**Out of scope (deferred):**
- `--branch` per repo. Server defaults branch to `main`; matches existing env-var default. Add later if needed.
- `--path` per repo. Server defaults to repo root. Add later if requested for monorepo setups.
- Per-repo tokens (`--token-for-<url>` or paired env vars). MVP uses the server's single `DOT_AI_GIT_TOKEN`. Limitation: a user with repos across different providers (GitHub OSS + private GitLab) cannot authenticate to both at once. To be revisited when requested.
- Parallel fetching of sources. MVP fetches sequentially. Easy to parallelize later if generate runtime becomes noticeable.
- Per-repo prefix on skill names (the "Option A" framing from dot-ai#575). Current first-wins collision policy preserves the trigger contract (`dot-ai-query` stays `dot-ai-query`). Prefixing would be a breaking UX change and is not part of this PRD.

## Architecture

### Current behavior (single repo)

```go
// generator.go:Generate
prompts, err := fetchPrompts(cfg)               // GET /api/v1/prompts
// ... filter, cleanExisting, write
for _, p := range prompts {
    rendered := renderPrompt(cfg, p.Name)        // POST /api/v1/prompts/:name
    writePromptSkill(outDir, p, rendered)
}
```

### Proposed behavior (multi-repo)

```go
// generator.go:Generate (sketch)
sources := []string{""}                          // "" = default (env-var repo + built-ins)
sources = append(sources, skillsRepos...)        // each --repo URL

type sourcedPrompt struct {
    prompt promptDef
    repo   string                                // "" for default, URL for --repo source
}

merged := map[string]sourcedPrompt{}             // first-wins by name
for _, repo := range sources {
    list, err := fetchPromptsFromSource(cfg, repo)
    if err != nil {
        return "", fmt.Errorf("fetch from %q: %w", displayRepo(repo), err)
    }
    for _, p := range list {
        if _, exists := merged[p.Name]; exists {
            continue                              // first-wins (default + --repo args in order)
        }
        merged[p.Name] = sourcedPrompt{prompt: p, repo: repo}
    }
}

// ... filter, cleanExisting (once), write all
for _, sp := range merged {
    rendered := renderPromptFromSource(cfg, sp.prompt.Name, sp.repo)
    writePromptSkill(outDir, sp.prompt, rendered)
}
```

New helpers:

```go
func fetchPromptsFromSource(cfg *config.Config, repo string) ([]promptDef, error) {
    path := "/api/v1/prompts"
    if repo != "" {
        path += "?repo=" + url.QueryEscape(repo)
    }
    // ... unchanged from fetchPrompts otherwise
}

func renderPromptFromSource(cfg *config.Config, name, repo string) *promptRenderResponse {
    path := "/api/v1/prompts/" + url.PathEscape(name)
    if repo != "" {
        path += "?repo=" + url.QueryEscape(repo)
    }
    // ... unchanged from renderPrompt otherwise
}
```

### CLI flag

```go
// cmd/skills.go
var skillsRepos []string

func init() {
    skillsGenerateCmd.Flags().StringArrayVar(&skillsRepos, "repo", nil,
        "Additional git repository URL to fetch skills from (repeatable). " +
        "Skills from these repos are composed with the server's configured repo. " +
        "On name collision, first source wins (server's configured repo, then --repo flags in order).")
    // ... existing flag registrations
}
```

`Generate()` signature gains a `repos []string` parameter; existing call sites pass `nil`/empty.

### Collision policy

First-source wins, in this order:
1. Server's env-var-configured repo + built-ins (always fetched, even if `--repo` is supplied)
2. `--repo` flags in the order they appear on the command line

When a collision occurs, the later source's skill is silently dropped from the merged result. Add a log message at debug or info level identifying the dropped duplicate to aid debugging without flooding normal output.

### Partial-failure handling

If any single source fails, the entire generate aborts before `cleanExisting()` runs. Rationale: a partial generate is worse than no generate — the user might end up with a stale skill set from the previous run plus a confusing error, or a half-written new skill set. Atomic-on-failure is cleaner.

Exception: if the default source (env-var repo) succeeds but a `--repo` source fails because of a network blip, the user can still re-run with that `--repo` omitted. The error message must clearly identify which source failed and how.

## Technical Decisions

### Why N+1 server calls instead of a single server-side list-of-repos call?

Server-side composition (PRD-B in dot-ai#575) would need indexed env vars (`DOT_AI_USER_PROMPTS_REPO_2`, etc.) parsed at startup, a server-side per-repo cache map, and a collision policy enforced on the server. All of that is bigger than the minimal server change being proposed in [dot-ai#581](https://github.com/vfarcic/dot-ai/issues/581) (one optional query parameter).

Moving the orchestration to the CLI keeps the server change tiny and gives the user per-invocation flexibility — they can compose differently in different sessions without changing server config.

### Why first-source-wins instead of prefixing skill names?

Prefixing (`dot-ai-orgA-query`, `dot-ai-orgB-query`) would change the trigger contract for every existing user — `/dot-ai-query` would no longer work. First-source-wins preserves the contract: the primary source's skills keep their names, additional sources extend the catalog with non-conflicting skills.

### Why fetch the default source even when `--repo` is supplied?

The env-var-configured repo is the admin-set baseline. Users supplying `--repo` are *extending* the catalog, not *replacing* it. Replacing would surprise users who expect the server's built-in skills (like the routing skill and tool skills) to remain available.

If a user truly wants to skip the default, that's a future `--no-default-repo` flag — out of scope here.

### Why abort on partial failure instead of skipping the failed source?

A skip-and-continue model means the user might not notice that orgB's skills are missing — they'd just see fewer skills with no error. Atomic-on-failure surfaces the problem loudly and gives the user a chance to fix the URL/connectivity before re-running. The cost is a single failed generate; the benefit is correctness.

## Success Criteria

1. `dot-ai skills generate` with no `--repo` flag behaves identically to today (no regression).
2. `dot-ai skills generate --repo X` writes the union of the server's default skills and X's skills, with a single wipe of the output directory.
3. `--repo X --repo Y` writes the union of all three sources (default + X + Y).
4. Collisions are resolved first-source-wins; a log line identifies dropped duplicates.
5. If any source fails, no files are written and the error identifies which source failed.
6. Integration tests against the mock server cover all four cases above plus rendering a prompt that came from a `--repo` source (verifies the `?repo=` query param plumbing).

## Milestones

- [ ] **M1: Server contract finalized** — Confirm [dot-ai#581](https://github.com/vfarcic/dot-ai/issues/581) merges with `?repo=` on both `GET /api/v1/prompts` and `POST /api/v1/prompts/:name`. Update mock server fixtures to support the query parameter.
- [ ] **M2: CLI flag plumbing** — Add `--repo` repeatable flag to `skills generate`. Thread through to `Generate()` as a `repos []string` parameter. Existing call sites pass `nil`.
- [ ] **M3: Multi-source fetch + merge** — Implement `fetchPromptsFromSource` and `renderPromptFromSource` helpers. Build the `sourcedPrompt` map with first-source-wins dedup. Single `cleanExisting()` + write loop using the merged map.
- [ ] **M4: Partial-failure handling** — Abort on any source error before any disk write. Error message identifies failing source.
- [ ] **M5: Integration tests** — Cover the five Success Criteria items. Reuse existing `runCLI` harness with mock-server fixtures for multi-repo responses.
- [ ] **M6: Documentation update** — Update `README.md` and any `docs/` references to mention `--repo`. Cross-link to [dot-ai#581](https://github.com/vfarcic/dot-ai/issues/581).

> **Implementation order**: M1 → M2 → M3 → M4 → M5 → M6.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Mock server doesn't support the `?repo=` query param | Add fixtures keyed by `?repo=` value to `dot-ai-mock-server`; publish updated image as part of M1. |
| Users hit the cross-provider token limitation | Document the single-token MVP limitation prominently in `--repo` help text and docs. Track demand for per-repo tokens. |
| First-source-wins masks a configuration mistake (user expected orgB's skill to win) | Log dropped-duplicate names at info level (visible in default verbosity) so users can spot unexpected collisions. |
| Slow generate when many `--repo` flags are supplied (sequential fetches) | Accept for MVP; parallelize later if needed. Per-server clone is already `--depth 1` so dominated by network round-trips, not data transfer. |

## Dependencies

- **[dot-ai #581](https://github.com/vfarcic/dot-ai/issues/581)** — server-side optional `repo` parameter on the prompts endpoints. M1 of this PRD blocks on that PRD landing.
- **`dot-ai-mock-server`** — fixture support for `?repo=` query parameter. Image republish required before M5 can complete.
