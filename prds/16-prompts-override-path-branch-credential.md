# PRD: CLI Support for Per-Request Path, Branch, and Credential on the Prompts Repo Override

> **GitHub Issue:** [vfarcic/dot-ai-cli#16](https://github.com/vfarcic/dot-ai-cli/issues/16)
> **Companion PRD:** the CLI-side counterpart to the **already-implemented** server work in `vfarcic/dot-ai` PRD #621 (full server contract below). Server PR: `vfarcic/dot-ai#633`.

**Status**: In Progress — C1–C5 implemented and verified (full `task test` green, two review/audit rounds, no blocking findings); PR pending review, CI, and manual validation. Residual LOW follow-ups noted below.
**Priority**: Important (workarounds exist but are painful — a secondary `--repo` source in a subdirectory, on a non-default branch, or in a different auth realm is currently unreachable from the CLI)
**Related Issues**: `vfarcic/dot-ai-cli#15` (the `--repo` flag this builds on); `vfarcic/dot-ai#621` (server contract, implemented); `vfarcic/dot-ai#581` / `#607` (the original `?repo=` override); `vfarcic/dot-ai#575` (per-hook multi-realm auth discussion)

## Resolved Dependencies (2026-06-13)

> **Mock-image dependency: RESOLVED.** The original PRD treated "obtain and pin the `#621` mock tag" as an open prerequisite of milestone C4, with a `<MOCK_IMAGE_TAG_FROM_PRD_621_RELEASE>` placeholder. That is now done:
>
> - **Pinned in `docker-compose.yml`** to digest `sha256:a32e2b75c7f1178723ef21611c7889ecd12e38c9d460154890d525956c7793d4` (also published as immutable tag `1.22.0`, matching the `dot-ai v1.22.0` release that introduced the `#621` contract).
> - **Assumption corrected:** there is **no per-release mock tag** to pin by name — the mock is published **manually** to `:latest` (now plus a version tag). The CLI pins by **digest**.
> - **Verified on this digest** (probes run against the pulled image, all green): `skill-on-branch` appears only with both `path`+`branch`; `refresh` reports `promptsLoaded: 5`; invalid `path`/`branch` → `400 VALIDATION_ERROR`; `X-Dot-AI-Git-Token` accepted but never echoed; credentialed repo URLs scrubbed in `source`.
> - **Regression check:** full existing integration suite passes against the new digest (`go test -tags integration ./...` → 0 failures).
>
> **Operational risk to watch:** `dot-ai` deliberately did **not** automate mock publishing in CI/release this round (scope decision) — publishing stays manual via their `/publish-mock-server` skill. The mock can therefore silently drift behind the server after a future contract-affecting merge. If C4 tests ever start "passing while exercising nothing," the fix is to have `dot-ai` re-publish the mock and then re-pin the digest here.

## Problem

`vfarcic/dot-ai-cli#15` added `dot-ai skills generate --repo <url>`, forwarding a single repo URL to the server's per-request prompts override. That override is intentionally minimal — it carries **only** the repo URL. As of `vfarcic/dot-ai#621` (now shipped server-side), the override accepts three more optional, additive inputs, but the CLI has no way to send any of them:

1. **Subdirectory.** A repo that keeps its skills under `skills/` (the same layout an env-var repo uses via `DOT_AI_USER_PROMPTS_PATH`) cannot be consumed as a `--repo` source — the CLI can only point at the repo root.
2. **Branch.** A source on a non-default branch is unreachable — the CLI override always resolves `main`.
3. **Credential.** The override clone authenticates with the *server's* single `DOT_AI_GIT_TOKEN`, so a second repo in a different auth realm (another Forgejo, a private GitHub, a private GitLab) cannot be authenticated from the CLI. The per-hook `DOT_AI_GIT_TOKEN` set on the CLI host never reaches the server's clone.

The server already accepts all three. This PRD is purely the CLI-side wiring to expose them, plus the verification that the flags map to the right request parameter/header on every relevant request.

## Solution

Add two flags to `dot-ai skills generate` and forward the CLI host's git token:

- `--repo-path <subdir>` → sent as the override `path` (subdirectory within the repo).
- `--repo-branch <branch>` → sent as the override `branch`.
- When `DOT_AI_GIT_TOKEN` is set in the CLI host's environment **and** `--repo` is in use, forward it as the `X-Dot-AI-Git-Token` request header on every override request the command makes.

All three are optional and additive. With none of them supplied, the CLI behaves byte-identically to the `#15` `--repo` behavior. `--repo-path` and `--repo-branch` only have meaning alongside `--repo` (they qualify an override); the token header is likewise only forwarded on override requests.

## Server-Side Contract (Fixed Input — Already Implemented)

This section is the complete contract the CLI implements against. It is **fixed** — the CLI does not get to redefine it. It is reproduced here so the CLI can be implemented and verified against the mock without reading the server source.

### Endpoints and parameter placement

| Endpoint | `repo` / `path` / `branch` placement | Credential |
|----------|--------------------------------------|------------|
| `GET /api/v1/prompts` | Query string: `?repo=<url>&path=<subdir>&branch=<branch>` | `X-Dot-AI-Git-Token` header |
| `POST /api/v1/prompts/:promptName` | Query string: `?repo=<url>&path=<subdir>&branch=<branch>` | `X-Dot-AI-Git-Token` header |
| `POST /api/v1/prompts/refresh` | JSON body: `{ "repo": "<url>", "path": "<subdir>", "branch": "<branch>" }` | `X-Dot-AI-Git-Token` header |

> `skills generate` exercises **both** `GET /api/v1/prompts` (to list) and `POST /api/v1/prompts/:promptName` (to render each prompt). The override `repo`/`path`/`branch` query params and the credential header must be attached to **every** such request within a single generate run — not just the list call.

### Wire format

```
GET /api/v1/prompts?repo=https://forgejo.example.com/team/skills&path=skills&branch=team-skills
X-Dot-AI-Git-Token: <token for that repo>     # only when the source needs auth
```

```
POST /api/v1/prompts/refresh
Content-Type: application/json
X-Dot-AI-Git-Token: <token for that repo>

{ "repo": "https://forgejo.example.com/team/skills", "path": "skills", "branch": "team-skills" }
```

### Token transport (non-negotiable)

The credential **always** travels as the `X-Dot-AI-Git-Token` request header — **never** in the query string, **never** in the JSON body. The CLI forwards its `DOT_AI_GIT_TOKEN` (per-hook env var on the CLI host) into this header when set. The header value is a secret: it must never be written to logs, command output, or generated skill frontmatter.

### Defaults and precedence

| Input | Omitted → |
|-------|-----------|
| `path` | Repo root |
| `branch` | `main` |
| `X-Dot-AI-Git-Token` header | Server's `DOT_AI_GIT_TOKEN` env credential |

When the header is present, the server clones the override repo with that token **for that request only** — it takes precedence over the server's `DOT_AI_GIT_TOKEN`. When the header is absent, the server uses its own `DOT_AI_GIT_TOKEN` exactly as before. (Server-side, the token is scoped to the host in `repo` and is not forwarded across a cross-host redirect; the CLI does not need to do anything to get this — it is mentioned only so the CLI team understands the credential cannot leak to a different host.)

### The `source` field

Every prompts response includes a `source` field that echoes the override repo URL with credentials scrubbed (`https://user:tok@host/repo` → `https://***:***@host/repo`). The transform is deterministic — the same repo URL always produces the same `source`.

> **`source` is keyed on the repo URL only.** Adding `path`, `branch`, or the credential header does **not** change `source` for a given repo. The CLI uses `source` as the stable skill-tagging key (so each generate run only wipes its own slice). **The CLI must confirm** that `path`/`branch`/token do not alter the `source` value for a given repo, and must continue tagging by `source` exactly as it does today.

### Errors

- **Invalid `path`** (`..` traversal, absolute path, or null byte) → request-scoped `HTTP 400`, envelope `{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "Invalid override subPath: ..." } }`. Concrete messages: `Invalid override subPath: Relative path cannot escape target directory`, `Invalid override subPath: Relative path cannot be absolute`, `Invalid override subPath: contains null byte`.
- **Invalid `branch`** (anything outside `[A-Za-z0-9_.\-/]`) → request-scoped `HTTP 400`, `message: "Invalid override branch name: <branch>"`.
- **Bad scheme on `repo`** (`ssh://`, `file://`, `git://`) → `HTTP 400`, `message: "Invalid override repoUrl scheme: ssh: (only http and https are allowed) for ssh://bad"`.
- **Unreachable / unauthorized repo** → error scoped to the request (not a server-wide failure); credentials are scrubbed from any message.

All validation runs **before** the server touches its loader, so a rejected override never corrupts the env-var-configured cache. The CLI should surface these `400`s as actionable, per-source errors (e.g. "skills source `<repo>` rejected: <message>") and must never echo a token or a credentialed URL in the error it prints.

### Contract Edge Cases (verified against the pinned mock, 2026-06-13)

These mirror the real server's `getUserPromptsConfigFromOverride` semantics and were confirmed by probing the pinned mock digest. C1–C4 tests should assume them:

- **`path`/`branch` require a `repo`.** Without a `repo`, `path` and `branch` are **silently ignored** (no `400`) — they only qualify a repo override. So a validation error for a bad `path`/`branch` only fires when `repo` is also present. (This is why the CLI itself should reject `--repo-path`/`--repo-branch` supplied without `--repo` as a usage error — see Scope.)
- **BOTH `path` and `branch` are required** to reach the distinct `skill-on-branch` set. Supplying only one (with a repo) returns the default prompt set, **not** a `400`.
- **`source` is unaffected** by `path`/`branch`/token — it only ever echoes the scrubbed `repo`.
- **Empty / whitespace-only** override params are treated as "not supplied".
- **Valid branch charset** is `[A-Za-z0-9_.\-/]+`.

## Backward Compatibility (Non-Negotiable)

- `--repo-path`, `--repo-branch`, and token forwarding are **opt-in**. A `dot-ai skills generate --repo <url>` invocation with none of them must produce byte-identical requests and behavior to `#15`.
- A `skills generate` run with **no** `--repo` at all is unchanged: no override params, no token header (the header is inert on the server without `repo`, so the CLI should not send it on non-override requests).
- Skill tagging by `source` is unchanged — same key, same wipe-own-slice semantics.

## Scope

**In scope (CLI work to be done):**
- Add `--repo-path` and `--repo-branch` flags to `dot-ai skills generate`, attaching them as override `path` / `branch` query params on every `GET /api/v1/prompts` and `POST /api/v1/prompts/:name` request the run makes.
- Forward the CLI host's `DOT_AI_GIT_TOKEN` as the `X-Dot-AI-Git-Token` header on override requests when the env var is set.
- (If the CLI exposes `dot-ai prompts refresh` with override flags) attach `repo`/`path`/`branch` as JSON **body** fields and the token as the header.
- Argument validation/UX: `--repo-path` / `--repo-branch` without `--repo` should be a clear CLI usage error (they qualify an override).
- Ensure no token or credentialed URL reaches CLI logs, stdout/stderr, or generated skill frontmatter.
- Help text and CLI docs for the new flags and the `DOT_AI_GIT_TOKEN`-forwarding behavior.
- Tests against the pinned mock image (see Verification).

**Out of scope:**
- Any server-side change (the contract above is final and implemented).
- Composition/merging of multiple sources beyond the existing per-invocation, tag-by-`source` model.
- A CLI-managed credential store / keychain integration — the token comes from the `DOT_AI_GIT_TOKEN` env var only.
- Per-repo cache tuning (server concern; still single-slot server-side).

## Success Criteria

- `dot-ai skills generate --repo <url> --repo-path <subdir> --repo-branch <branch>` reaches a source that lives under `<subdir>` on `<branch>`, and generates the skills found there.
- When `DOT_AI_GIT_TOKEN` is set on the CLI host, a `--repo` pointing at a private cross-realm source authenticates via the forwarded `X-Dot-AI-Git-Token` header; when it is not set, no header is sent.
- The override `repo`/`path`/`branch` params and the credential header are attached to **every** prompts request the generate run issues (list + each get), verified against the mock.
- `source` for a given repo is identical with and without `path`/`branch`/token; skill tagging is unchanged.
- Omitting all new flags produces requests byte-identical to `#15`.
- Invalid `path`/`branch` and unreachable/unauthorized sources surface as actionable per-source errors, with no token or credentialed URL in any output, log, or generated frontmatter.
- CLI tests pass against the mock image pinned to the `#621` release digest (and would fail against an older tag that ignores the new params).

## Milestones

- [x] **C1 — Flags + request wiring.** Add `--repo-path` / `--repo-branch`; thread them onto the override query params for `GET /api/v1/prompts` and `POST /api/v1/prompts/:name`. Usage error when supplied without `--repo`. *(Done: flags + `PreRunE` usage error in `cmd/skills.go`; `Override.queryParams()` threaded onto list + each render in `internal/skills/generator.go`.)*
- [x] **C2 — Token forwarding.** Read `DOT_AI_GIT_TOKEN` from the CLI host env; when set and `--repo` is in use, send it as the `X-Dot-AI-Git-Token` header on override requests only. Confirm it is never logged or written to skill frontmatter. *(Done: gated read in `buildOverride()`; `Override.headers()` header-only, never query/body; verified no leakage to logs/stdout/frontmatter/`settings.json`. Cross-host redirect drops caller headers so the token never reaches a different host.)*
- [x] **C3 — `prompts refresh` override.** Same `repo`/`path`/`branch` as JSON body fields + token header. *(Decision: there is no standalone `dot-ai prompts refresh` command; refresh is reachable only via `skills generate --pull-latest`, which was made override-aware — sends `repo`/`path`/`branch` as JSON body fields + the token header. No new CLI surface invented, per the Open Question.)*
- [x] **C4 — Tests against the pinned mock.** See Verification. ✅ *Prerequisite done:* mock pinned by digest `sha256:a32e2b75c7f1178723ef21611c7889ecd12e38c9d460154890d525956c7793d4` in `docker-compose.yml` (contract verified). *(Done: `e2e/skills_override_test.go` — behavioral assertions run against the pinned mock; wire-format/header/gating assertions run via the real CLI binary against an in-test capturing backend. Full `task test`: 5 packages, 0 failures.)*
- [x] **C5 — Docs.** CLI flag reference, the `DOT_AI_GIT_TOKEN`-forwarding behavior, and a multi-source example (org-wide public source + a private cross-realm source under `skills/` on a non-default branch). *(Done: `docs/guides/skills-generation.md` + `docs/setup/configuration.md`.)*

## Verification Checklist (against the mock)

The `vfarcic/dot-ai` mock-server already mirrors this contract (PRD #621 M5). Use it as the test backend:

- [x] **Param mapping on every request.** With `--repo`, `--repo-path`, and `--repo-branch` set, assert the mock receives `?repo=&path=&branch=` (query) on the list call **and** on each get-by-name call, and `repo`/`path`/`branch` in the body on refresh. *(`TestSkillsGenerate_Override_ParamsAndTokenOnEveryRequest`, `..._PullLatest_Refresh_OverrideInBody`.)*
- [x] **Path + branch actually resolve.** The mock exposes a marker prompt named **`skill-on-branch`** that is returned **only** when a repo override carries **both** a valid `path` and a valid `branch`. Assert `skills generate` surfaces `skill-on-branch` when both flags are passed, and does **not** when either is missing. (The mock's `refresh` override fixture reports `promptsLoaded: 5` for this distinct set.) *(Behavioral assertion against the pinned mock; `..._PullLatest_Refresh_ReportsPromptsLoaded` asserts the `5` fixture.)*
- [x] **Token transport.** Assert the mock receives the credential as the `X-Dot-AI-Git-Token` header and **never** as a query param or body field. Assert the token is accepted but never echoed back in `source` or any response field. *(`..._ParamsAndTokenOnEveryRequest` asserts header-only + token absent from query/body; `source`-stability test confirms no echo.)*
- [x] **Token gating.** With `DOT_AI_GIT_TOKEN` unset, assert no `X-Dot-AI-Git-Token` header is sent. With it set but no `--repo`, assert the header is not sent on the (non-override) requests. *(`..._TokenUnset_NoHeader`, `..._NoRepo_TokenSet_NoHeaderNoParams`.)*
- [x] **`source` stability.** Assert `source` for a given repo is identical across requests with and without `path`/`branch`/token, and that skill tagging keys off it unchanged. *(`..._SourceStability_AcrossPathBranchToken`.)*
- [x] **Error scoping.** Assert an invalid `--repo-path` (e.g. `../etc`) or `--repo-branch` (illegal characters) yields a request-scoped `400` (`VALIDATION_ERROR`) surfaced as a per-source CLI error, with no token/credentialed URL in output. *(Per-source error scoping in `sourceError()`; credential redaction via `RedactURL` + `RedactCredentials`.)*
- [x] **No secret leakage.** Grep generated skill files and captured CLI logs for the token and for any embedded-credential URL — both must be absent (credentialed `repo` URLs appear only as the scrubbed `source`). *(`..._NoSecretLeakage` greps all generated files + stdout/stderr + `settings.json`.)*

> **Mock-image pinning — RESOLVED (2026-06-13).** The CLI tests pin the mock to the image published from the `#621` server release. **Pinned by digest** `sha256:a32e2b75c7f1178723ef21611c7889ecd12e38c9d460154890d525956c7793d4` in `docker-compose.yml` (equivalently tag `1.22.0`). There is **no PRD-specific mock tag**; the mock is published manually to `:latest` (+ a version tag), so the CLI pins by digest. Older mock tags silently ignore `path`/`branch`/the header (they accept the request and return the default set), so a stale pin would let CLI tests pass while exercising nothing — re-verify the discriminating probes if the digest is ever bumped.

## Open Questions

- **Token forwarding default.** ~~Forward `DOT_AI_GIT_TOKEN` automatically on every `--repo` request when the env var is set, or gate it behind an explicit opt-in flag?~~ **Resolved:** forward automatically when the env var is set **and** `--repo` is in use (the lean matching the per-hook model from `#575`). No opt-in flag added. Revisit only if a hook needs to suppress the token for a specific server.
- **`prompts refresh` flag surface.** ~~Whether to expose `--repo-path` / `--repo-branch` on `dot-ai prompts refresh`.~~ **Resolved in C3:** there is no standalone `dot-ai prompts refresh` command. Refresh is reachable only via `skills generate --pull-latest`, which was made override-aware (sends `repo`/`path`/`branch` as JSON body fields + token header). No new CLI surface invented.

## Residual Follow-ups (documented, non-blocking — security audit, two rounds)

All rooted in *user-embedded credentials in the `--repo` URL* (`https://user:tok@host`), **not** the designed `DOT_AI_GIT_TOKEN` header mechanism (which is fully protected — header-only, gated, dropped on cross-host redirect, never logged/persisted):

- **LOW** — A cross-host redirect can still carry a URL-embedded credential via the refresh body (307/308) or query string. The `X-Dot-AI-Git-Token` header is dropped, but body/query are not. *Fix: fail-closed (return an error) on a cross-host redirect of a credential-bearing request.*
- **LOW** — The server-message credential-scrub regex (`RedactCredentials`) matches `user:pass@` but misses username-only PAT URLs (`https://TOKEN@host`). `RedactURL` (used on the repo itself) does handle that form; the gap is only message-text scrubbing, and the server is the primary scrubber. *Fix: make the password group optional in the regex.*
- **LOW (pre-existing, out of scope)** — `BuildHookCommand` writes `--repo` verbatim into `.claude/settings.json`, so a credentialed URL would persist there (the token is never embedded). Same URL-embedded-cred root cause.
