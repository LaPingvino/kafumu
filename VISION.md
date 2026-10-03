# Kafumu / Coffee Buddies — Vision

*kafumu.com · draft 2, 2026-10-03 · for Joop*

## 1. Thesis

Kafumu answers one question: **"What is near me right now that I would want to know about?"** People you've met before who happen to be in town. A meetup in a language you speak or a hobby you have. What people around here are saying. And, if you want, a café that would like to host you. Open it where you are; it tells you who and what is around. Scan a friend once, and the next time you are both in the same part of a city it says so, and you message them on whatever you already use.

The insight, in Joop's words: *if you bring people the things they want to know based on location and tags, you have the foundation for an unintrusive alternative to advertising, that saves people money and keeps more people happy.* Most of the web's money is advertising, spent on surveillance to guess what you want. Kafumu doesn't guess. You tell it a coarse location and a few tags; the matching happens on your device; and the same matching serves friends, meetups and paid listings alike. Relevance is the product, privacy is what makes the relevance honest, and the business model is the same mechanism pointed at businesses.

Architecturally, Kafumu is **a lens, not a silo**, built the way ATproto is built: a small, stable, local-first foundation, and every feature as a layer on top that never changes the foundation, the way new apps add lexicons without touching the protocol. Private data lives on your device. Public data lives on public networks you control (ATproto/Bluesky). The Kafumu server is a thin, cacheable index keyed by `#geo` cell plus a tiny ephemeral mailbox for signals. It stores nothing it doesn't strictly need, which is GDPR compliance by construction: we can't leak or hand over what we don't have. It is also what keeps the thing running on a free tier.

This document is therefore in two halves: **the foundation** (sections 2–3: cells, tags, pairs, three data tiers, and the rules), which should barely change after M1, and **the layers** (section 4: home list, proximity, people, meetups, signalling, patrons, listings), each described as what it adds on top. Section 4 ends with the checklist for adding a layer.

### Lineage

Kafumu is the grown-up version of whenwhere.cf and OLN (2019): `#geo` + a coarse pluscode cell, matching over public networks without revealing coordinates, a tag-indexed and TTL'd "bulletin board backend". Kept: that spirit, and the ring-by-ring outward search. Replaced: the Ulam-spiral digit arithmetic (Chebyshev rings of cell offsets, re-encoded, give the same ordering without carry bugs), Hive/IRC/Twitter as transports (ATproto now), the free-form message grammar (structured records). The old code is a source of intent, not a spec.

## 2. Foundation, part one: primitives

**The whole trick is turning location search into text search.** A `#geo` cell is just a string, so "what's near me" becomes a hashtag query that works on any network or index that can search text (Bluesky search, Mastodon, our own index, plain grep), with no geo-index anywhere. Languages get the same treatment: a language is a string too, and "who speaks what I'm learning, near me" is a text query over two tag families. From there everything reduces to one idea: **a tag is a string under which things are published and polled, with an expiry.** Some tags are public (`geo:9f469g`, `lang:epo`). Some are secrets only two people can compute. Whoever holds a tag can read under it; nobody else can tell what it means.

