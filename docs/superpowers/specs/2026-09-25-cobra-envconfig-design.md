# genos-cli: Cobra + envconfig rewrite

**Date:** 2026-09-25  
**Status:** Design approved in chat; awaiting spec file review before implementation  
**Repo:** AnthonyPoschen/genos-cli (still 0.0.1 — breaking changes OK, no migration note)

## Goal

Replace the hand-rolled CLI dispatcher and mega `usage` string with:

1. **spf13/cobra** — command tree and per-command help
2. **kelseyhightower/envconfig** — environment configuration

Humans and agents both use this CLI. Root help must stay short; nested detail lives under each group’s own `--help`.

## Non-goals

- Omarchy plugin argv updates (follow-up if the panel shells `genos`)
- Billing money flows, managed files, SSE beyond console, runtime-sessions download
- Changing Genos HTTP API shapes
- Compatibility aliases or a migration guide for old flat verbs

## Decisions (locked)

| Topic | Choice |
| --- | --- |
| Shipping | One-shot rewrite in a single PR |
| Compatibility | Break freely; no aliases; no migration note |
| Grouping | Usage-based noun groups (`server`, `profile`, `rcon`, …) |
| Product language | Drop “setup” from CLI; HTTP may still use `/setups` |
| Attach vs edit | `server profile` = attach only; `profile` = all library edits |
| RCON | Console send, broadcast, public credential under `rcon` |

## Command tree

Root `genos --help` lists **only** top-level groups (Short one-liners). Subcategory leaves do **not** appear at root.

### `genos server` — operate a game server

- `list` (was `servers`)
- `status`, `start`, `stop`, `force-stop`, `restart`, `rename`
- `order` (was `server-order`)
- `plan` (read-only; no spendy follow-ups)
- `config get|put`
- `mods …` / `saves …` (same verbs as today, nested here)
- `profile list|select|unload` — what’s attached + select/unload only  
  **Not here:** create/rename/delete/config/mods/saves for profiles

### `genos profile` — edit library profiles

- `list`, `games`, `create`, `rename`, `delete`
- `config get|put`
- `mods …` / `saves …`
- `copy destinations|start|status` (was `setup-copy`)

### `genos rcon` — in-game / RCON interaction

- `send` (was `console`)
- `broadcast`, `broadcast-status`
- `public-credential` (+ `--rotate` requiring `--yes`) (was `public-rcon`)

### `genos auth`

- `login`, `token`, `status`, `tokens`, `token-create`, `token-revoke`

### Awareness (top-level, small)

- `genos catalog`
- `genos me`
- `genos dashboard`
- `genos schema` (management schema by game id; was also `management-schema`)

## Help rules

1. **Root:** group names + Short only.
2. **Group** (`genos server --help`): direct children only (e.g. list `profile` and `mods` as children, not every mods leaf).
3. **Deeper** (`genos server mods --help`, `genos profile --help`): that subtree’s commands and flags.
4. Long agent-oriented prose stays in `SKILL.md` / `GAPS.md`, not root help.

## Internals

### Stack

- `github.com/spf13/cobra` for commands, flags, help
- `github.com/kelseyhightower/envconfig` with prefix `GENOS` for at least:
  - `GENOS_HOST`
  - `GENOS_TOKEN`
- After env: same resolution as today — `config.toml` `currentHost` → OS keyring (`service=genos`, attribute `host`) → `credentials.json` mode `0600`
- `GENOS_TOKEN` is never written to disk; `~/.config/genos/local.env` is not read

### Package layout

- `cmd/genos/main.go` — thin `main` → `cli.Execute`
- `internal/cli` — Cobra root + group files (`server.go`, `profile.go`, `rcon.go`, `auth.go`, …)
- Parent commands are containers (no Run that prints a mega-usage blob)
- Leaf commands call existing `internal/apiclient` helpers
- Keep `internal/apiclient`, `internal/creds`, `internal/config` unchanged in responsibility
- Remove the giant `usage` const and hand-rolled argv splitters as commands move

### Behavior to preserve

- JSON on stdout for mutations; progress on stderr
- `--yes` confirmation gates; no TTY → required prompt exits without sending
- Compare-and-swap autofill (`expectedSetupID`, `expectedName`, draft revision, etc.)
- Saves / profile-copy polling (`--wait`, interval/timeout caps)
- PAT secret and public-rcon password printed once
- No billing mutations; no `files` command
- HTTP paths unchanged (including `/setups` and `selected-setup` under the hood)

## Tests and docs

- Rewrite CLI tests to drive Cobra `Execute` with args and buffers
- Keep apiclient httptest coverage
- Update README, SKILL.md, GAPS.md command column to the new tree
- `go test ./...` green

## Success criteria

1. `genos --help` fits a short human scan (groups only)
2. `genos server --help` / `genos profile --help` / `genos rcon --help` show only that group’s direct children
3. All former customer capabilities from P1.1–P1.8 remain reachable under the new names
4. Env loads via envconfig; token/host precedence unchanged
5. One PR; squash-merge when Chief is happy

## Implementation note

Coder implements against this spec after Anthony reviews **this file**. Squash-merge remains Chief’s job.
