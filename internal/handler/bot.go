package handler

import (
	"net/http"
	"regexp"
)

// botUA matches self-identified crawlers and common scripted clients.
// Copied from esperanto-kurso, where crawlers caused ~95% of Datastore reads.
var botUA = regexp.MustCompile(`(?i)bot|crawl|spider|slurp|scrape|fetch|curl|wget|python|go-http-client|java/|headless|httpclient|axios|node-fetch|okhttp|libwww|facebookexternalhit|preview`)

// IsBot reports whether the request looks automated. Real browsers always
// send Accept-Language; most scrapers posing as Chrome don't. Bots never get
// an account, never cause a write, and never trigger upstream fetches.
func IsBot(r *http.Request) bool {
	ua := r.UserAgent()
	return ua == "" || botUA.MatchString(ua) || r.Header.Get("Accept-Language") == ""
}
