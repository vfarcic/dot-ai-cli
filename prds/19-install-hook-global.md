# PRD: `--install-hook --global` for a User-Level Claude Code Hook + Custom `--path`

> **GitHub Issue:** [vfarcic/dot-ai-cli#19](https://github.com/vfarcic/dot-ai-cli/issues/19)
> **CLI-only:** no server change, no companion `vfarcic/dot-ai` dependency, no mock-image bump. Every touched surface is local to this repo.

**Status**: Implementation complete — all milestones M1–M5 done; reviewed clean (reviewer + auditor, no blocking findings); full suite green (223 tests, 0 failures). Pending PR.
**Priority**: Medium (a real onboarding-UX gap; a workaround exists — hand-edit `~/.claude/settings.json` and run a one-shot `generate` — but it is exactly the "no hand-editing JSON" experience `--install-hook` was built to remove).
**Related Issues**: [`vfarcic/dot-ai-cli#12`](https://github.com/vfarcic/dot-ai-cli/issues/12) (the hook-per-source model + `source:`-tagged wipe-own-slice pipeline this rides on); [`vfarcic/dot-ai-cli#13`](https://github.com/vfarcic/dot-ai-cli/issues/13) (added the source flags that `BuildHookCommand` already round-trips — this PRD adds `--global`/`--path` to that same mechanism); [`vfarcic/dot-ai-cli#16`](https://github.com/vfarcic/dot-ai-cli/issues/16) (the `--repo`/token override that composes with `--global`).
**External stakeholder**: [@vtmocanu](https://github.com/vfarcic/dot-ai-cli/issues/19) filed this from real team-onboarding friction; keep them updated.

## Problem

`--install-hook` only supports **per-project** setups. Two hard-coded assumptions cause this:

1. **`cmd/skills.go` `PreRunE` rejects `--install-hook` with `--path`** (`--install-hook cannot be used with --path`), so you cannot install a hook whose skills land in a custom directory.
2. **`internal/skills/hook.go` hard-codes the settings target** to the project-local `.claude/settings.json` (`settingsFile = ".claude/settings.json"`), and `resolveDir` (in `generator.go`) resolves the claude-code default to the project-local `.claude/skills`.

For a team that wants the shared catalog available in **every** project (skills under `~/.claude/skills` or another home-level dir, hook in `~/.claude/settings.json`, refreshed on every session start), onboarding today requires hand-editing `~/.claude/settings.json` plus a one-shot manual `generate`. The naive one-liner errors:

```bash
dot-ai skills generate --agent claude-code --path ~/.claude/commands --install-hook
# Error: --install-hook cannot be used with --path
```

## Solution

Add a `--global` flag to `dot-ai skills generate`. When set, it:

- **Writes the `SessionStart` hook to `~/.claude/settings.json`** instead of the project-local `.claude/settings.json` (resolved via `os.UserHomeDir()`).
- **Lifts the `--install-hook` + `--path` conflict guard** — but *only* when `--global` is present. Project mode keeps the guard.
- **Defaults the skills output directory to `~/.claude/skills`** when `--path` is not supplied (instead of the project-local `.claude/skills`).
- **Round-trips `--global` (and `--path` when given) through `BuildHookCommand`**, so every session-start re-fire regenerates to the same place. This is one more flag on the existing round-trip mechanism `BuildHookCommand` already applies to every source flag (with shell-quoting + credential scrubbing).

Team onboarding becomes a single idempotent command:

```bash
dot-ai skills generate --agent claude-code --path ~/.claude/commands --install-hook --global
```

`--global` composes with the source flags (`--repo`, `--repo-fetch`, `--repo-dir`) unchanged — running once per source with `--global` generates each source's skill *files* into the global catalog (the `source:`-tagged wipe-own-slice pipeline keeps files from different sources side by side). **Correction (implementation finding):** the *hook* itself does **not** accumulate per source. `removeExistingHook` wipes **all** dot-ai `SessionStart` entries and inserts exactly one, so a later `--install-hook` run **replaces** the previous dot-ai hook rather than adding a second. Consequence: only the **last-installed** source is auto-regenerated on session start; earlier sources' files persist but are not refreshed by the hook. (The original Issue #19 "Use Case 2 — one hook per source" framing was inaccurate on this point; the shipped docs/changelog were corrected to match.)

### Bonus: `~/.claude/skills` is natively discoverable by opencode

The `~/.claude/skills` default is not Claude-Code-only. **opencode natively discovers skills from `~/.claude/skills/*/SKILL.md`** (alongside `~/.config/opencode/skills/` and `~/.agents/skills/`), and ignores unknown frontmatter fields — so our `source:` tag is harmless and our `dot-ai-*` skill names satisfy its lowercase-hyphen naming rule. A single `--global` install therefore lights up **both** Claude Code and opencode for skill *discovery*, with zero opencode-specific work.

This is a documentation note, **not** a scope item. Making opencode *auto-refresh* on startup is a separate follow-up (opencode has no `settings.json` `SessionStart` hook — its equivalent is a `session.created` **plugin** in `~/.config/opencode/plugins/` that can shell out to `dot-ai skills generate`). That follow-up (`--agent opencode` → install a `session.created` plugin) is **out of scope** here; file it separately once #19 lands.

## Backward Compatibility (Non-Negotiable)

- **Project mode is byte-identical.** A `dot-ai skills generate ... --install-hook` run **without** `--global` produces the exact same `.claude/settings.json` and `.claude/skills` output as today — same conflict guard, same paths, same stored command string.
- `--global` is purely additive and opt-in.
- Installing a global hook **preserves unrelated content** in `~/.claude/settings.json` — the existing merge logic (`readSettings` → `removeExistingHook` → `insertHook` → `writeSettings`) only touches `SessionStart` entries whose command has the `dot-ai skills generate` prefix and the `startup` matcher; any other user hooks/settings are left intact.

## Scope

**In scope (CLI work):**
- New `--global` bool flag on `dot-ai skills generate` (+ help text).
- `PreRunE`: lift the `--install-hook`/`--path` conflict guard when `--global` is set; keep it otherwise. Keep the existing `--install-hook` ⇒ `--agent claude-code` requirement.
- Global settings-path resolution: parameterize `InstallSessionHook` (today it hard-codes `settingsFile`) so it can target `~/.claude/settings.json`. `readSettings`/`writeSettings` already accept a path.
- Global default output dir: `resolveDir` (or its caller) returns `~/.claude/skills` for claude-code when `--global` is set and `--path` is empty.
- `BuildHookCommand` emits `--global` (and `--path <dir>` when the user supplied one) so a re-fire regenerates to the same location.
- Correct the success message (currently hard-codes the literal `.claude/settings.json`) to report the actual settings path written.
- e2e coverage (see Verification) and docs (flag reference + updated onboarding one-liner + the opencode-discovery note).

**Out of scope:**
- Any server-side change (there is none).
- opencode auto-refresh / a `--agent opencode` plugin installer (separate follow-up — see Solution).
- A global variant of `--repo-dir`'s `DOT_AI_ALLOW_REPO_DIR` opt-in or any other credential/opt-in change — the existing env-at-hook-run-time model is unchanged and is inherited as-is.
- Merging/deduplicating a project-local hook against a global hook — they are independent files by design (a user may legitimately want both).

## Success Criteria

1. `dot-ai skills generate --agent claude-code --path ~/.claude/commands --install-hook --global` succeeds (no conflict error), writes the hook to `~/.claude/settings.json`, and generates skills into `~/.claude/commands`.
2. `dot-ai skills generate --agent claude-code --install-hook --global` (no `--path`) writes the hook to `~/.claude/settings.json` and generates skills into `~/.claude/skills`.
3. The stored hook command round-trips `--global` (and `--path` when supplied): replaying the stored command through a shell regenerates skills to the **same** directory it first wrote to.
4. Project mode (no `--global`) is byte-identical to today: same guard behavior, same `.claude/settings.json`, same `.claude/skills` output, same stored command.
5. `--global` composes with each source flag (`--repo`, `--repo-fetch`, `--repo-dir`): the source is round-tripped as before and the hook lands in `~/.claude/settings.json`.
6. Installing a global hook preserves any pre-existing unrelated content in `~/.claude/settings.json`; re-running is idempotent (no duplicate `SessionStart` entries).
7. The success message reports the actual settings-file path written (global vs project), not a hard-coded string.

## Milestones

- [x] **M1 — Flag + guard.** Added `--global` (help text). In `PreRunE`, lift the `--install-hook`/`--path` conflict guard when `--global` is set (kept otherwise); retained `--install-hook` ⇒ `--agent claude-code`; added `--global` ⇒ `--agent claude-code` guard (a clear error beats a silent no-op for non-claude-code agents). Decided: `--global` **without** `--install-hook` is valid — a "write to the global catalog" mode that defaults skills to `~/.claude/skills`.
- [x] **M2 — Path resolution.** Parameterized `InstallSessionHook` to write the resolved settings path (`ResolveSettingsPath`); resolve `~/.claude/settings.json` in global mode via `os.UserHomeDir()` (`claudeHomeDir`, clear error on missing/unreadable home). `resolveDir` defaults to `~/.claude/skills` for claude-code in global mode when `--path` is empty. Success message now reports the actual settings path written.
- [x] **M3 — Round-trip.** `BuildHookCommand` emits `--global` (+ `--path <dir>` when the user gave one), shell-quoted. Decided: embed `--global` (re-resolved against `$HOME` each fire, host-portable) and store a user-supplied `--path` as an **absolute** path — `filepath.Abs` is applied before store because the shell only expands `~`, not relative paths, so a relative `--path` would otherwise break the round-trip. Project-mode command string is byte-identical when `--global` absent (guarded by test).
- [x] **M4 — Tests.** e2e coverage with `HOME=t.TempDir()` isolation: global hook lands in `$HOME/.claude/settings.json`; default skills land in `$HOME/.claude/skills`; `--path` override honored + round-tripped; shell-replay regenerates to the same dir; project mode byte-identical; `--global` composes with a source flag **with credential scrubbing verified on a token-bearing URL**; unrelated `~/.claude/settings.json` content preserved; idempotent re-run; plus a `--path` shell-injection regression subtest.
- [x] **M5 — Docs.** Updated `docs/guides/skills-generation.md` with the `--global` flag, the one-command team-onboarding example, the `~/.claude/skills` default, the host-portable round-trip note, the `--global`-without-hook mode, and the opencode-discovery note. Corrected the per-source hook wording to match actual behavior (see Solution correction). Added `changelog.d/19.feature.md`.

## Verification Checklist

- [x] **Guard lifted only with `--global`.** `--install-hook --path <dir>` without `--global` still errors; with `--global` it succeeds.
- [x] **Hook target.** With `--global`, the `SessionStart` entry is written to `$HOME/.claude/settings.json`; without it, to `./.claude/settings.json` (CWD-relative, as today).
- [x] **Default output dir.** `--global` with no `--path` writes skills to `$HOME/.claude/skills`; project mode writes to `./.claude/skills`.
- [x] **`--path` honored + round-tripped.** `--global --path <dir>` writes skills to `<dir>` and the stored command carries `--path <dir>` (shell-quoted, absolute); shell-replay regenerates to `<dir>`.
- [x] **`--global` round-tripped.** The stored command carries `--global`; shell-replay (with the same `$HOME`) regenerates to the same default dir.
- [x] **Project mode unchanged.** Stored command string, `.claude/settings.json`, and `.claude/skills` output are byte-identical to pre-#19 for a no-`--global` run.
- [x] **Compose with source flags.** `--global --repo-fetch <url>` round-trips the source AND lands the hook in `$HOME/.claude/settings.json`; credential scrubbing on the stored URL is verified unchanged (token-bearing URL test).
- [x] **Preserve + idempotent.** A pre-existing unrelated key/hook in `$HOME/.claude/settings.json` survives; running twice yields exactly one dot-ai `SessionStart` entry.
- [x] **Success message.** Output names the actual path written (global vs project).

## Open Questions

- **Round-trip form: embed `--global` vs. embed the resolved absolute `--path`?** **RESOLVED (M3): embed `--global`.** It re-resolves `~/.claude/skills` against `$HOME` at each fire — host-portable and consistent with the "round-trip every flag" philosophy. A user-supplied `--path` is stored **absolute** (`filepath.Abs`) so the round-trip holds even for a relative `--path` (the shell only expands `~`, not `./foo`), directly protecting Success Criterion #3.
- **Is `--global` meaningful without `--install-hook`?** **RESOLVED (M1): yes.** `dot-ai skills generate --agent claude-code --global` (no hook) is a valid "write to the global catalog" mode and defaults skills to `~/.claude/skills`. Keeps `--global`'s meaning consistent regardless of `--install-hook`.
- **Windows / non-standard `$HOME`.** **RESOLVED:** paths derive solely from `os.UserHomeDir()` (via `claudeHomeDir`); a missing/unreadable home fails closed with a clear `ExitToolError` and writes nothing — never a surprising location.
