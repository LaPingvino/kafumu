package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/meetup"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// The feeds job reads the sources of areas people looked at (81d): a news
// feed's items go out with the area's bundle, a calendar to the meetups.
func TestPullSources(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rss":
			fmt.Fprintf(w, `<rss version="2.0"><channel><title>Krant</title><item><title>Kermis op het plein</title><link>https://krant.example/kermis</link><pubDate>%s</pubDate></item></channel></rss>`, now.Add(-time.Hour).Format(time.RFC1123Z))
		case "/cal.ics":
			fmt.Fprint(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:x1\r\nDTSTART:"+now.Add(48*time.Hour).Format("20060102T150405Z")+"\r\nSUMMARY:Concert\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
		}
	}))
	defer srv.Close()
	store := oln.NewMemoryStore()
	home := &Home{Notes: oln.NewService(store), Cache: cache.New()}
	put := func(id, text, cell string) {
		store.Put(t.Context(), &oln.Note{ID: id, Raw: "v2;" + id, Text: text, Cell: cell, Tags: []string{"geo" + cell, "feed"}, Src: "joop", Author: "joop", Bits: 12, At: now, ExpiresAt: now.Add(24 * time.Hour)})
	}
	put(strings.Repeat("a", 40), "📰 Krant "+srv.URL+"/rss", "8ccgmw")
	put(strings.Repeat("b", 40), "📰 Zaal "+srv.URL+"/cal.ics", "8ccgmw")
	put(strings.Repeat("c", 40), "📰 Elders "+srv.URL+"/rss", "9f469g") // nobody looked there
	home.noteCell(t.Context(), "8ccgmw")
	m := &Meetups{Home: home, Svc: meetup.NewService(meetup.NewMemoryStore()), Importer: &importer.Importer{Client: srv.Client()}}
	cals, sum := m.pullSources(t.Context(), now)
	if len(cals) != 1 || cals[0].Kind != "ics" || cals[0].Cell != "8ccgmw" {
		t.Fatalf("calendars = %+v (%s)", cals, sum)
	}
	news := home.News(t.Context(), []string{"8ccgmw"})
	if len(news) != 1 || news[0].Title != "Kermis op het plein" || news[0].By != "joop" || news[0].Bits != 12 || news[0].Cell != "8ccgmw" {
		t.Fatalf("news = %+v (%s)", news, sum)
	}
	if other := home.News(t.Context(), []string{"9f469g"}); len(other) != 0 {
		t.Fatalf("an area nobody looked at was read: %+v", other)
	}
}
