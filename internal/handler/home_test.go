package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/meetup"

	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
)

func newHome() *Home {
	return &Home{
		Cfg:  &config.Config{Brand: "Kafumu"},
		Tmpl: template.Must(template.New("home.html").Parse(`{{.Cell}}`)),
		Bsky: bsky.NewClient(),
		Gaz:  gazetteer.Load(),
	}
}

func browser(r *http.Request) *http.Request {
	r.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Firefox/140.0")
	r.Header.Set("Accept-Language", "eo,en")
	return r
}

func TestBundleRejectsBotsAndBadCells(t *testing.T) {
	h := newHome()
	cases := []struct {
		req  *http.Request
		want int
	}{
		{httptest.NewRequest("GET", "/bundle?cells=9f469w", nil), http.StatusForbidden}, // no UA: bot
		{browser(httptest.NewRequest("GET", "/bundle?cells=nope,zzzzzz", nil)), http.StatusBadRequest},
		{browser(httptest.NewRequest("GET", "/bundle", nil)), http.StatusBadRequest},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.Bundle(w, c.req)
		if w.Code != c.want {
			t.Errorf("%s: got %d, want %d", c.req.URL, w.Code, c.want)
		}
	}
}

func TestHomeOnlyAtRootAndValidCell(t *testing.T) {
	h := newHome()
	w := httptest.NewRecorder()
	h.ShowHome(w, httptest.NewRequest("GET", "/wp-login.php", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown path: got %d, want 404", w.Code)
	}
	w = httptest.NewRecorder()
	h.ShowHome(w, httptest.NewRequest("GET", "/?cell=<script>", nil))
	if w.Body.String() != "" {
		t.Errorf("invalid cell echoed: %q", w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ShowHome(w, httptest.NewRequest("GET", "/?cell=9F469V", nil))
	if w.Body.String() != "9f469v" {
		t.Errorf("cell = %q", w.Body.String())
	}
}

func TestBundleNeverReturnsNullLists(t *testing.T) {
	h := newHome()
	h.Bsky.TTL = time.Hour
	// Pre-fill the cache so the test never reaches the network.
	h.Bsky.Prime("geo6fg222", 25, nil)
	w := httptest.NewRecorder()
	h.Bundle(w, browser(httptest.NewRequest("GET", "/bundle?cells=6fg222", nil)))
	if body := w.Body.String(); strings.Contains(body, "null") {
		t.Errorf("bundle has null lists: %s", body)
	}
}

func TestCanonicalHost(t *testing.T) {
	h := &Home{Cfg: &config.Config{Origin: "https://kafumu.com"}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	for _, c := range []struct {
		method, url string
		code        int
		loc         string
	}{
		{"GET", "https://www.kafumu.com/meetups/x?y=1", 301, "https://kafumu.com/meetups/x?y=1"},
		{"POST", "https://www.kafumu.com/api/box/abc", 308, "https://kafumu.com/api/box/abc"},
		{"GET", "https://kafumu.com/", 204, ""},
		{"GET", "https://lokumo.ew.r.appspot.com/", 204, ""}, // legacy origin: the page offers the move instead
	} {
		w := httptest.NewRecorder()
		h.CanonicalHost(ok).ServeHTTP(w, httptest.NewRequest(c.method, c.url, nil))
		if w.Code != c.code || w.Header().Get("Location") != c.loc {
			t.Errorf("%s %s → %d %q", c.method, c.url, w.Code, w.Header().Get("Location"))
		}
	}
}

func TestGuessCell(t *testing.T) {
	if c := guessCell("52.040000,5.665000"); c != "9f4729" && len(c) != 6 {
		t.Errorf("guessCell = %q", c)
	}
	for _, bad := range []string{"", "0.000000,0.000000", "abc", "95,0"} {
		if c := guessCell(bad); c != "" {
			t.Errorf("guessCell(%q) = %q", bad, c)
		}
	}
}

// Events are spotted from the bundle: three upcoming meetups sharing a tag
// make it an event; place names and Kafumu's own tags never do.
func TestFoundEvents(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	now := time.Now()
	b := bundle{}
	for i := 0; i < 3; i++ {
		b.Meetups = append(b.Meetups, &meetup.Meetup{Tags: []string{"pycon", "lisbon", "coffee"}, StartAt: now.Add(time.Duration(i+1) * 24 * time.Hour), EndAt: now.Add(time.Duration(i+1)*24*time.Hour + time.Hour)})
	}
	got := home.foundEvents(&b, now)
	if len(got) != 1 || got[0].Tag != "pycon" || !got[0].Found || got[0].N != 3 || got[0].Live {
		t.Fatalf("found = %+v", got)
	}
}
