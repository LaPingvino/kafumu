# Running your own Kafumu

Kafumu runs as one Go program plus one SQLite file. No Google Cloud needed:
the same code that serves kafumu.com on App Engine runs anywhere Go does.

## Build and run

```sh
git clone https://github.com/LaPingvino/kafumu && cd kafumu
go build -o kafumu .            # pure Go: no C compiler needed for SQLite
KAFUMU_SQLITE=/var/lib/kafumu/kafumu.db \
KAFUMU_ORIGIN=https://coffee.example.org \
KAFUMU_BRAND="Coffee Example" \
KAFUMU_ADMIN_TOKEN="$(openssl rand -hex 16)" \
PORT=8080 ./kafumu
```

Put it behind a reverse proxy that does HTTPS (Caddy, nginx…): passkeys,
push notifications and the service worker need HTTPS.

## Settings (environment variables)

| Variable | What it does |
|---|---|
| `KAFUMU_SQLITE` | Path of the database file. Everything persists there. Without it, data lives in memory and is gone on restart. |
| `KAFUMU_ORIGIN` | Your public address, e.g. `https://coffee.example.org` (links, QR codes, passkeys). |
| `KAFUMU_BRAND` | The name shown in the app. |
| `KAFUMU_ADMIN_TOKEN` | At least 16 characters. To become admin: sign in, then open `/admin/initial?token=<it>`. Remove it afterwards if you like; the role stays. |
| `KAFUMU_PASSKEYS=1` | Offer passkey sign-in. |
| `KAFUMU_ATPROTO=1` | Offer connecting a Bluesky account. |
| `KAFUMU_MAKER`, `KAFUMU_CONTACT` | The "contact the maker" footer: a username on your node, and a URL. Changeable in admin. |
| `PORT` | Port to listen on (default 8080). |

## What it does by itself

- Keeps every store in the SQLite file: accounts, usernames, local
  messages (OLN), meetups, encrypted mailboxes, sync vaults, business
  accounts, links, reports, push subscriptions.
- Forgets what Kafumu promises to forget (see `/privacy`), by the same rules
  as on App Engine: at start and every six hours.
- Back up by copying the database file (SQLite's `.backup` while running).

## Not there yet

- **Linking nodes**: your node keeps its own local messages; exchanging them
  with other nodes (OLN federation) is being built.
- **Business contacts synced between managers**: also still being built.

Questions and problems: https://github.com/LaPingvino/kafumu/issues
