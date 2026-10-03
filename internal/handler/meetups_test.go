package handler

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMeetupFlow(t *testing.T) {
	h, home, _ := newServerWithMeetups(t)
	w := do(h, "POST", "/account/start", url.Values{"next": {"/meetups/new"}}, "")
	cred := cookieFrom(w)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "next=/meetups/new") {
		t.Errorf("start redirect = %q", loc)
	}
	start := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	home.Meetups.Now = func() time.Time { return start.Add(-24 * time.Hour) }
	w = do(h, "POST", "/meetups", url.Values{"title": {"Kafo"}, "cell": {"8ccgqw"}, "start_iso": {start.Format(time.RFC3339)},
		"duration": {"60"}, "tags": {"esperanto"}}, cred)
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/meetups/") || strings.Contains(loc, "err") {
		t.Fatalf("create redirect = %q", loc)
	}
	ms, _ := home.Meetups.InCells(t.Context(), []string{"8ccgqw"})
	if len(ms) != 1 || ms[0].Tags[0] != "websummit" || ms[0].Going != 1 {
		t.Fatalf("meetups = %+v", ms)
	}
	if w = do(h, "GET", loc, nil, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Kafo") {
		t.Errorf("show: %d", w.Code)
	}
	// Without an account, RSVP sends you to make one.
	if w = do(h, "POST", loc+"/rsvp", url.Values{}, ""); !strings.Contains(w.Header().Get("Location"), "/account?next=") {
		t.Errorf("anonymous rsvp redirect = %q", w.Header().Get("Location"))
	}
	if w = do(h, "POST", "/meetups", url.Values{"title": {""}, "cell": {"8ccgqw"}}, cred); !strings.Contains(w.Header().Get("Location"), "err=invalid") {
		t.Errorf("invalid create redirect = %q", w.Header().Get("Location"))
	}
}
