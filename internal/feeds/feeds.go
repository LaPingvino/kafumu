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
	Kind string   `json:"kind"` // "jsonld" (a page with schema.org Events) or "ics"
	Tags []string `json:"tags,omitempty"`
	// Cell places events that carry no coordinates (most iCal feeds).
	Cell string `json:"cell,omitempty"`
	Note string `json:"note,omitempty"`
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
		body, err := im.Get(ctx, f.URL)
		if err != nil {
			r.Errors = append(r.Errors, err.Error())
			continue
		}
		var evs []*importer.Event
		if f.Kind == "ics" {
			evs = importer.ParseICS(body)
		} else {
			evs = importer.ParseAll(body)
		}
		host := hostOf(f.URL)
		for _, ev := range evs {
			r.Events++
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

func toMeetup(ev *importer.Event, f Feed, host string) *meetup.Meetup {
	cell := strings.ToLower(f.Cell)
	if ev.HasGeo {
		cell = geo.Cell(ev.Lat, ev.Lon)
	}
	if !geo.Valid(cell) {
		return nil
	}
	link := ev.Link
	if link == "" {
		link = f.URL
	}
	// Stable id per event, so every sync updates instead of duplicating.
	sum := sha256.Sum256([]byte(link + "|" + ev.Title + "|" + ev.Start.UTC().Format(time.RFC3339)))
	return &meetup.Meetup{
		ID: "f" + hex.EncodeToString(sum[:9]), AuthorID: "feed", Via: host,
		Title: ev.Title, Text: ev.Text, Venue: ev.Venue, Link: link, Cell: cell,
		StartAt: ev.Start.UTC(), EndAt: ev.End.UTC(), Tags: append([]string(nil), f.Tags...),
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
