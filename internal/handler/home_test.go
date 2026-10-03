package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
