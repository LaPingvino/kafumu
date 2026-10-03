# Kafumu

**What is near me right now that I would want to know about?**

Friends you've met who happen to be in town, meetups in a language you speak or a hobby you have,
and what people around here are saying — without tracking. Live at **<https://kafumu.com>**
(Google App Engine project `lokumo`).

## What it does

- **Around** — your area (search, map, or location after a tap) with friends who were nearby this week,
  signals, meetups (Kafumu's own, Luma, Smoke Signal/ATproto), people who chose to be findable (ranked
  by language exchange on your device), and local posts found through `#geo` cells and place hashtags.
- **Connect** — show a QR (or print it on your badge); whoever scans it swaps cards with you,
  end-to-end encrypted. Personas let you choose what to hand over each time.
- **Contacts** — kept on your device only; notes, tags, one-tap signals (☕ / 📍 / 👋), push
  notifications, vCard/backup export, and moving everything to a new device by scanning.
- **Accounts** without email or phone: magic link, passkeys, and optionally your Bluesky/ATproto
  account so meetups, RSVPs and posts are written to your own repo.
- English, Portuguese, Esperanto and Dutch.

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

## Deploying

```bash
gcloud app deploy app.yaml cron.yaml --project lokumo   # needs the app-engine-go component
gcloud app deploy index.yaml --project lokumo           # when Datastore indexes change
```

Place and language hashtags come from [LaPingvino/geotags](https://github.com/LaPingvino/geotags)
(GeoNames, CC BY 4.0).

## Lineage

Grown from [whenwhere / OLN](https://github.com/LaPingvino/olc-tools) (2019) and the account system
of [esperanto-kurso.net](https://github.com/LaPingvino/esperanto-kurso-gae).

## License

MIT — see [LICENSE](LICENSE).
