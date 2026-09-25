# genos-cli

Customer command line for Genos game servers.

## Install

```bash
go install github.com/AnthonyPoschen/genos-cli/cmd/genos@latest
```

Pin a commit or tag instead of `@latest` when you want a fixed build.

## Origin

`GENOS_HOST` selects the API origin for one command. When it is unset, genos reads `currentHost` from `$XDG_CONFIG_HOME/genos/config.toml`, which defaults to `~/.config/genos/config.toml`.

```toml
currentHost = "https://genosservers.com"
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

```text
genos servers
genos status <serverID>
genos start <serverID> [--yes]
genos stop <serverID> [--yes]
genos force-stop <serverID> [--yes]
genos restart <serverID> [--yes]
genos console <serverID> <text...>
genos setups <serverID>
genos select-setup <serverID> <setupID> [--expected <id>]
genos unload-setup <serverID> [--expected <id>]
genos config get <serverID>
genos config put <serverID> [--file path]
genos schema <gameID>
genos management-schema <gameID>
genos mods list <serverID>
genos mods search <serverID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]
genos mods show <serverID> <providerModID>
genos mods credentials <serverID> --username U --token T [--expected-setup ID]
genos mods credentials-clear <serverID> [--expected-setup ID]
genos mods stage <serverID> <providerModID> --provider ID [--expected-setup ID]
genos mods unstage <serverID> [--expected-setup ID]
genos mods discard <serverID> [--expected-setup ID]
genos mods apply <serverID> [--stage-id ID] [--expected-setup ID]
genos saves export <serverID> [--wait] [--interval 2s] [--timeout 15m] [--output path]
genos saves export-status <serverID> <exportID>
genos saves import <serverID> --file path.zip [--media-type application/zip] [--wait] [--interval 2s] [--timeout 30m] [--apply-save-mods|--no-apply-save-mods] [--yes]
genos saves import-status <serverID> <importID>
genos saves import-validate <serverID> <importID>
genos saves import-replace <serverID> <importID> --yes [--apply-save-mods|--no-apply-save-mods]
genos auth login
genos auth token
genos auth status
```

`servers` and `status` print the server name, game name, and status.

`start` does not prompt. `stop` asks `Stop <name>?` unless `--yes`. `force-stop` asks for confirmation that mentions unsaved progress unless `--yes`, then tells the API that unsaved progress may be lost. `restart` asks when `playerCount` is greater than zero or `notableUpdates` is not empty, unless `--yes`. If a prompt is required, stdin is not a terminal, and `--yes` was not passed, genos exits without sending the action.

`console` sends the text to the first runtime channel whose interaction is `interactive`, and prints the response.

`setups` lists each profile for a server as `id`, `name`, `game`, and marks the selected one with `*`.

`select-setup` and `unload-setup` change the selected profile (PUT/DELETE `/api/v1/servers/{id}/selected-setup`). They require the server to be confirmed Stopped on the API; a Running server surfaces the API error `server_not_confirmed_stopped` (no client-side fake success). Both send `expectedSelectedSetupID` for compare-and-swap. When `--expected` is omitted, genos GETs `/api/v1/servers/{id}/setups` first and uses that response's `selectedSetupID` (empty string if none). When `--expected` is passed, its value is sent as-is; the flag requires a following value.

`config get` prints the selected setup configuration JSON from `GET /api/v1/servers/{id}/configuration` (the inner `configuration` object, pretty-printed).

`config put` sends `PUT /api/v1/servers/{id}/configuration`. The body is read from `--file` or stdin and must be a JSON object that includes `values`. **Preferred agent path:** include the full concurrency fields yourself — `expectedSetupID`, `expectedUpdatedAt` (RFC3339 from `configuration.updatedAt`), `version`, and `values` (optional `secrets`). **Convenience (P1.1-style default):** if any of those three concurrency fields is omitted, genos GETs configuration first and fills the missing ones from the live object (`setupID` → `expectedSetupID`, `updatedAt` → `expectedUpdatedAt`, `version` → `version`). A non-object body or a body without `values` fails with a usage error before calling the API. PUT failures such as `server_not_confirmed_stopped`, `configuration_changed`, or `selected_setup_changed` are surfaced as-is (no client-side fake success). No `Idempotency-Key` is sent (matches the dashboard).


`schema` (alias `management-schema`) prints the public management-schema JSON from `GET /api/v1/games/{gameID}/management-schema`.

## Mods (server path)

Server mod routes under `/api/v1/servers/{serverID}/mods` (profile `/api/v1/profiles/.../mods` mirrors are out of scope).

| CLI | HTTP |
| --- | --- |
| `genos mods list <serverID>` | `GET …/mods` → prints the `mods` object |
| `genos mods search <serverID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]` | `GET …/mods/catalog?q&category&sort&page&pageSize` → prints the `catalog` object |
| `genos mods show <serverID> <providerModID>` | `GET …/mods/catalog/{modID}` → prints the `mod` object |
| `genos mods credentials <serverID> --username U --token T [--expected-setup ID]` | `PUT …/mods/credentials` `{expectedSetupID,username,token}` |
| `genos mods credentials-clear <serverID> [--expected-setup ID]` | `DELETE …/mods/credentials` `{expectedSetupID}` |
| `genos mods stage <serverID> <providerModID> --provider ID [--expected-setup ID]` | `PUT …/mods/staged-selection` `{expectedSetupID,providerID,providerModID}` |
| `genos mods unstage <serverID> [--expected-setup ID]` | `POST …/mods/staged-removal` `{expectedSetupID}` (stages removal of the enabled mod) |
| `genos mods discard <serverID> [--expected-setup ID]` | `POST …/mods/discard` `{expectedSetupID}` (discards staged selection/removal) |
| `genos mods apply <serverID> [--stage-id ID] [--expected-setup ID]` | `POST …/mods/apply` `{expectedSetupID,stageID}` |

**Autofill (P1.1/P1.2 style):** when `--expected-setup` is omitted, genos GETs mods and uses `setupID`. When `mods apply` omits `--stage-id`, genos GETs mods and uses `staged.stageID` (fails clearly if nothing is staged). `--provider` is **required** for `stage` (agents know the game provider). Examples: `factorio-mod-portal`, `steam-workshop` (and other provider IDs returned by catalog/list state).

List and mutating commands print the `mods` JSON object (pretty), same spirit as `config get`. Search/show print catalog JSON as returned. API errors (`server_not_confirmed_stopped`, `mod_provider_credentials_required`, `selected_setup_changed`, etc.) are surfaced as-is — never invent success. No `Idempotency-Key` (matches the dashboard).

**Deferred follow-ups:** `mods draft-set` → `PUT …/mods/draft`, `mods draft-apply` → `POST …/mods/draft/apply`, `mods import` → `POST …/mods/import`, and profile-library mod route mirrors.

## Saves (server path)

Server save export/import under `/api/v1/servers/{serverID}/save-exports` and `…/save-imports` (profile `/api/v1/profiles/…/save-*` mirrors are deferred).

| CLI | HTTP |
| --- | --- |
| `genos saves export <serverID> [--wait] [--interval 2s] [--timeout 15m] [--output path]` | `POST …/save-exports` → poll `GET …/save-exports/{id}` when `--wait` |
| `genos saves export-status <serverID> <exportID>` | `GET …/save-exports/{id}` |
| `genos saves import <serverID> --file path.zip […]` | `POST …/save-imports` → `PUT uploadURL` (zip, no bearer) → optional validate/replace |
| `genos saves import-status <serverID> <importID>` | `GET …/save-imports/{id}` |
| `genos saves import-validate <serverID> <importID>` | `POST …/save-imports/{id}/validate` |
| `genos saves import-replace <serverID> <importID> --yes […]` | `POST …/save-imports/{id}/replace` `{acknowledged:true, applySaveMods?}` |

**Export:** without `--wait`, prints export JSON after create. With `--wait`, polls until `succeeded|failed|expired` (default interval `2s`, timeout `15m`; caps: interval `100ms–1m`, timeout ≤`24h`). Progress on stderr; final JSON on stdout. On success prints `downloadURL` to stderr; downloads bytes **only** with `--output`. `failed`/`expired` exit non-zero after printing JSON. No Idempotency-Key (matches dashboard).

**Import:** create body `{fileName, mediaType, archiveBytes}` then PUT bytes to `uploadURL` with `Content-Type: application/zip` (no Genos bearer). Without `--wait`, stops after upload and prints import JSON. With `--wait`: validate → poll to `ready` → require `--yes` for replace (destructive); without `--yes`, print JSON + `import-replace --yes` hint and exit non-zero. With `--yes`: replace → poll to `succeeded|failed|expired` (default timeout `30m`). `--apply-save-mods` / `--no-apply-save-mods` set `applySaveMods` when the review requires a mod choice. `recovery` is shown on progress lines and in JSON. API errors pass through unchanged.

**Deferred:** profile `/api/v1/profiles/{profileID}/save-exports` and `…/save-imports` mirrors.
