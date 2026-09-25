# Gaps

**Usable today:** mint or copy a personal access token from the Genos account UI, then `genos auth token` (stdin) or the Omarchy panel Connect/paste flow. Both write the shared host store under `~/.config/genos/`.

**Blocked until Genos ships device auth:** `genos auth login` and panel device sign-in need `POST /api/v1/auth/device/codes` and `POST /api/v1/auth/device/tokens` on the deployment (production currently 404s those routes).

This repository is HTTP-only and cannot finish Clerk/device login by itself.

This repository does not do Discord work.

## Managed files (not released)

`GET /api/v1/servers/{serverID}/files` and `POST /api/v1/servers/{serverID}/files/archive-transfer` currently return `404 capability_not_released` ("managed file workflows are not released"). genos-cli does not expose a `files` command. Agent save workflows use `genos saves …` (export/import) instead.
