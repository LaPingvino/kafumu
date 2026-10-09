# Kafumu

**What is near me right now that I would want to know about?**

Friends you've met who happen to be in town, meetups in a language you speak or a hobby you have,
and what people around here are saying — without tracking. Live at **<https://kafumu.com>**
(Google App Engine project `lokumo`).

## What it does

- **Around** — your area (search, map, or location after a tap) with friends who were nearby this week,
  signals, meetups (Kafumu's own, Luma, Smoke Signal/ATproto, and Esperanto events from Eventa Servo),
  people and businesses who chose to be findable (ranked on your device), local messages, and local
  posts found through `#geo` cells and place hashtags. Quiet areas widen ring by ring until there's
  enough to see.
- **Local messages (OLN)** — no account needed, paid in a little proof of work (Argon2id) instead;
  anonymous posts can be answered privately, and contacts chat end-to-end encrypted over the same
  format. Nodes can link and pull each other's messages. React to anything (a message, a meetup, an
  online event, a person, a post, any web link) with a line that carries only `#re<id>`: it's found
  from the thing, wherever you are. A message can also go to everyone into a subject, with no place
  at all, and a reaction can be carried home to your own area.
- **Activity (🔔)** — what came back to you from any area: replies and reactions to your posts
  (strangers' too), private answers, chat lines and signals from contacts, your inbox, new
  connections, people going to your meetups, changes to ones you go to, and likes and replies on
  your Bluesky posts. Assembled on your device; chat lines and answers can wake your phone.
- **Connect** — show a QR (or print it on your badge); whoever scans it swaps cards with you,
  end-to-end encrypted. Personas let you choose what to hand over each time.
- **Contacts** — kept on your device only; notes, tags, one-tap signals (☕ / 📍 / 👋), push
  notifications, vCard/backup export, and moving everything to a new device by scanning. Send a
  contact anything you see with 👀 "Did you see this?", or 🤝 "Shall we go together?" for a meetup.
- **Accounts** without email or phone: magic link, passkeys, and optionally your Bluesky/ATproto
  account so meetups, RSVPs and posts are written to your own repo. Your contacts and cards sync
  between your devices, encrypted.
- **Business accounts** — act as a café, venue or organiser: its own card, contacts (synced between
  its managers, with the key on the server or only on their devices), @name page, inbox and wings.
  Pay what you want; nothing buys visibility.
- **Brands** — the same app and network under another name and domain, with its own wording and tags.
- 24 languages.

## The idea in one paragraph

Location search becomes text search. Your device turns its position into a `#geo` cell — the first
six characters of the [plus code](https://maps.google.com/pluscodes/), about 5.5 km wide — and
“what's near me” becomes a hashtag query on any network that can search text. Languages get the same
treatment (`#langepo`). Private data stays on your device, public data lives on ATproto/Bluesky in
your own account, and the server keeps only opaque or expiring data. See [VISION.md](VISION.md).

## Running locally

```bash
go run .            # http://localhost:8080 (in-memory stores, no credentials needed)
sh test/run.sh      # everything: Go tests, JS units, pairing E2E, two-browser + passkey tests
                    # (headless Chromium), and Datastore code against the emulator
```

## Running your own

One Go program and one SQLite file, no Google Cloud needed: see [SELFHOSTING.md](SELFHOSTING.md).

## Deploying

```bash
gcloud app deploy app.yaml cron.yaml --project lokumo   # needs the app-engine-go component
gcloud app deploy index.yaml --project lokumo           # when Datastore indexes change
```

Place and language hashtags come from [LaPingvino/geotags](https://github.com/LaPingvino/geotags)
(GeoNames, CC BY 4.0), and the alternate town names used to place events (Parizo, München…) from
[GeoNames](https://www.geonames.org/) (CC BY 4.0), built with `tools/townalts.py`.

## Lineage

Grown from [whenwhere / OLN](https://github.com/LaPingvino/olc-tools) (2019) and the account system
of [esperanto-kurso.net](https://github.com/LaPingvino/esperanto-kurso-gae).

## License

MIT — see [LICENSE](LICENSE).
