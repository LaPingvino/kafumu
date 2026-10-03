package importer

import (
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // TZIDs in calendars need zone data on App Engine too
)

// ParseICS reads VEVENTs from an iCalendar file (Luma calendars, Meetup
// groups, Google Calendar…). It understands folding, escapes, UTC and
// TZID times, all-day dates, LOCATION, GEO, URL and DESCRIPTION.
func ParseICS(text string) []*Event {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	var out []*Event
	var cur *Event
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, params, _ := strings.Cut(name, ";")
		switch strings.ToUpper(key) {
		case "BEGIN":
			if strings.EqualFold(value, "VEVENT") {
				cur = &Event{}
			}
		case "END":
			if strings.EqualFold(value, "VEVENT") && cur != nil {
				if cur.Title != "" && !cur.Start.IsZero() {
					out = append(out, cur)
				}
				cur = nil
			}
		}
		if cur == nil {
			continue
		}
		switch strings.ToUpper(key) {
		case "SUMMARY":
			cur.Title = unescape(value)
		case "DTSTART":
			cur.Start = icsTime(value, params)
		case "DTEND":
			cur.End = icsTime(value, params)
		case "LOCATION":
			cur.Venue = clip(unescape(value), 200)
		case "DESCRIPTION":
			cur.Text = clip(unescape(value), 900)
		case "URL":
			cur.Link = strings.TrimSpace(value)
		case "GEO":
			if la, lo, ok := strings.Cut(value, ";"); ok {
				lat, e1 := strconv.ParseFloat(la, 64)
				lon, e2 := strconv.ParseFloat(lo, 64)
				if e1 == nil && e2 == nil {
					cur.Lat, cur.Lon, cur.HasGeo = lat, lon, true
				}
			}
		}
	}
	return out
}

func icsTime(v, params string) time.Time {
	loc := time.UTC
	for _, p := range strings.Split(params, ";") {
		if k, val, ok := strings.Cut(p, "="); ok && strings.EqualFold(k, "TZID") {
			if l, err := time.LoadLocation(strings.Trim(val, `"`)); err == nil {
				loc = l
			}
		}
	}
	if strings.HasSuffix(v, "Z") {
		t, _ := time.Parse("20060102T150405Z", v)
		return t
	}
	for _, layout := range []string{"20060102T150405", "20060102"} {
		if t, err := time.ParseInLocation(layout, v, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

func unescape(s string) string {
	return strings.NewReplacer(`\n`, "\n", `\N`, "\n", `\,`, ",", `\;`, ";", `\\`, `\`).Replace(strings.TrimSpace(s))
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
