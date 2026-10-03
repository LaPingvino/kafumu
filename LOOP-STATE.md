# Loop state

Each loop tick: read this file, take the first unchecked slice, build it (~15 min of work), run
tests, commit, push, deploy if user-visible and green, tick it off here with a one-line note,
and add anything learned to "Notes". Keep slices small; split a slice if it runs long.

Production: https://lokumo.ew.r.appspot.com · Repo: https://github.com/LaPingvino/kafumu

**Reminder for Joop's next check-in:** kafumu.com is bought — set up the GAE custom domain
(`gcloud app domain-mappings create kafumu.com --project lokumo` + the DNS records it prints, managed
cert), then set `KAFUMU_ORIGIN=https://kafumu.com` in app.yaml. Decide apex vs www before passkeys.

## Target: Web Summit Lisbon, 9–12 Nov 2026 (MEO Arena / FIL, Parque das Nações)

Joop lives in Barreiro and has Open Source tickets. Goal: Kafumu is the less clunky way to meet
people there — scan someone, keep the note on your phone, see side events and who's around for coffee.
Everything before the event serves that; the rest of M1 follows after.

- [x] 0. Scaffold: `#geo` cells (Go + JS, cross-tested), home shell computing the cell on-device,
      `/bundle` from Bluesky `#geo` search with per-instance cache, `/about`, robots, IsBot, app.yaml.
- [x] 1. Place gazetteer v0: 60 hand-picked cities (centre + radius → cells, aliases, ambiguity),
      `/bundle` also searches the top 4 place tags; client labels "from #amsterdam", ranks below #geo
      posts, caps 2 posts per author. Central Amsterdam: 0 → 75 posts.
- [x] 2. Lisbon area (Barreiro, Almada, Seixal, Parque das Nações…) + time-bound event tags:
      `#websummit` is local to the venue from 45 days before to the last day; banner + compose link.
- [x] 2b. UI in EN, PT (European), EO, NL: `internal/locale` (adapted from esperanto-kurso, English
      fallback), cookie switcher set client-side (no server write), `js.*` strings handed to the client.
      Test enforces identical keys and placeholders across languages.
- [x] 3. Accounts (`internal/account`): created only by an explicit POST (never on page views, never
      for bots), cookie `id.token`, sha256 token hash, Get-by-key + 1-min instance cache, LastSeen hourly,
      usernames as separate keyed entities (transactional uniqueness), magic link, sign out, delete.
      Memory store for local runs. No email/phone collected.
- [ ] 3b. Passkeys (go-webauthn) on the current origin; magic link stays the recovery path.
- [ ] 4. Device store + "my card": IndexedDB; a card you choose to share (name, what you do, links:
      LinkedIn/Bluesky/Signal/WhatsApp/email). Works without an account.
- [ ] 5. Scan to connect: QR with K + mailbox id; scanner sends its card encrypted via the mailbox
      (`Box`, memcache + TTL); both sides end up with each other's card + a private note
      ("met at WS, robotics, coffee Thu"). This is the conference killer feature.
- [ ] 6. Contacts page: everyone you've scanned, notes, one-tap open of their links; export (vCard/CSV).
- [ ] 7. Meetups (fallback records, need account): "coffee at Pavilion 2, 15:00", side events, RSVP,
      tags `#websummit` + `lang:` + `tag:`; shown in the bundle; `.ics`.
- [ ] 8. Canned signals between scanned contacts: "I'm at the coffee bar", "join us at …".
- [ ] 9. Profile tags + discoverable people at the event (opt-in): languages, interests
      (opensource, esperanto, climate…), matched on the device.
- [ ] 10. Friends around (slots/beacons): "who you scanned is still in Lisbon today".
- [ ] 11. `/patrons` + `/for-cafes` pages (Liberapay, Stripe Payment Link), privacy page with data table.
- [ ] 12. Event polish: install prompt (PWA), offline shell, printable QR for a badge/T-shirt, PT/EN UI.
      Load test the bundle path; check free-tier quotas for ~1k users/day.

## Second beachhead: language events (Joop is a HYPIA member, visits language events)
amikumu grew through Esperanto events. Aim to be the best tool at polyglot/Esperanto/language-café
events early, so it spreads locally by word of mouth. Means: language tags + people matching early,
event entries for language gatherings (only with verified dates/venues — never guessed), EO/PT/EN/NL UI.

## After the event (rest of M1)
- `LaPingvino/geotags` public repo: generator from GeoNames + language list, CC-BY; Kafumu vendors it.
- Language gazetteer + canonical `lang:` tags (ISO 639-3, `#langepo`).
- Client ranking: complementary language exchange, locally rare shared language.
- `appengine.Main()` + memcache everywhere; Datastore TTL policies; purge cron; travel banner;
  UI locale from esperanto-kurso's locale files.

## Decisions taken without asking (Joop can overrule)
6-char cells only · export-QR for multi-device · `#langepo` + gazetteer · curated gazetteers ·
Liberapay now, Stripe later · discoverability opt-in · "Kafumu" everywhere · passkeys on from the
start (magic link re-binds them after a domain move).

## Notes
- Joop (2026-10-03): "be daring, corrections are cheap"; ping his phone only for urgent things.
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
