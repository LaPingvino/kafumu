// Package feeds pulls public event calendars (Luma city/calendar pages,
// iCal feeds of Luma calendars and Meetup groups) into Kafumu as meetups,
// so an area has things in it before anyone hosts there. The list is
// curated in feeds.json; a cron runs Sync.
package feeds

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

//go:embed feeds.json
var feedsJSON []byte

// Feed is one source of events.
type Feed struct {
	URL  string   `json:"url"`
	Kind string   `json:"kind"` // "jsonld" (schema.org Events), "ics", or "smokesignal" (ATproto events)
	Tags []string `json:"tags,omitempty"`
	// Cell places events that carry no coordinates (most iCal feeds).
	Cell string `json:"cell,omitempty"`
	// Place "town": events without coordinates are placed by the end of
	// their location, "…, City, CC" (Eventa Servo's calendars).
	Place string `json:"place,omitempty"`
	Note  string `json:"note,omitempty"`
}

// Load returns the curated feeds.
func Load() []Feed {
	var fs []Feed
	if err := json.Unmarshal(feedsJSON, &fs); err != nil {
		panic("feeds: " + err.Error())
	}
	return fs
}

// Horizon is how far ahead events are imported.
const Horizon = 60 * 24 * time.Hour

// Result reports what a sync did.
type Result struct {
	Feeds, Events, Saved, Skipped int
	Errors                        []string
}

// Sync fetches every feed and upserts its upcoming events as meetups.
func Sync(ctx context.Context, fs []Feed, im *importer.Importer, store meetup.Store, now time.Time) Result {
	var r Result
	for _, f := range fs {
		r.Feeds++
		var evs []*importer.Event
		if f.Kind == "smokesignal" {
			evs = atprotoEvents(ctx, im, f.URL)
		} else {
			body, err := im.Get(ctx, f.URL)
			if err != nil {
				r.Errors = append(r.Errors, err.Error())
				continue
			}
			if f.Kind == "ics" {
				evs = importer.ParseICS(body)
			} else {
				evs = importer.ParseAll(body)
			}
		}
		host := hostOf(f.URL)
		lookups := 0
		for _, ev := range evs {
			r.Events++
			if f.Place == "town" && !ev.HasGeo && !ev.Start.After(now.Add(Horizon)) && !endOf(ev).Before(now) {
				if lat, lon, ok := placeTown(ctx, im, ev.Venue, &lookups); ok {
					ev.Lat, ev.Lon, ev.HasGeo = lat, lon, true
				}
			}
			m := toMeetup(ev, f, host)
			if m == nil || m.StartAt.After(now.Add(Horizon)) {
				r.Skipped++
				continue
			}
			if err := m.Validate(now); err != nil {
				r.Skipped++
				continue
			}
			// Keep RSVPs that Kafumu users made on an earlier sync.
			if old, err := store.Get(ctx, m.ID); err == nil {
				m.RSVPs, m.CreatedAt = old.RSVPs, old.CreatedAt
			}
			if err := store.Put(ctx, m); err != nil {
				r.Errors = append(r.Errors, err.Error())
				continue
			}
			r.Saved++
		}
	}
	if len(r.Errors) > 0 {
		log.Printf("feeds: %d errors: %v", len(r.Errors), r.Errors)
	}
	return r
}

// Locate places a town by name in a country (the gazetteer's), for feeds
// whose events say where in words only; set by main.
var Locate func(name, cc string) (float64, float64, bool)

