package meetup

import (
	"strings"
	"testing"
	"time"
)

func TestICS(t *testing.T) {
	start := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	out := ICS("Kafumu #geo8ccgqw", "https://kafumu.com", []*Meetup{{
		ID: "abc", Title: "Kafo; and, more", StartAt: start, EndAt: start.Add(time.Hour), Venue: "Pavilion 2",
		Text: strings.Repeat("long text ", 20), CreatedAt: start, Tags: []string{"websummit"},
	}})
	for _, want := range []string{"BEGIN:VCALENDAR\r\n", "DTSTART:20261110T150000Z\r\n", `SUMMARY:Kafo\; and\, more`,
		"URL:https://kafumu.com/meetups/abc\r\n", "END:VCALENDAR\r\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, l := range strings.Split(out, "\r\n") {
		if len(l) > 75 {
			t.Errorf("unfolded line: %q", l)
		}
	}
}
