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
- [x] 7a. Meetups server side (`internal/meetup`): create (account; ≤10 upcoming per author), page
      /meetups/{id}, RSVP toggle (ids never shown, only a count), expire a day after the end, per-cell
      2-min cache + one `IN` query for missing cells, included in /bundle; auto-tag #websummit when inside
      the event; "Host a meetup" button on Around; account `?next=` flow. Indexes: `gcloud app deploy
      index.yaml` (the `datastore indexes` command needs the Firestore API, which is off).
- [x] 7b. Meetups section at the top of Around (ranked on the device: your persona tags + live event
      tags, then soonest; "Now" for running ones), tap → meetup page; host can delete. Browser test: A
      makes an account on the way to hosting, B sees it in Around, A deletes it. Test UA override in cdp.mjs
      (HeadlessChrome is a bot to us).
- [x] 7 (rest). `.ics`: /meetups/{id}/ics ("Add to calendar") and /cal/{cell} (cell + ring 1), RFC 5545
      folding, tested. Was: Meetups (fallback records, need account): "coffee at Pavilion 2, 15:00", side events, RSVP,
      tags `#websummit` + `lang:` + `tag:`; shown in the bundle; `.ics`.
- [x] 7b'. Import by link (`internal/importer`): paste a Luma/Meetup/any URL on /meetups/new → schema.org
      Event JSON-LD (incl. @graph, type arrays, geo → cell) prefills the form. Safe fetch: public IPs only
      (dial Control), 3 MB, 12 s, ≤4 redirects, 10-min cache, accounts only. Verified live on luma.com.
      Was: Import events by link: paste a Luma / Meetup / any event URL → read its schema.org Event JSON-LD
      (server fetch, cached) → title, time, venue, link; tag with cell + #websummit etc. Most Web Summit
      side events live on Luma, so this is high value before the event.
- [x] 7c. Calendar feeds (`internal/feeds`, curated `feeds.json`): JSON-LD pages incl. ItemLists (Luma
      city/calendar pages) and iCal (Luma calendars, Meetup groups; TZID via embedded tzdata). Cron every
      6 h (/cron/feeds, X-Appengine-Cron only) upserts upcoming 60 days as meetups "via luma.com" with
      stable ids, keeping Kafumu RSVPs. Seeded with luma.com/lisbon (19 events, 11 in central Lisbon).
      Add feeds: append to feeds.json (ics feeds without GEO need a "cell").
- [x] 8. Canned signals over the pair mailbox: ☕ Coffee? / 📍 I'm at… (short text) / 👋 Nice to meet you,
      sent from each contact on /contacts; received into the contact (last 10, unread flag); Around shows
      "From your contacts" (checks the 10 most recent contacts: ≤10 reads per open); opening Contacts
      marks them seen. Browser test: B taps Coffee?, A sees it in Around. Push notifications: later (M2).
- [x] 9. Be findable (opt-in, time-boxed ≤7 days, needs a username): Account → languages (curated ISO
      639-3 list incl. Esperanto, Toki Pona, sign languages; native/fluent/learning), interests, one line,
      "where to find me", cell. Bundle carries people as id-less Person views (cell+visible_until index,
      per-instance 60 s cache). Around "People here" ranked on the device: language exchange > speaks what
      you learn > learns what you speak > shared language weighted by local rarity > shared interests, with
      the reason shown. Strangers can't message. Bundle browser cache cut to 30 s (was 5 min: stale people).
      Was: Profile tags + discoverable people at the event (opt-in): languages, interests
      (opensource, esperanto, climate…), matched on the device.
- [x] 10. Friends around (`internal/slot`, pair.js checkIn/around): per pair and direction one random-keyed
      slot of ≤64 opaque tokens HMAC(pairKey, role, cell, UTC day) for the last week's check-ins (8-day
      TTL). Check-in only from a real GPS fix; a slot is rewritten only when the week's cell-days change.
      The friend fetches one slot per contact (≤30) and compares 3×3 cells × 7 days on the device: "Ana was
      here today / nearby on Tuesday". Server sees random ids and random-looking tokens. Tested in pair E2E.
- [x] 11. /patrons (Lichess-style: everything free forever; PayPal donate button live via KAFUMU_PAYPAL,
      Stripe/Liberapay slots via env; cosmetic thanks only), /for-cafes (relevance listings pitch, contact),
      /privacy (device vs server table with retention, never-have list, honest caveats). EN/PT/EO/NL,
      linked from the footer. Joop: a PayPal.me handle or hosted button would hide the address in app.yaml.
