# genos-cli

Customer command line for Genos game servers.

## Install

```bash
go install github.com/AnthonyPoschen/genos-cli/cmd/genos@feat/customer-control-clients
```

Use the branch or tag you want in place of `feat/customer-control-clients`.

## Origin

`GENOS_HOST` selects the API origin for one command. When it is unset, genos reads `currentHost` from `$XDG_CONFIG_HOME/genos/config.toml`, which defaults to `~/.config/genos/config.toml`.

```toml
currentHost = "https://genosservers.com"
```

That file has no token. `http` is allowed only for `localhost`, `127.0.0.1`, and `genos.localhost`. Any other host must be `https`.

## Credentials

The first match wins. A set `GENOS_TOKEN` is never written to disk.

1. `GENOS_TOKEN`, when it is set and non-empty.
2. The OS keyring, service `genos`, account equal to the API origin.
3. `$XDG_CONFIG_HOME/genos/credentials.json` (default `~/.config/genos/credentials.json`), mode `0600`:

```json
{"hosts":{"https://genosservers.com":{"token":"..."}}}
```

If `credentials.json` is group- or world-readable, genos refuses it until you `chmod 0600` it. `~/.config/genos/local.env` is never read.

`genos auth login` starts device login, prints the user code and approval URL, and stores the token in the keyring. When no keyring is available it writes `credentials.json` and says so. `genos auth token` reads a token from stdin and stores it the same way. Do not pass the secret as a flag. `genos auth status` prints the origin, the source (`env`, `keyring`, or `file`), and the first 16 characters of the token.

## Commands

```text
genos servers
genos status <serverID>
genos start <serverID> [--yes]
genos stop <serverID> [--yes]
genos force-stop <serverID> [--yes]
genos restart <serverID> [--yes]
genos console <serverID> <text...>
genos auth login
genos auth token
genos auth status
```

`servers` and `status` print the server name, game name, and status.

`start` does not prompt. `stop` asks `Stop <name>?` unless `--yes`. `force-stop` asks for confirmation that mentions unsaved progress unless `--yes`, then tells the API that unsaved progress may be lost. `restart` asks when `playerCount` is greater than zero or `notableUpdates` is not empty, unless `--yes`. If a prompt is required, stdin is not a terminal, and `--yes` was not passed, genos exits without sending the action.

`console` sends the text to the first runtime channel whose interaction is `interactive`, and prints the response.
