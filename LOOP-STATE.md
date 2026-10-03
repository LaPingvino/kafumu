# Loop state

Each loop tick: read this file, take the first unchecked slice, build it (~15 min of work), run
tests, commit, push, deploy if user-visible and green, tick it off here with a one-line note,
and add anything learned to "Notes". Keep slices small; split a slice if it runs long.

Production: https://lokumo.ew.r.appspot.com · Repo: https://github.com/LaPingvino/kafumu

**Domain: LIVE (2026-10-03 ~18:30).** https://kafumu.com serves (managed certs OK after re-requesting them
— the first attempt failed with FAILED_RETRYING_NOT_VISIBLE before DNS had propagated). Switched on:
KAFUMU_ORIGIN=kafumu.com, legacy-origin move banner (verified moving a contact from appspot in prod),
passkeys (prod test passes; fixed a cross-instance stale-cache bug in passkey sign-in), ATproto OAuth
(Bluesky fetched our client metadata; consent page reached). Waiting on Joop: connect Bluesky once.
Old notes: Joop set up DNS; App Engine mappings for kafumu.com + www exist, managed certs
pending. Prepared: www → apex redirect (CanonicalHost), and a move flow from the old appspot origin
(banner → opens kafumu.com/import → origin-checked postMessage hands over the device backup and the
sign-in link). When https://kafumu.com serves: set in app.yaml
`KAFUMU_ORIGIN: "https://kafumu.com"` and
`KAFUMU_LEGACY_ORIGINS: "https://lokumo.ew.r.appspot.com https://lokumo.appspot.com"`,
`KAFUMU_PASSKEYS: "1"`, `KAFUMU_ATPROTO: "1"`; deploy; run browser + passkey tests against kafumu.com;
ask Joop to connect his Bluesky to test the OAuth callback.

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
- [x] 3b. Passkeys built, OFF until the domain switch (KAFUMU_PASSKEYS=1, RP ID = Origin's host, www
      allowed as origin): discoverable (resident key required — esperanto-kurso likely lacks this),
      WebAuthn sessions in memcache (5 min), passkey sign-in adds a session token (last 5 kept) so the
      magic link and other devices keep working. test/passkey_test.mjs: Chrome virtual authenticator.
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

- [x] 14. Web Push for signals (internal/push, webpush-go): VAPID keys generated server-side and stored in
      Datastore (never in the repo); /api/push/{key,subscribe,unsubscribe} (anonymous, credentials omit);
      a subscription lists only the device's own inbox ids (+ invite/badge boxes) and a language; a mailbox
      append notifies the subscriptions watching that box with a content-free, localized line; 404/410
      drops the subscription; PushSub expires after 90 days (purged). Contacts: "🔔 Notify me of signals".
      Arch Chromium has no push service (no GCM keys) → verify on a real phone.

- [x] 12d. Map panning (arrows + drag, a cell per block width) and search finds towns of 15k+ (geotags
      towns.json, search only — not place tags): Ede works.

## OLN layer (Joop, 2026-10-03): local messages with time-biased PoW, views, Aardvark questions
Design in VISION §4 "Local messages". Benchmark: pure-JS SHA-1 (static/sha1.js, verified against Node
crypto) ≈ 172k hashes/s here, assume 4× slower on phones: 14 bits ≈ 0.4 s, 18 ≈ 6 s, 20 ≈ 25 s.
- [x] 16. Admin (Joop): /admin/initial behind App Engine `login: admin` grants the "admin" role to the
      signed-in Kafumu account (his passkey); /admin: counts, hide OLN note, delete meetup, run feeds/purge
      now (and see the result), recent errors. Role checked server-side on every admin route.
- [x] 15a. Server: internal/oln wired: POST /api/oln (402 when the work is short), bundle `notes` (≤50
      by priority) + `requiredBits`, Note index, purge. Interop verified: a message mined by eolnpoc's own
      CreatePoWMessage validates (UTC). eolnpoc formats the date in LOCAL time → Joop: use time.Now().UTC().
      Was: Server: internal/oln — parse/verify eolnpoc raw format (SHA-1 leading zeros), ±10 min clock
      window, adaptive required bits per cell (14 + log2(1 + last-hour count/30), ≤22), TTL from bits,
      dedupe by hash, length cap, per-cell cache like meetups, ≤50 per cell in the bundle (by priority),
      admin hidden set; POST /api/oln; bundle field `notes` + `requiredBits`.
