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
- [ ] 3b. Passkeys (go-webauthn) — deliberately AFTER kafumu.com is mapped (passkeys bind to the
      domain; registering on appspot now would break on the move). Magic link is the recovery path.
- [x] 4. `static/device.js` (IndexedDB kv + contacts, storage.persist, link normalisation tested in
      `test/device_test.mjs`) and `/card`: name, one-liner, email/phone/WhatsApp/Signal/Telegram/Bluesky/
      LinkedIn/website, live preview, stored only on the device. No account needed.
- [x] 5a. Mailbox API (`internal/box`): `/api/box/{id}` GET/POST + `/ack`; 64-hex ids, ≤8 KB, ≤32
      msgs, 7-day TTL, one Datastore entity per box (one Get per poll), ack-to-delete, per-IP limit,
      unknown == empty. ~60 reads per pairing with backoff polling → memcache slice later.
      Ops for Joop: Datastore TTL policy on `Box.expires_at` (one-time gcloud command).
- [x] 4b. Libadwaita look (like digwire): own ~200-line CSS replaces Pico (one less CDN request);
      header bar, view switcher in the header on wide screens and at the bottom on phones, boxed lists,
      cards, pill buttons, coffee accent from the GNOME brown palette, dark mode, safe-area insets.
      Tabs now: Around / My card / Account / About — Connect + Contacts slot in with 5c/6.
- [x] 5b. `static/pair.js` handshake, E2E-tested in `test/pair_test.mjs` (A↔B, A↔C, C can't read B's
      hello, cards only as ciphertext, signals both ways). Known gap: anyone holding the QR can ack (delete)
      hellos in the invite box — a nuisance, not a leak; fix later by signing acks or per-hello boxes.
      Design (advisor-reviewed): QR = `origin/c#<A's ephemeral P-256
      public key + invite box id>`; scanner does ECDH with its own ephemeral key → HKDF → pair key, so
      every scanner gets its own key and can't read other scanners' hellos. Hello = scanner pubkey (clear)
      + AES-GCM(card). Box ids = HMAC(pairKey, label); direction in AES-GCM AAD. A's invite private key in
      IndexedDB ~1 h. Node E2E test of A↔B (and A↔B,C isolation) against the local Go server.
- [x] 5c. `/connect` (QR made on the device with vendored qrcode-generator 1.4.4, MIT header kept;
      "send as link"; new code; keeps listening for more scanners) and `/c#…` (first-timers give just a
      name, explicit "Connect and share my card", 3 retries, code removed from the URL, no-referrer,
      install hint). Backoff 2 s → 5 s → stop at 3 min. Tabs: Around / Connect / My card / Account, About
      in the header. `test/browser_test.mjs` drives two headless Chromiums over CDP: A shows, B scans.
- [x] 6. `/contacts` tab: everyone you've connected with (newest first, search, notes, remove, late
      cards fetched on open), "Save to address book" (.vcf, tested escaping) and backup/restore JSON
      (includes pair keys — warned). Contact rendering shared via device.js. Browser test covers it.
      Was: Contacts page: everyone you've scanned, notes, one-tap open of their links; export (vCard/JSON)
      — NON-NEGOTIABLE before the event: Safari evicts non-installed site data after 7 idle days, and an
      iOS home-screen PWA has separate storage from Safari (contacts don't carry over on install).
- [x] 6b. Personas: several cards (label only you see), tag chips (suggested + your own), `personas.share`
      hands over name + ticked fields only; per-share picker on /connect and /c (remembered); receiver's
      own private tags on each contact; vCard CATEGORIES; backup includes personas. Browser test checks
      an unticked field is withheld. `test/cdp.mjs` = reusable CDP driver.
      Was: Personas, mix and match (Joop): several cards ("Work", "Esperanto", "Friends") each with
      its own fields and one-liners; the one-liner becomes pickable chips + free text. On /connect, before
      or while showing the code, tick which persona/fields to hand over this time (the hello/reply carries
      only those). The receiver can add their own chips/tags about the contact ("met at WS", "robotics").
- [x] 6c. Share with self: Contacts → "This is my new device — show code"; the old device scans it and
      lands on /m#…, confirms, and sends its backup as encrypted chunks (≤30 × 5k chars) through the
      same ECDH handshake (kind "move", own box); the new device confirms before merging. Tested in
      pair_test (40 contacts, chunked) and the browser test (third browser receives).
      Was: Share with self: move everything (personas, contacts, notes, keys) to a new device by scanning
      a "transfer" code — same ECDH handshake, a different message type, one-shot. Also JSON export/import.
      Lower priority than 6 export, but it IS the fix for the iOS Safari→home-screen storage split.
- [ ] 7. Meetups (fallback records, need account): "coffee at Pavilion 2, 15:00", side events, RSVP,
      tags `#websummit` + `lang:` + `tag:`; shown in the bundle; `.ics`.
- [ ] 7b. Import events by link: paste a Luma / Meetup / any event URL → read its schema.org Event JSON-LD
      (server fetch, cached) → title, time, venue, link; tag with cell + #websummit etc. Most Web Summit
      side events live on Luma, so this is high value before the event.
- [ ] 7c. Subscribe to calendars: Luma calendar / Meetup group iCal feeds per cell (cron, cached).
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
- ATproto events: read `community.lexicon.calendar.event`/`.rsvp` (Smoke Signal) into bundles by
  location → cell; then ATproto OAuth so RSVPs/events are written natively to the user's PDS (M2).
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
- Don't trust `--dump-dom`/`--screenshot` for IndexedDB-driven UI (virtual time stalls IDB); use cdp.mjs.
- Headless check: `chromium --headless=new --dump-dom` works outside the sandbox (needs a socket);
  run the server on PORT=18080 with absolute paths (TMPDIR differs outside the sandbox).
- Joop (2026-10-03): "be daring, corrections are cheap"; ping his phone only for urgent things.
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
