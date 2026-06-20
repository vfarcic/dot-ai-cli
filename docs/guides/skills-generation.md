# Skills Generation

Enable AI agents to use the DevOps AI Toolkit CLI and access server prompts as native skills.

## What Are Skills?

Skills are agent capabilities that make AI coding assistants (Claude Code, Cursor, Windsurf) aware of available tools and workflows. The CLI can generate skills from server capabilities.

## What Gets Generated

Skills generation serves two purposes:

### 1. CLI Awareness (Routing Skill)

Creates a `dot-ai` routing skill that makes agents aware of the CLI:
- Triggers on Kubernetes and DevOps operations
- Directs agents to use CLI instead of MCP
- Teaches agents to use `dot-ai --help` for command discovery
- Lower token overhead than MCP protocol

### 2. Server Prompts (Prompt Skills)

Exposes server prompts as native agent skills:
- Each server prompt becomes an agent skill (e.g., `dot-ai-projectSetup`, `dot-ai-query`)
- Users can invoke them as native skills in their agent
- Prefixed with `dot-ai-` to avoid naming conflicts
- Skills can include supporting files (shell scripts, templates, manifests) alongside `SKILL.md`

## Supported Agents

- **Claude Code** — `.claude/skills/`
- **Cursor** — `.cursor/skills/`
- **Windsurf** — `.windsurf/skills/`

Note: Cursor also auto-discovers skills from `.claude/skills/`, so Claude Code skills work in Cursor without duplication.

## Generate Skills

The server caches skills for performance (default: 24 hours). Use `--pull-latest` to force the server to pull the latest from the git repository before generating:

```bash
dot-ai skills generate --agent claude-code --pull-latest
```

**For Claude Code:**
```bash
dot-ai skills generate --agent claude-code
```

**For Cursor:**
```bash
dot-ai skills generate --agent cursor
```

**For Windsurf:**
```bash
dot-ai skills generate --agent windsurf
```

**Custom path (unsupported agents):**
```bash
dot-ai skills generate --path ./custom/skills/
```

## Custom-Only Mode

By default, `skills generate` creates skills for both MCP tools (query, recommend, remediate, etc.) and custom prompts (troubleshoot-pod, explain-resource, etc.). Use `--custom-only` to skip MCP tool skills and generate only custom prompt skills:

```bash
dot-ai skills generate --agent claude-code --custom-only
```

Persist it so all future generations respect it:

```bash
dot-ai config set skills.custom_only true
dot-ai skills generate --agent claude-code

# Clear it
dot-ai config reset skills.custom_only
```

