package atp

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Record builders: plain maps in the shape of the lexicons, so they're easy
// to test and to read next to the lexicon docs.

// EventRecord is a community.lexicon.calendar.event (as Smoke Signal uses).
// The location is the #geo cell centre — the same coarse place Kafumu
// shows — plus the venue line the host typed.
func EventRecord(name, text string, start, end time.Time, venue string, lat, lon float64, link string, now time.Time) map[string]any {
	loc := map[string]any{
		"$type":     "community.lexicon.location.geo",
		"latitude":  fmt.Sprintf("%.4f", lat),
		"longitude": fmt.Sprintf("%.4f", lon),
	}
	if venue != "" {
		loc["name"] = venue
	}
	rec := map[string]any{
		"$type":     "community.lexicon.calendar.event",
		"name":      name,
		"createdAt": now.UTC().Format(time.RFC3339),
		"startsAt":  start.UTC().Format(time.RFC3339),
		"endsAt":    end.UTC().Format(time.RFC3339),
		"mode":      "community.lexicon.calendar.event#inperson",
		"status":    "community.lexicon.calendar.event#scheduled",
		"locations": []any{loc},
	}
	if text != "" {
		rec["description"] = text
	}
	if link != "" {
		rec["uris"] = []any{map[string]any{"uri": link}}
	}
	return rec
}

// RSVPRecord is a community.lexicon.calendar.rsvp saying "going".
func RSVPRecord(eventURI, eventCID string, now time.Time) map[string]any {
	return map[string]any{
		"$type":     "community.lexicon.calendar.rsvp",
		"subject":   map[string]any{"uri": eventURI, "cid": eventCID},
		"status":    "community.lexicon.calendar.rsvp#going",
		"createdAt": now.UTC().Format(time.RFC3339),
	}
}

var tagRE = regexp.MustCompile(`(^|\s)#([\p{L}\p{N}_]+)`)

// PostRecord is an app.bsky.feed.post with a facet for every #hashtag, so
// #geo tags are searchable as tags, not just as text.
func PostRecord(text, lang string, now time.Time) map[string]any {
	text = strings.TrimSpace(text)
	var facets []any
	for _, m := range tagRE.FindAllStringSubmatchIndex(text, -1) {
		start, end := m[4]-1, m[5] // include the '#'
		facets = append(facets, map[string]any{
			"index":    map[string]any{"byteStart": start, "byteEnd": end},
			"features": []any{map[string]any{"$type": "app.bsky.richtext.facet#tag", "tag": text[m[4]:m[5]]}},
		})
	}
	rec := map[string]any{"$type": "app.bsky.feed.post", "text": text, "createdAt": now.UTC().Format(time.RFC3339)}
	if len(facets) > 0 {
		rec["facets"] = facets
	}
	if lang != "" {
		rec["langs"] = []string{lang}
	}
	return rec
}

// WebURL turns an at:// URI of a post or event into a link people can open.
func WebURL(uri string) string {
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(parts) != 3 {
		return ""
	}
	switch parts[1] {
	case "app.bsky.feed.post":
		return "https://bsky.app/profile/" + parts[0] + "/post/" + parts[2]
	case "community.lexicon.calendar.event":
		return "https://smokesignal.events/" + parts[0] + "/" + parts[2]
	}
	return ""
}
