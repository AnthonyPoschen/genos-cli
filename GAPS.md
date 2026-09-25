# Gaps

**Usable today:** mint or copy a personal access token from the Genos account UI, then `genos auth token` (stdin) or the Omarchy panel Connect/paste flow. Both write the shared host store under `~/.config/genos/`. Agents can also mint PATs via `genos auth token-create` (secret printed once on stdout) and revoke with `genos auth token-revoke --yes`.

**Blocked until Genos ships device auth on the deployment:** `genos auth login` and panel device sign-in need `POST /api/v1/auth/device/codes` and `POST /api/v1/auth/device/tokens` (production currently 404s those routes; tracked as genos #216). Browser approve/deny helpers (`GET …/device/pending`, `POST …/approvals|denials`) exist in master Genos but are not CLI-facing.

This repository is HTTP-only and cannot finish Clerk/device login by itself.

This repository does not do Discord work.

## Managed files (not released)

`GET /api/v1/servers/{serverID}/files` and `POST /api/v1/servers/{serverID}/files/archive-transfer` currently return `404 capability_not_released` ("managed file workflows are not released"). genos-cli does not expose a `files` command. Agent save workflows use `genos saves …` (export/import) instead.

## Remaining gaps

Customer `/api/v1` routes from genos `internal/api/api.go` (admin and Stripe webhook omitted). **Covered through P1.7** are marked below.

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
| `GET /dashboard` | `genos dashboard`; `genos profiles[ list]` | **P1.6** (no `GET /profiles` list route) |
| `GET /servers`, `GET /servers/{id}` | `genos servers`, `genos status` | Covered |
| `PATCH /servers/{id}` | `genos rename` | Covered (P1.5) |
| `GET /servers/{id}/events` | — | SSE beyond console |
| `GET …/runtime-channels`, `…/stream`, `POST …/commands` | `genos console` (interactive command only) | Stream/SSE not wrapped |
| `GET …/runtime-sessions`, `…/channels/{id}`, `…/download` | — | History download leftover |
| `GET …/plan` | — | Optional read-only caution slice later |
| `POST …/plan-changes`, `POST …/billing/reactivate` | — | Billing money flows — agent caution |
| `GET /profiles/games` | `genos profiles games` | **P1.6** |
| `POST /profiles` | `genos profiles create` | **P1.6** |
| `PATCH /profiles/{id}` | `genos profiles rename` | **P1.6** autofill expectedName |
| `DELETE /profiles/{id}` | `genos profiles delete --yes` | **P1.6** |
| `GET/PUT /profiles/{id}/configuration` | `genos profiles config get/put` | **P1.6** put autofill like server config |
| `…/profiles/{id}/mods…` | `genos profiles mods …` | **P1.7** |
| `GET/POST/PATCH/DELETE …/setups…`, `PUT/DELETE …/selected-setup` | `genos setups`, `create-setup`, `rename-setup`, `delete-setup`, `select-setup`, `unload-setup` | Covered; attach = select-setup |
| `GET/PUT …/configuration` | `genos config get/put` | Covered |
| `POST …/public-rcon/credential` | — | public-rcon leftover |
| `…/servers/{id}/mods…` | `genos mods …` | Core covered; draft/import leftovers |
| `PUT …/mods/draft`, `POST …/mods/import`, `POST …/mods/draft/apply` | — | Mods draft/import leftovers |
| `POST …/actions` | `genos start/stop/force-stop/restart` | Covered |
| `GET/POST …/broadcast` | `genos broadcast-status`, `genos broadcast` | Covered (P1.5) |
| `POST/GET …/save-exports`, `…/save-imports…` | `genos saves …` | Server side covered |
| `…/profiles/{id}/save-*` | `genos profiles saves …` | **P1.7** |
| `GET …/setup-copy-destinations` | `genos setup-copy destinations` | **P1.6** |
| `POST …/setups/{setupID}/copies` | `genos setup-copy start` | **P1.6** |
| `GET …/setup-copies/{copyID}` | `genos setup-copy status` | **P1.6** |
| `PUT /server-order` | — | Fleet order leftover |
| `GET …/files`, `POST …/files/archive-transfer` | — | Unreleased capability |
| `POST /billing/checkout-sessions`, `POST /billing/portal-sessions` | — | Billing money flows — agent caution |

### Explicit follow-ups

1. **Billing money flows** — `plan-changes`, `billing/reactivate`, checkout/portal sessions. Do **not** auto-ship spendy flows; if ever added, require explicit human confirmation and clear agent caution labels.
2. **Optional read-only `GET …/plan`** — future caution-labeled slice (no mutations).
3. **Leftovers** — `public-rcon/credential`, SSE/events beyond console, runtime-sessions download, managed files (unreleased), fleet `PUT /server-order`, mods draft/import leftovers.
4. **Device auth** — still blocked on prod until genos #216.
5. Out of scope here: Omarchy, CatalogStatus/sales, Factorio/PZ private product, #212/#64, private genos/controller PRs.