- [x] 12a. Service worker at /sw.js (pages network-first with offline fallback, versioned assets cache-first,
      API/bundles never cached) — verified: Contacts opens offline. Install bar (beforeinstallprompt; iOS
      "Share → Add to Home Screen" hint; dismissable). /badge: printable connect code for a badge or T-shirt,
      a 14-day "badge" invite (same box/URL as a normal invite); hellos via the badge are collected on any
      page with the device store. Pair E2E covers the badge.
- [x] 12b. Cost + load (2026-10-03). Per Around open, before: up to ~42 Datastore reads (≤10 box polls,
      ≤30 slots, account, badge) → 1k users × 10 opens ≈ 420k reads/day vs 50k free (≈ €0.25/day overage).
      Fix: App Engine bundled services (app_engine_apis, appengine.Main) + memcache in front of boxes (cache
      dropped on write) and slots (cache set on write): empty/unchanged polls cost no Datastore read.
      Load test on prod: 300 bundles @20 parallel p50 97 ms / p95 176 ms, 0 errors. Found: per-IP limits
      (120/240 per min) would block a conference behind venue NAT → raised to 6000/min/instance; per-box
      caps bound writes. Remaining write budget: slot rewrites ≈ contacts per user per new place-day (20k/day
      free ≈ 1k users × 20 contacts); if it bites, move to one slot per user (needs a card-exchange change).

## Second beachhead: language events (Joop is a HYPIA member, visits language events)
amikumu grew through Esperanto events. Aim to be the best tool at polyglot/Esperanto/language-café
events early, so it spreads locally by word of mouth. Means: language tags + people matching early,
event entries for language gatherings (only with verified dates/venues — never guessed), EO/PT/EN/NL UI.

- [x] 12c. Area picker (Joop's idea): no location request on load any more. First visit: "Where are
      you?" with search over the gazetteer (/places?q=, accent-insensitive, biggest first) and "📍 Use my
      location" (after a tap; remembered, so later visits locate silently). Choosing a place draws a 7×7
      grid of #geo blocks on OpenStreetMap tiles (static/area.js, attribution shown); tap a block to make it
      your area. "Change area" reopens it. Browser test covers search → map → tap.

## After the event (rest of M1)
- [x] `LaPingvino/geotags` (public, /home/joop/geotags): 6,278 cities ≥100k from GeoNames (CC BY 4.0) with
      tag, native + curated aliases, radius from population, centre cell, ambiguity (duplicates, ≤3 chars,
      curated noisy.json: #paris, #nice, #reading…); languages.json (≈75 language hashtags → 639-3, weight);
      gen/ regenerates. Kafumu vendors places.json as internal/gazetteer/places_geonames.json; its own
      places.json now only holds what GeoNames lacks (Barreiro, Parque das Nações, Wageningen…). Lookup
      searches outward from a cell over centre cells (memoised) instead of precomputing coverage.
- [x] ATproto events (read): feed kind "smokesignal" reads event links from smokesignal.events, resolves
      each DID via plc.directory, fetches the `community.lexicon.calendar.event` record from the author's
      own PDS, keeps in-person events with `location.geo` (→ cell), imports them "via smokesignal.events".
      First run: 17 such events. Address-only events need geocoding (later).
- [ ] ATproto OAuth so posts/events/RSVPs are written natively to the user's PDS (M2) — after kafumu.com
      (OAuth client metadata must live on the final domain).
- `LaPingvino/geotags` public repo: generator from GeoNames + language list, CC-BY; Kafumu vendors it.
- [x] Language gazetteer: geotags' languages.json vendored (internal/langs), ISO 639-1→3 map; Around boosts
      posts whose hashtags (#esperanto, #learnjapanese…) or ATproto langs match your languages — from your
      profile, else the browser's — entirely on the device (bundles stay per cell). Badge shows the tag.
      `#langepo`-style canonical public tags: not yet (no posts use them; revisit with ATproto posting).
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
- Meetup lists are cached per instance for 60 s: a new meetup can take up to a minute to appear for
  people served by another instance (the host's own instance forgets at once).
- Joop (2026-10-03): "be daring, corrections are cheap"; ping his phone only for urgent things.
- Datastore indexes: `~/google-cloud-sdk/bin/gcloud app deploy index.yaml --project lokumo`.
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