func toMeetup(ev *importer.Event, f Feed, host string) *meetup.Meetup {
	cell := strings.ToLower(f.Cell)
	// 0°,0° ("null island") is a missing location written as zeros, not an
	// event in the Gulf of Guinea.
	if ev.HasGeo && (ev.Lat != 0 || ev.Lon != 0) {
		cell = geo.Cell(ev.Lat, ev.Lon)
	}
	if !geo.Valid(cell) {
		return nil
	}
	link := ev.Link
	if link == "" {
		// The event's own page, when its text links to one on the feed's site.
		if m := linkRE.FindString(ev.Text); m != "" && hostOf(m) == host {
			link = m
		} else {
			link = f.URL
		}
	}
	// Stable id per event, so every sync updates instead of duplicating.
	sum := sha256.Sum256([]byte(link + "|" + ev.Title + "|" + ev.Start.UTC().Format(time.RFC3339)))
	return &meetup.Meetup{
		ID: "f" + hex.EncodeToString(sum[:9]), AuthorID: "feed", Via: host,
		Title: ev.Title, Text: ev.Text, Venue: ev.Venue, Link: link, Cell: cell,
		StartAt: ev.Start.UTC(), EndAt: ev.End.UTC(), Tags: append([]string(nil), f.Tags...), ATURI: ev.ATURI, ATCID: ev.ATCID,
		CreatedAt: time.Now(),
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// String summarises a result for the cron log.
func (r Result) String() string {
	return fmt.Sprintf("feeds=%d events=%d saved=%d skipped=%d errors=%d", r.Feeds, r.Events, r.Saved, r.Skipped, len(r.Errors))
}

var linkRE = regexp.MustCompile(`https?://[^\s"<>]+`)

// EventaServo is the Eventa Servo calendar of a country (two-letter code),
// or of online events ("ol"): Esperanto events, placed by town.
func EventaServo(cc string) Feed {
	return Feed{URL: "https://eventaservo.org/webcal/lando/" + strings.ToLower(cc) + ".ics", Kind: "ics",
		Tags: []string{"esperanto", "lang:epo"}, Place: "town", Note: "Eventa Servo"}
}

func endOf(ev *importer.Event) time.Time {
	if ev.End.IsZero() {
		return ev.Start
	}
	return ev.End
}

var postcode = regexp.MustCompile(`^[0-9][0-9A-Z -]{2,8}\s+`)

// bracket: "São Paulo (SP)", "Portugalete (Bizkaio)": the state or region.
var bracket = regexp.MustCompile(`\s*\([^)]*\)`)

// placeTown finds where "…, City[, Region], CC" is: the gazetteer first,
// trying each part from the end (a region may follow the city; "Dresden -
// Heidenau" is tried as both; postcodes are dropped), then OpenStreetMap
// for "City, CC" (at most 30 lookups a run, within Nominatim's policy).
func placeTown(ctx context.Context, im *importer.Importer, venue string, lookups *int) (float64, float64, bool) {
	parts := strings.Split(venue, ",")
	n := len(parts)
	if n < 2 {
		return 0, 0, false
	}
	cc := strings.TrimSpace(parts[n-1])
	if len(cc) != 2 {
		return 0, 0, false
	}
	var names []string
	for i := n - 2; i >= 0 && i >= n-4; i-- {
		p := postcode.ReplaceAllString(strings.TrimSpace(bracket.ReplaceAllString(parts[i], "")), "")
		names = append(names, p)
		for _, q := range strings.Split(p, " - ") {
			if q = strings.TrimSpace(q); q != p {
				names = append(names, q)
			}
		}
	}
	// Then the leading words of each part ("Struppen OT Naundorf").
	for _, nm := range append([]string(nil), names...) {
		ws := strings.Fields(nm)
		for k := len(ws) - 1; k >= 1; k-- {
			names = append(names, strings.Join(ws[:k], " "))
		}
	}
	if Locate != nil {
		for _, name := range names {
			if lat, lon, ok := Locate(name, cc); ok {
				return lat, lon, true
			}
		}
	}
	if im == nil || *lookups >= 30 || len(names) == 0 {
		return 0, 0, false
	}
	*lookups++
	return geocode(ctx, im, names[0]+", "+cc)
}

// EventLink is an event's own page: its URL, or the first link in its text
// on the feed's site, or the feed itself.
func EventLink(ev *importer.Event, feedURL string) string {
	if ev.Link != "" {
		return ev.Link
	}
	if m := linkRE.FindString(ev.Text); m != "" && hostOf(m) == hostOf(feedURL) {
		return m
	}
	return feedURL
}