Two tag families are first-class in the foundation, place and language, because together they are the primary axis of the whole product (amikumu's insight: geo × language). Hobbies are ordinary tags, secondary.

| Primitive | What it is |
|---|---|
| **Account** | esperanto-kurso auth, copied verbatim: anonymous user on first visit, magic link to come back, optional passkey, optional username. Holds only the credential and whatever public profile the user chooses to publish. |
| **Tag** | Namespaced string: `geo:9f469g`, `lang:epo/native`, `lang:por/learning`, `tag:chess`. Public hashtag forms: `#geo9f469g`, `#langepo`, `#chess`. |
| **Language tag** | `lang:<ISO 639-3>/<level>`; levels `native`, `fluent`, `learning` (optional CEFR letter, `learning-b1`). 639-3 because it covers constructed, minority and sign languages (`epo`, `tok`, `fry`, `ase`, `bfi`, `ngt`) that 639-1 doesn't. Public form **`#langepo`**: mirrors `#geo` (prefix + code), never collides with a word or a hobby (`#eo`, `#ase`, `#go` all do), and is one token for text search. Levels are not in the public hashtag: they are on the profile record, and a post is simply "about/in" a language. ATproto's native `langs` field on posts is a free extra signal for the same tag. |
| **Gazetteer** | Two small, mostly-read tables that connect Kafumu's canonical tags to the hashtags people already use. **Places:** `#amsterdam`, `#ams`, `#saopaulo`, `#london` → sets of `geo:` cells, and the reverse cell → place tags. **Languages:** `#esperanto`, `#español`, `#tokipona`, `#learnjapanese`, `#languageexchange`, `#langchat`, `#duolingo` → `lang:` codes (or "any language", for the generic ones). Every entry carries a **weight** (how specific the tag is: `#tokipona` ≈ 1.0, `#duolingo` ≈ 0.2) and an **ambiguity list** (`#paris` → Paris FR, Paris TX; `#georgia` → the country and the state; `#cambridge` → UK and MA): an ambiguous tag maps to several cell sets with lower weight each. Seeded from GeoNames/OSM (population + bbox → cells) and a hand-curated language list; later reinforced by usage, since a post carrying both `#geo9f469g` and `#amsterdam` is evidence for the link. A cell's search therefore expands to `#geo9f469g OR #amsterdam OR #ams`, which solves the empty-room problem on day one. |
| **Geo cell** | A `geo:` tag: the **first six characters of the Open Location Code, lowercased** (0.05° ≈ 5.5 km). Coordinates are encoded on the device and discarded. **Neighbourhood search** iterates Chebyshev rings of cell offsets (dx, dy) outward from your cell (ring 0 = you, ring 1 = the 8 neighbours, ring 2 = the next 16, …), re-encoding `(lat + dy·0.05°, lon + dx·0.05°)` to 6 chars for each, and stops when it has enough results. This keeps whenwhere's property (ordered, expanding, ring-by-ring, computable by anyone without coordinates leaving the device) and replaces the Ulam-spiral digit arithmetic because re-encoding handles OLC digit carries and the antimeridian for free. Proximity uses ring ≤ 1 (3×3); the home list rings 0–2 (5×5) by default. |
| **Pair** | Two people who scanned each other's QR in person and share a 32-byte secret K. Roles fixed at scan (shower = 0, scanner = 1). Exists only on the two devices. |
| **Beacon** | `HMAC(K, role, cell, day)`: a private tag meaning "I was in this cell today", readable by exactly one friend. Coarse in space and in time by design. |
| **Mailbox** | `HMAC(K, "box", role)`: two unidirectional queues per pair, addressed by random IDs, carrying ciphertext with a short TTL. No sender identity. The signalling channel. |
| **Record** | A public ATproto record: a post, a profile, an event, an RSVP, a listing. Carries tags. Lives in the author's own PDS. |
| **Bundle** | Everything public and current for **one cell**, assembled by the Kafumu index and cached: one blob per cell. The client asks for the cells it wants (`/bundle?cells=a,b,c`, merged from memcache), ring by ring until its list is full, and ranks the result against tags and pairs it holds locally. |

Note on the geo definition: the brief said "six chars after the four-char region", but whenwhere's code is `'geo' + code.substring(0,6)`, i.e. including the region, which is what makes the tag globally unique and 5.5 km wide. The definition above is the one to implement.

## 3. Foundation, part two: three data tiers and the rules

Every piece of data lives in exactly one of three places. Deciding which is most of the design. The rules that go with them:

1. **Private data never leaves the device unencrypted.** Pairs, notes, check-in history, preferences. Sync is an opaque blob under a key the server doesn't have.
2. **Public data is the user's, on ATproto.** If a thing is meant to be seen by strangers, it is a record in the user's PDS, and Kafumu only indexes it.
3. **The server holds only opaque or expiring data.** Random-keyed slots and boxes, cached bundles, TTL'd fallback records, and the minimal account. Every server entity has either no meaning without a client-held key, or an `expiresAt`, or both. For pair-derived keys, **only the owner side is authenticated; the other side is anonymous** (`credentials: 'omit'`), otherwise random ids would be linked by the cookies on both ends.
4. **Ranking and matching happen on the device**, against per-cell bundles, so the server never learns what matched you.
5. **A layer that needs a new rule, a new tier, or a new primitive is a red flag.** It should need only new tags, new record kinds and new client code.

### Tier 1 — Device-private (IndexedDB)

Pairs (K, role, display name), contact notes (phone, WhatsApp/Signal/Telegram handles, "met at Langfest"), your own check-in history, your private tags and ranking preferences. Never sent in clear. Optionally mirrored to the server as **one encrypted vault blob** only you can decrypt: the vault key comes from the passkey's WebAuthn PRF extension where supported (Chrome, Safari 18+), else a random key the user carries between devices in the fragment of a QR/magic link (`…#v=KEY`; browsers never send fragments; client JS appends it, the server renders the link without it). Safari evicts storage for sites not installed to the home screen after 7 idle days; installed PWAs are exempt, so the app insists on install and offers the vault.

### Tier 2 — Public, on ATproto (the user's own PDS)

Public profile (display name, public tags, home cell), local posts with `#geo…` hashtags, meetups as `community.lexicon.calendar.event` records (the Smoke Signal lexicon), RSVPs as `community.lexicon.calendar.rsvp`, and later listings as a Kafumu lexicon (`com.kafumu.listing`). The user owns these; they survive Kafumu; any ATproto client can show them; Smoke Signal RSVPs flow back for free. Reading needs no account: `app.bsky.feed.searchPosts?q=%23geo9f469g&tag=geo9f469g` on the public AppView works unauthenticated (verified against `api.bsky.app`; `public.api.bsky.app` refused this sandbox's requests, so the index tries both). The index expands each cell's query through the gazetteer (`#geo9f469g OR #amsterdam OR #ams`, plus `#lang…` tags when a language filter is on) so a cell has content from the existing hashtag ecosystem before any Kafumu user posts there. Event records are read by walking the PDSs of accounts known to the index, or via the Smoke Signal AppView, cached per cell.

**Honest limit:** writing requires an ATproto account (Bluesky, a self-hosted PDS, or a Kafumu-hosted PDS later). M1 writes via `https://bsky.app/intent/compose?text=…%20%23geo9f469g` (zero infra); M2 adds ATproto OAuth so Kafumu writes posts, events and RSVPs to the user's PDS. Users without an account are served by the **fallback-records layer** in tier 3 (see section 4), which exists only because of this limit and is deliberately second-class: TTL'd, not portable, migratable in one tap once the user has a PDS.

### Tier 3 — Ephemeral, on the Kafumu server (GAE)

Only what must be shared between two devices and can't be public:

| Data | Why the server needs it | Retention |
|---|---|---|
| `User`: id, hash of magic-link token, passkeys, username, public tags, home cell, `Discoverable`, patron flag, `CreatedAt`, `LastSeenAt` (hourly at most) | login; discoverability; patron badge | anonymous: 7 idle days; named: `KeepDataDays` (default 365); self-delete any time |
| Encrypted vault blob (opaque) | second device | with the account |
| Beacon slots: random key → list of HMAC tokens | friend proximity | 8 days (memcache + write-behind) |
| Mailbox messages: random queue id → ciphertext | signalling | 7 days or on acknowledgement |
| Push subscription ↔ mailbox ids the device owns | wake the recipient | until unsubscribed or 90 days unused |
| Fallback records (post/event/RSVP) for users without a PDS | usefulness without an account | `expiresAt` (chatter 48 h, event end + 1 day) |
| Listing orders: business contact, cells, Stripe ids, paid until | billing, moderation | while live + invoice retention (held by Stripe) |
| Patron: Stripe customer id, status | badge | while active + 30 days |
| Caches (Bundles, AppView results) | cost | minutes, memcache |
| GAE request logs (IP, path) | ops | shortest retention GCP allows (set to 7–30 days) |

**What the server never has:** coordinates, who is paired with whom (mailbox and slot ids are random to it), contact notes, which listings or people matched you (ranking is client-side), a browsing history beyond request logs.

**GDPR, by construction:** minimisation (table above is the whole list), no profiling (no server-side ranking or tracking, no third-party scripts, Stripe is the only processor), storage limitation via TTLs and the existing esperanto-kurso purge cron, trivial export (one JSON of your `User` + fallback records) and deletion (one button; everything else is already yours or already gone). A traffic-analysis caveat belongs in the privacy page: the server could correlate which account fetched which random slot if it logged that, so it doesn't log request bodies and keeps request logs short.

## 4. Layers

Each layer says what it adds on top of the foundation. None changes it.

### The home list (the product)

*Adds:* a ranking function on the device and one page. *Server sees:* a Bundle request per cell. *Tier:* none of its own.

One page: the Bundle for your current cell (rings 0–2, widened ring by ring if the list is thin), ranked on the device against your tags and pairs. Geo × language is the primary axis; hobbies are secondary. Posts that match on both a gazetteer place tag and one of your language or interest tags get boosted; a post matching only a noisy place tag (`#paris`) is shown low and labelled "from #paris", so gazetteer noise is visible rather than silently ranked. Sections fall out of the record kinds in the Bundle: friends around (tier 1 pairs × tier 3 beacons), meetups that match your tags (tier 2 events + tier 3 fallback), people here who share a tag (tier 2 discoverable profiles), local posts (tier 2 `#geo` posts), one labelled listing if you opted in. Travel mode is nothing extra: when the cell is far from your usual cells the page gets a banner, "You're in Lisbon (9c2x…): 2 friends were here this week, 3 things match you". Bundles are per cell, not per user, so the server never learns your interests to serve them.

### Friend proximity (the coffee)

*Adds:* the beacon tag derivation and a slot per pair. *Tiers:* pairs in 1, slots in 3 (opaque, 8-day TTL). *Server sees:* random keys and token lists.

1. **Pairing.** A shows a QR containing K, and A's app claims its inbox `HMAC(K,"box",0)` with its credentials (so it is bound to A's push endpoint; unclaimed inboxes reject writes). B scans, stores (K, role 1), derives both mailbox ids, and drops an encrypted "hello, I'm Bea" into A's inbox **anonymously**. A's app polls its inbox while the QR is on screen and stores (K, role 0, "Bea"). The server saw one account claim a random id and an anonymous blob arrive; it does not know who B is. Scanning in person is the trust ceremony; it is what the product is about.
2. **Check-in.** When you open the app, the client gets one geolocation fix, encodes cell `c`, and for each pair updates its slot `HMAC(K,"slot",myRole)`: a list of `HMAC(K,"beacon",myRole,c,utcDay)` tokens for the (cell, day) pairs you've checked in from during the last 7 days. Read-merge-write (two devices don't clobber), and write only when a new (cell, day) appears: about one write per pair per travelling day, not per open. Memcache-first; written behind to a tiny Datastore entity under the same random key, read only on a memcache miss, so "Ana was here on Tuesday" survives a cold instance.
3. **Check.** For each pair, fetch the friend's slot **anonymously** (no cookie; one read) and compare locally against the 9 cells × 7 days you can compute. A hit says "Ana was in or near here today / on Tuesday". Labels are relative; UTC day boundaries can shift a label by a day, never miss a match. The server saw a fetch of a random key.
4. **Act.** Tap Ana: her contact note is there (tier 1). Message her on Signal, or send a one-tap signal through the mailbox: "I'm around, coffee?" (section on signalling). Nobody is pinged by location, nobody forces themselves on anyone; Ana learns you were around when she opens the app, or when a signal wakes her phone.

