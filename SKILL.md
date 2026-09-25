---
name: genos
description: Control a customer's Genos game servers by running the genos binary. Do not call the Genos HTTP API yourself.
---

# Genos customer CLI

Run the `genos` binary. Do not invent HTTP calls, curl the API, or talk to Kubernetes. There is no separate agent protocol.

Before passing `--yes`, name the server and the action. Do not hide a stop, force-stop, or restart behind `--yes` until that is stated.

## Commands

- `genos servers` lists each server's name, game name, and status.
- `genos status <serverID>` prints those fields for one server.
- `genos start <serverID>` starts a server and does not prompt.
- `genos stop <serverID>` asks `Stop <name>?` unless `--yes`.
- `genos force-stop <serverID>` warns that unsaved progress will be lost unless `--yes`.
- `genos restart <serverID>` asks when players are online or notable updates are pending, unless `--yes`.
- `genos console <serverID> <text...>` sends one console command and prints the response.
- `genos setups <serverID>` lists profiles (`id`, `name`, `game`) and marks the selected one with `*`; also prints `creatable\t<gameID>\t<name>` lines from `creatableGames`.
- `genos select-setup <serverID> <setupID> [--expected <id>]` selects a profile. Without `--expected`, GETs setups and uses `selectedSetupID`.
- `genos unload-setup <serverID> [--expected <id>]` unloads the selected profile (same `--expected` default).
- `genos create-setup <serverID> --game <gameID>` creates a profile (JSON). Game ids come from `genos setups` creatable lines.
- `genos rename-setup <serverID> <setupID> <name> [--expected-name <name>] [--expected <id>]` renames a profile (JSON). Autofills expected* from GET setups when omitted.
- `genos delete-setup <serverID> <setupID> --yes [--expected-name <name>] [--expected <id>]` deletes a profile (JSON). Requires `--yes`.
- `genos rename <serverID> <name>` renames a server (JSON). API must confirm Stopped; never invent success.
- `genos broadcast <serverID> [--message <text>]` POSTs an in-game notice (JSON).
- `genos broadcast-status <serverID> [--message <text>]` GETs broadcast availability/preview (JSON).
- Managed `files` / `files/archive-transfer` are **not released** (`capability_not_released`). Do not invent a files command; use `genos saves …` for saves.
- `genos config get <serverID>` prints selected setup configuration JSON.
- `genos config put <serverID> [--file path]` PUTs configuration. Preferred: full body with `expectedSetupID`, `expectedUpdatedAt`, `version`, `values` (+ optional `secrets`) on stdin/`--file`. If concurrency fields are omitted but `values` is present, GETs configuration first and fills them. API errors (e.g. `server_not_confirmed_stopped`) are surfaced as-is.
- `genos schema <gameID>` (alias `management-schema`) prints the public management-schema JSON.
- `genos mods list <serverID>` prints selected-setup mods JSON (`GET /api/v1/servers/{id}/mods`).
- `genos mods search <serverID> [--query q] [--category c] [--sort s] [--page n] [--page-size n]` prints catalog JSON.
- `genos mods show <serverID> <providerModID>` prints one catalog mod JSON.
- `genos mods credentials <serverID> --username U --token T [--expected-setup ID]` stores provider credentials.
- `genos mods credentials-clear <serverID> [--expected-setup ID]` clears provider credentials.
- `genos mods stage <serverID> <providerModID> --provider ID [--expected-setup ID]` stages a catalog mod (`--provider` required; e.g. `factorio-mod-portal`, `steam-workshop`).
- `genos mods unstage <serverID> [--expected-setup ID]` stages removal of the enabled mod.
- `genos mods discard <serverID> [--expected-setup ID]` discards staged selection/removal.
- `genos mods apply <serverID> [--stage-id ID] [--expected-setup ID]` applies the staged change. Without `--expected-setup` / `--stage-id`, GETs mods and autofills `setupID` / `staged.stageID` (fails clearly if nothing staged). API errors pass through.
- Profile `/api/v1/profiles/.../mods` mirrors, `draft-set`/`draft-apply`, and `import` are out of scope here.
- `genos saves export <serverID> [--wait] [--interval 2s] [--timeout 15m] [--output path]` starts a save export; `--wait` polls to a terminal status. Prints `downloadURL` on success; downloads bytes only with `--output`.
- `genos saves export-status <serverID> <exportID>` prints one export JSON.
- `genos saves import <serverID> --file path.zip […]` creates an import, PUTs the zip to `uploadURL` (no Genos bearer). Without `--wait`, stops after upload. With `--wait`, validates, polls to ready, and replaces only with `--yes` (destructive).
- `genos saves import-status <serverID> <importID>` / `import-validate` / `import-replace --yes` cover the stepped flow. `--apply-save-mods` / `--no-apply-save-mods` set `applySaveMods` when required. failed/expired exit non-zero; never invent success.
- Profile `/api/v1/profiles/.../save-*` mirrors are out of scope here.
- `genos auth status` shows the origin, whether the token came from `env`, `keyring`, or `file`, and a token prefix only.
- `genos auth login` and `genos auth token` are for a human. Never pass a token as an argument.

## Credentials

`GENOS_HOST` overrides `currentHost` in `$XDG_CONFIG_HOME/genos/config.toml`. `GENOS_TOKEN` overrides stored credentials and must not be written to disk. Do not read `~/.config/genos/local.env`.

If stdin is not a terminal and a confirmation is required, the command exits without sending the action. Pass `--yes` only after the server and the action have been named.
