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