Why slot-key + token-value rather than token-as-key: one read per pair instead of 63, and the server never observes a "hit". Why fixed roles: without them two friends in the same cell write identical tokens. Why a day and 5.5 km: coarse in time and space is the feature. It is enough to make a coffee happen, useless for stalking, and it removes any need for background geolocation, which iOS PWAs don't have anyway. Whoever opens the app publishes their day; the other sees it on their next open. Symmetric, lazy, sufficient.

### People à la amikumu

*Adds:* `lang:`/`tag:` tags on the public profile, a `Discoverable` flag, and a language-match ranking function. *Tier:* in M1 the public profile is the one thing the user explicitly publishes on `User` (tier 3, part of the minimal account); from M2 a profile record in the PDS (tier 2). Either way it is mirrored into the Bundle. *Server sees:* what the user chose to publish.

amikumu = "find people nearby who speak or learn your language, chat, meet". In Kafumu: languages are `lang:xxx/level` tags on the public profile; "nearby" is "same or neighbouring cell", deliberately no km and no map pin; the people list is the discoverable profiles in the Bundle, matched client-side. The core match is **complementary exchange**: someone who speaks what you're learning and is learning what you speak scores highest; one-directional matches next. The second strong signal is **shared rare language**: the ranking weights languages by local rarity (how few profiles and posts in the Bundle carry the tag), so an Esperanto, Frisian or ASL speaker in your cell surfaces at the top, while sharing English in London is noise. Hobbies (`tag:`) are secondary boosts. Contact is "scan each other at the meetup" or a mailbox signal once paired. Discoverability is **opt-in** (`Discoverable` flag): nothing about you is in any Bundle until you say so. Kafumu generalises amikumu, since people, meetups, posts and listings all carry the same geo × language tags and share one list; and it differs on purpose: strangers can't DM you, they can meet you at an event.