- [x] 15b. OLN first class (Joop): Around's main actions are "☕ Who's up for coffee?" (explains pairing;
      posts a local message carrying your connect code; others get a Join button) and "💬 Say something
      here" (no account; choose ~1 h / ~1 day / a week; cost estimate from this device's hashrate; mined in a
      Web Worker; retries with +1 bit on 402). "Here now" is the first feed section (⚡bits, time left, hide on
      the device). Bluesky is the secondary "or post on Bluesky". Report (PoW'd) still to do.
      Was: Client: mining Web Worker (sha1.js) with progress, composer in Around ("Say something here",
      no account), "Around here now" section ranked bits+recency, hide/report (report = PoW'd message).
- [x] 15c+18. Explore (was "Your area"): place (search/map/location) + language (full list) + interest;
      filters the bundle on the device (posts by hashtags or ATproto langs, meetups, notes, people); shareable
      URL ?cell=&lang=&tag=; "☆ Save this view" kept on the device as chips (long-press to remove), active
      filter chip with ×. Browser-tested. Still open: multi-ring bundles for big regions.
      + (Joop) biases, not just filters: strength slider (ignore / prefer a little / prefer strongly / only
      these, URL w=0..3); matches move up with the weight. Composer: language + interest tags go into the
      OLN keywords (prefilled from the current view).
      Was: Views: filter chips (language, interest) + URL params (?cell=&lang=&tag=) applied on the
      device; explore any place; multi-ring requests for big regions (holiday destinations).
- [x] 15d. Questions, Aardvark-style: "❓ Ask around" = an OLN note tagged #ask (+ language/interests,
      any place via Explore) carrying a connect code; "★ for you" when its tags match your card/profile tags
      or languages (device-side); answers: "🔒 Answer privately" (Join → connect) or "💬 Answer" = a note
      tagged #re<10-hex id>, threaded under the question. Swipe a message sideways to hide it (Joop).
      Browser test: ask → other browser answers publicly → threaded. Was: Questions (Aardvark): ask with interest tags into a place's cells; question carries an
      invite payload; matching devices see "Questions for you"; answer privately (pair.js) or publicly.
- [x] 15f. Minimal PoW on writes (internal/pow): mailbox POST and slot PUT need X-Kafumu-Work =
      "<nonce>;<UTC date>" with SHA-1("<nonce>;<date>;<b64url(sha256(body))>;#box<id>|#slot<id>") ≥ 10 bits
      (~1k hashes, ms on a phone), ±10 min; else 402. Size checked first. pair.js mines with sha1.js.
      Was: Minimal PoW on mailbox posts (Joop: "no PoW should be minimal PoW"): ~10 bits over
      nonce;date;base64url(sha256(body));#box<id>, ±10 min — instant for people, a cost for bots.
- [x] 15g-a. Per-peer price, server: public inbox on the account (box id + device-made ECDH public key +
      price 12–24 bits), POST /account/inbox, InboxPrice entity (box id → bits, 1-min cache) enforced on
      mailbox writes (CheckBits), Person in bundles carries {box, pub, bits}; deleting the account drops it.
- [x] 15g-b. Per-peer price, client (browser-tested): Be findable → "✉️ Let people who find me write to me"
      + price slider with a time hint; People shows ✉️ and "Write (≈ N of work)" — encrypted to their key,
      mined at their price in the worker, card optional; Contacts → "Messages to you" (decrypted on the
      device, Connect back = pair from the envelope); the inbox is in the push subscription.
      Also (Joop): OLN base 12 bits; difficulty follows the busier of last hour and last 10 min (bursts dear).
      Was: Per-peer price, client: open/close inbox + price slider (time hint) in Be findable; "✉️ Write"
      on People (encrypt to their key, mine at their price in the worker, optional card); Contacts "✉️ Messages
      to you" (decrypt on device, Connect back = pair from the envelope).
- [x] 15g. (done as 15g-a/b) Per-peer price (Joop): people set the minimum bits for unsolicited messages to them (questions,
      offers); commercial senders pay in work for what reaching you is worth to them.
- [x] 18b. Contacts: "Nearest" sort + "📍 seen 12 km on Tuesday" from friends-around hits stored per contact
      on the device (lastSeen {cell, day}); distance from your current area. Was: sort by distance from where they were last seen (friends-around hits give a day+cell
      per contact — device only).
- [x] 19. Short link code (internal/short): /api/short (min PoW) → 6 chars of the plus-code alphabet,
      ShortCode entity 1 h (purged), /j/{code} → /c#…; "Make a short code" on Connect; privacy caveat added;
      browser test follows the short code. Was: Short link code for connect codes (Joop): kafumu.com/j/XXXXXX → the /c#… invite, kept 1 h
      server-side (public key only; mention on /privacy). The full link is already shown under the QR.
- [ ] 20. Profile language list = holywritings.net's 83 languages (from /home/joop/holywritings.db
      `writings.language`: en, pt, fa, de, ja, nl, … gil, ch, nai-US/CA, haw, fo, ga) mapped to ISO 639-3
      with native names, plus the conlangs and sign languages already listed. (UI translations stay
      EN/PT/EO/NL unless Joop wants more.)
