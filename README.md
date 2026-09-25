# genos-cli

Customer command line for Genos game servers.

## Install

```bash
go install github.com/AnthonyPoschen/genos-cli/cmd/genos@latest
```

Pin a commit or tag instead of `@latest` when you want a fixed build.

## Origin

The API origin defaults to `https://genosservers.com`. You do not need to set a host for production.

Optional overrides, first match wins:

1. `GENOS_HOST` — debug/local override for one command (or process).
2. `currentHost` in `$XDG_CONFIG_HOME/genos/config.toml` (default `~/.config/genos/config.toml`).

```toml
currentHost = "http://127.0.0.1:8000"
```

That file has no token. `http` is allowed only for `localhost`, `127.0.0.1`, and `genos.localhost`. Any other host must be `https`.

## Credentials

The first match wins. A set `GENOS_TOKEN` is never written to disk.

1. `GENOS_TOKEN`, when it is set and non-empty.
2. The OS keyring, service `genos`, attribute `host` equal to the API origin (same item the Omarchy panel uses; the `username`/`account` attribute is not used).
3. `$XDG_CONFIG_HOME/genos/credentials.json` (default `~/.config/genos/credentials.json`), mode `0600`:

```json
{"hosts":{"https://genosservers.com":{"token":"..."}}}
```

If `credentials.json` is group- or world-readable, genos refuses it until you `chmod 0600` it. `~/.config/genos/local.env` is never read.

**Primary path today:** paste a personal access token (PAT) from your Genos account, then store it with `genos auth token` (reads the token from stdin; do not pass it as a flag). That writes the shared host store (keyring attribute `host`, or `credentials.json` when no keyring is available) so the Omarchy panel can reuse it.

`genos auth login` (device-code flow) is implemented in this CLI but **requires an upcoming Genos API** (`POST /api/v1/auth/device/codes` and `…/tokens`). Until those routes ship on your deployment, use the PAT path.

`genos auth status` prints the origin, the source (`env`, `keyring`, or `file`), and the first 16 characters of the token.

## Commands