`--custom-only` follows the standard 4-tier precedence (see [Filter Precedence](#filter-precedence) below) and can be combined with `--include`/`--exclude` to further filter within the custom skills.

## Filtering Skills

By default, `skills generate` creates skills for all tools and prompts from the server. You can filter which skills are generated using include/exclude regex patterns.

### One-Time Filtering with Flags

```bash
# Generate only query and recommend skills
dot-ai skills generate --agent claude-code --include "query|recommend"

# Generate all except management skills
dot-ai skills generate --agent claude-code --exclude "manage.*"

# Combine: include all, then exclude specific ones
dot-ai skills generate --agent claude-code --include ".*" --exclude "manage.*"
```

### Persistent Filtering with Config

Set filters once so all future generations respect them:

```bash
# Persist an include filter
dot-ai config set skills.include "query|recommend|remediate"

# All future generates use the filter
dot-ai skills generate --agent claude-code

# One-time override to generate everything
dot-ai skills generate --agent claude-code --include ".*"

# Persist an exclude filter
dot-ai config set skills.exclude "debug-.*|experimental-.*"

# Clear persistent filters
dot-ai config reset skills.include
dot-ai config reset skills.exclude
```

### Filter Precedence

Filters follow the standard 4-tier precedence:

1. `--include` / `--exclude` / `--custom-only` flags (highest priority)
2. `DOT_AI_SKILLS_INCLUDE` / `DOT_AI_SKILLS_EXCLUDE` / `DOT_AI_SKILLS_CUSTOM_ONLY` environment variables
3. `settings.json` → `skills_include` / `skills_exclude` / `skills_custom_only`
4. Default: empty (no filtering — generate all skills)

### Filter Logic

- Patterns are regular expressions matched against skill names (without the `dot-ai-` prefix)
- If `--custom-only` is set, MCP tool skills are skipped entirely
- If `--include` is set, only skills matching the pattern are kept
- If `--exclude` is set, skills matching the pattern are removed
- If both are set, include is applied first, then exclude
- `--include`/`--exclude` filters apply to both tool skills and prompt skills (or just prompt skills when `--custom-only` is active)

## Auto-Update with SessionStart Hook

For Claude Code, you can install a hook that automatically regenerates skills at the start of every session:

```bash
dot-ai skills generate --agent claude-code --install-hook
```

This adds a `SessionStart` hook to `.claude/settings.json` that runs `dot-ai skills generate --agent claude-code` on session startup. The hook captures the flags you pass — `--custom-only`, `--include`, `--exclude`, `--repo`, `--repo-path`, `--repo-branch`, and the CLI-side source flags `--repo-fetch`, `--repo-dir` / `--source-label`, and `--no-cache` — so each firing reproduces the same source.

```bash
dot-ai skills generate --agent claude-code --install-hook --custom-only --exclude "debug-.*"
# Hook command: dot-ai skills generate --agent claude-code --custom-only --exclude 'debug-.*'
```

Secrets and opt-ins are **never** written to `settings.json` — they are read from the environment each time the hook fires, so they must be set per session for the hook to behave the same:

- A `--repo` or `--repo-fetch` URL is stored **credential-scrubbed** (a `user:token@…` URL lands as the bare URL), and `DOT_AI_GIT_TOKEN` is read from the environment.
- A `--repo-dir` hook does **not** embed `DOT_AI_ALLOW_REPO_DIR` — a committed `settings.json` must not let a clone side-load skills without consent. The opt-in must be set in the environment for the hook to fire (mirroring `DOT_AI_GIT_TOKEN`).

The hook is idempotent — running the command again with the same flags won't create duplicates. Re-running with different flags replaces the existing hook. It merges with any existing settings. See [Which Source Flag?](#which-source-flag) for the flag details.

To compose skills from multiple repos, install one hook per source:

```bash
dot-ai skills generate --agent claude-code --install-hook                                           # env-var repo
dot-ai skills generate --agent claude-code --install-hook --repo https://github.com/orgA/skills    # org-wide
dot-ai skills generate --agent claude-code --install-hook --repo https://gitlab.corp/team/skills   # team-private
```

Each invocation of `--install-hook` writes one hook scoped to its source. See
[Composing Skills From Multiple Repositories](#composing-skills-from-multiple-repositories) for the full model.

## Updating Skills

Re-running the command updates the `dot-ai-*` skills owned by the current source:

```bash
dot-ai skills generate --agent claude-code
```

Each generated skill carries a `source:` field in its YAML frontmatter
recording which repo it came from (`built-in` for the server's configured
default, or the repo URL passed via `--repo`). On every run, the CLI only
wipes-and-replaces skills tagged with the *current* source — skills from other
sources are left in place. This is what lets you compose skills from multiple
repos via multiple invocations (see [Composing Skills From Multiple Repositories](#composing-skills-from-multiple-repositories)).

## Composing Skills From Multiple Repositories

The `--repo <url>` flag overrides the server's configured default prompts repo
for one invocation:

```bash
dot-ai skills generate --agent claude-code --repo https://github.com/orgA/skills
```

Each invocation is **self-contained and source-scoped** — it fetches from a
single repo, tags every generated skill with that repo's source identifier,
and only manages files from that source. Composition across multiple sources
is achieved by running the command multiple times — typically as one agent
hook per source:

```bash
# Hook A: server's configured default repo (env-var DOT_AI_USER_PROMPTS_REPO on the server)
dot-ai skills generate --agent claude-code

# Hook B: explicit org-wide repo
dot-ai skills generate --agent claude-code --repo https://github.com/orgA/skills

# Hook C: a self-hosted team-private repo, with its own credentials in DOT_AI_GIT_TOKEN
DOT_AI_GIT_TOKEN=$TEAM_TOKEN dot-ai skills generate --agent claude-code --repo https://gitlab.corp/team/skills
```

Each hook owns its slice. The hook is the scoping unit: each one points at one
source and carries that source's subdirectory, branch, and credential.

### Which Source Flag?

By default the **server** fetches every skill source: with no flag it clones its
own configured repo; with `--repo <url>` it clones the override you name. That is
the right model whenever the server can both **reach** and **authenticate** to the
source. Two flags — `--repo-fetch` and `--repo-dir` — instead have the **CLI host**
read the source and upload it for the server to render, for the cases server-side
fetch can't cover:

| Your source is… | Use | Who fetches |
|---|---|---|
| The server's configured default repo | *(no source flag)* | Server |
| Any repo the server can reach with a **static credential** (public, or private + a token) | `--repo <url>` (+ `DOT_AI_GIT_TOKEN` for private) | Server |
| A repo only the **CLI host** can reach/authenticate — SSO/OIDC/device-attested VPNs, or a hardened cluster with no egress and no static token | `--repo-fetch <url>` | CLI host |
| Work-in-progress skills on local disk with no remote at all | `--repo-dir <path> --source-label <label>` | CLI host (no clone) |

**Prefer `--repo` + `DOT_AI_GIT_TOKEN` whenever a static credential reaches the
source.** `--repo-fetch` and `--repo-dir` are an **escape hatch**, not a general
alternative to server-side fetch: they exist only for sources the server
*fundamentally cannot* authenticate or route to (no static token exists, or the
host simply isn't reachable from the server) and for on-disk source with no
remote. If a token works, server-side fetch keeps your credentials in one place
(see [Tradeoffs](#tradeoffs)).

> **Both flags need a matching server.** `--repo-fetch` and `--repo-dir` upload the
> source to a server **ingestion endpoint** that ships in the same release as this
> CLI. They are not usable against an older server that lacks it.

### Subdirectory, Branch, and Per-Source Credentials

A `--repo` source does not have to live at the repo root on the default branch,
and it does not have to share the server's git credential. Three additive,
opt-in inputs qualify a `--repo` override:

| Flag / env var | Sent as | Default when omitted |
|----------------|---------|----------------------|
| `--repo-path <subdir>` | override `path` (subdirectory within the repo) | repo root |
| `--repo-branch <branch>` | override `branch` | `main` |
| `DOT_AI_GIT_TOKEN` env var | `X-Dot-AI-Git-Token` request header | the server's own `DOT_AI_GIT_TOKEN` credential |

```bash
# Skills kept under skills/ on the team-skills branch of a private repo,
# authenticated with this hook's own token (a different auth realm than the
# server's default credential).
DOT_AI_GIT_TOKEN=$TEAM_TOKEN dot-ai skills generate --agent claude-code \
  --repo https://forgejo.example.com/team/skills \
  --repo-path skills \
  --repo-branch team-skills
```

Key rules:

- **`--repo-path` / `--repo-branch` require `--repo` or `--repo-fetch`.** They
  qualify a repo-bearing source, so supplying either alone is a usage error (a
  local `--repo-dir` takes no subdir/branch qualifier either):
  ```text
  Error: --repo-path and --repo-branch require --repo or --repo-fetch
  ```
- **The credential is forwarded only on override requests.** When
  `DOT_AI_GIT_TOKEN` is set in the environment **and** `--repo` is in use, the
  CLI forwards it as the `X-Dot-AI-Git-Token` header — on every prompts request
  the run makes — so the override repo is cloned with *that* token for that
  request only. With no `--repo`, the token is never sent (it would be inert
  server-side anyway). The token is a secret: it never appears in logs, command
  output, or generated skill frontmatter.
- **`source` is keyed on the repo URL only.** Adding a path, branch, or token
  does not change the `source` value for a given repo, so skill tagging (and the
  wipe-own-slice behavior) is unaffected.

### `--repo-fetch`: Clone From the CLI Host

`--repo-fetch <git-url>` makes the **CLI** clone the repo using the host's local
git stack, then upload the resulting source to the server, which renders it
exactly as it renders a `--repo` source. Use it for a source the server can't
reach but your laptop can:

```bash
dot-ai skills generate --agent claude-code --repo-fetch https://vpn.corp/team/skills
```

```text
Uploaded source as https://vpn.corp/team/skills (ingested)
Skills generated successfully in .claude/skills
```

The clone runs through your host's normal git authentication — **SSH agent, the
`git credential` helper, `~/.gitconfig`, `GIT_SSH_COMMAND`, `GIT_CONFIG_GLOBAL`,
`GIT_TERMINAL_PROMPT=0`** — so a live SSO session, a device-attested VPN, or an SSH
key the server will never hold all work. It authenticates via that stack, **not**
`DOT_AI_GIT_TOKEN` (that token qualifies `--repo` only; pairing it with
`--repo-fetch` is out of scope).

`--repo-fetch` accepts the same subdirectory/branch qualifiers as `--repo`, plus
`--no-cache`:

| Flag / option | Effect | Default when omitted |
|---------------|--------|----------------------|
| `--repo-path <subdir>` | Upload only this subdirectory of the clone | repo root |
| `--repo-branch <branch>` | Clone (and upload) this branch | the repo's default branch |
| `--no-cache` | Clone to a throwaway temp dir, use it, delete it (skip the persistent cache) | use the clone cache |

The `source:` frontmatter is the git URL with **any credentials scrubbed**: a
`https://user:token@…` URL is tagged, logged, and stored as `https://…`. The
credential never reaches the server, the frontmatter, stdout/stderr, or the cache
path (see [Credential Safety](#credential-safety)).

### The `--repo-fetch` Clone Cache

To keep re-runs cheap — a SessionStart hook fires `--repo-fetch` on every session
— the CLI keeps a persistent clone cache:

- **Location:** `$XDG_CACHE_HOME/dot-ai-cli/repos/<sha256-of-url>/` (defaults to
  `~/.cache/dot-ai-cli/repos/…`). The directory is created mode `0700` — it can
  hold private source. The cache key is the **scrubbed** URL, so no credential is
  ever part of the path.
- **First run** does a shallow `git clone --depth 1`; **subsequent runs** do an
  incremental `git fetch` + checkout, so a re-run costs O(diff), not a full
  re-clone.
- **Concurrency:** two `--repo-fetch` runs of the *same* URL serialize on a
  per-URL lock; the second waits, then reuses the first's checkout — no race, no
  corruption.
- **Unchanged source skips the upload.** The CLI content-hashes the source and, if
  it matches the last upload for that identifier, reports `unchanged, skipping
  upload` instead of re-sending it:

  ```text
  Source https://vpn.corp/team/skills unchanged, skipping upload
  Skills generated successfully in .claude/skills
  ```

- **`--no-cache`** bypasses all of this: it clones to a throwaway temp dir and
  deletes it after the run, leaving no persistent entry.

#### Pruning the Cache

`dot-ai skills cache prune --older-than <duration>` garbage-collects clone-cache
entries (and stale upload-state records) whose **last use** is older than a Go
duration. Last use is refreshed on every successful `--repo-fetch`, so an
actively-used cache is never pruned — only idle entries. An entry a concurrent
`--repo-fetch` is using (its per-URL lock is held) is skipped, not deleted.

```bash
# Remove entries unused for 30 days.
dot-ai skills cache prune --older-than 720h
```

```text
skills cache prune: removed 1 clone-cache and 0 upload-state entries older than 720h0m0s (kept 1, skipped 0 in use)
```

A missing or empty cache is a clean no-op (exit 0):

```text
skills cache prune: cache is empty; nothing to prune
```

`--older-than` is required and takes a [Go duration](https://pkg.go.dev/time#ParseDuration)
(e.g. `720h`, `30m`); an invalid value exits non-zero with a clear error.

### `--repo-dir`: Read a Local Directory

`--repo-dir <path> --source-label <label>` reads skills straight from a local
directory — **no network, no clone** — uploads them, and renders them through the
server. It is the tight dev-loop tool for work-in-progress skills with no remote
yet:

```bash
DOT_AI_ALLOW_REPO_DIR=1 dot-ai skills generate --agent claude-code \
  --repo-dir ./my-skills --source-label team-wip
```

```text
Uploaded source as local:alice-team-wip (ingested)
Skills generated successfully in .claude/skills
```

`--source-label` is **required** (a filesystem path is not a stable cross-machine
identifier). Skills are tagged `source: local:<label>`, auto-prefixed with your
host identity for per-server uniqueness — two laptops uploading `local:team-wip`
would otherwise overwrite each other in the server's source cache. The prefix is
resolved in this order:

1. `local:<user>-<label>` — `$USER`, else the OS user.
2. `local:<host>-<label>` — `$HOSTNAME`, else the system hostname, when no user is
   known.

So `--source-label team-wip` run by user `alice` produces `source:
local:alice-team-wip`.

#### Security Model (opt-in, default-off)

`--repo-dir` accepts an arbitrary filesystem path — a side-loading vector for
arbitrary skill code — so it is **disabled by default** and hardened:

- **`DOT_AI_ALLOW_REPO_DIR=1` is required.** Without it the run refuses (non-zero
  exit, nothing generated):
  ```text
  Error: --repo-dir is opt-in: set DOT_AI_ALLOW_REPO_DIR=1 to allow reading skills from a local directory. It accepts an arbitrary filesystem path (a side-loading vector for arbitrary skill code), so it is disabled by default; prefer --repo with DOT_AI_GIT_TOKEN whenever a static credential reaches the source.
  ```
- **Paths under `/tmp` / `$TMPDIR` are refused** — shared, world-writable temp
  space is a side-loading vector.
- **World-writable directories (or any world-writable ancestor) are refused** —
  another user could swap the source out from under you. Tighten with `chmod o-w`.
- **Optional allowlist:** set `DOT_AI_REPO_DIR_ALLOW` to a colon-separated list of
  base directories; any `--repo-dir` outside all of them is refused.
- The source is capped at **100 files / 256 KiB** total (a pre-upload check).

`--repo-dir` without `--source-label` (and vice versa) is a usage error:

```text
Error: --repo-dir requires --source-label
```

### Credential Safety

No credential — or credentialed URL — ever reaches a log, stdout/stderr,
`settings.json`, the clone-cache path, or the generated `source:` frontmatter:

- A `--repo-fetch` (or `--repo`) URL embedding `user:token@…` is scrubbed to its
  bare form everywhere it surfaces.
- `--repo-fetch` authenticates through the host git stack; the CLI never reads or
  transmits those credentials itself.
- `DOT_AI_GIT_TOKEN` and `DOT_AI_ALLOW_REPO_DIR` are read from the environment at
  run time and are never persisted into a hook command.

### Tradeoffs

CLI-side fetching moves credentials from the server to each developer's machine.
That is the point — it serves sources the server can't authenticate — but it
trades away **centralized credential audit**: with server-side fetch, every
skill-fetch credential lives in one place to rotate and audit. Organizations that
need that central control should stay on `--repo` + `DOT_AI_GIT_TOKEN` (plus a
network-level fix where viable) and reserve `--repo-fetch` / `--repo-dir` for the
sources that genuinely have no static-credential path.

### Multi-Source Example

Compose all four source types into one skills directory — one hook per source,
each tagged with its own `source:` so the [wipe-own-slice](#updating-skills) and
[first-source-wins](#collision-policy-first-source-wins) rules keep them from
clobbering each other:

```bash
# 1. The server's configured default repo (no source flag)   -> source: built-in
dot-ai skills generate --agent claude-code

# 2. An org-wide repo the server clones, with a per-hook token -> source: <url>
DOT_AI_GIT_TOKEN=$ORG_TOKEN dot-ai skills generate --agent claude-code \
  --repo https://github.com/orgA/skills

# 3. An SSO-gated repo only the laptop can reach   -> source: <scrubbed-url>
dot-ai skills generate --agent claude-code \
  --repo-fetch https://vpn.corp/team/skills

# 4. Work-in-progress skills on local disk  -> source: local:<user>-wip
DOT_AI_ALLOW_REPO_DIR=1 dot-ai skills generate --agent claude-code \
  --repo-dir ./my-skills --source-label wip
```

Each run is self-contained and source-scoped, so the four sets compose cleanly —
every run manages only the skills tagged with its own `source:`. The first hook
to write a given skill name wins; later hooks skip that name with a warning (see
[Collision Policy](#collision-policy-first-source-wins)).

When `--repo` is supplied, the CLI prints the source it received from the
server, redacted of any userinfo for safety:

```text
Skills generated successfully in .claude/skills
Source: https://github.com/orgA/skills
```

The no-flag invocation does not print a `Source:` line (it stays
byte-for-byte equivalent to pre-multi-repo behavior).

### Collision Policy (First-Source-Wins)

If two sources expose a skill with the same name, the first invocation to
write the skill wins; subsequent invocations skip the colliding name and log
a warning to stderr:

```text
warning: skipping "query": already provided by source "https://github.com/orgA/skills" (first-source-wins)
```

The named "other source" is redacted before logging. To resolve a real
collision, rename one of the skills upstream or drop one of the sources.

### Concurrent Invocations

If two hooks fire in parallel and both target the same skills directory, they
serialize on an exclusive lock file (`<outDir>/.dot-ai.lock`). The first
acquirer proceeds; the second waits briefly and, if the lock stays held, fails
fast with:

```text
Error: another `dot-ai skills generate` is in progress
```

Re-run the second hook after the first finishes (or rely on the agent's hook
runner to retry).

### Source Frontmatter

Every generated `SKILL.md` includes a `source:` field recording where the skill
came from. Its value depends on the source flag:

| Source flag | `source:` value | Example (as written on disk) |
|-------------|-----------------|------------------------------|
| *(none — env-var default)* | `built-in` | `source: built-in` |
| `--repo <url>` | the repo URL (credential-scrubbed) | `source: "https://github.com/orgA/skills"` |
| `--repo-fetch <url>` | the git URL (credential-scrubbed) | `source: "https://vpn.corp/team/skills"` |
| `--repo-dir … --source-label <label>` | `local:<user>-<label>` (host-prefixed) | `source: "local:alice-team-wip"` |

```yaml
---
name: dot-ai-troubleshoot-pod
description: Generate a troubleshooting guide for a pod
user-invocable: true
source: built-in
---
```

For `built-in` and `--repo`, the value comes from the server's response; for
`--repo-fetch` and `--repo-dir` the CLI computes it (the scrubbed URL, or the
`local:` identifier) and sends it as the server's `?source=` identifier. Either
way the CLI never alters the value between runs — it must match exactly between a
write and the subsequent wipe, which is what scopes each source's
[updates](#updating-skills).

### Legacy File Migration

Skills generated by earlier versions of the CLI do not have a `source:` field.
The first time you re-run `dot-ai skills generate` after upgrading:

- **No `--repo`** (env-var path): untagged legacy skills are assumed to belong
  to the env-var repo and are wiped as part of the normal per-source cleanup.
  Same effect as the pre-multi-repo behavior.
- **With `--repo X`**: untagged legacy files survive the per-source scan
  (they don't belong to source `X`). If a new skill from source `X` collides
  by name, the legacy file is overwritten. Subsequent runs without `--repo`
  then sweep up any remaining legacy files.

For most users, simply re-running the env-var-path invocation once after
upgrade is enough to retag everything.

## How It Works

1. CLI fetches prompts and tool metadata from the server
2. Generates a routing skill for CLI awareness
3. Creates individual skills for each server prompt, including any supporting files
4. All skills use `dot-ai-` prefix for namespacing

The generated skills include both built-in skills that ship with the server and any user-defined skills you've configured (see [Adding Custom Skills](#adding-custom-skills) below).

Each skill is a directory containing `SKILL.md` and optionally supporting files — shell scripts, templates, manifests, or other resources the skill references:

```text
.claude/skills/dot-ai-worktree-prd/
├── SKILL.md
├── create-worktree.sh
└── templates/
    └── branch-config.yaml
```

Supporting files are written with executable permissions. Nested paths automatically create intermediate directories.

## Adding Custom Skills

You can serve your own skills alongside the built-in ones by organizing them in a git repository.

### Repository Structure

Skills can be defined as single markdown files or as directories with `SKILL.md` and optional supporting files:

```text
my-team-skills/
├── deploy-app.md                    # Single-file skill
├── worktree-prd/                    # Skill with supporting files
│   ├── SKILL.md
│   └── create-worktree.sh
└── k8s-debug/                       # Skill with nested supporting files
    ├── SKILL.md
    ├── debug.sh
    └── templates/
        └── pod-debug.yaml
```

### Skill File Format

Each skill uses YAML frontmatter:

```yaml
---
name: deploy-app
description: Deploy an application to the specified environment
arguments:
  - name: environment
    description: Target environment (dev, staging, prod)
    required: true
---

# Deploy Application

Deploy the application to {{environment}}.
```

### Referencing Supporting Files

When your skill's `SKILL.md` references supporting files, **always use relative paths** — either bare (`analyze.sh`) or dot-prefixed (`./analyze.sh`):

~~~markdown
Run the analysis:
```bash
bash analyze.sh
```
~~~

During generation, the CLI automatically rewrites these to the correct full path based on the target agent and directory structure. For example, with `--agent claude-code`, the above becomes:

~~~markdown
```bash
bash .claude/skills/dot-ai-my-skill/analyze.sh
```
~~~

**Do not hardcode full paths** like `.claude/skills/my-skill/analyze.sh` in your source skill files. The final path depends on:
- The **agent** (`--agent claude-code` → `.claude/skills/`, `--agent cursor` → `.cursor/skills/`)
- The **`dot-ai-` prefix** added during generation

The rewrite applies to all files listed in the skill's supporting files. Nested paths work too — `templates/deploy.yaml` becomes `.claude/skills/dot-ai-my-skill/templates/deploy.yaml`.

### Server Configuration

To point the server at your skills repository, see [User-Defined Prompts](https://devopstoolkit.ai/docs/ai-engine/tools/prompts#user-defined-prompts).

Once configured, running `dot-ai skills generate` pulls your custom skills alongside the built-in ones.

## Agent Behavior

Once skills are generated:

**Routing:**
- Agents become aware of CLI for Kubernetes operations
- Agents prefer CLI over MCP when both are available
- Agents use `dot-ai --help` to discover commands

**Skills:**
- Server prompts appear as native agent skills
- Users can invoke them directly in their coding assistant
- Skills stay in sync with server capabilities

## Next Steps

- **[Automation](automation.md)** — Use CLI in scripts and CI/CD
- **[Output Formats](output-formats.md)** — Control output format
- **[Configuration](../setup/configuration.md)** — Configure server URL
