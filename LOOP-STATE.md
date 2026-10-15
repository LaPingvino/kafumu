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
- [x] 20. Profile language list: + the holywritings.net languages (afr, amh, aze, … haw, ipk, kal, mah, tpi,
      srn, gil, cha, nai…), with native names and 639-1 mappings; test lists all 82 codes. Was: Profile language list = holywritings.net's 83 languages (from /home/joop/holywritings.db
      `writings.language`: en, pt, fa, de, ja, nl, … gil, ch, nai-US/CA, haw, fo, ga) mapped to ISO 639-3
      with native names, plus the conlangs and sign languages already listed. (UI translations stay
      EN/PT/EO/NL unless Joop wants more.)
- [x] 21. Account-bound move (Joop, replaces the QR move — code substitution could leak everything): new
      device signed in (link/passkey) registers a move request {box, pub} on the account; the old device,
      signed into the same account, sees "Your other device wants your data" + a short matching code on both
      screens → sends E2E-encrypted (server can't read). Requests expire in 15 min. Built: /account/move
      (GET/POST/DELETE, cache), static/move.js, browser-tested (C signs in with B's link, emoji match).
- [x] 17. Card themes from local activity: Around keeps a decaying tag tally on the device (posts, meetups,
      notes, people; not geo/lang/place/system tags); the card editor leads with 📍 local themes. Browser-tested.
      Was: Card themes from local activity (Joop): count tags seen in Around bundles (posts, meetups,
      people, notes) on the device, per area; the card editor suggests the local ones first.
- [x] 15e. /oln.json?cell=… (cell + ring 1) in eolnpoc's olnjson.Format: server info, messages (raw, ttl in
      days, tags), tag index, push = /api/oln; CORS open, 60 s cache. Was: /oln.json per cell in eolnpoc's olnjson.Format for other OLN nodes.

## Joop's phone feedback (2026-10-04)

- [x] 21. OLN messages shown twice: overlapping Around loads both appended; only the newest load renders now.
- [x] 22. Dates were US-style: all dates follow the UI language with en-GB for English (day month, 24 h).
- [x] 23. Connect without data: scanning/showing a code never waits for a name; an empty card is fine; you
      can name a nameless contact yourself (alias, device-only); your card follows later ("Send my card",
      or automatically when you fill in the optional name on the connect page). Browser-tested with D.

- [x] 24. Polish: Connect shows the QR (and the scan side its Connect button) first; the optional name
      form sits below it ("Add to my card"). 

- [x] 25. Polish: Around's heading names the place ("Barreiro", "Areeiro, Lisbon") via Gazetteer.Nearest
      (towns 15k+ and cities within 20 km; city added when within 8 km), the #geo tag as a caption.

- [x] 26. Polish: Around's first screen: coffee/say/ask stay big; change area, share and host a meetup
      move to a smaller outlined row, so the content starts higher. "suggested" buttons (coffee, Connect,
      device move, print badge) now actually get the accent colour; only links and submits did.

- [x] 27. Polish: meetup page: repeated address parts dropped ("Lisboa, Portugal"), "Open on luma.com ↗"
      instead of the raw URL, "N going on Kafumu" hidden at 0 for imported events, a ← Around link.

- [x] 28. No defaults tied to Joop (Joop): phone examples use the area's country code (Around remembers
      near.country; else browser region; else "+…"), examples neutral (no Esperanto outside eo), suggested
      tags add the browser's languages instead of a fixed "Esperanto".
- [x] 29. Bluesky posts as real cards (Joop): letter avatar (no CDN images: privacy), name + @handle (✓ own
      domain), age · ♥ 🔁 💬 · "on Bluesky since…" / 🆕 new account, via/lang/bot badges, ⚠ moderation labels
      (adult ones hidden behind "Show anyway"), Reply on Bluesky ↗, Hide + swipe like local messages.

- [x] 30. (30a+30b+30c done in one go) One feed: sinks per source → drawFeed merges by score (contacts
      1000, live meetup 12, local messages 10…, meetups 8+tags−days, people 4…, posts 3+score); chips: All /
      tap a kind = only that kind / further taps add-remove kinds; remembered on the device; kind label on
      each card when mixed. Was: One weighted feed (Joop): Here now (local messages), Meetups, Around here (Bluesky) and nearby
      coffee/contacts (high priority) become cards in ONE list, ranked by one device-side score (kind weight
      × freshness × distance ring × your tags/languages/views × reliability). Filter chips per kind, and a
      mode switch: Mixed (default) or one kind at a time; the choice is remembered on the device. Split:
      30a. shared card shell + score; 30b. merge the lists + chips/modes; 30c. contacts-around cards on top.
**Order (Joop, 2026-10-04, later): 42 (card fields), then 41 (how this was built), then the rest; business
(53, 46, 45) last; translation only as idle filler. Nothing on the site may promise what doesn't exist.**

- [x] 38a. Admin: 13 stat tiles (accounts, named, active 24h/7d/30d, findable, synced, meetups + from feeds,
      local messages, mailboxes, push, Bluesky linked) via aggregation counts; accounts list (named, search by
      prefix or id) with role (user/host/moderator/admin), rename, release name, keep forever, delete (+vault).
      Render-tested. 38b (report tool + queue) next.
- [x] 38b. Report tool: ⚑ on local messages, Bluesky posts, people and meetups → reason chips → stamped
      POST /api/report (internal/report: 30-day Report, keyed by stamp); /admin queue (most reported first)
      with Hide (note hide / meetup delete / 90-day Hidden set for posts and people, filtered from bundles)
      and Dismiss; moderators see /admin with only the queue. Admin: counts log errors and fall back to a
      keys-only count; accounts grouped per area (details), "accounts by area" line. Browser tests delete
      their accounts afterwards. Was: 38. Admin worth opening (Joop: "doesn't show a lot yet, not even stats"), like esperanto-kurso.net's:
      stats (accounts named/anon, active 1/7/30 d, passkeys, ATproto-linked, findable people, inboxes,
      meetups by source, notes by cell, vaults), a named-accounts list with search; per account: rename,
      roles (admin / moderator / trusted host), reset keep-days, delete; moderation: hide a meetup or note.
      Report tool (Joop): a "Report" action on every card (local message, meetup, person, Bluesky post):
      reason chips (spam, scam, harassment, wrong place, other), PoW-paid like everything else (no account
      needed), stored as a short-TTL Report entity keyed to the item; the admin gets a queue (newest, most
      reported first) with hide / dismiss / delete; items with several reports fold on the device meanwhile.
      Top-bar ⚙ for admins: done.

- [x] 31. Gazetteer.CountPlaces (all city tags + aliases) → post.placeTags in the bundle; device score: ≥5 city
      tags −8 with a "tags N cities" badge, new account −1, labels −3, engagement up to +1. Was: Reliability: Bluesky posts tagging many places (≥5 city tags: "#London #Paris #Berlin…") rank
      far down with a "tags N cities" badge; new accounts and labelled posts weigh less in the score.

- [x] 32. Naming cloud: villages.tsv.gz (geotags/villages, GeoNames cities1000: 171k places incl. neighbourhoods,
      2.4 MB gz, loaded lazily, indexed by cell); heading = town/village with least (km+0.3)/√(pop/1000), the city
      added when a district; "also" cloud of neighbourhoods ≤3 km and villages ≤8 km. Lunteren → Lunteren.
      Was: Naming cloud (Joop: Lunteren shows as Bennekom): towns < 15k are missing, so the nearest-name
      picks a neighbour. Add villages/neighbourhoods (geonames cities1000 / PPLX for the heading only, via
      geotags), and show a cloud: the main name big, nearby names (villages, districts) small beside it.
- [x] 33. First visit without a location (city-name fallback via X-Appengine-City + gazetteer search; admin
      shows your connection's App Engine geo headers; Claude's own connection reads "DE / ? / ? / 0,0", i.e.
      no city: the guess only shows when App Engine can place a connection): X-Appengine-CityLatLong → cell (page only, Cache-Control: private,
      never stored) + nearest name; card "Are you in or near Ede?" Yes / Use my location / Somewhere else.
      Browser-tested locally with the header; in production it only checks a question appears. Was: Desktop first visit (Joop): guess the area from App Engine's X-Appengine-CityLatLong header
      (free, no lookup service, never stored), turn it into a cell and ask "Are you in Ede?" [Yes] [Pick].

- [x] 34a. Say placeholder fits the moment (Joop: "near the main stage" was a poor default): at a running
      event / in the place / around here × morning coffee, lunch, afternoon break, evening drink. About:
      "Ni kafumu" = let's have a coffee together, with esperanto-kurso.net.
- [x] 34a. UI languages batch 1: es, de, fr, it (444 keys each, placeholders/HTML verified, du/tu forms;
      notes for a native-speaker pass: à/en/au before places (fr), gendered forms, "{from}–{to}" as dates).
- [x] 34b. UI languages batch 2: pl, ru, uk, tr (plurals as "label: {n}", case-free phrasing around place
      names). 12 UI languages now. Next batches: cs sv da ro hu el · ja ko zh id vi · ar fa he ur (RTL) · hi bn sw tok.
- [x] 34c. UI languages batch 3: cs, sv, da, ro, hu, el. 18 UI languages now. Left: ja ko zh id vi · ar fa he
      ur (RTL) · hi bn sw tok.
- [x] 34d. UI languages batch 4: ja, ko, zh, id, vi. 23 UI languages. Left: ar fa he ur (RTL) · hi bn sw tok.
- [~] 34. (Closed by Joop: 24 UI languages is enough; RTL etc. only on request.) More UI languages (Joop: "4 is too few"): the 32 locales of esperanto-kurso.net (ar bn cs da de
      el en eo es fa fr he hi hu id it ja ko nl pl pt ro ru sv sw tok tr uk ur vi zh), translating Kafumu's
      strings; RTL (ar fa he ur) via dir="rtl". Plus a gentle esperanto-kurso.net nudge where it fits (the
      language picker, Esperanto users). Split per batch of languages.