Language also drives three mundane things for free: UI locale (reuse esperanto-kurso's `internal/locale/`, 30+ languages, keyed by the user's `lang:` tags and `Accept-Language`), the language a meetup or listing is held in (`lang:` tag on the record, so a Portuguese conversation table is findable as `#langpor`), and the AppView `langs` filter for local posts.

### Meetups (useful from day one, free to host)

*Adds:* event and RSVP record kinds (existing community lexicons) and `.ics` rendering. *Tier:* 2, or the fallback layer. *Server sees:* public records it indexes.

A meetup is an event record: title, start/end, venue line (optional pluscode, revealable only to attendees), tags (`geo:`, `lang:`, `tag:`), optional external link (Meetup.com, Luma). Anyone can create one, nobody pays, no attendee cap, no "organiser subscription": the pain Meetup.com created is the opening. A language café or chess night moves its listing here in two minutes and appears in the home list of everyone in the cell with matching tags. Because it is an ATproto record it also shows up in Smoke Signal and any other event client; iCal feeds (`/cal/geo:9f469g.ics`, `/cal/me.ics`) put it in people's calendars; paste-a-link imports title/time from OpenGraph. Users without an ATproto account use the fallback tier and still get all of this inside Kafumu.

### Fallback records (ephemeral, for users without a PDS)

