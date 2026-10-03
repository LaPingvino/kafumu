package atp

import (
	"testing"
	"time"
)

func TestPostRecordFacets(t *testing.T) {
	r := PostRecord("Kafo en Lisbono? #geo8ccgqw #esperanto ĉu?", "eo", time.Unix(0, 0))
	fs := r["facets"].([]any)
	if len(fs) != 2 {
		t.Fatalf("facets = %v", fs)
	}
	text := r["text"].(string)
	for i, want := range []string{"#geo8ccgqw", "#esperanto"} {
		ix := fs[i].(map[string]any)["index"].(map[string]any)
		if got := text[ix["byteStart"].(int):ix["byteEnd"].(int)]; got != want {
			t.Errorf("facet %d covers %q, want %q", i, got, want)
		}
	}
	if r["langs"].([]string)[0] != "eo" {
		t.Errorf("langs = %v", r["langs"])
	}
}

func TestEventAndRSVP(t *testing.T) {
	start := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	e := EventRecord("Kafo", "", start, start.Add(time.Hour), "Pavilion 2", 38.7675, -9.0925, "", start)
	loc := e["locations"].([]any)[0].(map[string]any)
	if e["$type"] != "community.lexicon.calendar.event" || loc["latitude"] != "38.7675" || loc["name"] != "Pavilion 2" || e["startsAt"] != "2026-11-10T15:00:00Z" {
		t.Errorf("event = %v", e)
	}
	r := RSVPRecord("at://did:plc:x/community.lexicon.calendar.event/abc", "bafy", start)
	if r["status"] != "community.lexicon.calendar.rsvp#going" {
		t.Errorf("rsvp = %v", r)
	}
	if WebURL("at://did:plc:x/community.lexicon.calendar.event/abc") != "https://smokesignal.events/did:plc:x/abc" {
		t.Error("web url")
	}
}
