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
- `genos auth status` shows the origin, whether the token came from `env`, `keyring`, or `file`, and a token prefix only.
- `genos auth login` and `genos auth token` are for a human. Never pass a token as an argument.

## Credentials

`GENOS_HOST` overrides `currentHost` in `$XDG_CONFIG_HOME/genos/config.toml`. `GENOS_TOKEN` overrides stored credentials and must not be written to disk. Do not read `~/.config/genos/local.env`.

If stdin is not a terminal and a confirmation is required, the command exits without sending the action. Pass `--yes` only after the server and the action have been named.