- [x] 21. Account-bound move (Joop, replaces the QR move — code substitution could leak everything): new
      device signed in (link/passkey) registers a move request {box, pub} on the account; the old device,
      signed into the same account, sees "Your other device wants your data" + a short matching code on both
      screens → sends E2E-encrypted (server can't read). Requests expire in 15 min. Built: /account/move
      (GET/POST/DELETE, cache), static/move.js, browser-tested (C signs in with B's link, emoji match).
- [ ] 17. Card themes from local activity (Joop): count tags seen in Around bundles (posts, meetups,
      people, notes) on the device, per area; the card editor suggests the local ones first.
- [ ] 15e. /oln.json per cell in eolnpoc's olnjson.Format for other OLN nodes.

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
      First run: 17 such events. Address-only events are geocoded via Nominatim (≤1 req/s, identifying UA,
      per-instance cache, ≤15 per sync): +6 placed on the first run.
- [x] 13a. ATproto OAuth (internal/atp on indigo's atproto/auth/oauth; OFF until KAFUMU_ATPROTO=1 at the
      domain switch): public client, metadata at /oauth/client-metadata.json, granular scopes (posts,
      calendar events, RSVPs only), Datastore session store (90-day sessions, 15-min auth requests, purged
      daily), connect/disconnect on Account, "Continue with Bluesky" creates a Kafumu account; deleting the
      account revokes the session. Verified live up to Bluesky's consent page (PAR + DPoP nonce OK).
- [x] 13b. Writes to the PDS when connected: hosted meetups → community.lexicon.calendar.event (cell centre
      as coarse geo + venue name, link back), uri+cid kept on the meetup; "I'm going" → .rsvp for any event
      with uri+cid (ours, and imported Smoke Signal ones — feeds now keep uri+cid); Around's "Post here"
      opens a composer → app.bsky.feed.post with hashtag facets (byte offsets tested) and #geo added.
      Untested against a real PDS until Joop connects after the switch.
- [x] Language gazetteer: geotags' languages.json vendored (internal/langs), ISO 639-1→3 map; Around boosts
      posts whose hashtags (#esperanto, #learnjapanese…) or ATproto langs match your languages — from your
      profile, else the browser's — entirely on the device (bundles stay per cell). Badge shows the tag.
      `#langepo`-style canonical public tags: not yet (no posts use them; revisit with ATproto posting).
- (done: memcache, purge cron, travel banner, own locale files; TTL policies replaced by the purge cron)
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
- Cron: "every N hours" without `synchronized` counts from deploy time; now clock-aligned (feeds 00/06/
  12/18 UTC, purge 04:00). Verify the first purge run in the logs — /privacy depends on it.
- Joop (2026-10-03): "be daring, corrections are cheap"; ping his phone only for urgent things.
- Datastore indexes: `~/google-cloud-sdk/bin/gcloud app deploy index.yaml --project lokumo`.
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