*Adds:* a `Record` entity kind with a mandatory `expiresAt`. *Tier:* 3. *Server sees:* the record and its author account.

This is the one layer where the server stores user content in clear, so it has to justify itself: without it, nobody can host a meetup or post locally until they have an ATproto account, and "useful from day one" fails. It stays within rule 3 by being strictly expiring (chatter 48 h, event end + 1 day, a listing its paid period, never longer), by being indexed only by cell and tag, and by having an exit (migrate to PDS). If a Kafumu-hosted PDS appears in M3, this layer is retired.

### Signalling (not a chat app)

*Adds:* message types inside the mailbox and a push relay. *Tiers:* 3 (boxes: opaque, 7-day TTL; push subscriptions). *Server sees:* random queue ids, ciphertext, and which push endpoint owns which inbox.

The mailbox carries small, structured, encrypted messages between paired devices: "I'm around, coffee?", "here's my number" (fills the recipient's contact note), a meetup invite (an event reference), "let's unpair". AES-GCM with keys derived from K and a counter via WebCrypto; no double ratchet, said plainly. Web Push wakes the recipient ("someone you've scanned sent you a signal", unnamed; the app decrypts and shows who). Unpair = delete your pair locally and stop reading; block = same, plus delete your inbox. Free-text chat may come later if users ask, but the loop "they're around → signal → meet" does not need it; existing messengers do the talking. Full SimpleX interop is out of scope; the design borrows its shape (random unidirectional queues, no identifiers), not its protocol.

### Money: patrons and listings

*Patrons add:* a flag on `User` and a Stripe webhook. *Listings add:* a `listing` record kind with cells and tags, ranked like any other record. *Tiers:* 3 for billing state, 2 (or fallback) for the listing itself. *Server sees:* Stripe ids; never who saw which listing.

Two halves, both opt-in, neither paywalls anything. **Proximity, signalling, search, posting and hosting meetups stay free for everyone, forever.** That constraint is the brand.

**User-funded: Patrons, positioned like Lichess.** Pay-what-you-want recurring ("Kafumu friend", monthly or yearly, suggested €3/€5/€10, any amount from €1) and one-off ("buy the community a coffee", €3+). Recognition without a two-class system: a small cup badge, a thank-you page, an opt-in supporters list (pseudonyms fine), a line in each Bundle, "this area is kept alive by 7 patrons". Cosmetic only. Provider: **Stripe Checkout + Customer Portal + one webhook** fits a Go/GAE app with minimal infra (≈1.5% + €0.25 per EU card, hosted pages, Kafumu stores only `customerID, status, since`); add a **Liberapay** link for people who prefer zero-fee donations (badge granted by hand). Ko-fi/OpenCollective add a second brand and 0–10% fees. Tax in one sentence: patronage with a cosmetic badge is borderline between donation and digital service; as an individual in the UK/NL, stay under VAT registration thresholds at first, enable Stripe Tax if it ever matters, and get a one-line accountant opinion before launch. Regional pricing: pay-what-you-want makes PPP tiers unnecessary (Lichess doesn't need them); the client can pick the suggested default from country/cell (€2 in Brazil, €5 in NL) with no server-side price logic. Messaging: one gentle nudge, rarely, at a good moment (after a proximity hit or an attended meetup: "Kafumu helped you meet someone. It's run by one person; chip in?"), dismiss once and it stays away for months, never a wall or a counter. Expectation, honestly: patron models convert 1–3% of monthly actives at €3–5 average. 1,000 MAU → €40–150/month (GAE overage and a domain); 20,000 MAU → €800–3,000/month, a real side income, not a salary.

**Business-funded: relevance listings, the unintrusive alternative to advertising.** A listing is a record by a business or organiser with cells and interest tags and a label ("Listing · Café Tortoni · sponsors this area"). It is ranked on the device in the same list as everything else, against tags the client already has. No tracking, no third-party scripts, no per-user impressions: the business sees "live in cell X for week Y" and, optionally, a click count from a Kafumu redirect. Users opt in to "show listings" (default off until a cell has a sponsor, then on with a one-tap off, pending Joop's call). The first case is the **coffee spot**: when two friends match in a cell, or a meetup is created without a venue, the app suggests the sponsoring café, hours included, "we welcome Kafumu meetups". Pitch to cafés: cheaper than Google/Meta, lands exactly when someone within 5 km is choosing where to sit, and your customers like you for it. Pitch to users: it answers "where shall we go?". Pricing sketch (M2, Stripe Payment Links and manual approval first): community and non-commercial events free, always; commercial listing **€10/cell/week or €30/cell/month**, ≤ 3 tags, neighbouring cells sold separately; paid-ticket events free to list, optional "featured" €5, never a per-attendee cut; early cells free to seed. 50 paying cells ≈ €1,500/month; sales, not tech, is the bottleneck, so Joop's own cities first. Ethics: labelled, matched on coarse data only, never in friend or signal surfaces, never ranked above a friend being nearby, and the cell's patron line sits next to its sponsor line so users see both halves fund the same thing.

### How to add a layer (checklist)

Before building anything new, answer these in five lines; if any answer is awkward, the feature is wrong or the foundation is, and it is almost always the feature.

1. **Which tier does its data live in?** Device-private, the user's PDS, or ephemeral server. If "server, and it must persist", stop.
2. **Which tags does it publish and poll under?** Public (`geo:`, `lang:`, `tag:`, a new record kind) or pair-derived (an HMAC of K with a new label). New tag kinds are fine; new ways of addressing are not.
3. **What does the server see?** Say it in one sentence that could go on the privacy page.
4. **TTL and cost.** Every server entity gets an `expiresAt` or a client-held key; estimate Datastore ops per user-day and keep it near one.
5. **Can it be ranked client-side from the Bundle?** If it needs a per-user server query, redesign.

Worked example, "lost and found": a `tag:lostfound` record kind in tier 2 (fallback in 3, 7-day TTL), polled via the cell Bundle, filtered on the device by cells and `tag:`, server sees a public post. Passes: no foundation change.

## 5. Architecture on lokumo / GAE free tier

`/home/joop/lokumo` is a 2019 `go112` hello-world: reuse the **GCP project, billing account and quotas**, rewrite the app. Map `kafumu.com` as a custom domain with a managed cert, and decide **www vs apex before the first passkey is registered** (WebAuthn RP ID is origin-bound: `WEBAUTHN_RPID=kafumu.com`).

**Stack:** Go 1.2x on GAE Standard, `app_engine_apis: true` (legacy memcache via `google.golang.org/appengine/v2/memcache`: free, unmetered), Datastore, Go templates + HTMX + Pico CSS, a service worker for install/offline shell/Web Push, OLC encoding in a few lines of client JS, WebCrypto for HMAC/AES/PRF. `app.yaml` copied from esperanto-kurso (`max_instances: 3`, `static_dir`, static `robots.txt`), `cron.yaml` with one daily sweep guarded by `X-Appengine-Cron`. Deploy with `gcloud app deploy --project lokumo` (no sudo; the agent may run it, gated by the permission prompt).

**Components:** `auth` (copied: `internal/auth`, `handler/auth.go`, `model/user.go`, `store/user_store.go`; tweak: cookie carries `userID.token` so auth is a Get-by-key, and the token is stored hashed), `geo` (OLC, neighbours), `index` (the Bundle builder: fallback records by cell, AppView `#geo` search, event records, discoverable profiles, patron count, listings), `slot` and `box` (beacons, mailboxes, push relay), `records` (fallback posts/events/RSVPs, `.ics`), `atproto` (M2: OAuth, write records to the user's PDS), `money` (Stripe webhook, patron flag, listing admin), `home` (one HTMX page; ranking is JavaScript against the Bundle plus tier-1 data).

**Datastore kinds:** `User` (as in the table), `WebAuthnSession`, `UserAlias`, `Slot{Key=HMAC, Tokens noindex, UpdatedAt}`, `Box{Key=id, Blobs noindex, ExpiresAt}`, `PushSub{UserID, Endpoint, Keys, BoxIDs}`, `Record{AuthorID, Kind post|event|rsvp|listing, Text, Tags[], Cells[], StartAt, EndAt, Venue, Link, ExpiresAt}`, `Listing{RecordID, Cells, PaidUntil, StripeIDs}`, `CellMeta{Version}`, and the two gazetteers, `PlaceTag{Tag, Cells[], Weight, Ambiguous[]}` and `LangTag{Tag, Codes[], Weight}` (a few thousand entities, loaded into instance memory with the `content_cache.go` version pattern, or shipped as a static JSON in M1 and moved to Datastore when usage starts writing to it). No `Pair` kind: the server doesn't know pairs; unpair and block are client-side plus inbox deletion. Bundles, AppView caches and rate limits live in memcache only.

**Static vs dynamic:** app shell, CSS, JS, manifest, `/about`, `/privacy`, `/patrons`, `/for-cafes` are static. Dynamic: `/` (home, needs cookie), `/r/{record}`, `/cal/*.ics`, `/u/{username}`. Everything crawlable is a cached Bundle render or static.

### Cost discipline (lessons from esperanto-kurso 4000154 / 492d019)

Datastore entity reads are the cost on GAE (50k/day free). esperanto-kurso hit ~1M/day, nearly all crawlers on list pages. Rules from the first commit:

1. **Never query per request.** The Bundle is the only list anyone reads and it is one memcache hit; cold build is one query over `Record` by cell (tens of entities; the 5×5 neighbourhood is 25 `IN`-style cell values, or 25 cached single-cell bundles merged), cached, invalidated by a `CellMeta.Version` bump checked at most once a minute, refreshed every 10 minutes. Same pattern as `content_cache.go`. People search is a filter on the Bundle, not a `User` query.
2. **Ephemeral data stays out of Datastore.** AppView caches, rate limits, Bundles: memcache with TTL. Slots are memcache-first with a write-behind only when a new cell-day appears. Mailboxes are few, small entities with TTL and delete-on-ack.
3. **Few large entities over many small ones.** A user's private data is one `noindex` vault blob. A mailbox is one entity holding its pending blobs.
4. **Bots are not users.** Reuse `IsBot` (UA regex + missing `Accept-Language`); bots get cached/static renders, never an auto-created `User`, never a write on GET; static `robots.txt` blocks SEO crawlers; the catch-all returns 404.
5. **Throttle presence writes.** `LastSeenAt` hourly; check-ins touch memcache; profile edits are the only frequent Datastore write and they're user-initiated.
6. **Push reads to the client and to public networks.** Tier 1 and tier 2 do most of the storing; the client does the ranking, neighbour math and token comparison; local posts are read from the AppView, not stored.
7. **Datastore TTL policies** on `Record`, `Box`, `Slot`, `WebAuthnSession` (TTL deletes count as delete ops, 20k/day free; fine). Keep the esperanto-kurso purge cron for users.
8. **Budget:** 1,000 MAU × 5 opens/day ≈ 1 `User` Get per open + a Bundle rebuild per active cell per 10 minutes + a handful of slot/box ops: well under 10k reads/day. F1 instances, `max_instances: 3`, scale to zero.

Storing less is also why this is cheap: tiers 1 and 2 cost Kafumu nothing. Dolt is not a fit for the serving path; at most a versioned export of public records for analysis later.

## 6. Milestones

**M1 — the small shippable thing (a few weekends)**
- Accounts copied from esperanto-kurso; username; public tags (`lang:xxx/level` with a 639-3 picker, `tag:`); home cell from one geolocation fix; `Discoverable` off by default; UI locale from esperanto-kurso's locale files.
- Gazetteers as static JSON: top ~500 cities by population from GeoNames (bbox → cells, with the obvious ambiguities marked) and a curated list of ~200 language hashtags. The cell query expands through them from the first day.
- QR pairing through the mailbox handshake; pairs and contact notes in IndexedDB; export/import QR for a second device (vault blob can wait).
- Check-in on open → slots → "friends around here today / this week" with the contact note one tap away. Canned signals through the mailbox ("I'm around, coffee?", "here's my number"), polled on open and while the pairing QR is on screen; no push yet; no free-text chat.
- Home list ranked on the device from the cell Bundle: friends, matching meetups, discoverable people ranked by complementary-language and rare-language match, `#geo` posts from the AppView, a "post to Bluesky with #geo" compose link. Travel banner when the cell is far from home.
- Meetups in the fallback tier: create, list, RSVP, `.ics` per tag and per user. Free hosting, no fees.
- `/patrons` with Stripe Payment Links (one-off + recurring) and a Liberapay link; badge set by hand.
- Cost rules 1–8, `robots.txt`, `IsBot`, Bundle cache. Clean cuts if it must shrink: export QR, `.ics`, canned signals.

**M2 — the lens points at ATproto**
- ATproto OAuth: Kafumu writes posts, `community.lexicon.calendar.event` and RSVPs to the user's PDS; migration of fallback records; external events in the Bundle; paste-a-link import.
- Web Push for signals and meetup reminders; encrypted vault blob with PRF-derived key.
- Stripe Checkout + Portal + webhook: automatic badge, supporters list, patron count per cell.
- Listings: self-service submit, approval, Payment Links, coffee-spot suggestion on a match or venue-less meetup, opt-in toggle, `com.kafumu.listing` lexicon draft.

**M3 — opening it up**
- Publish the Bundle, Slot and Box formats so a native app or third-party client works against them; ATproto DIDs as an alternative login; a Kafumu-hosted PDS for users without one if demand exists.
- Free-text signals if users ask; WebRTC direct channel when both are online; a per-cell Bluesky feed generator; other servers indexing the same tags (the OLN federation idea).

## 7. Open questions for Joop

1. **Geo tag:** confirm "first six OLC chars including region" (`#geo9f469g`, 5.5 km) as canonical. Also want an optional 8-char tag (`#geo9f469g3h`, 275 m) for dense cities?
2. **Domain and RP ID:** apex `kafumu.com` or `www`? Which GCP project is lokumo, and where does kafumu.com's DNS live?
3. **Multi-device:** M1 export QR only, or build the vault blob now (and PRF vs fragment key)?
4. **Public language hashtag:** `#langepo` as drafted (prefix + 639-3, mirrors `#geo`), or ride on the human tags only (`#esperanto`) and rely on the gazetteer? Posting both is the likely answer; confirm.
5. **Gazetteer growth:** should usage (posts carrying both `#geo…` and `#amsterdam`) automatically reinforce links, or stay curated until there is a moderation story?
6. **Patron legal form:** Stripe as an individual (UK or NL, accountant question), or Liberapay-only until income justifies it?
7. **Listings default:** off until a cell has a sponsor then on with one-tap off, or opt-in forever?
8. **Discoverability default:** opt-in as drafted, or on for anyone who adds a language tag (what amikumu users expect)?
9. **Name:** "Kafumu" everywhere, "Coffee Buddies" as the English tagline?
10. **Length:** this document grew to ~5,000 words with the later inputs; want a one-page summary on top, or Money and Cost discipline trimmed to bullets?
