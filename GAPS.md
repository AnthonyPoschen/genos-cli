# Gaps

**Usable today:** mint or copy a personal access token from the Genos account UI, then `genos auth token` (stdin) or the Omarchy panel Connect/paste flow. Both write the shared host store under `~/.config/genos/`. Agents can also mint PATs via `genos auth token-create` (secret printed once on stdout) and revoke with `genos auth token-revoke --yes`.

**API origin:** defaults to `https://genosservers.com`. `GENOS_HOST` is an optional debug/local override, not required for normal use; `currentHost` in `config.toml` overrides the default when set.

**Blocked until Genos ships device auth on the deployment:** `genos auth login` and panel device sign-in need `POST /api/v1/auth/device/codes` and `POST /api/v1/auth/device/tokens` (production currently 404s those routes; tracked as genos #216). Browser approve/deny helpers (`GET …/device/pending`, `POST …/approvals|denials`) exist in master Genos but are not CLI-facing.

This repository is HTTP-only and cannot finish Clerk/device login by itself.

This repository does not do Discord work.

## Managed files (not released)

`GET /api/v1/servers/{serverID}/files` and `POST /api/v1/servers/{serverID}/files/archive-transfer` currently return `404 capability_not_released` ("managed file workflows are not released"). genos-cli does not expose a `files` command. Agent save workflows use `genos server saves …` (export/import) instead.

## Remaining gaps

Customer `/api/v1` routes from genos `internal/api/api.go` (admin and Stripe webhook omitted). **Covered through P1.8** are marked below.

| Route | CLI today | Notes |
| --- | --- | --- |
| `GET /health`, `GET /ready` | — | Ops probes; not customer CLI |
| `GET /me` | `genos me` | **P1.6** |
| `POST /auth/device/codes`, `POST /auth/device/tokens` | `genos auth login` | Blocked on prod until #216 |
| `GET /auth/device/pending`, `POST /auth/device/approvals`, `POST /auth/device/denials` | — | Browser device-approval helpers |
| `GET /auth/tokens` | `genos auth tokens` | **P1.6** metadata only |
| `POST /auth/tokens` | `genos auth token-create` | **P1.6** secret once on stdout |
| `DELETE /auth/tokens/{tokenID}` | `genos auth token-revoke --yes` | **P1.6** |
| `GET /catalog` | `genos catalog` | **P1.6** public |
| `GET /games/{gameID}/management-schema` | `genos schema` | Covered earlier |
| `GET /dashboard` | `genos dashboard`; `genos profile list` | **P1.6** (no `GET /profiles` list route) |
| `GET /servers`, `GET /servers/{id}` | `genos server list`, `genos server status` | Covered |
| `PATCH /servers/{id}` | `genos server rename` | Covered (P1.5) |
| `GET /servers/{id}/events` | — | SSE beyond console |
| `GET …/runtime-channels`, `…/stream`, `POST …/commands` | `genos rcon send` (interactive command only) | Stream/SSE not wrapped |
| `GET …/runtime-sessions`, `…/channels/{id}`, `…/download` | — | History download leftover |
| `GET …/plan` | `genos server plan` | **P1.8** read-only; do not invent spendy follow-ups |
| `POST …/plan-changes`, `POST …/billing/reactivate` | — | Billing money flows — agent caution |
| `GET /profiles/games` | `genos profile games` | **P1.6** |
| `POST /profiles` | `genos profile create` | **P1.6** |
| `PATCH /profiles/{id}` | `genos profile rename` | **P1.6** autofill expectedName |
| `DELETE /profiles/{id}` | `genos profile delete --yes` | **P1.6** |
| `GET/PUT /profiles/{id}/configuration` | `genos profile config get/put` | **P1.6** put autofill like server config |
| `…/profiles/{id}/mods…` | `genos profile mods …` | **P1.7** |
| `GET/POST/PATCH/DELETE …/setups…`, `PUT/DELETE …/selected-setup` | `genos server profile list|select|unload`; library CRUD via `genos profile create|rename|delete` | Attach = select; server-scoped setup CRUD CLI dropped |
| `GET/PUT …/configuration` | `genos server config get/put` | Covered |
| `POST …/public-rcon/credential` | `genos rcon public-credential` | **P1.8** password once; `--rotate` needs `--yes` |
| `…/servers/{id}/mods…` | `genos server mods …` | Core + **P1.8** draft/import/draft-apply |
| `PUT …/mods/draft`, `POST …/mods/import`, `POST …/mods/draft/apply` | `genos server mods draft|import|draft-apply` (+ `profile mods …`) | **P1.8** |
| `POST …/actions` | `genos server start|stop|force-stop|restart` | Covered |
| `GET/POST …/broadcast` | `genos rcon broadcast-status`, `genos rcon broadcast` | Covered (P1.5) |
| `POST/GET …/save-exports`, `…/save-imports…` | `genos server saves …` | Server side covered |
| `…/profiles/{id}/save-*` | `genos profile saves …` | **P1.7** |
| `GET …/setup-copy-destinations` | `genos profile copy destinations` | **P1.6** |
| `POST …/setups/{setupID}/copies` | `genos profile copy start` | **P1.6** |
| `GET …/setup-copies/{copyID}` | `genos profile copy status` | **P1.6** |
| `PUT /server-order` | `genos server order` | **P1.8** |
| `GET …/files`, `POST …/files/archive-transfer` | — | Unreleased capability |
| `POST /billing/checkout-sessions`, `POST /billing/portal-sessions` | — | Billing money flows — agent caution |

### Explicit follow-ups

1. **Billing money flows** — `plan-changes`, `billing/reactivate`, checkout/portal sessions. Do **not** auto-ship spendy flows; if ever added, require explicit human confirmation and clear agent caution labels. `genos server plan` is read-only only.
2. **Device auth** — still blocked on prod until genos #216.
3. **Deferred leftovers** — SSE/events beyond console, runtime-sessions download, managed files (unreleased).
4. Out of scope here: Omarchy, CatalogStatus/sales, Factorio/PZ private product, #212/#64, private genos/controller PRs.