- [x] 35. 🗣 Learn <language> on Around when the area's country (near.country → COUNTRY_LANG) has a main
      language you don't speak: card with "Language events and people" (view lang=<code>, w=2: matching
      meetups/posts/people first, exchange partners top), "Up for a chat?" (coffee-style local message
      #lang<code> #learn #coffee with your connect link; postCoffee shared with the coffee button), and
      "Add it to my profile". All 12 UI languages. Browser-tested (Lisbon, English UI → Portuguese).
      Was: "I want to learn the local language" button (Joop): one tap marks you as learning the area's main
      language (from the country), surfaces language events/meetups and people who speak it (exchange
      matches first), and offers a coffee signal "learner looking to chat" (#lang<code> + #learn).

- [x] 36a. Events found from the bundle (foundEvents: a tag on 3+ upcoming meetups in 14 days or 5+ local
      messages; no places, languages or Kafumu's own tags; top 3) shown as event cards "#tag is happening around
      here (N)"; every event card is local-first: "💬 Say something with #tag" (composer with the tag), "Also on
      Bluesky" second. Unit-tested. Named link moved to My card, per persona ("Use this card for my link"; people
      connecting through /@name get that card). 36b (composer tag suggestions) and the business ad wait.
- [x] 36. (36a found events + local-first banner earlier; 36b now) Composer tag suggestions: "Used around here:"
      chips under the tags field — events around now first, then the device's local tag tally; tap adds, chips
      for tags already entered drop out. Browser test: B saw the #food question → suggested → tapped → added.
      Was: Events found automatically + local-first tags (Joop): detect big events from what the feeds
      already bring (many Luma/Smoke Signal events or one large one in a cell, a burst of one tag in local
      messages and posts) instead of hand-made events.json; suggest the tags that make sense here in the
      composer; the event banner posts LOCALLY (OLN) first, "also on Bluesky" second.
- [x] 37. (37a+37b) Full sync: internal/vault (one encrypted blob per account, versioned, If-Match/409,
      900 KB cap, deleted with the account and by the idle purge); device.js pulls/merges/pushes (contacts per
      id newer-wins via updatedAt, deletions final via 30-day tombstones, personas+share choice as one); key
      made on the first device, carried by the account-bound move/backup; Contacts shows "synced" or "get
      them here once". Browser-tested (B→C move, note B→C, removal C→B). Open: 37c PRF (passkey-derived key,
      no second device needed); residual race: device A acks a message and goes offline before pushing.
      Was: FIRST. Full sync: cards/personas AND contacts across your passkey devices (Joop), desktop included. Design: the device key
      comes from the passkey PRF extension (no key on the server); contacts + personas encrypted into one
      opaque blob per account, versioned, last-writer-wins per contact. Joop's caveat: messaging gets
      convoluted, because two devices share a pair's mailbox and one device's ack hides a message from the
      other. Fix: acks become per-device read marks (mailbox keeps messages until TTL); signals dedupe by id.
      Fallback without PRF: the existing account-bound move.

- [x] 38c. Admin bulk actions: checkboxes + "select all shown", apply delete / keep 1 day / keep forever /
      normal retention; searches list up to 300; purge candidates are accounts idle ≥1 day (keep rules decide).
- [x] 37d. Sync fixes (Joop: cards didn't sync): personas merge per id (own updatedAt, tombstones "p:<id>"),
      cards from before sync still come across; moves/backups use the same merge; a bar on every page when
      this device needs the key ("Get them" → /contacts#get starts it) or another device asks ("Send");
      "Sync now" on Contacts and My card; browser test: B's card on C after the move, rename B → C.
- [x] 40. Queued connects (Joop: "I already missed two possible contacts"): hellos wait in the mailbox (7 days)
      but were only taken in on Connect; now Around and Contacts take them in too (invite + badge) and answer
      with your card. Browser-tested: D connects while A is on another page; A finds D on opening Contacts.
- [x] 40b. Replaced codes kept (Joop: friends' link accepts still missing): a code renewed after an hour
      (or "New code", or coffee) overwrote the old one with its private key, so later uses of an old link
      were unreadable. Now replaced codes (up to 20, kept a week) are checked too. Pair E2E test added.
      Known gap: codes are per device (not in the vault); a link shared from the phone arrives on the phone.
- [x] 47. Chat with contacts (Joop: "no chat option"): 💬 Chat per contact, end-to-end encrypted ({t:"msg"} over
      the pair mailbox), kept in the contact (last 200; synced to your devices), unread count, polls while open.
      Duplicates (same name): the newer offers "Remove this one"; removed contacts are never re-created by a
      late hello (tombstone check in checkInvite). Browser-tested both ways.
- [x] 48. Chat over encrypted OLN: a private OLN message = no #geo, one keyword #p<32 hex = hmac(pair key,
      "chat"+recipient role), text sealed with the pair key; base work, 7-day life, indexed by Note.Pair, read via
      GET /api/oln/pair/{tag} (never in area lists or oln.json); Contacts sends chat lines that way (mined in the
      worker) and reads them on load and while a thread is open; cards/signals/alive stay on the mailbox. MaxRaw
      4000, chat lines ≤ 500 chars. Browser chat test passes over OLN. Was: Chat over encrypted OLN (Joop: "best to use encrypted OLN for the messages"): send chat messages as OLN
      messages whose text is the pair-encrypted ciphertext and whose only keyword is an unguessable pair tag
      (#p<hmac(pairkey, "chat"|day)>, no #geo); the server indexes notes by that tag too (GET /api/oln?tag=…),
      normal PoW and TTL; federatable via other OLN nodes. Contact record/UI unchanged; mailbox stays for hellos.
- [x] 41. /built "How this was built" (footer + About): honestly vibecoded (Claude writes most code, tests and
      translations; Joop decides and tests), plans long before AI (olc-tools since 2019: #geo + whenwhere; OLN
      whitepaper + eolnpoc; lokumo; esperanto-kurso.net, holywritings.net), why (one person, no burnout), what it
      means (machine translations, privacy by design + open code, tested). In Joop's voice: Joop to correct.
      Was: "How this was built" page (Joop): honest note that the code is written with LLM help ("vibecoded"),
      built on plans and experience from long before LLMs (whenwhere, OLN, lokumo, amikumu…), and that it
      would not be viable for one person without that help (burn-out). Linked from About and the footer.
- [x] 42. Card fields: Instagram, Facebook, Mastodon (@you@server → https://server/@you), TikTok, YouTube with
      one-tap links; your own fields (label + value: links and emails tappable, the rest shown as text, no
      javascript:), pickable per share as "custom". Unit-tested. Was: More card fields (Joop): Instagram, Facebook (and Mastodon/TikTok/website…) as first-class fields
      with one-tap links, plus your own fields (label + value or link), all pickable per share like the rest.
- [x] 43. React to anything: every card (local message, Bluesky post, meetup, person) has 👍 ❤️ 😂 ☕ and
      💬 React; reactions are OLN messages tagged #re<10 hex> (a note's id, or sha1(kind:id) for the rest);
      emoji-only ones show as counts, text ones threaded under the card (attached on every feed draw).
      Browser-tested (👍 1 under a message). Was: React to anything (Joop, OLN): every card (local message, meetup, person, Bluesky post) gets a
      reply/react action that posts a local message tagged #re<id> (or #re<hash of the uri/id> for posts,
      meetups, people), threaded under it like answers; quick emoji reactions as tiny PoW messages.
- [x] 44. /oln: what OLN is (place as #geo text, PoW instead of accounts, TTL doubling per bit, busy areas ask
      more), eolnpoc and format compatibility, the line format, GET /oln.json, POST /api/oln (+ new GET
      /api/oln/required?cell=), a JS mining snippet (verified end to end against a local server), GET /api/asks,
      running your own node. Title/lead in all 23 languages, body in English. Linked from About. Was: /oln page (Joop): what the Open Location Network is (local messages as text + #geo tags, proof of work
      instead of accounts, time-biased validity), the eolnpoc proof of concept (link, how its format works),
      and how to integrate: GET /oln.json?cell=… (olnjson.Format), POST /api/oln (raw nonce;date;b64;keywords,
      SHA-1 leading zeros, current required bits from the bundle), with a curl/JS example. Linked from About.
- [x] 62. Language levels: CEFR (native, C2…A1; old fluent→C1, learning→A2 on display), learner = A1–B1;
      add any language by code ("pt-br", "tlh") or by name ("x:Ladino"), names kept as typed (cleanLangs: codes
      lower, levels upper). Caught by the browser test: the old cleaner lowercased "A2" and dropped the language.
      Was: Language levels (Joop): CEFR-style levels (A1–C2 + native) instead of fluent/learning, and hand-added
      languages: by code, or free text without one (rare languages); matching treats levels sensibly.
- [x] 65. Footer "Contact the maker" (Joop): @maker on Kafumu (only while that account's /@name link is live,
      checked ≤ every 5 min per instance) and a contact link (default GitHub issues); maker, link and link text
      editable in /admin (Config/footer), env as defaults.
- [x] 63. test/fuzz.mjs: seeded PRNG, 6 people coming/going: show/renew codes, use current or old codes, chat
      over encrypted OLN (real Argon2 mining), take in; invariants after everyone catches up: each connection on
      both sides with one key, every chat line exactly once (matched by key). In run.sh with a random seed (80
      steps; a failure prints the seed). Found: mutual scans make two connections between the same people → 66.
      Was: Fuzz the network (Joop): many simulated devices/accounts going online and offline at random (pairing,
      sync, chat over OLN, named links, moves), with invariants checked (no lost contacts or messages, no
      resurrections, keys never on the server); a Node harness against a local server, seeded and repeatable.
- [x] 66. One person, one contact: a random person id ("me", synced, oldest wins) in hello/card/alive (encrypted);
      a second connection with the same pid folds into the older contact (altKeys kept for reading mailboxes and
      chat; messages, signals, notes, tags merged; the other removed with a tombstone); existing duplicates fold
      via the weekly alive. Fuzz now demands exactly one contact per person and equal key sets: 8 seeds incl.
      3 mutual pass. Was: One person, one contact (found by the fuzzer; Joop saw it as a duplicate): when two people use each
      other's code before either takes the other's hello in, they get two connections. Recognise the same person
      (a random person id in hello/card, synced via the vault) and fold the second connection into the first
      (keep both keys for reading; send on the older one).
- [x] 67. Offline outbox (Joop: "check if adding by link works when the app is offline… queue"): pair.js queues
      posts that fail for lack of network (not server refusals) — hellos (contact marked "⏳ waiting to send"),
      cards/signals/alive, chat lines (re-mined at send time); flush() on page open and on "online". The service
      worker precaches /c and /contacts with their assets, so a connect link opens offline. Fuzz: networks drop at
      random, 1–5 actions per run happen offline, outboxes must end empty, nothing lost or doubled.
- [x] 70. Long chat lines were refused (found while sizing 69): a sealed line is base64 ciphertext, so beyond ~330
      characters it broke the 500-character text limit meant for readable text. Pair notes now have MaxPairText
      (3000 bytes: 500 characters of any script); public notes keep 500 code points. pair.js cuts at whole code
      points (never half an emoji). Fuzz sends lines up to the limit in 1–4-byte scripts (old server: 400).
- [x] 71. Acting as a business (Joop: "don't see how to switch… under the username button… switch after creating
      it"; "nudges say thanks…"; "the previous way was very subtle, you can't get people to pay for that"): cookie
      kafumu_as, checked per page against the business (1-minute cache) and that you manage it; the username button
      shows "🏢 Name" as a badge; /account has "Use Kafumu as" (you / each business); creating one switches to it;
      meetups/new preselects it; the nudge bar thanks you instead. Browser test: switched, preselected, back.
- [x] 72. Business wings (#i-wings, Lichess-patron style): gold while the business is live, sad grey when not —
      on the username button while acting, the composer ("Post as 🪽 Café X"), meetups it hosts, and OLN posts made
      as it (Note.Biz/BizLive set at post time via BizFor; the manager goes to Note.By, json:"-", Author stays
      empty: "the business speaks, not the person"; ranks/lives as named; oln.json shows the business name).
      Was: Business swag (Joop: "Lichess-style icon… even when you don't organize anything… sad grey when the trial is
      over or they didn't pay"): a business badge (patron-wings-like icon) next to the business's name wherever it
      shows (meetups, named OLN posts as the business, the profile); coloured while Live, grey when not.
- [x] 75. The business runs the thing (Joop: "switching to the business account should fully make it feel like the
      business running the thing and keeping the personal part out of it"):
      [x] a. Done: Business.Username claimed in the people's registry as "biz:<id>" (one namespace; "add manager"
         refuses business names); set on /business; /@name falls back to the business page (wings, kind, contact;
         single cached reads, no query). Browser test: set, visitor sees it. Meetups list on it: later (needs an
         index by business). Was: A business username (/@cafe-x; managers set it; same namespace as people, so no clash), and its public
         page (name, kind, contact, upcoming meetups, wings).
      [x] b. Done: a device store per identity (IndexedDB "kafumu-biz-<id>" from body data-acting-id), so card,
         personas, codes, contacts, outbox are the business's while acting; personal vault sync, device moves and
         the personal /@name link are off then; a "working as" banner on Card/Connect/Contacts says business data
         stays on this device until 75c. Caveat: the offline-precached /c and /contacts carry the identity of the
         moment they were cached. Browser test: separate card, own card back. Was: While acting: the Card page edits the business's card; Connect shares it; new contacts land in the
         business's contacts, not yours (and vice versa).
      c. DECIDED (Joop 2026-10-05: "enable both, company admin configures"): per business, managers choose
         SyncMode: "server" (server holds the business key, gives it to managers' devices: simplest) or
         "private" (key only on managers' devices; a new manager's device requests it, an existing one wraps
         it for them after both show the same short code; server sees public keys + ciphertext only). Both use
         a business vault (versioned blob like the personal one, managers only). 75c-1: vault + server mode +
         the setting + device sync while acting; 75c-2: private mode key handover.
      [x] 75c-1. Business vault (/api/business/{id}/vault, managers only, 404 when sync is off; shares the
         vault store as "biz:<id>"), server mode (Business.SyncKey; /api/business/{id}/key for managers; off →
         server forgets it; switching to server can take a device's key, else starts fresh), the setting on
         /business, device.js syncs the acting business's own store (key from the server), banner says
         "in sync". Tests: access control (Go), second device of a manager gets the business card (browser).
      [x] 75c-2. Private mode: Business.KeyReqs per DEVICE (pub), ≤ 5 open per user, a week's life, dropped
         when no longer a manager; device asks with a one-off P-256 key; a key-holding device sees the request
         with the 6-digit code (SHA-256 of the pub) and seals the key to it (ECDH + HKDF + AES-GCM); server sees
         pubs + ciphertext. /business shows "needs key, code …" / "@x asks, code … [Send key]"; bars elsewhere.
         Private→server hands the device's key over. Fixed on the way: /business didn't load device.js.
         Browser test: third device asks, first sees the same code, sends, third gets the business card.
      [x] 75c. Business contacts synced between managers: both modes (Joop: "enable both").
      (was) c. Business contacts synced across its managers ("company accounts could use a bit more server side help"):
         a business vault (encrypted like yours, with a business key); the key reaches a newly added manager
         through an existing manager's device (wrapped for the new manager when they first open /business, after
         the existing one approves) — the server stores only ciphertext. Design to confirm with Joop.
- [x] 73. Paid until: Business.PaidUntil (start of the day after, UTC) + AfterPaid stop|stay; Live() honours it
      (wings go grey after it unless "stay"); PaidOver() flags admin follow-up ("paid period over (still live):
      contact?"); admin sets date + after; /business shows "Paid until …" or "paid period over". PaidDay() does
      the date maths in UTC (templates did it in local time). Was: Paid until (Joop: "when putting on active, add a 'paid until' date, and configure active beyond or
      inactive beyond that date"): admin sets Status active + PaidUntil + AfterPaid (stay active / go inactive);
      Live() honours it; the business page shows "paid until"; admin follow-up lists those past PaidUntil.
- [x] 74. Admin OLN tools: counts expired + hidden ("stored but not live") and a purge for them (expired and
      hidden notes, plus all hidden markers); a list of live public messages, newest first (300 max; area, work,
      author or 🪽 business with the manager, times), Hide / Delete per message. Same single read as the stats.
      Emulator test (run.sh): stats, purge, markers gone. Was: Admin OLN tools, complete (Joop: "doesn't enable me to remove stored but not live, or to see all live
      messages"): list all live messages (paged, newest first, with cell, bits, author, expiry, hide/delete), and a
      purge of stored-but-expired/hidden notes now (not waiting for the TTL policy), with counts.
- [x] 69a. Protocol done (pair.js): replyKey(text, life) → "#rka<33 hex> #rkb<33 hex>" (compressed P-256, BigInt
      decompression), kept in kv "replyKeys" until life + a week; answer(note, text, mine) posts b64(eph).seal on
      #p<sha256("kafumu answer v1|"+rk)>; readAnswers() opens new ones into kv "answers" threads (not contacts)
      and reads follow-ups with readChat (saveThing → threads). pair_test: post, answer, read, reply, no dupes,
      keyless post refuses. Next 69b: UI (composer adds the key; "Answer privately" on posts with one; answers
      under your posts; hide #rka/#rkb from tag lists; poll on Around).
- [x] 69b. UI: every top-level post carries a reply key; others' posts get "🔒 Answer privately" (inline form;
      added only after the own-keys set loaded, so never on your own post); "🔒 Private answers" above the feed
      lists threads with the post they're about, last lines, a reply box; polled on open, every minute and on
      return to the tab; #rka/#rkb hidden from tag lists. Browser test: B answers A's post, A replies, B reads.
- [x] 69. Private answers to anonymous posts (Joop: "OLN should also enable privately answering anonymous
      posts"): posting, the device makes a reply key per post (P-256) and adds its public half as a keyword
      (#rk<b64>; format-level, any OLN client can use it). Answering: encrypt to it (ephemeral ECDH), post as a
      pair note under #p<hash of the reply key> (no place, base work, a week), fetched with credentials omitted.
      The poster's device keeps the reply keys (vault-synced, expire with the post), polls those tags, shows
      answers on the post ("2 private answers"), can answer back on the same key, and can turn one into a
      contact (send a card). The server sees opaque lines only; neither side learns who the other is unless
      they choose to.
- [~] 64a. Checked running outside GAE: the binary runs fine (memory cache, env config), but the Datastore
      emulator rejects IN queries ("Filter has 9 properties, expected 1"), which every area list uses — so the
      emulator is no self-hosting path. Export now logs store errors. Next: 64b, a SQLite store. Datastore is used
      directly (not via an interface) in account, box, meetup, slot, oln, vault, handle, business, report, short,
      push, atp, purge, admin stats, prices: 64b introduces store interfaces where missing and a SQLite
      implementation (KAFUMU_SQLITE=path), core first (oln, meetups, accounts, boxes, slots, vault), then the rest.
- [~] 64b-1. internal/sqlstore (modernc.org/sqlite, pure Go): Open(path) with schema; Notes = oln.Store on SQLite
      (columns for cell/pair/asks/expiry, JSON for the rest), service-level test incl. reopen; main picks it with
      KAFUMU_SQLITE. Next stores: meetups, accounts (+usernames), boxes, slots, vault, then handle/business/report/
      short/push/atp/prices, purge, admin stats.
- [~] 64b-2. Meetups on SQLite (sqlstore.Meetups: cell/author/expiry columns, full-field JSON via a tag-only
      mirror struct so a new Meetup field fails to compile rather than vanish; RSVP Update in a transaction);
      tested through meetup.Service incl. reopen; wired in stores() under KAFUMU_SQLITE. Next: accounts.
- [x] 68. OLN pricing by arrival (Joop: pre-mine for a moment a day ahead; "offset a little"): Note.Recv (node's
      receive time) drives the area price, not the claimed time (which can sit ±10 min off and dropped out of the
      10-minute count in seconds: offset-trickle got 30/30 through at base, now ~half); a future stamp buys no
      extra life. Tests: TestMassRelease, TestOffsetTrickle (fails on the old counting).
- [~] 64b-3. Accounts on SQLite (sqlstore.Accounts: users JSON + cell/visible_until/last_seen columns; usernames
      table shared with businesses "biz:<id>", claim = insert-or-nothing then check owner); tested through
      account.Service incl. reopen; live: an account made on a SQLite server is still signed in after a restart.
      Next: boxes + slots, vault, then handle/business/report/short/push/atp/prices, purge, admin stats.
- [~] 64b-4. Mailboxes + slots on SQLite (box_msgs rows, limit counted in a transaction, week's life by time;
      slots with expiry; Purge* for the self-hosted purge). Tested incl. reopen; live: the full pairing E2E and a
      fuzz run against a SQLite-only server pass. Next: vault, then handle/business/report/short/push/atp/prices,
      a self-hosted purge loop, admin stats.
- [~] 64b-5. Sync vault on SQLite (optimistic version check in a transaction: two devices at once → conflict;
      size limit). Remaining: handle, short, business, report, push, atp, prices all fall back to memory without
      Datastore — next: one small JSON kv table in sqlstore that backs their memory paths; then a self-hosted
      purge loop, admin stats, and the self-hosting guide on /business.
- [~] 64b-6. internal/kv: the memory fallbacks persist through kv.Default (sqlstore.KV: one (kind, id, gob)
      table; gob keeps fields hidden from JSON), loaded at construction — businesses, named links, short codes.
      Restart test: business with managers/trial, link, delete sticks. Next: report, push, atp, prices; then a
      self-hosted purge loop, admin stats, the guide.
- [~] 64b-7. kv write-through also for reports + moderators' hides, inbox prices, Bluesky (ATproto) sessions and
      push (subscriptions and the VAPID keys: regenerated keys would break every phone's push). Restart test covers
      them. Every store now persists under KAFUMU_SQLITE. Next: a self-hosted purge loop (sqlstore Purge* + kv
      expiry), admin stats without Datastore, and the self-hosting guide on /business (linking nodes = 64c).
- [~] 64b-8. Self-hosted purge (sqlstore.Purge, same rules as purge.Run): expired notes/meetups/box messages/
      slots, expired kv entries (gob-decoded ExpiresAt only), idle accounts with vault + username (30 days /
      a year / KeepDays, -1 kept). Runs at start and every 6 h, and as the admin "purge" job. Test covers each
      rule. Next: admin stats without Datastore, the self-hosting guide on /business; then 64c node linking.
- [x] 64b. Self-hosted admin + guide: KAFUMU_ADMIN_TOKEN bootstrap (/admin/initial?token=, ≥16 chars, constant-time,
      never on App Engine); /admin without Datastore shows SQLite stats (sqlstore.Stats) and account search
      (json_extract), businesses and footer (footer kept via kv). SELFHOSTING.md (build, env vars, admin, what it
      does, what's not there yet) linked from /business. Live: wrong token 403, right one → admin, stats shown.
      Remaining for 64: node linking (64c, below).
- [~] 64c-1. Relay + Pull (oln): Service.Relay(raw, via) — format + work checked, clock only "not ahead" (and
      ≤ MaxTTL old), life = this node's anonymous rule from the line's time (never extended), no area/repeat
      pricing, Note.Via = peer; Service.Pull(client, peer, cells) fetches peer/oln.json?cell= (≤ 20 cells) and
      relays. Test: two httptest nodes, old line arrives, no dupes, expired/future refused, bad address.
      Next 64c-2: peers in admin (add/remove, last pull, seen/new, error), a pull job (cron on GAE when peers
      exist, a loop self-hosted) over the cells this node has lines/meetups in; "via" shown on relayed posts.
- [x] 64c-2. Linked nodes in admin: add (https origin + optional areas) / unlink, "Pull now", last pull per peer
      (seen, new, error); stored in Config/peers (Datastore) or kv; pulls cover the peer's areas + the areas people
      here looked at in the last hour (Service.RecentCells). Job "oln-pull"; /cron/oln-pull exists but is NOT in
      cron.yaml (each wake costs instance hours on the free tier: Joop to decide); self-hosted pulls every 10 min.
      Relayed posts show "↪ host". Test: peer with a message → linked → pulled with via → admin shows it.
- [x] 64. Self-hosting and OLN nodes (Joop): done in 64a–64c.
      Was: OLN nodes linking (Joop): run Kafumu outside GAE (plain Go binary + a file/SQL store, no
      memcache), a page on how to self-host and link a node into OLN, and linked nodes on the admin page
      (peers pulled from / pushed to, last seen). Also mention self-hosting on /business.
      Joop (2026-10-05): bahais.in will be a GAE-less fork (adding local-community options). Most useful here: a
      SQLite implementation of the store interfaces, picked by env var, so forks needn't write their own.
- [x] 60. Eventa Servo without a key (Joop 2026-10-05): per-country iCal feeds,
      https://eventaservo.org/webcal/lando/<cc>.ics (de, fr, …) and ol.ics (online). Checked: 200 with an honest
      UA (curl's default gets 403), de.ics 230 events, ol.ics 376; no GEO, but LOCATION ends "…, City, CC" →
      gazetteer town lookup → cell; feeds include past events (drop them).
      [x] 60a. Done: bundles note the country of the cell (gazetteer CountryOf, nearest town; bots never get a
           bundle) in one shared-cache entry "seen-countries" (≤ once an hour per country per instance); the feeds
           job adds feeds.EventaServo(cc) for countries seen in the last 3 days (≤ 10). Placement: gazetteer
           Locate on each venue part from the end (postcodes dropped, "A - B" split), then Nominatim (≤ 30 a run).
           Event link from the description (eventaservo.org/e/…). Live, Germany: 230 events → 152 past,
           25 beyond the horizon, 52 placed, 1 not ("Struppen OT Naundorf"). Was: Country feeds into meetups: the feeds job (every 6 h) pulls the countries people looked at recently
           (bundle requests note the cell's country; bots don't count; ≤ 10 countries a run), imports upcoming
           events as meetups (Via eventaservo.org, tag #esperanto, lang epo, link to the event), placed by town.
      [x] 60b. Done: GET /api/esperanto/online (ol.ics, next 60 days, ≤ 30, upcoming first then running ones;
           shared cache 6 h; bots 403); Around shows "🌐 Online Esperanto events" when the UI language is
           Esperanto (Joop's bonus) or the view is Esperanto (language chip / #esperanto). Live: 30 events.
           Placement improved (Joop: "English, German and Esperanto names… fuzzy matching"): GeoNames alternate
           names (tools/townalts.py → townalts.tsv.gz: Parizo, Varsovio, Munkeno, München…) + villages 1k+ +
           leading words + bracket regions dropped: 292/297 upcoming events in 7 countries placed without
           Nominatim. Fuzzy matching TRIED AND DROPPED: all 4 fuzzy-only hits were wrong places (Bouresse →
           Paris "Bourse", Ommel → Ommen, Ashausen → Ahausen, Aßweiler → elsewhere); those go to Nominatim.
      [x] 60. Eventa Servo without a key. Was: Online events (ol.ics): when the Esperanto language chip or #esperanto is active, Around shows them
           under "Elsewhere · online" (fetched at most every 6 h, never for bots).
      Was: 60. Eventa Servo (Esperanto events, Joop): its API (eventaservo.org/api/v2) needs a key; ask them for one
      (or for a public per-country iCal), then it is one more entry in the feeds list (internal/feeds).
- [x] 45. DECIDED (Joop 2026-10-05): "Mix" + pay what you want + manual payments for now.
      [x] 45a. Done: User.PatronUntil (admin: "patron until" date per account, through the end of that day UTC;
           empty = not a patron), User.Patron(now); gold wings on the username button, on posts made under the
           name (Note.Patron set at posting via PatronFor), on findable people (Person.Patron); lapsed = no
           wings. Tests: model, post flag (named yes, anonymous never), admin render. Was: Patron wings for anyone: User.PatronUntil (admin sets it, like a business's paid-until); gold wings
           on a patron's named posts, @page and username button while it runs (grey after? no: patrons just
           lose the wings). Patrons fund the commons; they get nothing that buys visibility.
      [x] 45b. Done: not Live → adding managers, setting the @name and turning sync on redirect with
           "biz.paid_feature"; business vault/key/handover answer 402 (devices keep their data, sync just stops);
           /business shows the paused note; the @name page says "paused". Hosting meetups as the business no longer
           needs Live (free). Test: lapsed business → 402s, refusals, vault kept. Was: Office tools for paying businesses: free = host meetups and post under its own name; Live (trial or
           paid) adds more than one manager, synced card + contacts, the @name page, gold wings. Lapsed: nothing
           deleted; sync pauses (devices keep their data), no new managers, wings grey, @page stays but says so.
      [x] 45c. Done: /patrons#pay "Pay what you want" (€2/month patron, €5/month business, at least €1, via the
           configured links) with the reference to quote: "Kafumu @business" while acting, "Kafumu @you", or the
           account id + a nudge to pick a username; "set by hand within a day or two". Each business on /business
           links there. patrons.thanks reworded in 24 languages (patrons now get the wings). Test: references.
      [x] 45. Paid services: Mix + pay what you want + manual (45a–c). Was: How to pay: "pay what you want" on /business and /support: suggested €5/month (business), €2/month
           (patron), floor €1; the configured links (Liberapay, PayPal, bunq, Stripe link) + a reference to quote
           (the business's id/@name, or your @username); admin sets the dates.
      Was: OPEN DESIGN (with Joop, not a tick): a better model for paid services. Joop (2026-10-04): the
      "For cafés and venues" page "isn't well thought out", "we'll need a better model for paid services".
      Constraints from VISION: core use free forever, Lichess-style patrons, relevance instead of ads, nothing
      that buys visibility over what's actually near and relevant. Don't build until Joop picks a direction.
- [x] 76. Business ≡ person (Joop: "a check for the equivalence of business accounts to personal accounts…
      that would be the better place to fix"). One "who": an owner is a user id or "biz:<id>"; actsFor(u, owner)
      = it's u, or a business u manages (identity.go; brand admins use it). Gaps found:
      [x] a. Done: "add manager" takes a business's @name (not itself); managesBiz (direct, or manager of a
         managing business; one level) replaces b.Manages in every handler check (acting, /business, sync,
         key handover, hosting as, brand admins via actsFor); bizFor lists direct + through-a-business ones
         (one indexed query per business of yours); manager list shows businesses by @name. Test: agency
         manages café, ana manages both, chains not followed. Was: A business as manager of another business (managers may be "biz:<id>": its managers manage; ForUser
         needs the indirect ones).
      [x] b. Done: handleOwner (acting as a named, live business → its @name, owned "biz:<id>"; else you) for
         set/delete/views of the named link; Card offers the business's link while acting (key in the business's
         own device store; weekly refresh too); its @name page stays the profile, with "☕ Connect" when the link
         is live. Browser test: on, Connect on the page, visitor lands on connecting. Was: A business's /@name connects like a person's (a named link holding the business card, kept by a
         manager's device acting as it), with its profile page shown alongside.
      [x] c-1. Findable businesses: acting, /findable edits the business's profile (Cell, VisibleUntil indexed,
         Bio, Where, Langs, Tags; free, like a person's; the inbox part hidden until c-2); bundles add findable
         businesses from one cached list (Store.Findable: one query a minute per instance, filtered by area) as
         people with Biz (🏢) and Patron = live (wings). Browser test: findable, B sees "@biz 🏢".
         FLAKY "business card synced up" (seen twice): the check called sync() every 250 ms, overlapping its
         own syncs (version conflicts). Now paced (500 ms, up to 20 s) and it reports what sync answered;
         passed 3 runs in a row since.
      [x] c-2. Business public inbox: acting, Findable's inbox controls set Business.InboxBox/Pub/Bits (price
         set like a person's), shown on its findable entry (✉️). And for everyone: inbox keys are now made
         exportable (JWK) and the vault snapshot carries the inbox key (newest wins) + received inbox messages
         (union by id): a public inbox is readable on all your devices, a business's on all its managers'.
         Inboxes opened before keep a device-only key until switched off and on again. Browser test: B writes
         to the business, A acting reads it.
      [x] 76. Business ≡ person (a–c); d (Bluesky for a business) only if wanted.
      (was) c. Findable + public inbox for businesses (messages to the business, read by its managers' devices).
      d. Later, if wanted: Bluesky for a business.
      Already equal: named posts, hosting, wings (patron / live), synced card + contacts, brand admin.
- [x] 46. DECIDED (Joop 2026-10-05: "2 and 4"): brands are set up by Joop in admin as a PAID service (manual
      payment, like 45c), with brand admins; and forks (self-hosting + OLN linking, done in 64) are the free
      route. [x] 46a done: internal/brand (host-keyed, Datastore or kv, all brands loaded together and kept a
      minute: choosing a request's brand is free); newPage picks it from r.Host (name, accent colour as a CSS
      variable (hex only), main-button wording, tagline, Around's starting tag); admin "Brands" section (save,
      remove; note: the domain must be mapped to the App Engine app). Tests: store, brand page vs plain host,
      admin. [x] 46b done: brand admins (admin sets @usernames per brand; resolved to ids, businesses refused);
      /brand on the brand's own host lets them edit name, tagline, main button, colour, starting tags (not
      the host); "🎨 <brand> →" on their account page. English-only, like /admin. Test: 404 for others and
      other hosts, admin saves. [x] 46c done: on a brand's domain, absolute links come from Home.Origin(r)
      (sign-in link, named-link URL, meetup links, calendar feeds), passkeys use a WebAuthn per brand domain
      (RP ID = the brand host, made on first use), and the "we moved" banner (meant for old domains, which
      also switched passkeys off) never shows there. Cookies were already per domain, so accounts are
      separate per brand; areas, local messages and connect codes stay shared. Test: banner, origin, RP ID.
      [x] 46. Brands (46a–c). Going live per brand: map its domain to the App Engine app.
      Then (Joop: "either personal or business"): a brand admin is a person's
      @username or a business's @name ("biz:<id>"); its managers manage the brand, acting as it or not, and
      access follows the manager list. Test: person, manager, stranger, manager removed. Before 46a: 46b brand admins + wording/tags/colours;
      46c brand-local accounts (passkeys per domain) and the shared network underneath.
      Was: Brands on other domains (Joop's idea, 2026-10-04; part of 45): one app, several faces chosen by
      Host: e.g. bahais.in / localprayers.net with "Who wants to pray with me?" and "Register our local
      events". A Brand entity (domain, name, tagline, colours/icon, wording of the main button, default view
      tags, feeds, which kinds show), set up by an admin; brand admins (role scoped to one brand) manage it.
      Same #geo/OLN network underneath, each brand a lens (default tags), so people meet across brands.
      Paid: a brand for your community/organisation. Accounts: separate per brand domain (Joop: passkeys are
      bound to their domain); the shared layer is the network (#geo cells, OLN, connect codes), not accounts.
      Open: pricing.
- [x] 49a. Filter chips row above the feed: events (🔴 live / 📅 upcoming), the local language (🗣, replaces the
      Learn button; the learn card opens with it when you don't speak it), top local tags and your interests.
      Tap = filter (tags strict, w=3; language boosts), tap again = clear. Heading becomes "#tag · Place" /
      "🗣 Language · Place". Browser-tested in Lisbon (🗣 Portuguese, #websummit). 49b next: widen + Elsewhere +
      interest synonyms.
- [x] 49b. Elsewhere: a tag filter with < 5 matching posts here fetches GET /tagposts?tag= (Bluesky, cached,
      no bots) and shows them at the bottom labelled "Elsewhere · #tag"; interests linked in TAG_GROUPS
      (#opensource=#foss=#floss, #ai=#machinelearning…), used for posts, notes, meetups and people; language
      chip shows the native name only. Browser-tested (#coffee on an empty cell). Was: Filter chips on Around (Joop): one row of tappable hashtags that narrow Around: running
      or upcoming events (#websummit), the local language (merges the 🗣 Learn button into this), interests
      (yours + local themes). Tapping one sets the view; the heading becomes "#websummit in Lisbon".
      Few results → widen: same tag in the ring around, then general posts with that tag (no location) at
      the bottom under "Elsewhere". Interests linked like places: synonym groups (#opensource #foss #floss,
      #ai #artificialintelligence…) in a small table, matched as one.
- [x] 50. Chips you choose: "🗣 +" (all languages), "# +" (any subject, suggestions from linked interests);
      adding pins the chip; ✎ edit mode: × unpins yours or hides a suggestion. Stored as "chips" {pinned,
      hidden, at} in the device store + vault (newer wins), localStorage mirror for drawing. Browser-tested.
      Was: NEXT. Chips you choose (Joop): a "🗣 +" chip opens a language picker (all 98) and a "# +" chip a
      subject box (free text, with linked-interest suggestions); the suggested subjects are configurable:
      pin/unpin chips (long-press or an edit mode), kept with your personas so they sync to your devices.
- [x] 51. Questions reach people with that subject: Note.Asks (indexed: a question's subject tags), GET /api/asks?tags=
      (no bots; one query per tag, cached 60 s); Around fetches questions for your profile interests + pinned
      chips, keeps those within 200 km, nearest first, "❓ for #x · 45 km away". Also (Joop): language chips now
      filter (w=3) with Elsewhere via the language's own hashtag, and a line under the chips says what's shown
      ("🗣 toki pona: 0 here, so #tokipona from elsewhere is shown below"). Was: Questions reach people with that subject (Joop: "a wide range towards people with that subject
      set up"): an #ask tagged with subjects goes beyond the cell to people whose interests include them:
      GET /asks?tags=… returns live #ask notes with any of those tags (one indexed query per tag, cached
      60 s); the device of someone with those interests shows them in Around (ranked by distance, wider
      radius than ordinary messages) and, opt-in, as a push. The asker sees "reaches people into #x nearby".
- [x] 52. Travelling prompt: away > 50 km from your usual area, the travel note adds "It looks like you're
      travelling. Want to be visible in the area while you're here?" → "Be findable here" (/account?cell=…:
      area filled in, a few days' visibility pre-selected) and "Check my card". Browser-tested.
- [x] 53a. Business accounts: internal/business (name, kind, contact, managers, 30-day trial, status), /business
      (start, managers by username; promises only what exists: host meetups as the business), meetups "Host as"
      → "hosted by <Business>", admin list (trial over first, "contact?", status + note), event-card ad, privacy
      row; all languages. Named link: its key (as JWK) and setting (incl. off) travel in the vault, so every device
      answers /@name; "👀 Someone opened your link N min ago · N today" on My card (cache-only counter). v1 OLN
      leftovers no longer listed. Was: Business accounts (Joop's paid model, answers 45): a business/organisation account on the server
      (name, kind, contact), first month free; personal accounts added/removed as its managers; it can host
      meetups and post as the business, and later its own brand (46). After the trial it pops up in the admin
      panel ("trial ended: contact?") for Joop to reach out and set up a contract; status trial/active/paused.
      Advertised on event labels: the Web Summit card (and any event found automatically, 36) gets "Here with
      your company? Business account, first month free".
- [x] 54. Findable tab (/findable, 6th tab, eye icon): public profile, languages, interests, public inbox; Account
      keeps sign-in, link, passkeys, name, Bluesky, leave. "👁 Be findable" button on Around (with this cell); travel
      prompt and other links go to /findable. Also: /for-cafes removed (→ /about) until a real model exists; the
      Patrons page no longer promises a supporter badge that doesn't exist; code blocks scroll inside their box.
      Was: "Be findable" as its own tab (Joop: confusing inside Account), plus a button for it on Around.
- [x] 55. Account nudge (Joop): with contacts or a card on this device but no way back in, pages (Around,
      Connect, Contacts, My card) show a dismissable bar: not signed in → "make an account with a passkey";
      signed in without username or passkey → "add a username and a passkey". Dismissed: a week. Browser-tested.
- [x] 56. Last seen per contact: c.lastHeard set whenever anything arrives (pair.checkContact); a weekly
      {t:"alive"} per contact from Contacts (X-Kafumu-Quiet: no push); contact head shows "💤 quiet for N days"
      from 14 days; sort option "Quiet 90+ days" lists only those with "Remove all" (tombstoned, synced).
      Was: Last seen per contact (Joop: "clean up your list when connections break"): c.lastHeard = last time
      anything arrived from them (card, signal, chat, check-in); a light weekly "alive" ping per contact when
      the app opens (one mailbox write each, only to contacts not heard from in a week); Contacts shows "last
      heard 3 months ago" and a "Quiet for 90+ days" filter with "Remove all" (tombstoned, synced).
- [x] 57. Named links: Handle entity (username → public invite payload, 90 days, purged), PUT/DELETE /api/handle,
      /@name → /c?from=name#payload ("@name invites you to connect"); a long-lived "named" invite on the device
      (box = invite), taken in on Around/Contacts and renewed weekly; Findable has "Your link" (off by default,
      turn on and share / turn off). Badge printing of /@name still to do. Plus zh-hant (Taiwan vocabulary;
      zh-TW/HK/MO/Hant pick it), zh labelled 简体中文. Was: NEXT. Named links (Joop): kafumu.com/@name, a long-lived connect link for a named account (e.g. "have
      a coffee with Joop at Web Summit", and on printed badges). The owner's device keeps a long-lived invite key
      (like the badge code) and registers its public payload under the username (Handle entity: name → payload,
      renewed whenever the owner opens the app, expires after 90 days unused). /@name shows the card preview and
      "Connect"; scans become contacts like any code. Badges print kafumu.com/@name when there is one.
      First user: kafumu.com/@lapingvino (Joop's account), e.g. "have a coffee with Joop at Web Summit".
- [x] 58. "😕 I'm confused" on Around: an 8-step tour (area, coffee, say, ask, chips, feed, change area, tabs), the
      explained part outlined, text floating above the tabs, Back/Next/Done, in all 24 languages. Badge uses
      kafumu.com/@name (printed under the QR) when the named link is on. Was: "I'm confused" on Around (Joop: location-first is new to most people): a small button that starts a
      short guided tour on the page itself: step by step it highlights the area heading ("this is where you
      are, as a #geo cell"), coffee/say/ask, the filter chips, the feed kinds, Connect, Contacts, My card and
      Findable, each with one plain sentence; Next/Back/Done; remembers it was seen; all languages.
- [x] 59. Repeat policy (this node's): same normalised text within 24 h is dropped in the same cell (409) and costs
      +4 bits per other cell it already went to; reactions and texts < 12 chars exempt; per instance, in memory.
      402 answers carry "need" and the client mines straight to it. Also: two flaky tests fixed (TestAuthor compared
      lives at different work; pow's "other body" check at 2 bits passes 1 in 4 — now at 16). Was: Repeats cost more (Joop: "we want the useful kind of directed spam"): the same message text (normalised)
      posted again to another area or subject within a day needs sharply more work (e.g. +4 bits per repeat), and
      exact repeats in the same area are dropped; per node. Location already makes untargeted spam expensive.
- [x] 61ab. Proof of work v2 (Argon2id): measured first (Go↔hash-wasm vector identical; phone ~70 ms/attempt at
      4 MiB throttled 4×, server verify ~6 ms), then one cutover (Joop: day two, no transition): line "v2;nonce;
      date;b64;keywords", work = leading zero bits of Argon2id(line, "OLN-v2-proofwork", t=1, m=4 MiB, p=1, 32 B);
      OLN BaseBits 4 / MaxBits 14; stamps "v2;nonce;date", MinBits 2; ≤4 concurrent checks; inbox prices 2–12 (v1
      prices converted −10); worker + async stamps via vendored hash-wasm; work rate in attempts/s; /oln docs
      (example verified). 61c done: eolnpoc on GitHub speaks v2 (pow.Work/Create/Parse, shared Argon2id
      vector test, olnhash uses the package, README format section); it validates Kafumu's production
      oln.json (v2 messages ≥4 bits; v1 leftovers expire). Was: PRIORITY. Memory-hard proof of work, before anyone else adopts the format (Joop: "prevents an IPv6-like
      deployment issue"). SHA-1 leading zeros lets a GPU outrun a phone ~10⁶×; Argon2id (memory-hard) narrows
      that to ~10×. A versioned v2 line ("v2;…"), v1 still accepted for a transition; Kafumu's mailbox/short/
      report stamps move too; the browser miner via a vendored WASM Argon2id in the worker; eolnpoc updated to
      match. Design to settle: parameters (memory, passes) so a phone does ~1 s at the base price and the server
      verifies in a few ms; how "bits" map to attempts (TTL doubling per bit stays).
Principle (Joop): OLN decides the format, not how messages are treated; dropping, pricing repeats (59), per-IP
limits, hiding are each node's own policy.
- [x] 39. Post with or without your name: composer "Post as @name" (named accounts; on by default, remembered);
      the server vouches (Note.Author from the session, only with X-Kafumu-As: 1); anonymous public messages
      live half as long; named rank +150 (≈3 bits); cards show "@name ✓"; oln.json origin.display. Also fixed
      v2 leftovers: chat lines and the default required bits were still 12 (a minute of Argon2); now 4.
      Was: Post with or without your name (Joop): a local message can carry your account name (signed by the
      server as @name, linkable to your profile) or stay anonymous; anonymous ones rank lower and expire
      sooner (shorter TTL at the same work), named ones get the normal TTL and a trust bonus.

**Idle-tick rule (Joop):** when no slice is open, a tick is not idle: use the app in a real browser
(test/cdp.mjs), pick one area, and make it look and work a bit nicer. Fix one concrete thing per tick.
Translation as a fallback is retired (Joop, 2026-10-05: 24 languages is enough). When no slice is open and
polish runs thin, slow the loop to every two hours (Joop: "most time goes to testing anyway").

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
  12/18 UTC, purge 04:00). Feeds VERIFIED 2026-10-04 00:00 UTC (50 events, 40 saved, 0 errors; Lisbon
  shows Luma meetups). Purge VERIFIED 2026-10-04 04:00 UTC (meetups+notes=38, boxes+pushsubs+short=3,
  atproto=1, users=0, err=nil): /privacy's retention promises hold in production.
- Browser tests post into cells 6fg222/6fg223 (0°,0°, open sea) so production runs never show up anywhere real.
- Joop (2026-10-03): "be daring, corrections are cheap"; ping his phone only for urgent things.
- Datastore indexes: `~/google-cloud-sdk/bin/gcloud app deploy index.yaml --project lokumo`.
- Deploy with the user-installed SDK: `~/google-cloud-sdk/bin/gcloud app deploy --project lokumo --quiet`
  (the pacman gcloud lacks app-engine-go). Needs the sandbox disabled.
- `#ams` is full of flight-tracker bots; short aliases are marked ambiguous and weigh less.
- No posts on Bluesky carry `#geo…` tags yet (checked 2026-10-03): the gazetteer slice matters most.
- `public.api.bsky.app` may 403 from some networks; client falls back to `api.bsky.app`.
- AppView marks some authors with a `bot` label; the client downranks them.
- [x] 77. Around widens like a spiral (Joop: "at least 3 circles… increase circles until you have a certain amount
      of posts… that's what the Ulam spiral approach achieved"): start at 3 rings (49 cells; Bluesky still only
      for the inner 25); while fewer than 15 things show, the device fetches the next band (rings 4–5, 6–7, 8)
      as a wide bundle (wide=1, ≤ 120 cells: local messages, meetups, people only, no upstream calls, cached a
      minute). Datastore "in" queries are chunked by 30. Also: city-wide events (placed by city name) carry
      the city's radius (AreaKm) and reach the whole city via its centre cell. Tests: wide bundle, city-wide.
- [x] 78. Slow Bluesky for the wider rings (Joop: "slowly retrieve from Bluesky for the wider areas if the server
      is idle enough… to get to the 15 things in the least covered areas"): wide bundles include only cached
      Bluesky posts for their cells (bsky.Cached) and queue the rest (bsky.Later); one worker per instance
      searches one queued tag every 3 s, only after 2 s without a foreground search, queue ≤ 300 (deduped,
      fresh tags skipped), stops when empty. The device, still under 15 at ring 8, looks once more after 25 s.
      Tests: trickle (once per tag, then cached), wide bundle (cached posts in, the rest queued, no fetch).
- [x] 79. (Polish, measured in production: near bundle 0.15 s warm / 1.3 s cold, wide bands 0.06–0.4 s; rural
      Alentejo empty.) Cell #geo tags are rarely used on Bluesky, so the slow searches for the wider rings now
      also cover their towns' tags: gazetteer.TownTags (villages and towns of 1,000+ people, not neighbourhoods,
      biggest first, 4 per band; "#vianadoalentejo #portel" around Évora). Test: a town's cached post reaches
      the wide bundle.
- [x] 80. Bluesky checked (polish): tag search ignores case (#lisboa = #Lisboa) but not accents (#evora ≠ #évora,
      different posts): town tags now come accented and plain. public.api.bsky.app answered 403 from the dev
      machine while api.bsky.app answered: the client now tries the host that answered last first, instead
      of spending a refused request on every search. Tests: both spellings, refusing host asked once.
- [x] 81. (Polish: production logs clean, but the 00:00 feeds run did 2 feeds, not 4.) The countries looked at
      lived only in shared memcache, which may evict any time: the Eventa Servo calendars silently stopped.
      Now also in Datastore (Config/seen-countries, written ≤ once an hour per country per instance; read only
      when the cache lost it, then put back). Emulator test: survives losing the cache.
- [x] 76d. Bluesky for a business (Joop: "sounds obvious"): Business.DID/ATHandle/ATSession; "Connect" while acting
      links the business's account (target kept in a 10-min cookie for the OAuth round trip, management checked
      again on return); meetups hosted as a business (any you manage) are published to its account or not at
      all, posting while acting posts as it (bskyAccount: never the manager's own); disconnect while acting, and
      closing a business, end its session. Account page shows the business's Bluesky above the greyed part.
      Test: bskyAccount. (The OAuth round trip itself needs a real PDS: not testable locally.) Follow-up: the
      Around post box shows the acting account's handle (page.BskyHandle), and "going" while acting RSVPs as it.


## 77. Activity: one list of everything that happened to you (Joop, 2026-10-07)
Problem: responses are scattered. A post in another area loses its replies; chat lines and reactions only show
on a contact's card; strangers' reactions show only under the post in its own cell. Built on the device (rules
1/4): the list is assembled in the browser from what it already holds plus anonymous per-id fetches.
- [x] 77a. Remember own posts: at post time save {id, cell, reid, text(80), until} in kv `myPosts` (notes, asks,
      coffee, replies; meetups hosted by id; Bluesky posts by URI, which /post must return). Pruned at `until`.
      Done: rememberPost (notes, asks, replies, coffee; max 200); /post returns the URI (&at=, &t=) and the
      page now says "Posted to Bluesky" (posted=1/0 was never read, and "/&posted" was a broken URL). Meetups
      come from the account in 77d. Browser test: the posted note lands in myPosts.
- [x] 77b. Public replies/reactions to own posts across cells: index Note `re` (the #re<id> tag, public anyway);
      `GET /api/oln/re?ids=a,b,…` (anonymous, ≤20 ids, memcached 1 min per id, no account).
      Done: Note.Re (indexed, set by Parse), Store.RepliesTo (Datastore, memory; SQLite via json_extract, no
      migration), Service.Replies (1-min per-instance cache per id, hidden notes left out), HandleRe (public,
      max-age=60). Deployed early so replies get indexed before 77c uses them; older replies (no `re`) won't show.
- [x] 77c-1. /activity page + 🔔 in the header with an unread count (and setAppBadge), gathered on the device by
      static/activity.js: public replies/reactions to myPosts (/api/oln/re, credentials omit, 20 ids a call), private
      answer threads, contacts' last message and last signal, public-inbox messages. Seen marks per item in kv
      `activity:seen` (2 months); contact/answer items stay new until opened there ("hot"). Home and Contacts
      refresh the count (4 s after load, then every 5 min); other pages show the stored count. Browser test: B's
      answer → A's bell counts it → Activity lists it as new, linking to its area → seen on the next visit.
- [x] 77c-2. Also in Activity: new connections through your code/invite, coffee joiners; Activity refreshes
      answers itself (readAnswers) instead of relying on Home's polling.
      Done: contacts with role 0 (they came to your code: Connect, Coffee, a question) from the last 30 days show
      as "🤝 … connected with you"; the Activity page runs readAnswers before gathering. Browser test checks it.
- [x] 77d. Meetups: RSVPs to meetups you host (going count per id, compared on the device); a meetup you're going
      to that changed or was cancelled.
      Done: the meetup page keeps kv `myMeetups` (host/going, with title, time, venue, going as you last saw it);
      GET /api/meetups?ids= (anonymous, ≤20, 1-min per-id cache; missing ones in "gone"); Activity shows 🙋 new
      going counts, ✏️ a new time/venue (feeds can move events), ❌ cancelled (gone before its end). News time =
      first seen (kv `activity:first`). Tests: TestMeetupsByIDs; browser: hosted meetup remembered → 🙋 in Activity.
- [x] 77e. Bluesky: replies, likes, reposts on your own posts (public AppView getPostThread / getLikes per saved
      URI, fetched by the device, no server cost).
      Done: activity.js asks the public AppView itself (CORS *, last good host first, credentials omit): one
      getPosts for ≤25 remembered Bluesky posts → ❤️/🔁 counts (dated first seen), getPostThread for those with
      replies → 🦋 replies by others (newest 20 a post). Checked by hand against a real post; not in run.sh
      (needs the network).
- [x] 77f. Push for answers and replies too: subscriptions also watch your answer tags (Notify on pair notes);
      the notification opens /activity instead of /contacts.
      Done: push subscriptions also take OLN pair tags (p+32 hex): the device watches its live answer tags and
      the chat tags of contacts and answer threads (pair.watched; subscribe code moved from contacts.js to
      pair.pushSubscribe, refreshed by Contacts, Activity and after posting). oln.Service.OnPair → push.Notify;
      a pair-tag push says "💬 A new message or answer for you" and opens /activity. Contact chat lines didn't push
      before either. Public #re replies: not pushed (watching post ids would tell the server which posts are yours).
      Also: TestPullFromPeer flaked when mining got lucky extra bits (longer life); mineExact fixes it.
- [x] 77g. Reactions to reactions (Joop): a reply under a card gets its own react row; its reactions (#re<its id>)
      nest one level under it, and count as replies to your reply in Activity. Uses 77b's `re` index for the
      ones not in the cell's bundle.
      Done: replies under a card get the react row; their reactions show under them, one level deep. Your own
      replies are in myPosts (77a), so reactions to them already reach your Activity. Reactions still carry #geo;
      #re-only lines come with 78a. Browser test: 👍 on an answer shows under it; the Activity check reloads past
      the 1-minute reply cache (it was a timing flake).

## 78. Reactions everywhere, and carrying them home (Joop, 2026-10-08)
Tagging rule (Joop): tag a reaction with the clearest indication of what it reacts to, and #re<id> alone is
that (the most focused): a reaction line is just `…;#re<id>` (plus #lang for text), no #geo. It's found from
the thing through the 77b index, wherever the thing is.
- [x] 78a-1. Server: lines with a #re and no #geo are accepted, filed under oln.Everywhere ("000000", not a valid
      area cell, so no bundle includes it), priced by the replies to that same target (requiredForRe; also
      /api/oln/required?re=). OLN page documents it and /api/oln/re. Test: TestReactionWithoutPlace. Not yet:
      linked nodes don't pull Everywhere lines (pull is per area).
- [x] 78a-2. Client: react buttons and 💬 replies post `#re<id>` only (+#lang for text), mining at required?re=;
      Around fetches the reactions of the cards it shows from /api/oln/re (≤20 ids a call) and merges them with
      the bundle's (old #geo-tagged ones).
      Done: react buttons post "#re<id>" only, 💬 replies "#lang… #re<id>"; pullReactions fetches the reactions of
      the cards and replies on screen (not asked in the last minute), merged with the bundle's; your own show at
      once. Server: RepliesTo takes a list — one Datastore "in" query for all uncached ids of a request (was one
      per id). Activity links a placeless reply to the post's area. Browser test: A sees B's answer and its 👍
      on A's own Around, fetched by id. Datastore "in" wants []interface{} (a []string 503'd in production for
      ~10 min after the first deploy); checked live with a two-id lookup; the emulator test skips (no "in" there).
- [x] 78a-3a. General lines, server + reading: no #geo, no #re, ≥1 subject → filed Everywhere with Note.Subj
      (indexed); Store.About (one "in" query; SQLite json_each); Service.General (per-tag 1-min cache);
      GET /api/oln/general?tags=; priced by recent lines about its first subject. Around shows them (🌍 everywhere ·
      #tag) only while filtering on that subject. Posting UI: 78b. Tests: TestGeneral (memory, SQLite), browser.
      Also fixed TestRepeats' lucky-bits flake (mineExact). Flaky still: "business card never synced" (2× today).
- [x] 78a-3 (reference). "General" notes: needed for "general" reactions and for links with only subject tags.
      Decided (Joop, 2026-10-08): the one-cell rule is Kafumu's API, not the OLN message format. A general line
      simply has no #geo (`…;#esperanto #re…`); the API files it under the internal cell "000000" (all zero =
      everywhere), so Store/Note stay the same. Parse: no cell + ≥1 subject tag (not lang/re/ask-only) → Cell
      "000000"; its own price like any cell; subjects indexed like `asks`; read through the subject fetch Around
      does when you filter. Bundles never include 000000.
      Also lines with a #re and no place (reactions, per the tagging rule): filed under 000000 too. Around then
      fetches reactions for the cards it shows through /api/oln/re (≤20 ids a call, cached) instead of from the
      cell bundle, and the react buttons and replies post `#re<id>` only. Old #geo-tagged reactions still show.
- [x] 78b. Online events (Esperanto online list, and any event without a place) get the react row, with a choice:
      "here" (#geo your cell) or "everyone into #esperanto" (78a). Verify local Eventa Servo meetups' reactions in
      production while at it.
      Done (reactions are "#re"-only now, so the here/everyone choice is for text posts): online events get the
      react row (reID("event", link)) and fetch their reactions like cards; the composer offers "🌍 Everyone into
      #tag, not just here" once a subject is typed → a general line (no #geo). Your own general lines show at once
      while filtering. Browser test: posted through the composer, seen by B elsewhere when filtering.
- [x] 78c. Carry a reaction home: reacting to something from another area (wider ring, travel, elsewhere) can also
      post it in your own cell (your #geo + its #re + its title/link in the text), so local friends see "X is
      going on there"; such a note shows as a small card about the thing, with its reactions.
      Done: a 📍 toggle on react rows for things >20 km from your home cell (homeCell, from GPS use) or with no
      place (online events); on, the reaction posts "#geo<home> #re<id>" and its text gets two context lines (title,
      link). At the thing only the first line shows (a carried 👍 still counts as 👍); at home, carried reactions
      whose thing isn't on screen show as a "📍 from elsewhere" card (counts + title, linking to it). Questions have
      no react row (answers instead), so no 📍 there. Browser test: A carries ☕ home, B sees it there.

## 79. React to any web link (Joop, 2026-10-08)
Per the OLN json-spec, a link is an index key like a tag (index["link"] → messages).
- [x] 79a. "Share a link" in the composer: paste a URL → the note carries it and #re<reID("link", normalised URL)>;
      cards show it as a link card (host + the poster's text), with the react row; all reactions to that URL from
      anywhere come from the 77b index.
      Done, minimal (Joop): a post with a link carries "#re" + reID("link", normalised URL: no #fragment, no utm_/
      fbclid/gclid) — sharing a link is reacting to it. Such a note shows as a card with a 🔗 line; its react row
      reacts to the link, so every share and reaction to that URL, from anywhere, gathers under each share (77b
      index). Your own share's Activity follows the link's id. Browser test: A shares, B sees it cleaned up, 👍.
- [x] 79b. Post it to your area (#geo) or to a subject community only (78a); subject-only links show when you filter
      for that subject.
      Done by 78b: type a subject, tick "🌍 Everyone into #tag" → a link line without a place.
- [x] 79c (Joop: "good to add already"). By site: also tag the host (#site<host>, normalised) so a site's discussion can be browsed (the json-spec's
      link index). Later: a browser extension that shows a page's OLN reactions and lets you react from there.
      Done: link shares also carry #site_<host> (no www., dots as _); the link card's "🌐 example.org" filters
      Around on it (local shares + general ones via /api/oln/general). Extension still later. Also: tagText renders
      every displayed tag — "langepo"/"lang:epo" as "🗣 Esperanto", site tags as 🌐 — in meta lines, chips, tag
      suggestions, headings, filter notes, why-lines (Joop: raw #lang tags were still showing in places).

## 80. Send things to a contact (Joop, 2026-10-08)
- [x] 80a. "Did you see this?": every card (meetup, note, post, link, person) can be sent to a contact you pick,
      as a structured message in your pair chat ({t:"share", kind, title, url}); it shows as a small card in the
      chat and in their Activity.
      Done, minimal: a 👀 on every card with a link (notes, meetups, online events, shared links) lists your
      contacts; a tap sends "👀 Did you see this?" + title + link as a line in your encrypted chat with them (no new
      message type); chat bubbles make links clickable; it reaches their Activity as a message. Browser test: B →
      A, A's Activity shows it. Suite now ~450 s of the 590 s limit: trim before adding much more.
- [x] 80b. "Shall we go together?" on meetups: the same, with Yes / Can't buttons; Yes RSVPs them and tells you
      (in the chat and your Activity).
      Done, minimal: 🤝 on meetup cards sends "🤝 Shall we go together?" + title + link as a chat line (same picker
      as 👀). In their chat that line gets Yes / Can't: either answers in the chat; Yes then opens the meetup page,
      where they tap "I'm going" (the RSVP endpoint toggles, so no blind RSVP). Browser test: Can't answers, the
      buttons go. Suite ~465 s of 590.
- [x] Polish: "business card never synced" browser-test flake (sync() keeps answering "off" for 20 s; 3× on
      2026-10-08). The test now reports actingSync and the vault's answer when it fails: read that next time.
      Found (the report said actingSync unset, vault 404 "not synced"): a lost update, real in production too.
      Handlers saved a whole business they'd read earlier — the inbox and profile ones even the cached ActingAs
      copy — so one landing right after a sync-mode change put the old mode back. business.Store.Update (Datastore
      transaction / lock) now changes only what each handler means to change: sync mode, key requests/grants,
      managers, @name, findable profile, inbox, Bluesky, leaving on account deletion. Also bizByID won't cache a
      copy read before a forgetBiz. Test: TestUpdateConcurrent (20 at once, -race). Admin edit converted too (next tick).
- [x] Test time: 465 s → 250 s. The browser test spent most of its 396 s waiting out one-minute caches in reload
      loops. oln.Fresh / meetup.Fresh (default a minute) set the per-instance caches and Cache-Control; the test
      server runs with KAFUMU_FRESH=2s. A 👍 check that only passed while a carried ☕ was still cached now looks for
      "👍 1" among the reactions.
- [x] Accounts: the same lost-update audit. Requests load the user once (UserFrom) and many handlers Save it
      whole (profile, inbox, Bluesky, LastSeen…); two overlapping ones can undo each other. An atomic
      account Update (like business.Store.Update) for those, field by field.
      Done: account.Store.Update (Datastore transaction, memory/SQLite lock) and Service.Update (refreshes the
      caller's copy and the cache). Converted: the hourly LastSeen touch, new sessions, passkeys, @name set/clear,
      inbox, profile, Bluesky connect/disconnect, admin role/keep/patron. The touch was the dangerous one: from an
      instance's cached copy it could drop a session another instance had just added (a new device signed out).
      Test TestTouchKeepsOtherInstancesSession fails on the old code, passes now.
- [x] Found (Joop's Chromebook): a saved "quiet for 90+ days" view with nobody quiet showed "nobody yet" and hid the
      sort menu, so there was no way back (other devices didn't have that view saved). Now the list's tools always
      show, "nobody yet" only when there's nobody; browser step.
- [x] (diagnostics added: Contacts shows "On this device: n contacts · host · store" + last sync, sync errors and a
      key that doesn't open the vault say so, storage that won't open says so, console logs each sync state)
      Joop 2026-10-13: Contacts empty on one Chromebook, sync doesn't bring them back (same machine has storage
      trouble with other apps). Asked: which address (kafumu.com vs the old appspot one: separate storage), acting as
      a business (separate store), DevTools → Application → IndexedDB → kafumu → contacts. Alias flake: fixed below.
- [x] Device lost updates (the "A's own name for D kept" flake): pages and polls held older copies of a contact and
      wrote them whole. putContact now merges with what's stored in one IndexedDB transaction (mergeContact): your
      edits (alias, note, tags) carry times in c.edits and the newest wins; messages and signals are joined (read
      stays read, answered stays answered); the newer card wins. Sync merges contacts the same way instead of
      newest-wins. Test: device_test mergeContact.
- [ ] Polish: flaky browser step "third device asks for the key, with a code" (business private mode), 1× on
      2026-10-10 (passed on rerun). Make its failure report what the key-request API answered, like the sync one.
      Diagnostics added (actingSync, the key panel, GET keyreq): read them when it next fails.


Order (Joop 2026-10-13: mailbox first, reorder freely): 83 → 82 → 87a → 85 → 86 → 88 → 81c2–f → 84 → 89 → 91 → 90b–c.
## 83. One mailbox for every conversation (Joop 2026-10-13)
Private answers (now on the home page), chat messages and their answers (now on contact cards), replies to
your posts and meetups (Activity, 77) in one mailbox, and notifications that always say who and about what.
- [x] 83a. Map (2026-10-13). Channels → where they show → push:
      · signals (☕, mailbox box) → contact card → "☕ Someone you've connected with sent you a signal", /contacts
      · chat lines (pair tag p…) → contact card chat → "💬 A new message or answer for you", /activity
      · private answers to your lines, both ways (pair tags from replyKeys/"answers" threads) → home "🔒 Private
        answers" above the feed → same "💬 …" text, /activity
      · public replies/reactions (#re), meetup RSVPs/comments, Bluesky replies → Activity (77), no push
      · invite/badge/public-inbox boxes (someone connected, a paid inbox message) → contacts → signal text
      Why notifications lack context: the server can't know who or what (rule 1), and the payload doesn't even
      say which box, so every push reads the same and chat pushes open Activity, where chats don't show.
      Fix (83d) without the server learning anything: the payload names the box (the push service already
      knows it; the payload is encrypted to the device); the device keeps box → {icon, who, about, url} in its
      IndexedDB when it subscribes (watched()), and the service worker reads that to say "💬 Anna" /
      "🔒 An answer to “Coffee at …”" and open that thread.
- [x] 83b. Done: private-answer threads (read + reply) moved to /activity (static/answers.js, polled every minute,
      #answer-<id> scrolls and focuses); the area keeps a "🔒 Private answers →" link with the new count. Was: The mailbox (grow /activity into it): threads by channel and person, newest first, unread marked;
      private answers move off the home page into it (a small "🔒 n new" link stays on the area).
- [x] 83c. Done: "💬 Chats" in the mailbox: contacts you've written with, newest first, last line, the same chat
      (device.chatPanel, now shared with the contact card) to read and answer in place; the 10 newest are read on
      open; Activity's message items open #chat-<id> there. "🔔 Recent" heads the rest. Was: Chats: each contact's messages as a mailbox thread (still opening the chat on the contact).
- [x] 83d. Done: the push payload names its box; pair.labelled() composes each box's line on the device ("💬 A
      message from Anna", "☕ Anna sent you something", "🔒 A private answer to “…”", invite, inbox; 24 languages) and
      where it opens (#chat-, #answer-, #c-); saveLabels keeps them in IndexedDB for sw.js; one notification per
      conversation; a tap reuses an open window. Also fixed a test race (business sync wait passed before the POST).
      Was: Notifications: every push names the sender/area and a line of context, and opens that thread.

## 82. Answer a newcomer's questions up front (Joop 2026-10-13, a friend's questions: "a local paper, then?",
"can concerts, fairs, festivals go in?")
- [x] 82a. Done: a closable card under the area name: "Your area's notice board: what's on around here (concerts,
      markets, meetups) and what people here are saying. Anyone can add to it." + 💬 Say something / ＋ Host a meetup;
      ✕ remembers (kafumu.introSeen). Sources get a mention once 81 exists. Was: The area page says what it is in one line before anything else (the digital town crier: what's on
      and what people say around here, events included) and where to add an event / a source, without a tour.
      Look at it as that friend would at 390 px; 24 languages.

## 81. Local sources per area (Joop 2026-10-13: "I want people to be able to care about their local area")
Kafumu can't know what an area cares about; its people can. Sources are added by named accounts with a
passkey, always bound to an area, carry an OLN value (mined, sorted by it), capped per account, and are pulled
only for areas people actually look at. Each slice passes one test: does the area feel more alive to a local?
- [x] 81a. Done: /rss/{cell} (the cell's public lines without reactions/private, meetups within a ring, newest
      first, ≤ 50, 10 min cache, bots 404) + 📅 Calendar (webcal:// /cal/{cell}.ics) and 📰 RSS buttons, and a
      <link rel=alternate> on the area page. Tests: TestMeetupFlow, browser step. Was: Out: an RSS feed per area (`/rss/{cell}`: its local lines and meetups, from the cached bundles),
      and two buttons on each area like Eventa Servo's per country: 📅 Calendar (the existing /cal/{cell}) and
      📰 RSS. 24 languages.
- [x] 81b. Done: importer.ParseFeed/FetchFeed (RSS 2.0, RSS 1.0, Atom; HTML stripped, ≤ 30 items, GUID or link
      for dedupe), checked on NOS, Guardian and GitHub Atom feeds. The SSRF guard already existed (dial-time
      check after DNS, redirects ≤ 4, http(s) only, 12 s, 3 MB); added carrier-grade NAT 100.64/10. Was: In, parsing: importer.ParseFeed (RSS 2.0 + Atom: title, link, date, guid), tests. Fetching
      user-supplied URLs needs an SSRF-safe client (http/https only, no loopback/private/link-local/metadata
      addresses after DNS, also on redirects; timeout, body cap) — importer.Get has none of that yet.
- [x] 81c. Server half done: a line tagged #feed with a link is a source; PostAs refuses it (403, ErrSource) unless
      posted under a name by a named account with a passkey (Service.SourceOK) in an area (never 000000); Note.Src
      (indexed) names the adder; ≤ MaxSources (20) live per account (ErrSourceCap). All three stores (Datastore,
      memory, SQLite via json). Test: TestSources. Was: A source is an OLN line in the area with the URL and `#feed` (no new entity, rule 5): the server
      accepts `#feed` only from a signed-in named account with a passkey, a loose cap per account (20 live; a
      degree of spam is wanted, Joop: the value sorts it, not the gate), cell required (never 000000). "📰 Add a source for this area" in the composer; listed on the area page with
      who added it. oln.html: a source lives as long as its line (renewable), nothing permanent. 24 languages.
- [x] 81c-ui. Done: "📰 Add a source" among the area's minor buttons opens a card: link + what it is → "📰 <name>
      <url>" #geo… #feed, posted under your name (named passkey accounts; others see what it takes + Account link);
      source lines are marked "📰 source" in the feed. Browser test covers the refusal path only (no passkey there).
- [x] 81c-ui2. Done: sources mined for a day or a week (default week: +8 bits); Account → "📰 Your sources" (kept on
      the device): area, until when, ↻ Renew (mined again; refused within a day of the last one), Remove (POST
      /api/oln/source/remove, only the account that added it: TestSources); oln.html documents #feed.
      link + a few words → the #feed line, posted under your name; sources show on the area as "📰 <title> · via
      <host> · added by @name"; your own sources listed on Account with remove; oln.html: a source lives as long as
      its line. 24 languages.
- [x] 81c2. Done: sources are reported with the ⚑ every line has; on /admin a reported source shows "Stop @name
      adding sources" and "Stop that network (90 days)", each also taking the source down. Note.SrcNet = HMAC of the
      IPv4 address or IPv6 /64 under a node key (never the address); stops live in the hidden set ("nosrc:",
      "net:<hash>:<until>", "netkey:"), which the OLN purge now leaves alone. Test: TestStopSources. Brand admins:
      not yet (their queue is the admin's for now).
- [x] 81d. Done: the bundle notes its cell in the shared cache ("seen-cells", ≤ once an hour per cell per instance,
      3 days; bots never get a bundle). The feeds job reads the sources (#feed lines) of seen cells and their first
      ring (≤ 200 cells, ≤ 40 sources, ≤ 5 per cell per run): RSS/Atom → up to 10 items per source, 7 days back,
      cached per cell for 12 h and sent with the bundle as "news" (title, link, host, who added it, the source line's
      bits); a calendar (ICS or JSON-LD) goes through the usual feed sync into meetups. News items are cache-only:
      not OLN lines, nothing stored. Test: TestPullSources.
- [x] 81e. Done: a "News" kind in the one feed (and its filter chip): "📰 host · added by @name · when", the title as
      a link out, its summary, link reactions (79a: reacting ties to the site), ⚑ reports the source line; ranked by
      the source line's bits under what locals say, fresher first. Checked by hand with injected news (390 px) — no
      browser test: adding a source needs a passkey, and the server rightly won't fetch a local test feed.
- [x] 81c3. Sources also from named accounts with any role (host, moderator, admin), not only with a passkey (Joop).
- [x] 81g. Done: the first reaction (quick or 💬) to a news item or a Bluesky post (now reacted to as its link) also
      posts the item as a link line in its area ("📰 title" / "🦋 @handle: line" + link, #re<link id>, #site_…), unless a
      line with that link is there; the news card then gives way to the line. Checked by hand with injected news; the
      browser step runs when the test area has Bluesky posts. Reactions to the link now keep that line alive too
      (keepAlive follows lines with the same #re that carry a link; TestKeepAliveLink).
- [x] 81h. Done: Service.RenewVoted, run per area by the feeds job (every 6 h): of an area's sources, the 10 with the
      most reactions (#re to the source line) in the last week live at least another week; unreacted ones fade as
      before. Reaction counts already show under the line. oln.html says so. Test: TestRenewVoted.
- [x] 81f. Done: Bluesky posts carry their links (facets); building a bundle counts the sites its posts link to (≤ once
      an hour per cell per instance, platforms and shorteners never count); the feeds job looks on the most shared
      ones (≥ 3 posts) for a feed via <link rel=alternate> (≤ 10 new probes per run through the guarded client, cached
      a week, "no feed" too) and keeps ≤ 3 suggestions per area for a day; the area shows "📰 People here often share
      X. Add its news to this area?" → the source form, prefilled (✕ remembers per site). Tests: TestSuggestSources;
      the card checked by hand.

## 84. The area on a public screen (Joop 2026-10-13: for displays in public places; "a different view of the
local page", configurable, but nothing added to the main view)
A separate page, `/screen?cell=…`, showing the area's own content (the same bundle, filters and ☆ views as the
area page) laid out for a physical screen. Two versions: **display** (not interactive) and **touch** (an
interactive public screen). Configurable on the screen page itself, never on the main area page: the config lives
in the URL (+ the screen device's storage), the server stores nothing for a screen.
- [x] 84a. Done: /screen?cell=…[&speed=s]: a standalone page (no navigation) reading the area's bundle (cell + ring):
      meetups of today/this week first, then local lines, news, a few Bluesky posts; cards fill the screen (columns
      from its width, as many as fit without scrolling) and page every 12 s; place name, clock, and a corner with a QR
      code + link to the area ("Add something here"); refreshes every 10 min; wake lock, fullscreen on first tap.
      Checked at 1920×1080 and a portrait tablet; browser step.
- [x] 84b. Done: ?mode=touch: cards are tappable (and keyboard-reachable) and open large with the full text and a QR
      code to take it along (meetup, /n/<id>, news or Bluesky link); ‹ › page by hand; paging pauses under an open
      card and runs at half speed; a minute without a touch closes it and goes back to the first page. Browser step.
- [x] 84c. Done: ⚙ on /screen (faint corner; opens by itself without a cell): area (#geo, plus code or lat,lon;
      default your last area), display/touch, which kinds, seconds per page, style (auto/light/dark/high contrast/large
      print), your own CSS; → the link + its QR code to open on the screen device (nothing stored). Browser step.
      Was: Configure (a ⚙ panel on /screen itself): area, view (filters / a ☆ view), display or touch, paging speed,
      a list of styles (light, dark, high contrast, large print, brand colours) and custom CSS; the result is a
      link (+ QR) to open on the screen device.
- [x] 84d. Done: "📺 Public screen" in the footer (to the current area's screen); nothing added to the area page.
      Was: Where to find it without touching the main view: a "📺 Public screen" link in the footer and on /about.

- [x] Polish: a contact is called what you call them (alias first, then their card's name, else "someone new",
      never "?"): dev.nameOf in the share picker, signals, coffee joins, mailbox and notifications. The intro card
      lost its two buttons (they repeat the ones right below).

## 87. Every page explains itself (Joop 2026-10-13)
The newcomer bar (82a) on every page, each with its own one line (Around, Connect, Contacts/mailbox, My card,
Findable, Account, Business, meetups…), and with a "👀 Show me" button that starts the tour for that page (the
😕 I'm confused button moves there, off the area's button rows). × closes a page's bar (remembered per page); a
"?" in the top bar brings it back.
- [x] 87a. Done: page.Help + the bar in base.html for Around, Connect, Contacts, My card, Activity, Account, Findable
      (help.* in 24 languages); × per page (kafumu.helpSeen.<page>), ? in the header brings it back; "👀 Show me" (the
      old 😕 tour button) sits on Around's bar. Was: The bar in base.html for every tab with a line (home's text stays), × per page, "?" in the header
      reopens it; "👀 Show me" starts the existing tour where there is one; 😕 leaves the area's rows. 24 languages.
- [x] 87b. Done: "👀 Show me" on the bars of Connect, Contacts, My card and Activity runs a short tour of that page
      (app.js, same .tour look as Around's); 24 languages; browser step on Contacts. Also: the contacts list's "quiet
      for 90+ days" view is no longer remembered (an old saved one counts as everyone). Was: Tours for the pages that don't have one yet (only where a tour adds something to the line).

## 88. Joop 2026-10-13 (more)
- [x] 88a. Done (Joop: phones get a chat page, desktops dock and pop out): static/chatdock.js on every page; chat
      links are /chat?k=c:<id> / a:<id>; on ≥800 px they open as docked windows (kept across pages in localStorage,
      ⤢ pops one out to its page), on a phone the link opens /chat (← 🔔 back to the mailbox, sized to the screen);
      a tapped notification with Kafumu open asks the page (postMessage) to open the chat in place. 24 languages.
- [x] 88b. Done (simpler than a signature): every line sent from this device carries #wd<16 hex> = SHA-256 of a
      secret kept in myPosts; POST /api/oln/withdraw?id&secret hides it (no account; TestWithdraw); "✕ Take back" on
      your own lines (says, questions, coffee) and next to "coffee sent", which also stops listening; #wd is never
      a subject; oln.html documents it. 24 languages.

- [x] 81i. Done: POST /api/oln/source/pull?id (SourceOK; once per 10 min per source) reads that source now: news merged
      into its area's cache, a calendar synced into meetups; the device calls it right after adding. TestPullSourceNow.
      Was: Read a source right away (Joop, adding Rostos): after "Add a source", the device asks the node to read
      that one source now (POST /api/oln/source/pull?id=…, same rules as adding: named + passkey/role), so its
      news shows at once instead of after the next feeds run.
- [x] 81j. Done: source lines leave "Here now": each is a compact "📰 source · host · added by @name" + its name as a
      link, reactions (its votes) folded behind 😊, at the head of its news; a chip per source next to the kinds shows
      just that source's news. Checked by hand at 390 px. Was: Sources as filters (Joop): the source line isn't a big message in "Here now" but a compact "📰 Rostos ·
      rostos.pt · @name" at the head of its news (reactions there are the votes, 81h); a chip per source next to
      Meetups / News / Bluesky shows just its news — for finding local events while travelling.

- [x] Polish: an area's news puts local stories first: items mentioning one of the area's places (its place tags
      and nearest name, accents folded, unambiguous, ≥ 4 letters) rank higher and say "📍 Barreiro"; the rest a little
      lower (Joop: Rostos also carries EU pages). The News chip counts news, not the sources' heads. (The touch
      screen's corner QR was dimmed after all: checked.)

- [x] Polish (Joop: the phone chat scrolled and wasn't stable): the chat page is a fixed screen: header on top, the
      input at the bottom, only the messages scroll; with the keyboard open (visualViewport) the input sits right above
      it and the tab bar steps aside; no footer or bars under it. Checked at 390 px.

- [x] Polish (Joop): the bottom tab bar ran off the right edge (the profile tab cut off): the six tabs now share the
      width (no sideways scroll), long names end in …; checked at 360 px. IPTV: dropped unless it needs no server of
      Joop's (it does: ffmpeg/Chromium aren't on App Engine) — not planned.

- [x] Polish (Joop: the tour box was see-through on a phone and blocked the view): opaque in its own right, smaller;
      the thing it points at scrolls near the top, and the box sits at the bottom, or at the top when that thing is
      in the lower half (the tab bar); its step shows again. Checked at 390 px.

- [x] Joop: Rostos showed only its feed link. It had been added before sources were read on the spot, and no feeds
      run had read it yet. Now an area whose source has none of its news in the bundle asks the node to read it (anyone
      may, not bots; once per 10 min per source); the source's head links to the site, not the feed file.

- [x] Sources reach further (Joop): a source's head offers "📍 Also in my area" (your home area, when it's another)
      and "🗺 Another area…" (a #geo code, plus code or lat,lon): the same source line posted there under your name,
      mined for a week, read at once; only for those who may add sources (body data-sources). 24 languages.

## 89. Four tabs instead of six (Joop 2026-10-14: "tab proposal sounds good")
Around · Chats (mailbox + Contacts, 🔔 folded in) · Connect (your code + My card) · Me (Account + Findable).
- [x] 89a. Done (first step): the Contacts tab is now "Chats" (→ the mailbox, highlighted on /contacts too) with the
      unread count on the tab; the header 🔔 is gone; the Chats page links "👥 Contacts →" (cards, backup, sync stay
      there for now); tours find the first visible match (tabs are in the page twice). 24 languages.
      Was: Chats: /contacts and /activity become one page (the chat list with a Contacts filter, contact cards
      reachable from it, backup/sync tools on it); the 🔔 in the header goes, its count moves to the Chats tab.
- [x] 89b. Done (as one tab): My card's tab merged into Connect; a "📷 My code · 🪪 My card" switch on both pages.
      Was: Connect: your code and your card on one page (card editor under the code, the share picker between).
- [x] 89c. Done (as one tab): Findable's tab merged into Me (your @name / Sign in); "👤 Account · 👁 Findable" switch.
      Tabs now: Around · Chats · Connect · Me. Tour text about the tabs rewritten, 24 languages. Was: Me: Account and Findable as sections of one page; the tab shows your @name / business as now.
- [x] 89d. Done: every old address is still its own page (nothing to redirect); help bars follow each page; the Chats
      page's title says Chats; tours find the visible tab; the 390 px check passes (browser test). Was: Old addresses keep working (redirects), tours and help bars follow, 390 px check, 24 languages.

- [x] Polish (Joop, phones): the header made pages wider than the screen when ⚙ / ? / ⓘ were all there (377 px on a
      360 px phone): items no longer grow it, the language menu gives way; long code wraps. Measured on 12 pages at
      360 px. Quiet sources (O Setubalense's Barreiro page: nothing this week) show their newest 3 of the last month
      (TestQuietSource).

- [x] Fix (Joop, Azeitão: sources shown, no news): an area shows sources from all its rings but read news only for its
      9 nearest cells. Now the bundle (and the wide one) read the news of exactly the sources in view, from their own
      cells (Home.NewsFor), a busy area never cuts its sources off, and news of a source that's gone goes with it.
      TestNewsForNeighbours.

## 91. The area page, much simpler on a phone (Joop 2026-10-14, screenshot: "almost only buttons, very confusing")
- [x] 91a. Done: views, tag filters, kinds/sources and the mailbox filters are one line each that scrolls sideways;
      a wrapper (app.js) shows a fade with › / ‹ while there's more that way, re-measured as chips come and go.
      Checked at 390 px. Was: Chip rows (filters, kinds, sources) each one line that scrolls sideways, with a visible hint (a fade
      and a › at the edge) that there's more.
- [x] 91b. Done: a text field on top (placeholder fits the moment) with 💬 Say · ❓ Ask · ☕ Coffee? under it, one row:
      with text they send it as that kind (coffee: your words + your connect code), empty they open that kind's form
      as before; 24 languages; browser step. Was: The three big buttons become one text field on top ("Say something here…") with send buttons next to
      it: 💬 Say · ❓ Ask · ☕ Coffee (the text optional for coffee); the composer's options fold under it.
- [x] 91c-0. The tag row's ✎ / 🗣+ / #+ come first on its line (Joop: hard to find); an event card (Web Summit) has ✕
      (remembered per device).
- [x] 91c. Done: ✎ after the place name, ↗ after the #geo line, 📅 / 📰 as icons in the heading's corner; those four
      left the button row (host a meetup, add a source, be findable remain until 91e). Checked at 390 px.
      Was: The small buttons find their place (Joop): ✎ right after the big place name changes the area; share right
      after the #geo tag line; 📅 calendar and 📰 RSS as small icons in the heading's top-right corner (like Eventa
      Servo); the row of small buttons goes.
- [x] 91e. Done (first step): under the field on top, "📅 Host a meetup" (the field's text becomes its title, the
      area comes along) and "📰 Add a source" (a link typed on top becomes the source); "be findable" moved into the
      coffee card with a clear line (coffee now, or findable for a while); the separate button row is gone. 24 languages.
      A fully switching form (kinds changing the form in place) stays open. Was: One post interface (Joop): the field on top gets a kind switch that changes the form below it: a line, a
      question, ☕ coffee / be findable here (merged, with a clear "let people here meet you, now or later" text), a
      meetup (host one: the meetup form in place), a source (add one). The separate "Host a meetup", "Add a source"
      and "Be findable here too" buttons go.
- [x] 91f. Done: ✎ turns the heading into the place search: the name makes way for the field (its text to start),
      📍 next to it, results and the map right under it; picking closes it; Esc too. The view settings (strength,
      save as ☆) open from a ⚙ at the start of the filter row. Checked at 390 px. Was: The area search in place (Joop): ✎ turns the place name itself into the place search field (same look as
      the heading), results right under it; the separate area-picker card goes.
- [x] 91d. Done: "🔍 Search here… or #tag" right under the field on top (outlined, not another field to write in): words
      narrow the list as you type (accents ignored), "#tag" or a language name + Enter becomes the area's filter; the
      tag row follows it, the event and travel cards come after. 24 languages; browser step. Was: A search box near the top: words filter the feed as you type, a #tag or language becomes the view filter.

## 90. Filters reach everything (Joop 2026-10-14)
- [x] 90a. Done: filterBundle marks news (its source's tags, or the words of title/summary for a tag; a language
      filter needs the source's language), matched news ranks up like other items; the wide bundle carries news, so a
      filter that leaves the area empty finds news further out. Was: Tag and language filters apply to news from sources too (title, summary, the source line's tags), and when a
      filter leaves the area (nearly) empty, the wider rings bring news too (the wide bundle carries news).
- [x] 90b. Done (first step): GET /api/online?tag= serves the online events of a tag from a list of online calendars
      per tag; Eventa Servo's online calendar is simply the one for #esperanto (/api/esperanto/online stays as an alias).
      The area shows "🌐 Online: #tag" for whatever tag you filter on (or a language that maps to one: epo →
      esperanto), each with its host and reactions. Next step: people adding online calendars per tag (sources
      without an area, filed by subject, same rules as 81). Was: Online events per tag, everywhere: like lines about a subject (78a), events without a place are filed by
      their tags and shown to anyone filtering on that tag; Eventa Servo's online events fit in there instead of
      being a special case.
- [x] 90c. Done: checked: your own words (a line, a question, coffee, from the field on top or the composer) already
      carry #lang of the site's language unless you pick another. Sources and kept news/Bluesky items aren't your
      words, so they don't get your language: a feed's own language (RSS <language>, Atom xml:lang) now comes with
      its news, and a language filter uses it. Was: The site's language rides along as #lang<xyz> on what you post (unless you pick another), so language
      isn't a special step: check the composer, coffee, questions and sources.

## 85. Every conversation as a chat (Joop 2026-10-13)
One list of chats, like a messenger, merging the mailbox and Contacts (filter: "only contacts"). Every thing that
talks to you is a thread: a contact, a private answer, your post (its replies and reactions), your meetup (RSVPs,
changes), a share ("👀 Did you see this?"): one-way when it can't be answered (its notifications collected in
order), two-way when it can, always in the same bubble format. Contacts are always in the list (also without
messages), and the chat moves there fully from the contact card. Mobile: a standard full-screen chat window;
desktop: chat windows docked at the bottom.
- [x] 85a. Done: static/threads.js build() (pure; test/threads_test.mjs): contacts always (quiet ones last), messages +
      signals + "connected" in one two-way thread, answers two-way, inbox and notices one-way grouped per thing
      (Activity's replies/meetups/Bluesky items now carry thread/kind/title). Was: The model: one thread list built on the device from contacts, answers, myPosts, myMeetups, shares;
      each thread {kind, who, title, twoWay, items[]}, rendered as bubbles. Unit test.
- [x] 85b. Done: /activity is one chat list (threads.js): rows with title, last line, time, unread count; filters
      All / Contacts / Unread; a row opens in place: contacts and private answers as the same two-way chat
      (device.chatPanel, now with opts.save), posts/meetups/Bluesky as one-way bubbles + "Open ↗"; #chat-/#answer-/
      #answers deep links open and focus. Old answers.js and the separate lists are gone. Was: The list page (mailbox + contacts merged; "Contacts" filter; the contact card reachable from a thread).
- [x] 85c. Done: an open thread is a .chat-window: full screen with ← on a phone (body scroll locked, only the
      newest open), docked at the bottom on ≥800 px (340 px wide, side by side from the right, oldest closes when
      they don't fit); title + 🪪 Card in its head, typing at the bottom, Esc closes; checked at 390 and 1280 px.
      Was: Mobile full-screen chat window with back; desktop docked windows at the bottom.
- [x] 85d. Done: a contact's chat shows signals (📶) and when you connected (🤝) as small notes in time order among
      the messages (shares were already messages, with their link); opening it clears unread signals too; the
      contact card's 💬 is now a link to /activity#chat-<id> (with the unread count). Was: Shares and every other activity as bubbles in their thread; the contact card loses its chat.

## 86. Permalinks for everything (Joop 2026-10-13)
Everything shown on the site has a shareable permalink, and a shared thing carries it: a line, a reaction, a
question, a meetup, an answer. Opening it shows the thing in its area, highlighted, with its reactions and answer
buttons (so a "👀 Did you see this?" to a contact arrives with where it was and how to react).
- [x] 86a. Inventory (2026-10-13): had links: areas (/?cell=), meetups (/meetups/id), online/Luma events and link
      lines (their own URL), named people and businesses (/@name), #re-<id> scrolling to a line in its area. Lacked:
      local lines (shared as just the area), Bluesky posts and people in the feed (no 📤 at all), reactions/replies
      and questions (no share; replies are under their line, so its link covers them). Was: Inventory: what has a link now (meetups /meetups/id, areas /?cell=, #re-<id>) and what doesn't.
- [x] 86c. Done: GET /n/<id> (40 hex; bots 404): live → 302 to /?cell=…#re-<id>; expired but still stored → its
      text marked expired + why lines fade + /oln; gone, hidden or private → 404 with the same explanation. Shares
      (👀, 🔗, carried 📍) of a line now carry /n/<id>. The service worker leaves /n/ and /rss/ alone. Tests:
      TestShowNote, browser step following a carried link. Was: /n/<id> (Joop): a line's own address. Live → its area at the line. Expired but still stored (Datastore's
      TTL deletes late) or in a cache → shown as "expired" with its text. Gone → a friendly "this message has expired"
      page explaining how OLN works (lines fade; the network keeps nothing forever) with a link to /oln.
- [x] 86d. Done: Store.ByPrefix (Datastore key range, memory, SQLite) + Service.keepAlive: a #re line that outlives
      the line it answers extends it (an expired one still stored comes back; private lines never); 🔁 on every
      local line sends that, mined for a day, and counts like a reaction; oln.html says so. Tests: TestKeepAlive,
      browser step. Was: "↻ Send again" on a line (yours or anyone's): a new line that refers to it (#re<id> + its text as context),
      so it lives on; policy: a line stays retrievable (/n/<id>, replies) while a live line refers to it, so a
      chain is kept alive by its newest link (purge skips referenced lines; Datastore TTL: extend expires_at of the
      referred line when a reference arrives).
- [x] 86b. Done: a line's link is its area at the line (/?cell=…#re-<id>: scrolls there and glows, with its reactions);
      Bluesky posts (their bsky.app URL) and named people (/@name) get 📤 too; "🔗 Link" in every 📤 group (share sheet,
      else copied). /n/<id> skipped: a line lives ≤ 7 days and its area link already says where. Was: A line's permalink (`/n/<id>` → its area, highlighted) + "🔗 Link" in every item's share row; shares
      carry it.

- [x] 91g. A pass over every page for the same kind of thing (Joop): controls next to what they act on, separate
      cards and panels that can be the thing itself in place, near-duplicate buttons merged; one page per tick.
      Done: Contacts (a contact's tags, note and Remove fold behind ✎ in its head; a note you wrote shows as a
      line); Chats (one "Contacts": the filter leads to the cards, notes and backup). Connect (Joop: the QR is the short code by default,
      smaller and easier to scan, renewed before its hour ends; "⏱ 1 hour · ∞ until a new code" replaces the "make a
      short code" button). My card (the preview first; ways to reach you
      show when filled in, the rest as "＋ Email"… chips, no wall of 13 empty fields). Me (no second "Be findable →" next to the switch;
      Findable's main choice, findable or not and how long, moved from the bottom to the top). a meetup (its place by name, "📍 Barreiro",
      linking to the area, not "#geo8ccgmw"). Pass done for the main pages; the rest as they come up.

- [x] Fix (overnight check): the 06:00 feeds run read 0 sources (3 at midnight): the "seen cells" list lived only in
      memcache, which can drop it. Now kept in Datastore too (Config/seen-cells), like the seen countries, and read
      back into the cache when it's gone.

- [x] Polish (Joop: the › hint did nothing on a computer): ‹ › are real buttons that move the row most of its width,
      and the mouse wheel scrolls a chip row sideways while you point at it (until its end, then the page scrolls).

- [x] Polish (Joop: no way to find a message's address): every local line shows 🔗 at the end of its info line
      (its /n/<id>; a tap copies it, ctrl/long-press opens it). Browser step.

- [x] Fix (Joop: a taken-back coffee couldn't be resent, "this message is already here"): the repeat check now
      remembers which line said it and lets it through when that line was taken back (asked of the store, so it
      holds across instances). Go test.

## 92. Look, access, media (Joop 2026-10-14; after 91 and 90; the last item lowest)
- [x] 92a. Done: Me → 🎨 Look (for everyone, per device): 🌗 automatic (follows the device) · ☀️ light · 🌙 dark · ◐
      high contrast (black, white borders, yellow accent, underlined links, a strong focus ring); html[data-theme] is
      set before the page draws. 24 languages; checked dark and contrast at 390 px. Was: Theming: light / dark / automatic (follows the device) / high contrast, chosen per device (Me), applied
      everywhere via the color tokens.
- [x] 92b. Done (first pass): a visible focus ring in every theme, a "Skip to content" link (first on every page), main
      focusable; Esc closes the tour or the open panel (through its own ✕); every control already had a name (the
      390 px check counts them). Browser step. More as we find it. Was: Accessibility: keyboard use everywhere (focus visible, tab order, Esc closes, no keyboard traps) and screen
      readers (names, roles, live regions for what changes); an audit pass with fixes.
- [x] 92c. Done: Bluesky posts carry their pictures and a video's still (embed views, also in quotes), shown in a grid
      (lazy, with alt text, ▶ on videos, opening the post); a line linking a picture shows it, a YouTube link its still
      with ▶ (only the still loads); Account → Look: 🖼 show / hide, per device; the privacy page says where pictures
      load from. Posting = putting such a link in a line. 24 languages. Was: Pictures and videos: show or hide them (per device), and post them through services we can embed
      (links to Bluesky images, PeerTube, YouTube… shown as embeds when on).
- [x] 92d. Done (simple form): a "🖼 Pictures and videos n" chip next to the kinds shows only the cards with
      pictures, bigger, one after another (27 around Barreiro/Lisbon when checked). Was: (lowest) A picture/video view of an area: only the photos and videos people share there (Bluesky and the
      like), swiped through one by one.

## 93. A post's replies as a group chat (Joop 2026-10-14: "for events that enables really interesting interactions")
- [x] 93a. Done: a card with replies shows "💬 n · Open as chat": its public replies as a chat window (docked on a
      computer, /chat?k=r:<id> on a phone), oldest first, reactions as small notes, refreshed every 20 s while open;
      writing posts a reply (#re, as you unless anonymous). The card itself keeps the newest 3. Browser step. Was: "💬 n" on a line, a meetup or an event opens its public replies (#re) as a chat window (the same window
      as private chats: docked on a computer, its own page on a phone), oldest first, refreshing while open; writing
      in it posts a reply (#re, mined, as you or anonymous), reactions as small notes in between.
- [x] 93b. Done: a meetup's page has "💬 Chat with the people coming" (its public replies, inline, said to be
      public); meetup cards in Around show 💬 Open as chat also before anyone wrote. Each line lives its own OLN time
      (not yet tied to the meetup's end). Browser steps (A writes on the page, B reads from the card). Was: Meetups and events: the window is where people coming talk ("I'm at the door", "running late"); a meetup's
      page shows it too; it lives as long as the meetup (replies kept alive by the thread, 86d).

## 94. Cards with few, obvious controls (Joop 2026-10-14; before 93)
- [x] 94a. Done: 📍 I'm here! opens how long + where to find you right on the area page (POST /account/here; the
      rest of the profile stays; ⚙ More settings → /findable), then folds into "👁 Findable here until …  Change";
      ☕ beside it sends the coffee line at once (your words if typed). Signed-in save not browser-tested yet. Was: it opens the findability settings right on the area page (how
      long, what people see), with a small "☕ Coffee?" at its right that just sends the coffee line; once set, it
      folds into a one-line shortcut ("👁 Findable here until 18:00 · change"), and coffee stays one tap.
- [x] 94b. Done: local lines, Bluesky posts and the other reactable cards carry a reply box (type → it answers
      there, as you unless you chose anonymous); 📤's 👀/🤝 send what you typed as your comment; Hide, ⚑ and 🔁 behind ⋯
      on lines and posts. Browser step. Was: Each card: a reply box that's simply there (type → it answers; quick reactions next to it), "📤 to a
      friend" sends the card with what you typed as your comment; hide, report and the rest behind one ⋯.

## 95. Coffee goes to your contacts first (Joop 2026-10-14; before 93)
- [x] 95a. Done: "📍 Your contacts can see you're around here today: the area, not your exact spot" + ⏸ Pause /
      ▶ Share again (per device; nothing shared without a location fix); ☕ Coffee? on a contact around today goes to
      just them; the ☕ broadcast is gone (brands keep their own public call button). Was: (Joop: no broadcast; transparency so people can opt in to coffee when close.) Your contacts see the
      area you're in today (the existing day/cell beacons, only from a location fix), said plainly on the area page
      with ⏸ pause; a contact around today shows ☕ Coffee? that goes to just them; the ☕ broadcast button goes.
- [x] 95b. Done: your own tags on contacts are the groups; the banner's "Share with" picks All contacts or one
      tag; contacts outside it get their slot emptied (they stop seeing you). Pair test. Was: Contact groups: tag contacts into groups on the device (Friends, Work…), and send the signal to one group.
- [x] 95c. Done: an area you shared from before goes on by itself (seeded from the device's check-in history);
      a new one asks first ("Tell your contacts you're around here today?" ✓ Yes / Not here); "not here" stays
      quiet there with ▶ Share here. Browser step with a faked location. (Per day, so no hourly refresh needed.) Was: Auto-signal (Joop's own default): on, the same area refreshes by itself (once an hour at most while the
      page is open there); a new area asks first ("Tell your contacts you're in Arnhem?").
- [x] 95d. Done: the banner came with 95a; ⏸ Pause now also empties what contacts could see (pair.clearSlots,
      once per contact), so "Your contacts don't see where you are" is true at once; ▶ writes it again. Pair test. Was: A clear banner while your contacts know where you last were ("📍 Your contacts see you around Barreiro
      since 14:00 · stop"); stop sends a "gone" signal.

## Open questions for Joop
- [x] 93c. Answered (Joop: "let them expire but not remove, and show in the meetup context until it's over; the
      event keeps a hold on GC"). Done: GET /api/meetups/{id}/chat returns the meetup's replies, expired ones too,
      until it ends (then only live ones; a minute per instance); purge skips expired replies to meetups that haven't
      ended; the meetup page and meetup cards read their chat through it. Lines still expire everywhere else. Go
      tests (oln RepliesHeld, purge on the emulator) + the browser steps. If held replies ever fill purge's first
      2000 expired lines, others wait a day: fine at today's scale.
- [x] Polish: an empty group chat now says "No messages yet" (was drawn blank).
- [x] Polish (idle tick): a card's controls were three rows (🔒 Answer privately / Reply… / reactions); now two:
      the reply box with a 🔒 beside it (label as title), then the reactions and ⋯. Checked on kafumu.com, Barreiro.
- [x] Polish (idle tick): source suggestions count different people sharing a site, not posts (vers2045.com was
      suggested in Barreiro from one account sharing its own site); still 3 needed. Go test.
- [x] Polish (idle tick): on a phone's first visit the install bar and the page's help bar stacked (~180 px); now one
      at a time: the install offer waits until the help bar is closed. Checked with an iPhone UA (not in the suite).
- [x] Polish (idle tick): narrow screens wrap a field with a long button (the Bluesky field read "you.bsky.soc"). Checked on kafumu.com.
- [x] Polish (idle tick): no more "Showing a shared cell:" / "Your area:" over the place name (the heading says it); after a location fix: "📍 Your location, rounded on this device to this area" (24 languages).
- [x] Fix (idle tick): meetup cards grew one "💬 Open as chat" per redraw (4 seen in Lisbon); now one. Browser check (before the click: on a phone the click leaves the page).
- [x] Polish (idle tick): meetup, news, source and people cards fold ⚑ behind ⋯ like lines and posts (one pattern on every card).
- [x] Polish (idle tick): before the place name loads (or where there is none), the heading is the #geo code on one line with ✎ after it (it broke mid-word, "#geo8ccgm / w", ✎ in front).
- [x] Polish (idle tick): the stray footer line "Only people you have connected with can see this… · Be findable here too" under the feed is gone (the where-you-are banner, 95a, says it in context; 📍 I am here! is the findable button).
- [x] Fix (idle tick): an empty public group chat said "Only you two can read them" (false); now "Public: anyone who opens this can read them" (24 languages). Its title is the card title, not date+title glued. Browser checks.
- [x] Polish (idle tick): replies under a card no longer carry their own reply box (reactions + a small 💬 instead); the card box and its chat do the talking. Browser check.
- [x] Polish (idle tick): the Look section (theme, pictures) showed on Findable too; now only on Account.
- [x] Polish (idle tick): Findable signed out no longer repeats the sign-in block; it says what it is and links "First pick a name for your account →" (/account?next=/findable).

## 96. CoffeeBucks (K$): Kafumu as a LiquidityPub node (Joop 2026-10-15)
Kafumu runs ~/liquiditypub's protocol as its money layer, alongside OLN ("fits well alongside OLN"). Decisions:
currency **CoffeeBucks, K$**; Universal Dividend (reference ~10%/year, daily); no external contact points yet;
regulation: go ahead, learn from use. Sybil answer (Joop: "pretend groups are separate nodes until they link up,
like Circles"): **groups are explicit** (made by someone, joined by invite link from a member = vouching), never
derived from the private contact graph (the server doesn't know pairs). Each group is its own K$ node with its own
dividend; two groups linking = a contact pool seeded at UD parity, price floating. A fake group mints only its own
K$, worth something to others only through a pool someone chose to open. Kafumu = a clearing house of group nodes
(PROTOCOL §12), internal clearing between them; one external identity at kafumu.com.
Shape: the node runs as its own GAE service `lp` (max 1 instance) in project lokumo; dispatch sends
/.well-known/liquiditypub and /lp/* to it; Kafumu's app stays as it is. Node key in Datastore, never in git.
Code in ~/liquiditypub is Joop's (github LaPingvino/liquiditypub): commit there with the same trailers.
- [x] 96a. Done: VISION rule 3 names the money layer as its one exception; §4 "Money: CoffeeBucks" with the
      checklist answers (privacy page waits for 96d/96g). Was: VISION.md: the money tier as a stated exception to rule 3 (a LiquidityPub ledger is public and permanent
      by design; accounts pseudonymous; groups explicit) + the add-a-layer answers; privacy page paragraph (only
      when 96d is live: say what exists).
- [x] 96b. Done: ~/liquiditypub/gae serves one node (CoffeeBucks K$, UD 274 ppm/day, pseudonymous) from app.yaml
      env (service lp, max 1 instance); conformance/v0.2.0 + node/v0.2.0 tagged on GitHub, go.work for local dev;
      builds standalone (GOWORK=off) as App Engine will; identity doc + signed checkpoint served locally. No
      persistence yet; admin token random per process (closed). Was: ~/liquiditypub: make node/ deployable as a GAE service (conformance importable: tag or vendor; a
      cmd/lpgae main that serves one or more hosted nodes under one base); runs locally.
- [x] 96c. Done: gae/dsstore.go: each ledger record its own entity (append-only), the rest of the snapshot in
      chunks under LPState (namespace lp) whose version makes one writer (a second exits and reloads); admin token
      made once and kept in LPConfig. Emulator tests: 600-record ledger survives a restart, chain intact; second
      writer gets ErrConflict; token kept. Was: ~/liquiditypub: a Datastore store: ledger records as append-only entities + one small state entity with
      a version check (single writer; the 1 MiB entity limit rules out one snapshot blob). Tests on the emulator.
- [x] 96d. Done: service lp deployed from ~/liquiditypub/gae; dispatch.yaml sends /.well-known/liquiditypub and
      /lp/* to it (https://kafumu.com/.well-known/liquiditypub serves CoffeeBucks K$, its key kept across instances);
      cron.yaml: /cron/ud daily 00:05 (target lp; 404 from outside). Deploy: gcloud app deploy app.yaml in gae/,
      dispatch.yaml cron.yaml here. No members yet. Was: Live: service `lp` + dispatch.yaml + cron (daily dividend); the K$ identity document at
      kafumu.com/.well-known/liquiditypub. Nothing user-facing yet.
- 96e design (from reading ~/liquiditypub/node): one lp process can host many nodes (no globals; the federation
      Sender is an interface: Deliver, FetchIdentity/Outbox/Checkpoint). Contacts are keyed by the peer's host, so
      each group gets its own host as its base: https://<gid>.g.kafumu.com (an identifier, internal clearing only
      for now: an in-process Sender delivers envelopes straight to the other group's ProcessInbound, the full
      signed protocol without HTTP). Each group: its own DSStore id, key, K$ dividend. Public reads per group at
      /lp/g/<gid>/... A Kafumu account is a member as "u" + 16 hex of HMAC(server secret, user id): pseudonymous,
      derived, nothing stored that maps it back. Missing in the node: a local member-to-member payment (TxPayment
      exists, unused).
- [x] 96e1. Done: node.Pay (TxPayment between active members, balance checked, persisted, no note on the
      ledger), test; node/v0.2.1 tagged, gae requires it. Also fixed: gae restart test compared two zero balances
      (member prefix is m:), now asserts a real one. Was: ~/liquiditypub/node: Node.Pay(from, to, amount, note): a local payment between active members (balance
      checked, persisted), test; tag node/v0.2.1, gae requires it.
- [x] 96e2a. Done: gae/host.go: Host runs many group nodes (own store id g-<gid>, base https://<gid>.g.kafumu.com),
      in-process Sender, Link (contact seeded at UD parity), RunUD over all groups. Tests: two groups dividend + pay
      inside + link + pay across, settled; two nodes paying each other concurrently 30x under -race: all settle or
      refused, channel root/op seq/reserves agree, supply unchanged. That test found a real data race in the node
      (values read after Unlock): fixed, node/v0.2.2.
- [x] 96e2b. Done + deployed: LPGroup registry opened at start; /cron/ud pays the node and every group;
      /lp/g/<gid>/ public reads; admin API (Bearer token from LPConfig/coffeebucks, ns lp): group, member, balance,
      pay, link, transfer (+ state). HTTP test of the whole API; live: 401 without token, 404 unknown group, node key
      unchanged. Was: ~/liquiditypub/gae: wire Host into main (groups registry LPGroup; daily cron = Host.RunUD), /lp/g/<gid>/
      public reads, admin API for Kafumu (create group, add member, pay, link, cross-group transfer), deploy.
- [x] 96e2. (split into 96e2a/b) Was: ~/liquiditypub/gae: host many group nodes (LPGroup registry; in-process Sender; /lp/g/<gid>/ reads; an
      admin API for Kafumu: create group, add member, pay, link two groups = a contact seeded at UD parity,
      cross-group transfer). Test: two groups, dividend, link, a payment across, both ledgers verify.
- [x] 96e3a. Done (server, not deployed: no UI yet): internal/coffee client (admin token from LPConfig ns lp;
      Kafumu's own key in Config/coffee, made once); member id u+16 hex HMAC(user id); invite token HMAC(gid, inviter)
      — nothing stored; User.Groups (max 5); POST /coffee/group (named + passkey or role), GET /coffee/invite (members),
      POST /coffee/join (named; valid invite; not your own). KAFUMU_LP_BASE in app.yaml. Handler test against a fake lp.
- [x] 96e3b. Done + deployed: /coffee (your groups: name from the group's identity doc, balance in K$ and in days
      of dividend, today's dividend, members, share an invite link; start a group, gated) and /coffee/join (names the
      group, Join, or pick a name first; bad link says so); ☕ CoffeeBucks card on Me → Account; 18 strings x 24.
      Handler test renders both against a fake lp; live page checked with @claude-admin-test. Not exercised live: a
      real group (ledgers are permanent: Joop's first group is the first live round trip). Was: The page: /coffee (your groups, balance, dividend, invite link to copy/QR, start a group) and
      /coffee/join (which group, who invited, Join), linked from the Me tab; 24 languages; deploy.
- [x] 96e3. (split into 96e3a/b) Was: Kafumu: start a group, invite link, join (calls the lp admin API with the token from Datastore).
- [x] 96f1. Done: 💸 Pay @name in K$ on each group card (/coffee/pay: both in the group, amount 2 / 2.5 / 2,50,
      not yourself, enough K$; the ledger gets two member ids, never names); result line on the card; 9 strings x 24.
      Tests: ParseMoney/Money; pay paid/not_member/self/bad_amount/funds/non-member against the fake lp.
- [x] 96f2a. Done + deployed (lp): GET /lp/admin/links?gid= (linked groups with names), GET /lp/admin/quote
      (what the payee gets, PoolPrice on the live reserves); test: the settled transfer pays exactly the quote.
- [x] 96f2b. Done: someone in two groups links them (/coffee/link; "Link your groups" lists unlinked pairs);
      group cards say what they're linked with; paying @name who is in a linked group goes to /coffee/confirm (you pay,
      they receive, today's rate) then /coffee/across (the lp transfer; busy/funds/error said); 13 strings x 24.
      Test: link (only by a member of both), pay across → confirm with the quote → sent; outsider refused. Was: Kafumu: link your groups (a member of both), pay @name in a linked group: quote shown, then confirm;
      the transfer's state on the card; strings x 24.
- [x] 96f2. (split into 96f2a/b) Was: Across linked groups: link two groups (members of both), pay @name in another group at the pool's price,
      shown before paying (the lp transfer + its state).
- [x] 96f. (split into 96f1/f2) Was: Wallet on the Me tab: balance in K$ and in UD, dividends received, 💸 pay @name in your group; across
      groups at the pool's price, shown before paying.
- [x] 96g. Done: privacy "CoffeeBucks: the one thing kept for good" (public, permanent ledger; a pseudonymous id;
      the server can tell which id is yours and sees payments, like a bank; no purpose written down; deleting your
      account leaves the id with nothing linking it to you); /about "A local currency" + link; 4 strings x 24.
      (A first draft promised "leaving a group stops your dividend": no leave exists yet, so it says what's true.)
- 96h design (Joop 2026-10-15: "a group migration mechanism, a full payout to what becomes their account in a new
      group"; "when leaving completely, pick between destroy, redistribute in the next dividend, or move it all to
      someone else"; "destroy can be the default for unlinked accounts"). Already true: one account, up to 5 groups,
      a balance in each. Unlinked = an account deleted from Kafumu (its ledger id links to nobody any more).
- [x] 96h1. Done: node.Leave(member, destroy|redistribute|give|keep, to) then deactivate; issuance.burn and
      fund.redistribute (paid out equally with the next RunUD, remainder kept) named in PROTOCOL §9; tests (supply,
      shares, leavers get no dividend, no leaving twice, chain); node/v0.2.3, gae on it. Was: ~/liquiditypub/node: Burn(member) (balance back to issuance: supply shrinks; a published tx type);
      a redistribution fund (fund:redistribute) paid out equally to active members with the next RunUD (supply
      unchanged); Leave = deactivate (exists). Tests: conservation, supply, chain; tag node/v0.2.3.
- [x] 96h2. Done + deployed: POST /lp/admin/leave {gid,member,mode,to?}; POST /lp/admin/move (whole balance through
      the pool to your account in the other group, waits for SETTLED, then leave keep; not settled = still a member).
      Admin test: move empties and leaves, arrives; give; no leaving twice; no move to a missing account. Was: admin API: POST /lp/admin/leave {gid, member, mode: destroy|redistribute|give, to?};
      move = transfer the whole balance to your own member id in a linked group, then leave. Deploy.
- [x] 96h3. Done: "Leave this group" on each card → /coffee/leave: move to my account in a linked group I'm in
      (pool rate) / give it all to @name / share it out at the next dividend / destroy, an "I understand" tick; the
      group comes off the account; 13 strings x 24. Test: page, tick needed, give to a non-member refused, share and
      move call lp and drop the group. Not yet: rejoining a group you left (the node keeps you, inactive). Was: Kafumu: on a group card "Leave this group": move to <linked group you're in> / give it to @name /
      share it out at the next dividend / destroy it (a clear confirm); strings x 24.
- [x] 96h4. Done: deleting an account (yourself, or purge after a year idle: purge.OnUserDeleted) destroys its
      balance in every group (coffee.DestroyAll: leave destroy); the delete confirmation says so when you're in a group;
      the privacy paragraph adds it; 2 strings x 24. Tests: handler delete sends destroy per group; purge (emulator)
      calls the hook for a named idle account in groups. Was: Kafumu: deleting an account destroys its balances in every group (the default for unlinked accounts),
      said on the delete confirmation and the privacy page.
- [x] 96i. Done + deployed (lp): node.Reactivate (+ MemberKnown), node/v0.2.4; /lp/admin/member reactivates an
      inactive member; Kafumu's join unchanged (leaving drops the group from the account, a fresh invite works).
      Tests: no dividend while gone, dividend after rejoining, no double activation. Was: Rejoin a group you left (Joop: "rejoining/reactivating sounds good"): node.Reactivate (the same
      member id, dividend again; balance as it was); lp /lp/admin/member reactivates an inactive member instead of
      refusing; Kafumu's join then just works (a fresh invite). Tests; tag node, deploy lp.
- [x] 96j. Done: the last member's leave page says the group goes quiet until someone joins with an invite and
      offers move or destroy only; give/share refused server-side when last (balance API now counts active members).
      Found on the way: the node's identity doc published active_members counting members who left (peers price
      dilution from it): fixed, node/v0.2.5 (the PHP node was right). String x 24, handler test.
- [x] 96k1. Done + deployed (lp): node.SetIdentity (node/v0.2.6); LPGroup keeps currency/symbol, applied at start;
      POST /lp/admin/rename {gid,name?,currency?,symbol?} (≤60/30/5, control chars dropped). Admin test.
- [x] 96k2. Done: ✎ Rename on each group card (any member: group name, currency name, symbol); every amount on
      the coffee pages in the group's own symbol (GroupInfo from its identity doc; template func amount fills
      "{amount} K$"); 8 strings x 24; handler test (symbol shown, non-member can't rename). Was: Kafumu: a rename form on each group card (any member); each group's own symbol instead of K$ (from its
      identity doc; "{amount} K$" in strings filled with amount + symbol); strings x 24.
- [x] 96k. (split into 96k1/k2) Rename your group, its currency and its symbol (Joop: "a nice bit of flair"; "any member can rename,
      that encourages small groups where you truly know and trust each other"; "keeps the whimsy in it, and makes a
      good argument about the 'is this real money' thing"): LPGroup keeps group name, currency name (≤30) and symbol
      (≤5); POST /lp/admin/rename {gid, name?, currency?, symbol?} sets them on the node (identity doc); any member
      renames from the group card; Kafumu shows each group's own symbol instead of a fixed K$; strings x 24.
- [x] 96m1. Done + deployed (lp): LPGroup.EmptySince (set on the last leave/move, cleared on a join); daily cron
      sweepGroups after 90 days: Host.Remove (links closed both sides, linked groups RetireReserve: node/v0.2.7,
      reserve.retire in PROTOCOL §9), DSStore.Delete, registry gone. Tests: Remove; sweep on the emulator.
- [x] 96m2. Done: privacy title/paragraph rewritten in 24 languages ("kept as long as the group lives"; 90 days
      with nobody in it and it's deleted; the deleted-account sentence kept), the last member's leave page says it,
      VISION's exception and checklist reworded. Was: Wording: VISION's exception and the privacy page ("public and permanent", "kept for good") become "kept as
      long as the group exists; 90 days with nobody in it and it's deleted"; the last member's leave page says so;
      strings x 24.
- [x] 96m. (split into 96m1/m2) A group nobody's in for 90 days is deleted (Joop: "give it 90 days for someone to return with an invite
      link, then garbage collect"): LPGroup.EmptySince (set when the last member leaves, cleared on a join); the daily
      cron closes the group's contacts first (contact.close: each linked group gets its own seed back), then deletes
      its LPState/LPStateChunk/LPRecord/LPGroup; leftover balances of members who left go with it. Wording: VISION's
      exception and the privacy page say "kept as long as the group exists; 90 days with nobody in it and it's gone";
      the last member's leave page says it too. Tests (emulator for the delete).
- [x] 96n1. Done (server; NOT deployed: Pay now redirects to /coffee/linkpay, which comes in 96n2): LinkAsk
      (expiring entity, a week; purged), User.CoffeeAsks; POST /coffee/linkask (payer in g, payee in tg), /linkaccept
      (only the payee: link the groups, then the payment, retried while the link opens), /linkdecline. Handler test.
- [x] 96n2. Done + deployed: /coffee/linkpay (plain words: members pay each other, each group puts a hundred days
      of dividend into a shared pot, newly issued, nobody's balance; the rate moves with payments; no undo yet) and
      "Ask @name to link and accept"; the payee's requests on /coffee (same words, Link and accept / Decline); 13
      strings x 24. Also fixed: group-card messages read the symbol from the message string ("paid" etc. would not
      render since 96k2): $g.Sym, and a test renders every card message. Was: The pages: /coffee/linkpay (what a link is, what it costs, the rate, undoing: plain words; "Ask @name to
      link and pay"), the payee's requests on /coffee (accept / decline, the same explanation); messages; strings x 24;
      then deploy.
- [x] 96n. (split into 96n1/n2) Linking from a payment (Joop: "first person tries a cross-group payment, it starts linking? might be the
      main way"; "be very clear about what the group linking does"): paying @name in a group not linked to yours
      offers "link our groups and pay"; the payee gets a request (Chats) to accept; accepting opens the pool (each
      group puts in a hundred days of dividend) and the payment goes through. Both sides see, before agreeing, in
      plain words: what a link is (two groups can pay each other), what it costs (each group puts a hundred days of
      dividend into a shared pot, its own money, nobody's balance), what moves the rate (payments), and that it can
      be undone later only by both. Today: linking needs one person in both groups ("Link your groups" on /coffee).
- [x] 96p. Done + deployed: the quote returns the pool's reserves (lp); impact = amount/(payer reserve + amount);
      the confirm page warns at ≥5% ("this payment gets N% less than the pot's current rate…"); every linking
      explanation adds "the pot can never run dry, but the bigger a payment, the worse its rate: a hundred days of
      dividend across gets about half"; 2 strings x 24; test. Was: The pot's cap, said plainly (Joop: "the pot system means there can be a cap on payments between groups"):
      constant product: a payment can never empty a pot, but the bigger it is the worse the rate (with a hundred days
      of dividend a side: paying 10 days' worth gets ~9.1 across, 100 days' worth only ~50). One sentence in the
      linking explanation; on the confirm page a clear notice when the quote is >5% below the pot's current rate.
- [ ] 96q. Send a group invite to a contact (Joop): next to "Share an invite link", "Send to a contact" lists your
      contacts (device-side, like 👀 "Did you see this?") and sends "☕ Join my CoffeeBucks group <name>: <link>" as a
      chat line through the pair (device.js/pair.js on /coffee); strings x 24.
- [ ] 96l. Payment requests (Joop): "ask @me for 3.50 in <group>" as a link and a QR code (/coffee/pay?g=&to=&a=&for=
      prefilled, a short note not stored on the ledger); scanning or opening it shows the pay form filled in (or the
      across confirm when the payer is in a linked group); strings x 24.

## Open design questions for Joop (not built)
- Unlinking, "pay without a link", weak links (Joop 2026-10-15): unlink by consent (reserve.adjust, both sides);
  "pay without link" = link, pay, unlink. Note: with today's protocol, unlinking means each group retires the
  reserve it holds for the other — so a link-pay-unlink would leave the payee's group having issued money to pay
  a stranger and keep nothing durable (a free loan from that group). Joop's guards: a weak link capped until N
  different members of a group have paid the same other group; weak links bias against the new group's trust unless
  the group votes it in. Needs: unlink by consent, per-link caps, a vote — design before building.
