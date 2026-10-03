package feeds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

func TestToMeetupAndIDsAreStable(t *testing.T) {
	ev := &importer.Event{Title: "Kafo", Start: time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC), Link: "https://luma.com/x", Lat: 38.768, Lon: -9.094, HasGeo: true}
	a := toMeetup(ev, Feed{URL: "https://luma.com/lisbon", Tags: []string{"lisbon"}}, "luma.com")
	b := toMeetup(ev, Feed{URL: "https://luma.com/lisbon"}, "luma.com")
	if a.ID != b.ID || a.Cell != "8ccgqw" || a.Via != "luma.com" || a.AuthorID != "feed" {
		t.Errorf("a = %+v", a)
	}
	if toMeetup(&importer.Event{Title: "x", Start: ev.Start}, Feed{}, "") != nil {
		t.Error("event without a place imported")
	}
	if Load()[0].URL == "" {
		t.Error("no feeds")
	}
}

func TestSyncKeepsRSVPs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<script type="application/ld+json">{"@type":"Event","name":"Kafo","startDate":"2026-11-10T15:00:00Z",
		"url":"https://luma.com/x","location":{"geo":{"latitude":38.768,"longitude":-9.094}}}</script>`))
	}))
	defer srv.Close()
	im := importer.New()
	im.Client = srv.Client() // the test server is on loopback, which New() refuses
	store := meetup.NewMemoryStore()
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	fs := []Feed{{URL: srv.URL, Kind: "jsonld"}}
	if r := Sync(context.Background(), fs, im, store, now); r.Saved != 1 {
		t.Fatalf("first sync: %s %v", r, r.Errors)
	}
	ms, _ := store.InCells(context.Background(), []string{"8ccgqw"}, now)
	store.Update(context.Background(), ms[0].ID, func(m *meetup.Meetup) error { m.RSVPs = []string{"u1"}; return nil })
	Sync(context.Background(), fs, im, store, now)
	ms, _ = store.InCells(context.Background(), []string{"8ccgqw"}, now)
	if len(ms) != 1 || len(ms[0].RSVPs) != 1 {
		t.Errorf("after resync: %+v", ms)
	}
}

func TestATEvent(t *testing.T) {
	raw := `{"value":{"name":"Kafo","startsAt":"2026-11-10T15:00:00Z","mode":"community.lexicon.calendar.event#inperson",
	"locations":[{"$type":"community.lexicon.location.geo","latitude":"38.768","longitude":"-9.094","name":"FIL"}]}}`
	ev := toATEvent(raw, "https://smokesignal.events/x/y")
	if ev == nil || !ev.HasGeo || ev.Venue != "FIL" || ev.Title != "Kafo" {
		t.Fatalf("event = %+v", ev)
	}
	addrOnly := `{"value":{"name":"Kafo","startsAt":"2026-11-10T15:00:00Z","locations":[{"$type":"community.lexicon.location.address","name":"Café X","street":"Rua Y 1","locality":"Lisboa"}]}}`
	if ev := toATEvent(addrOnly, ""); ev == nil || ev.HasGeo || ev.Venue != "Café X, Rua Y 1, Lisboa" {
		t.Errorf("address-only event = %+v", ev)
	}
	virtual := strings.Replace(raw, "#inperson", "#virtual", 1)
	if toATEvent(virtual, "") != nil {
		t.Error("virtual event imported")
	}
}
