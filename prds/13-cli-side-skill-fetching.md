# PRD: CLI-Side Fetching for Skill Sources Unreachable by the Server

> **GitHub Issue:** [vfarcic/dot-ai-cli#13](https://github.com/vfarcic/dot-ai-cli/issues/13)
> **Hard companion dependency:** a **not-yet-filed** server-side ingestion endpoint in `vfarcic/dot-ai` (`POST /api/v1/prompts/sources`). This feature is **not shippable without it** — see [Blocking Dependency](#blocking-dependency-must-resolve-first). Both halves must ship in the same release.

**Status**: In Progress — **M0 resolved** (companion `vfarcic/dot-ai` ingestion endpoint delivered via [PR #655](https://github.com/vfarcic/dot-ai/pull/655), mock pinned to `sha256:97b9bee85…`); **M1 complete** (flags + validation, commit `cf80617`); **M2 implemented + security-hardened** (`--repo-dir` upload/render via `?source=`, commit `86ef73a`) but **NOT fully verified** — generating brand-new skills from an uploaded source needs a **list-by-source endpoint** (`GET /api/v1/prompts?source=`) the frozen contract omits, now requested from `vfarcic/dot-ai` PRD #647; render-by-`?source=` is verified, the enumerate-new-skills e2e test is `t.Skip`'d until the mock is republished. **M3 is gated on the same endpoint.** Scope below is deliberately re-cut for the **post-#16 reality** (see next section).
**Priority**: Medium (the highest-value slice of the original #13 was absorbed by #16; what remains is real but narrower — see Motivation)
**Related Issues**: [`vfarcic/dot-ai-cli#12`](https://github.com/vfarcic/dot-ai-cli/issues/12) (the `--repo` flag + source-tagging pipeline this builds on); [`vfarcic/dot-ai-cli#16`](https://github.com/vfarcic/dot-ai-cli/issues/16) (per-request path/branch/**credential** forwarding — **the reason this PRD is re-scoped**); [`vfarcic/dot-ai#581`](https://github.com/vfarcic/dot-ai/issues/581) (server `?repo=` override); [`vfarcic/dot-ai#575`](https://github.com/vfarcic/dot-ai/issues/575) (original multi-source / per-hook multi-realm discussion); **TBD** — companion `vfarcic/dot-ai` ingestion-endpoint issue (to be filed before implementation starts).
**External stakeholder**: [@vtmocanu](https://github.com/vfarcic/dot-ai-cli/issues/13#issuecomment-4565887861) is waiting on this; keep them updated as scope narrows.

## Why This PRD Is Re-Scoped (read first)

Issue #13 was filed on 2026-05-24, **before** [#16](https://github.com/vfarcic/dot-ai-cli/issues/16) landed. #16 added forwarding of the CLI host's `DOT_AI_GIT_TOKEN` as the `X-Dot-AI-Git-Token` header on `--repo` override requests, so the **server** can now clone a private, cross-realm git source using a **per-hook static credential supplied from the CLI host**.

That directly absorbs the largest slice of #13's original motivation. #13's own tradeoff table conceded it: *"The VPN allows programmatic egress with a static credential" → the server-side / network-level fix works.* With #16 shipped, that row is **already solved** — a network-isolated host reachable with a static token no longer needs CLI-side fetching; it needs `--repo <url>` + `DOT_AI_GIT_TOKEN`.

**This PRD therefore deliberately leads with the cases #16 does *not* solve**, and de-emphasizes the static-token case:

| Original #13 use case | Post-#16 status | Covered by this PRD? |
|---|---|---|
| Network-isolated host, **static credential** works | **Solved by #16** (`--repo` + `DOT_AI_GIT_TOKEN`) | ❌ Out — use #16 |
| VPN needs **SSO / OIDC / device attestation** (no static credential exists) | Unsolved — no token to hand the server | ✅ **Primary** (`--repo-fetch`) |
| **Managed / hardened k8s**: operator can't deploy an egress pod **and** has no static-token path | Unsolved — server simply cannot reach the source | ✅ **Primary** (`--repo-fetch`) |
| **On-disk WIP skills** for dev loops (no remote at all) | Unsolved — nothing to fetch server-side | ✅ **Primary** (`--repo-dir`) |
| Heterogeneous per-source boundaries needing per-user (not service-account) creds | Partially solved by #16 for static-token sources | ⚠️ Secondary (only the non-static-token members) |

If, during implementation, a candidate source turns out to be reachable with a static token, the correct answer is **#16, not this feature**. CLI-side fetching is the escape hatch for sources where the server *fundamentally cannot* authenticate or route — not a general alternative to server-side fetch.

## Blocking Dependency (must resolve first)

This feature **cannot be delivered from `dot-ai-cli` alone.** The chosen render design (below) requires a new server endpoint that **does not yet exist and has not yet been filed**:

- **`POST /api/v1/prompts/sources`** (name TBD) on `vfarcic/dot-ai` — accepts skill source **uploaded by the CLI**, caches it under the same key space the existing git-fetch cache uses, keyed by a stable source identifier (the git URL for `--repo-fetch`, `local:<label>` for `--repo-dir`). The existing `POST /api/v1/prompts/:name` render path then looks the source up by identifier and renders it **unchanged**.

**Action before any CLI implementation begins:** file the companion issue on `vfarcic/dot-ai`, agree the contract (endpoint shape, identifier key space, cache lifecycle, content-hash dedup, source-label uniqueness rules), and get the mock image updated to mirror it. The CLI cannot be meaningfully tested until the mock exposes the ingestion endpoint. **Both halves ship in the same release** or the flags produce skills the server can't render.

> This PRD covers **only** the CLI-side work (new flags, source upload, local clone cache). The server-side ingestion endpoint, its cache lifecycle, and the per-server source-label uniqueness policy are tracked by the companion `vfarcic/dot-ai` issue.

## Problem

The current architecture fetches **all** skill sources server-side: the CLI forwards a repo URL (`--repo`, optionally `--repo-path`/`--repo-branch`/`DOT_AI_GIT_TOKEN`) and the dot-ai server does the clone. That assumes the server can both **reach** the source and **authenticate** to it. After #16, that assumption holds for every source reachable with a static credential. Two classes of source still break it — and in both, **the developer's machine (where `dot-ai-cli` already runs) can read the source fine**:

1. **Sources the server cannot authenticate or route to, even with a static token.** VPNs gated by SSO / OIDC / device attestation (no static credential to hand the server); managed or hardened clusters where the operator can neither deploy an egress sidecar nor produce a service-account token for the source. The user's laptop holds the live SSO session / device posture / network route; the server never will.
2. **On-disk directories.** Work-in-progress skills on a developer's filesystem with no remote at all — needed for tight dev loops where pushing to a remote on every change is friction.

In both cases the laptop can fetch; the server can't. There is no CLI surface for "the CLI reads the source, the server only renders it."

## Solution

Add two additive, mutually-exclusive source flags to `dot-ai skills generate`, each a complete source per invocation (same one-source-per-invocation shape as `--repo`):

- **`--repo-fetch <git-url>`** `[--repo-path <subdir>] [--repo-branch <branch>]` — the **CLI** clones (and caches) the repo using the **host's local git stack** (SSH agent, `git credential`, `~/.gitconfig`, `GIT_SSH_COMMAND`, `GIT_CONFIG_GLOBAL`, `GIT_TERMINAL_PROMPT=0`, …), then uploads the resulting skill source to the server's ingestion endpoint. `source:` frontmatter = the git URL verbatim (credentials scrubbed, reusing #16's `RedactURL`).
- **`--repo-dir <path> --source-label <label>`** — the CLI reads skill source from a local directory (no network, no clone) and uploads it. `source:` frontmatter = `local:<label>`. `--source-label` is **required** (a path is not a stable cross-machine identifier).

Both reuse the existing #12/#16 pipeline:
- Skills are tagged with `source:` frontmatter so the **wipe-own-slice** pipeline (`internal/skills/generator.go`, `frontmatter.go`) works unchanged — each hook wipes only files matching its own `source:` and leaves others alone.
- Cross-source name collisions → **skip + warn, first-arrived-wins** (matching #12's policy).
- Output-dir concurrency reuses the existing `flock` (`internal/skills/lock.go`). `--repo-fetch` additionally takes a **per-URL `flock` on its clone cache dir**.

The server renders exactly as today — **one renderer, server-side** — the only change is how the source reaches the server's cache (CLI upload instead of server-side git clone).

### Why CLI-uploads-source-server-renders (and not the alternatives)

Most dot-ai skills take arguments (`dot-ai-recommend "deploy postgres"`, the `dot-ai-prd-*` family, …) and rely on the server's renderer to substitute them at invocation time. To keep that working for CLI-sourced skills:

- **Chosen — CLI uploads source, server caches + renders.** Preserves the single-renderer property; all argument-taking skills behave identically to `--repo`; purely additive server-side; symmetric with the existing "fetch git → cache → render on demand" model (just a different ingestion path). Cost is bandwidth (kilobytes–low-MB per `generate`), gated by a content hash so unchanged sources skip re-upload.
- **Rejected — CLI-side rendering (re-implement the template engine in Go).** Forks the renderer between CLI and server forever; maintainers want one renderer.
- **Rejected — pre-rendered only (skip rendering at the CLI).** Skips argument substitution entirely, excluding essentially every operational dot-ai skill — a strictly worse feature than the user stories ask for.

## Scope

**In scope (CLI work):**
- `--repo-fetch <git-url>` with optional `--repo-path` / `--repo-branch` reuse, CLI-side clone via the host git stack, source upload to the ingestion endpoint, `source: <scrubbed-url>` tagging.
- `--repo-dir <path>` + **required** `--source-label <label>`, local read (no network), source upload, `source: local:<label>` tagging.
- Mutual exclusion among `--repo`, `--repo-fetch`, `--repo-dir` (clear usage error when more than one is supplied), and the existing rule that `--repo-path`/`--repo-branch` qualify a repo-bearing flag.
- Clone cache for `--repo-fetch`: XDG-respecting dir (`~/.cache/dot-ai-cli/repos/<sha256(url)>/`, honoring `XDG_CACHE_HOME`), `git clone --depth 1` on first fetch, `git -C <dir> fetch --depth 1` + checkout on subsequent runs, per-URL `flock` on the cache dir, `--no-cache` for a fresh clone-to-temp-then-delete, and a `dot-ai skills cache prune --older-than <duration>` GC subcommand.
- Content-hash gating so an unchanged source skips re-upload on subsequent hook fires.
- `--repo-dir` security posture (default-off, opt-in): require `DOT_AI_ALLOW_REPO_DIR=1`; optional base-path allowlist; refuse paths under `/tmp` or world-writable dirs.
- Hook-builder support (`internal/skills/hook.go`): emit the new flags so `--install-hook` round-trips them.
- Ensure no credential / credentialed URL reaches logs, stdout/stderr, or generated frontmatter (reuse #16's `RedactURL` / `RedactCredentials`).
- Help text + docs for the new flags, the cache, and the `--repo-dir` opt-in.
- Tests against the mock image **once it exposes the ingestion endpoint**.

**Out of scope:**
- The server-side ingestion endpoint and its cache lifecycle (companion `vfarcic/dot-ai` issue).
- The **static-credential** network-isolated case — **use #16** (`--repo` + `DOT_AI_GIT_TOKEN`); explicitly not re-solved here.
- `--repo-fetch` + `DOT_AI_GIT_TOKEN` mixing — `--repo-fetch` authenticates via the host's local git stack, not the env-var token.
- Auto-detecting an unreachable `--repo` and transparently falling back to `--repo-fetch` — explicit-flag-per-source is the design; implicit failover hides outages.
- CLI-side template rendering (rejected above).
- Skipping the server for invocation/rendering — render still happens server-side.

## Backward Compatibility (Non-Negotiable)

- All new flags are opt-in. `dot-ai skills generate` with no new flags is **byte-identical** to current behavior (no upload, no cache, no new requests).
- Existing `--repo` / `--repo-path` / `--repo-branch` / `DOT_AI_GIT_TOKEN` behavior is unchanged.
- Skill tagging by `source:` and the wipe-own-slice semantics are unchanged — `--repo-fetch` / `--repo-dir` are just additional `source:` values.

## Success Criteria

- `dot-ai skills generate --repo-fetch <git-url>` succeeds when the URL is **unreachable/unauthenticatable from the server but reachable from the CLI host** (e.g. an SSO/device-attested VPN or a clone that only the host's git credentials can do), producing skills with `source: <scrubbed-url>` frontmatter.
- Re-running the same `--repo-fetch <git-url>` produces a `git fetch` (not a full re-clone), completing in O(diff-size); the source is re-uploaded only if its content hash changed.
- `dot-ai skills generate --repo-dir <path> --source-label foo` reads from `<path>` with **zero network calls** for the fetch step and produces `source: local:foo` frontmatter; `--repo-dir` without `--source-label` fails with a clear, non-zero-exit error naming the missing flag; `--repo-dir` without `DOT_AI_ALLOW_REPO_DIR=1` is refused.
- `--repo`, `--repo-fetch`, `--repo-dir` are mutually exclusive; supplying two errors with a clear message.
- Two concurrent `--repo-fetch <same-url>` invocations serialize via the per-URL cache `flock`; the second sees the first's checkout, no race.
- An **argument-taking** skill loaded via `--repo-fetch` or `--repo-dir` and invoked through the server renders correctly with substituted arguments — behavior identical to the same skill loaded via `--repo`.
- Composition: several hooks in sequence (env-var default + `--repo` + `--repo-fetch` + `--repo-dir`, distinct identifiers) produce a `~/.claude/commands/` containing skills from every source, no clobbering between sources, deterministic first-arrived-wins on any cross-source name collision.
- No credential or credentialed URL appears in any log, stdout/stderr, or generated frontmatter.

## Milestones

> **M0 gates everything.** No CLI milestone below can be verified end-to-end until the companion server ingestion endpoint exists and the mock mirrors it.

- [x] **M0 — Companion server dependency resolved.** File the `vfarcic/dot-ai` ingestion-endpoint issue; agree the contract (endpoint, identifier key space, cache lifecycle, content-hash dedup, `local:<label>` per-server uniqueness policy); confirm the mock image exposes it. **Blocking prerequisite for M2–M6.**
- [x] **M1 — Flags, mutual exclusion, usage UX.** Add `--repo-fetch`, `--repo-dir`, `--source-label`; enforce mutual exclusion with `--repo` and each other; `--repo-dir` requires `--source-label`; reuse the `--repo-path`/`--repo-branch` qualifier rule. (No network/upload yet — pure CLI surface + validation.)
- [ ] **M2 — `--repo-dir` local source end-to-end.** Read source from a local dir, upload to the ingestion endpoint, tag `source: local:<label>`, render an argument-taking skill correctly via the server. Security posture: `DOT_AI_ALLOW_REPO_DIR=1` opt-in + path refusals.
- [ ] **M3 — `--repo-fetch` network source end-to-end.** CLI clone via the host git stack, upload source, tag `source: <scrubbed-url>`, render correctly. Verify a source the server cannot reach but the host can.
- [ ] **M4 — Clone cache, concurrency, GC.** XDG cache dir, shallow clone + incremental fetch, per-URL `flock`, `--no-cache`, content-hash upload gating, `skills cache prune --older-than`.
- [ ] **M5 — Composition + hook round-trip.** Source-tagged wipe-own-slice and first-arrived-wins verified across env-var + `--repo` + `--repo-fetch` + `--repo-dir`; `--install-hook` emits and round-trips the new flags.
- [ ] **M6 — Tests against the updated mock.** Integration/e2e coverage for all of the above against the mock once it exposes the ingestion endpoint (per the project's integration-test convention).
- [ ] **M7 — Docs.** Flag reference, the cache + `cache prune`, the `--repo-dir` opt-in/security model, a worked multi-source composition example, and explicit guidance to **prefer #16 (`--repo` + `DOT_AI_GIT_TOKEN`) whenever a static credential works**.

## Tradeoffs (carried from #13, still in force)

- **Loss of centralized credential audit.** Server-side fetch uniquely enables "all skill-fetch credentials live in one place, rotated/audited centrally." CLI-side fetching trades that for per-user creds on developer laptops. Orgs needing centralized audit should stay on #16's `--repo` + `DOT_AI_GIT_TOKEN` (plus a network-level fix where viable).
- **Wider supply-chain trust boundary for `--repo-dir`.** It accepts any filesystem path — a side-loading vector for arbitrary skill code. Default-off + opt-in (`DOT_AI_ALLOW_REPO_DIR=1`) is the minimum; allowlist + refuse-world-writable are the hardening.
- **No automatic failover.** A server-unreachable `--repo` will **not** transparently fall back to `--repo-fetch`; explicit hooks per source mean explicit, observable failure modes.
- **`--source-label` uniqueness is per-server, not per-host.** Two hosts using `local:foo` against the same server collide in the server's source cache. Convention (`local:<user>-<label>` / `local:<host>-<label>`, or CLI auto-prefix with `$USER`/`$HOSTNAME`) must be nailed down in the **companion server issue**.

## Open Questions

- **Does the candidate set of sources actually justify the build post-#16?** With #16 covering every static-token source, confirm there are real sources that need SSO/device-attested fetch or are genuinely server-unroutable, plus the `--repo-dir` dev-loop demand — before committing M1–M7. (External stakeholder @vtmocanu is a data point here.)
- **`--source-label` namespacing** — CLI auto-prefix (`$USER`/`$HOSTNAME`) vs. documented convention vs. server-enforced uniqueness. Decide jointly with the companion server issue.
- **Cache location & GC defaults** — confirm `~/.cache/dot-ai-cli/repos/` (XDG) and the default `cache prune` retention.
- **Content-hash scope** — hash the rendered source set vs. the raw checkout; how it interacts with `--repo-path` subdir selection.
