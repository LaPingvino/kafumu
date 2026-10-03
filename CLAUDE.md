# Kafumu — working rules

Read VISION.md before designing anything. LOOP-STATE.md is the build plan and progress log.

## Stack
Go (GAE Standard `go127`, project `lokumo`), net/http ServeMux, html/template (embedded), Pico CSS,
plain JS (HTMX when it helps), Datastore. Static assets are served by GAE from `static/`, not the app.

## Foundation rules (VISION.md §3) — every change must respect them
1. Private data never leaves the device unencrypted (pairs, contact notes, check-in history → IndexedDB).
2. Public data is the user's, on ATproto; Kafumu only indexes it.
3. The server holds only opaque or expiring data. Every server entity has an `expiresAt`, or means nothing
   without a client-held key, or is the minimal account. Pair-derived ids are fetched anonymously
   (`credentials: 'omit'`).
4. Ranking and matching happen on the device, against per-cell bundles.
5. A feature that needs a new rule, tier or primitive is a red flag. Use the "add a layer" checklist.
Coordinates never reach the server: only 6-char cells (`internal/geo`, mirrored in `static/kafumu.js`;
`test/geo_test.mjs` keeps them in sync).

## Cost discipline (free tier; lessons from esperanto-kurso commit 4000154)
- No Datastore query per request. Lists come from per-cell bundles cached in memory/memcache.
- Bots (`handler.IsBot`) never get an account, never cause a write, never trigger upstream fetches.
- Accounts are created lazily on the first action that needs one — never on a page view.
- Throttle presence writes (LastSeen hourly). Prefer few large entities. TTL everything ephemeral.
- `max_instances: 3`. Unknown paths 404.

## Working
- `sh test/run.sh` before every commit: go vet/test, client JS units, and the pairing E2E against a local server.
- Commit + push every loop tick. Deploy (`~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`; the pacman gcloud lacks app-engine-go) only when tests
  are green and something user-visible changed — Cloud Build minutes are limited.
- Never commit secrets. Config comes from env vars in app.yaml (non-secret only).
- Domain is not final: brand/origin come from config (`KAFUMU_BRAND`, `KAFUMU_ORIGIN`).