Built with [Cobra](https://github.com/spf13/cobra). Root help lists **groups only**; use `genos <group> --help` for direct children.

Environment: optional `GENOS_HOST` (debug/local API origin override) and `GENOS_TOKEN` via [envconfig](https://github.com/kelseyhightower/envconfig) (prefix `GENOS`). Origin resolution: `GENOS_HOST` → `config.toml` `currentHost` → default `https://genosservers.com`. Token resolution after env: OS keyring → `credentials.json` `0600`. `GENOS_TOKEN` is never written to disk; `local.env` is not read.

```text
genos server list
genos server status <serverID>
genos server start|stop|force-stop|restart <serverID> [--yes]
genos server rename <serverID> <name>
genos server order <serverID> [<serverID>...]
genos server plan <serverID>
genos server config get|put <serverID> [--file path]
genos server mods … / genos server saves …
genos server profile list|select|unload …

genos profile list|games|create|rename|delete …
genos profile config get|put …
genos profile mods … / genos profile saves …
genos profile copy destinations|start|status …

genos rcon send <serverID> <text...>
genos rcon broadcast|broadcast-status <serverID> [--message text]
genos rcon public-credential <serverID> [--rotate --yes]

genos auth login|token|status|tokens|token-create|token-revoke …

genos catalog
genos me
genos dashboard
genos schema <gameID>
```

### `genos server` — operate a game server

`list` / `status` print a headered table of ID, NAME, GAME, STATUS (`No servers.` when the list is empty).

`start` does not prompt. `stop` asks `Stop <name>?` unless `--yes`. `force-stop` asks for confirmation that mentions unsaved progress unless `--yes`, then tells the API that unsaved progress may be lost. `restart` asks when `playerCount` is greater than zero or `notableUpdates` is not empty, unless `--yes`. If a prompt is required, stdin is not a terminal, and `--yes` was not passed, genos exits without sending the action.

`profile list` prints a headered table (ID, NAME, GAME, SELECTED) and a separate `Creatable games:` section when present. `profile select` / `profile unload` change the selected profile (PUT/DELETE `/api/v1/servers/{id}/selected-setup`). They require the server to be confirmed Stopped on the API. Both send `expectedSelectedSetupID` for compare-and-swap. When `--expected` is omitted, genos GETs setups first and uses `selectedSetupID`. **Attach only** — create/rename/delete/config/mods/saves for profiles live under `genos profile`.

`rename` renames a server (`PATCH /api/v1/servers/{id}`). API must confirm Stopped.

`order` replaces fleet display order (`PUT /server-order`); positional order must list every owned server exactly once.

`plan` is read-only (`GET …/plan`). Do not invent plan-changes / reactivate / checkout / portal follow-ups.

`config get` / `config put` mirror the former top-level config commands (put autofills concurrency fields from GET when omitted).

`mods …` / `saves …` keep the same leaf verbs as before, nested under `server`.

### `genos profile` — library edits

`list` prints a headered library-profile table then a separate `Capacity: used/limit` line. `games` / `create --game` / `rename` / `delete --yes` manage library profiles (`POST /profiles`, etc.). Attach to a running server with `genos server profile select`.

`config get|put`, `mods …`, and `saves …` prep a library profile before attach (same flags as the server-side commands).

`copy destinations|start|status` covers setup copy/transfer between owned servers (was `setup-copy`). `--wait` polls until succeeded|failed.

### `genos rcon`

`send` sends interactive console text (was `console`). `broadcast` / `broadcast-status` send or preview in-game notices. `public-credential` reveals the public RCON password once; `--rotate` requires `--yes`.

### Awareness

`me`, `dashboard`, `catalog`, and `schema <gameID>` (management schema; former `management-schema` alias dropped).

### Auth

`auth login`, `auth token` (stdin), `auth status`, `auth tokens`, `auth token-create`, `auth token-revoke --yes` — unchanged behavior.

**Files not released:** no `files` command. Save workflows use `genos server saves …` / `genos profile saves …`.


## Mods (server path)

Server mod routes under `/api/v1/servers/{serverID}/mods`. Library prep uses the same shapes under `/api/v1/profiles/{profileID}/mods` via `genos profile mods …` (see below).

| CLI | HTTP |
| --- | --- |
| `genos server mods list <serverID>` | `GET …/mods` → prints the `mods` object |
| `genos server mods search <serverID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]` | `GET …/mods/catalog?q&category&sort&page&pageSize` → prints the `catalog` object |
| `genos server mods show <serverID> <providerModID>` | `GET …/mods/catalog/{modID}` → prints the `mod` object |
| `genos server mods credentials <serverID> --username U --token T [--expected-setup ID]` | `PUT …/mods/credentials` `{expectedSetupID,username,token}` |
| `genos server mods credentials-clear <serverID> [--expected-setup ID]` | `DELETE …/mods/credentials` `{expectedSetupID}` |
| `genos server mods stage <serverID> <providerModID> --provider ID [--expected-setup ID]` | `PUT …/mods/staged-selection` `{expectedSetupID,providerID,providerModID}` |
| `genos server mods unstage <serverID> [--expected-setup ID]` | `POST …/mods/staged-removal` `{expectedSetupID}` (stages removal of the enabled mod) |
| `genos server mods discard <serverID> [--expected-setup ID]` | `POST …/mods/discard` `{expectedSetupID}` (discards staged selection/removal) |
| `genos server mods apply <serverID> [--stage-id ID] [--expected-setup ID]` | `POST …/mods/apply` `{expectedSetupID,stageID}` |
| `genos server mods draft <serverID> --provider ID [--mod-id ID …] [--expected-setup ID]` | `PUT …/mods/draft` `{expectedSetupID,providerID,directModIDs}` |
| `genos server mods import <serverID> [--file path.json] [--expected-setup ID]` | `POST …/mods/import` `{expectedSetupID,content}` |
| `genos server mods draft-apply <serverID> [--expected-setup ID] [--expected-revision N]` | `POST …/mods/draft/apply` `{expectedSetupID,expectedRevision}` |
| `genos rcon public-credential <serverID> [--rotate --yes]` | `POST …/public-rcon/credential` `{rotate}` — password shown once |
| `genos server order <serverID> [<serverID>…]` | `PUT /server-order` `{serverIDs}` |
| `genos server plan <serverID>` | `GET …/plan` → prints `plan` object (**read-only**) |

**Autofill (P1.1/P1.2 style):** when `--expected-setup` is omitted, genos GETs mods and uses `setupID`. When `mods apply` omits `--stage-id`, genos GETs mods and uses `staged.stageID` (fails clearly if nothing is staged). `--provider` is **required** for `stage` (agents know the game provider). Examples: `factorio-mod-portal`, `steam-workshop` (and other provider IDs returned by catalog/list state).

List and mutating commands print the `mods` JSON object (pretty), same spirit as `config get`. Search/show print catalog JSON as returned. API errors (`server_not_confirmed_stopped`, `mod_provider_credentials_required`, `selected_setup_changed`, etc.) are surfaced as-is — never invent success. No `Idempotency-Key` (matches the dashboard).

**P1.8 leftovers:** `server mods draft|import|draft-apply` (and `profile mods …` mirrors), `rcon public-credential`, `server order`, and read-only `server plan` are implemented. Billing money flows remain out of scope.

## Saves (server path)

Server save export/import under `/api/v1/servers/{serverID}/save-exports` and `…/save-imports`. Library prep uses `genos profile saves …` against `/api/v1/profiles/{profileID}/save-*`.

| CLI | HTTP |
| --- | --- |
| `genos server saves export <serverID> [--wait] [--interval 2s] [--timeout 15m] [--output path]` | `POST …/save-exports` → poll `GET …/save-exports/{id}` when `--wait` |
| `genos server saves export-status <serverID> <exportID>` | `GET …/save-exports/{id}` |
| `genos server saves import <serverID> --file path.zip […]` | `POST …/save-imports` → `PUT uploadURL` (zip, no bearer) → optional validate/replace |
| `genos server saves import-status <serverID> <importID>` | `GET …/save-imports/{id}` |
| `genos server saves import-validate <serverID> <importID>` | `POST …/save-imports/{id}/validate` |
| `genos server saves import-replace <serverID> <importID> --yes […]` | `POST …/save-imports/{id}/replace` `{acknowledged:true, applySaveMods?}` |

**Export:** without `--wait`, prints export JSON after create. With `--wait`, polls until `succeeded|failed|expired` (default interval `2s`, timeout `15m`; caps: interval `100ms–1m`, timeout ≤`24h`). Progress on stderr; final JSON on stdout. On success prints `downloadURL` to stderr; downloads bytes **only** with `--output`. `failed`/`expired` exit non-zero after printing JSON. No Idempotency-Key (matches dashboard).

**Import:** create body `{fileName, mediaType, archiveBytes}` then PUT bytes to `uploadURL` with `Content-Type: application/zip` (no Genos bearer). Without `--wait`, stops after upload and prints import JSON. With `--wait`: validate → poll to `ready` → require `--yes` for replace (destructive); without `--yes`, print JSON + `import-replace --yes` hint and exit non-zero. With `--yes`: replace → poll to `succeeded|failed|expired` (default timeout `30m`). `--apply-save-mods` / `--no-apply-save-mods` set `applySaveMods` when the review requires a mod choice. `recovery` is shown on progress lines and in JSON. API errors pass through unchanged.

## Profiles mods/saves (library prep, P1.7)

Prep a library profile the same way you prep a live server, then attach with `genos server profile select`.

| CLI | HTTP |
| --- | --- |
| `genos profile mods … <profileID> …` | `/api/v1/profiles/{profileID}/mods…` (same subcommands/flags as `genos server mods`, including draft/import/draft-apply) |
| `genos profile saves … <profileID> …` | `/api/v1/profiles/{profileID}/save-exports` / `save-imports…` (same flags/poll/`--wait`/`--output` as `genos server saves`) |

On profiles, `--expected-setup` autofills from GET mods `setupID` (the profile id). Live-server targeting stays `genos mods|saves …`.
