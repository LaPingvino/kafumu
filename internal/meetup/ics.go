package meetup

import (
	"fmt"
	"strings"
	"time"
)

// ICS renders meetups as an iCalendar file, so they land in people's
// calendars and in any aggregator that reads .ics.
func ICS(name, origin string, ms []*Meetup) string {
	var b strings.Builder
	line := func(s string) {
		// Fold at 75 octets as RFC 5545 asks.
		for len(s) > 75 {
			cut := 75
			for cut > 0 && (s[cut]&0xC0) == 0x80 {
				cut--
			}
			b.WriteString(s[:cut] + "\r\n")
			s = " " + s[cut:]
		}
		b.WriteString(s + "\r\n")
	}
	stamp := func(t time.Time) string { return t.UTC().Format("20060102T150405Z") }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Kafumu//Meetups//EN")
	line("X-WR-CALNAME:" + esc(name))
	for _, m := range ms {
		line("BEGIN:VEVENT")
		line("UID:" + m.ID + "@kafumu")
		line("DTSTAMP:" + stamp(m.CreatedAt))
		line("DTSTART:" + stamp(m.StartAt))
		line("DTEND:" + stamp(m.EndAt))
		line("SUMMARY:" + esc(m.Title))
		if m.Venue != "" {
			line("LOCATION:" + esc(m.Venue))
		}
		desc := m.Text
		if m.Link != "" {
			desc = strings.TrimSpace(desc + "\n" + m.Link)
		}
		if desc != "" {
			line("DESCRIPTION:" + esc(desc))
		}
		line(fmt.Sprintf("URL:%s/meetups/%s", origin, m.ID))
		if len(m.Tags) > 0 {
			line("CATEGORIES:" + esc(strings.Join(m.Tags, ",")))
		}
		line("END:VEVENT")
	}
	line("END:VCALENDAR")
	return b.String()
}

func esc(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", "").Replace(s)
}
