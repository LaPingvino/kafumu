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

// Suggested sources (81f): the sites an area's posts share most, when they
// have a feed and aren't a source there yet; platforms never count.
func TestSuggestSources(t *testing.T) {
	if got := discoverFeed(`<html><head><link rel="stylesheet" href="/s.css"><link rel="alternate" type="application/rss+xml" title="Nieuws" href="/feed/?x=1&amp;y=2"></head>`, "https://krant.example/"); got != "https://krant.example/feed/?x=1&y=2" {
		t.Fatalf("discoverFeed = %q", got)
	}
	if got := discoverFeed(`<link rel="icon" href="/f.ico">`, "https://x.example/"); got != "" {
		t.Fatalf("no feed: %q", got)
	}
	home := &Home{Cache: cache.New()}
	home.noteCell(t.Context(), "8ccgmw")
	var links [][]string
	for i := 0; i < 4; i++ {
		links = append(links, []string{"https://www.krant.example/a" + fmt.Sprint(i), "https://youtube.com/watch?v=" + fmt.Sprint(i)})
	}
	links = append(links, []string{"https://once.example/x"})
	home.noteHosts(t.Context(), "8ccgmw", links)
	home.Cache.Set(t.Context(), feedOfKey("krant.example"), []byte("https://krant.example/feed"), time.Hour) // probed before
	m := &Meetups{Home: home, Importer: &importer.Importer{Client: http.DefaultClient}}
	m.suggestSources(t.Context(), time.Now())
	got := home.Suggestions(t.Context(), "8ccgmw")
	if len(got) != 1 || got[0].Host != "krant.example" || got[0].Feed != "https://krant.example/feed" || got[0].Count != 4 {
		t.Fatalf("suggestions = %+v", got)
	}
}

// Reading a source right after it's added (81i): only for people who may add
// sources; its news lands in its area at once.
func TestPullSourceNow(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<rss version="2.0"><channel><item><title>Feira do Barreiro</title><link>https://rostos.example/feira</link><pubDate>%s</pubDate></item></channel></rss>`, now.Format(time.RFC1123Z))
	}))
	defer srv.Close()
	store := oln.NewMemoryStore()
	notes := oln.NewService(store)
	allowed := false
	notes.SourceOK = func(*http.Request) bool { return allowed }
	home := &Home{Notes: notes, Cache: cache.New()}
	id := strings.Repeat("e", 40)
	store.Put(t.Context(), &oln.Note{ID: id, Raw: "v2;e", Text: "📰 Rostos " + srv.URL + "/rss", Cell: "8ccgmw", Tags: []string{"geo8ccgmw", "feed"}, Src: "lapingvino", At: now, ExpiresAt: now.Add(time.Hour)})
	m := &Meetups{Home: home, Svc: meetup.NewService(meetup.NewMemoryStore()), Importer: &importer.Importer{Client: srv.Client()}}
	pull := func() int {
		w := httptest.NewRecorder()
		m.PullSource(w, browser(httptest.NewRequest("POST", "/api/oln/source/pull?id="+id, nil)))
		return w.Code
	}
	_ = allowed
	if c := pull(); c != 200 {
		t.Fatalf("pull: %d", c)
	}
	if news := home.News(t.Context(), []string{"8ccgmw"}); len(news) != 1 || news[0].Title != "Feira do Barreiro" {
		t.Fatalf("news = %+v", news)
	}
}

// A quiet source (nothing this week) still shows its newest few of the
// month; nothing older than that.
func TestQuietSource(t *testing.T) {
	now := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<rss version="2.0"><channel>`)
		for i, d := range []int{8, 9, 12, 15, 40} {
			fmt.Fprintf(w, `<item><title>Item %d</title><link>https://x.example/%d</link><pubDate>%s</pubDate></item>`, i, i, now.Add(-time.Duration(d)*24*time.Hour).Format(time.RFC1123Z))
		}
		fmt.Fprint(w, `</channel></rss>`)
	}))
	defer srv.Close()
	m := &Meetups{Home: &Home{}, Importer: &importer.Importer{Client: srv.Client()}}
	items, _ := m.readSource(t.Context(), &oln.Note{ID: strings.Repeat("f", 40), Text: "📰 Quiet " + srv.URL + "/rss", Src: "x"}, "8ccgmw", now)
	if len(items) != 3 || items[0].Title != "Item 0" || items[2].Title != "Item 2" {
		t.Fatalf("items = %+v", items)
	}
}
