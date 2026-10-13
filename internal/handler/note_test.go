package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// /n/<id> (86c): live → its area at the line; expired but stored → shown
// as expired; gone → a page about lines fading; private lines never.
func TestShowNote(t *testing.T) {
	store := oln.NewMemoryStore()
	home := &Home{Cfg: &config.Config{Brand: "Kafumu"}, Notes: oln.NewService(store),
		Tmpl: template.Must(template.New("").Funcs(Funcs).ParseGlob("../../templates/*.html"))}
	now := time.Now()
	put := func(id, text, cell, pair string, exp time.Time) {
		if err := store.Put(t.Context(), &oln.Note{ID: id, Text: text, Cell: cell, Pair: pair, At: now.Add(-time.Hour), ExpiresAt: exp}); err != nil {
			t.Fatal(err)
		}
	}
	live, old, private := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	put(live, "Coffee?", "8ccgmw", "", now.Add(time.Hour))
	put(old, "Market today, 10:00", "8ccgmw", "", now.Add(-time.Minute))
	put(private, "secret", "", "p0123", now.Add(time.Hour))
	get := func(id string) *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /n/{id}", home.ShowNote)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, browser(httptest.NewRequest("GET", "/n/"+id, nil)))
		return w
	}
	if w := get(live); w.Code != http.StatusFound || w.Header().Get("Location") != "/?cell=8ccgmw#re-"+live[:10] {
		t.Errorf("live: %d %q", w.Code, w.Header().Get("Location"))
	}
	if w := get(old); w.Code != 200 || !strings.Contains(w.Body.String(), "Market today, 10:00") || !strings.Contains(w.Body.String(), `href="/oln"`) {
		t.Errorf("expired: %d", w.Code)
	}
	for _, id := range []string{private, strings.Repeat("d", 40)} {
		if w := get(id); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), `href="/oln"`) {
			t.Errorf("%s…: %d", id[:4], w.Code)
		}
	}
}
