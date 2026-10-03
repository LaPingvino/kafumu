# Kafumu

**What is near me right now that I would want to know about?**

Friends you've met who happen to be in town, meetups in a language you speak or a hobby you have,
and what people around here are saying — without tracking. Working name for the domain: kafumu.com
(“Coffee Buddies”). Runs on Google App Engine as `lokumo`: <https://lokumo.ew.r.appspot.com>.

## The idea in one paragraph

Location search becomes text search. Your device turns its position into a `#geo` cell — the first
six characters of the [plus code](https://maps.google.com/pluscodes/), about 5.5 km wide — and
“what's near me” becomes a hashtag query on any network that can search text. Languages get the same
treatment (`#langepo`). Private data stays on your device, public data lives on ATproto/Bluesky in
your own account, and the server keeps only opaque or expiring data. See [VISION.md](VISION.md).

## Running locally

```bash
go run .            # http://localhost:8080
go test ./...       # Go tests
node test/geo_test.mjs   # checks the client-side cell code against the Go vectors
```

## Deploying

```bash
gcloud app deploy --project lokumo
```

## Lineage

Grown from [whenwhere / OLN](https://github.com/LaPingvino/olc-tools) (2019) and the account system
of [esperanto-kurso.net](https://github.com/LaPingvino/esperanto-kurso-gae).

## License

MIT — see [LICENSE](LICENSE).
