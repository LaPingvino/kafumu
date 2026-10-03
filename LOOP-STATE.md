# Loop state

Each loop tick: read this file, take the first unchecked slice, build it (~15 min of work), run
tests, commit, push, deploy if user-visible and green, tick it off here with a one-line note,
and add anything learned to "Notes". Keep slices small; split a slice if it runs long.

Production: https://lokumo.ew.r.appspot.com · Repo: https://github.com/LaPingvino/kafumu

**Reminder for Joop's next check-in:** kafumu.com is bought — set up the GAE custom domain
(`gcloud app domain-mappings create kafumu.com --project lokumo` + the DNS records it prints, managed
cert), then set `KAFUMU_ORIGIN=https://kafumu.com` in app.yaml. Decide apex vs www before passkeys.

## M1 slices

- [x] 0. Scaffold: `#geo` cells (Go + JS, cross-tested), home shell computing the cell on-device,
      `/bundle` from Bluesky `#geo` search with per-instance cache, `/about`, robots, IsBot, app.yaml.
- [x] 1. Place gazetteer v0: 60 hand-picked cities (centre + radius → cells, aliases, ambiguity),
      `/bundle` also searches the top 4 place tags; client labels "from #amsterdam", ranks below #geo
      posts, caps 2 posts per author. Central Amsterdam: 0 → 75 posts.
- [ ] 2. `LaPingvino/geotags` public repo: generator from GeoNames (cities15000, top ~500 by population,
      bbox → 6-char cells, ambiguity), plus the language list; CC-BY attribution. Kafumu vendors the JSON.
- [ ] 3. Language gazetteer + canonical `lang:` tags (ISO 639-3, `#langepo`), embedded JSON with names.
- [ ] 4. Accounts: copy esperanto-kurso auth; cookie `userID.token`, token stored hashed, Get-by-key;
      created lazily on first action; magic-link page; delete-account button.
- [ ] 5. Passkeys (go-webauthn) on the current origin; magic link stays the recovery path.
- [ ] 6. Profile: username, `lang:xxx/level` picker, hobby tags, home cell, `Discoverable` (opt-in).
- [ ] 7. People in the bundle: discoverable profiles per cell, in-memory cache + `CellMeta` version.
- [ ] 8. Client ranking: complementary language exchange, locally-rare shared language, hobbies.
- [ ] 9. Switch to `appengine.Main()` + memcache (bundled services), cache layer interface with an
      in-memory fallback for local runs.
- [ ] 10. Device store: IndexedDB pairs + contact notes; pairing QR show/scan; mailbox handshake (`Box`).
- [ ] 11. Slots + beacons: check-in on open, "friends around today / this week" on the home list.
- [ ] 12. Contact notes UI and canned signals ("I'm around, coffee?", "here's my number").
- [ ] 13. Meetups (fallback records): create, list in bundle, RSVP; expiry.
- [ ] 14. `.ics` feeds per cell/tag and per user.
- [ ] 15. `/patrons`: Liberapay link + Stripe Payment Link placeholders, gentle copy.
- [ ] 16. Travel banner; daily purge cron; Datastore TTL policies; privacy page with the data table.
- [ ] 17. UI locale from esperanto-kurso's locale files.

## Decisions taken without asking (Joop can overrule)
6-char cells only · export-QR for multi-device · `#langepo` + gazetteer · curated gazetteers ·
Liberapay now, Stripe later · discoverability opt-in · "Kafumu" everywhere · passkeys on from the
start (magic link re-binds them after a domain move).

## Notes
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
