package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseLuma(t *testing.T) {
	page, err := os.ReadFile("testdata/luma.html")
	if err != nil {
		t.Skip("no Luma sample")
	}
	ev, err := Parse(string(page))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Title == "" || ev.Start.IsZero() || !ev.HasGeo || !strings.Contains(ev.Venue, "Templo da Poesia") || !strings.HasPrefix(ev.Link, "https://luma.com/") {
		t.Errorf("event = %+v", ev)
	}
}

func TestParseGraphAndArrayTypes(t *testing.T) {
	page := `<html><script type="application/ld+json">{"@graph":[{"@type":"WebPage"},
	{"@type":["Thing","SocialEvent"],"name":"Esperanto &amp; kafo","startDate":"2026-11-10T15:00:00+00:00",
	"location":[{"@type":"Place","name":"Café X","address":"Rua Y, Lisboa","latitude":"38.7","longitude":"-9.1"}]}]}</script>`
	ev, err := Parse(page)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Title != "Esperanto & kafo" || ev.Venue != "Café X, Rua Y, Lisboa" || !ev.HasGeo || ev.Lat != 38.7 {
		t.Errorf("event = %+v", ev)
	}
	if _, err := Parse(`<html><title>nothing</title></html>`); err != ErrNoEvent {
		t.Errorf("no event: %v", err)
	}
}

func TestRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	if _, err := New().Fetch(context.Background(), srv.URL); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("loopback fetch err = %v", err)
	}
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "ftp://x"} {
		if _, err := New().Fetch(context.Background(), bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}

func TestParseAllItemList(t *testing.T) {
	page := `<script type="application/ld+json">{"@type":"ItemList","itemListElement":[
	{"@type":"ListItem","item":{"@type":"Event","name":"A","startDate":"2026-11-10T10:00:00Z","location":{"geo":{"latitude":38.7,"longitude":-9.1}}}},
	{"@type":"ListItem","item":{"@type":"Event","name":"B","startDate":"2026-11-11T10:00:00Z"}}]}</script>`
	evs := ParseAll(page)
	if len(evs) != 2 || evs[0].Title != "A" || !evs[0].HasGeo || evs[1].HasGeo {
		t.Errorf("ParseAll = %+v", evs)
	}
}

func TestParseICS(t *testing.T) {
	cal := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Esperanto\\, kafo\r\nDTSTART;TZID=Europe/Lisbon:20261110T150000\r\n" +
		"DTEND:20261110T170000Z\r\nLOCATION:Café X\\, Lisboa\r\nGEO:38.71;-9.14\r\nURL:https://luma.com/x\r\n" +
		"DESCRIPTION:Line one\\nline two that is long enough to be\r\n  folded\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nSUMMARY:no time\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	evs := ParseICS(cal)
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	e := evs[0]
	if e.Title != "Esperanto, kafo" || e.Venue != "Café X, Lisboa" || !e.HasGeo || e.Link != "https://luma.com/x" {
		t.Errorf("event = %+v", e)
	}
	if e.Start.UTC().Hour() != 15 { // Lisbon is UTC+0 in November
		t.Errorf("start = %v", e.Start)
	}
	if e.Text != "Line one\nline two that is long enough to be folded" {
		t.Errorf("text = %q", e.Text)
	}
}
