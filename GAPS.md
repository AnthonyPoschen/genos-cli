# Gaps

**Usable today:** mint or copy a personal access token from the Genos account UI, then `genos auth token` (stdin) or the Omarchy panel Connect/paste flow. Both write the shared host store under `~/.config/genos/`.

**Blocked until Genos ships device auth:** `genos auth login` and panel device sign-in need `POST /api/v1/auth/device/codes` and `POST /api/v1/auth/device/tokens` on the deployment (production currently 404s those routes).

This repository is HTTP-only and cannot finish Clerk/device login by itself.

This repository does not do Discord work.
