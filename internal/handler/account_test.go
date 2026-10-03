package handler

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

func newServer(t *testing.T) (http.Handler, *account.Service) {
	h, _, svc := newServerWithMeetups(t)
	return h, svc
}

func newServerWithMeetups(t *testing.T) (http.Handler, *Home, *account.Service) {
	t.Helper()
	tmpl := template.Must(template.New("").Funcs(Funcs).ParseGlob("../../templates/*.html"))
	home := &Home{Cfg: &config.Config{Brand: "Kafumu", Origin: "https://kafumu.test"}, Tmpl: tmpl, Bsky: bsky.NewClient(),
		Gaz: gazetteer.Load(), Meetups: meetup.NewService(meetup.NewMemoryStore())}
	svc := account.NewService(account.NewMemoryStore())
	a := &Accounts{Home: home, Svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /account", a.Show)
	mux.HandleFunc("POST /account/start", a.Start)
	mux.HandleFunc("POST /account/name", a.SetName)
	mux.HandleFunc("POST /account/delete", a.Delete)
	mux.HandleFunc("GET /auth/link", a.Link)
	m := &Meetups{Home: home, Svc: home.Meetups}
	mux.HandleFunc("GET /meetups/new", m.New)
	mux.HandleFunc("POST /meetups", m.Create)
	mux.HandleFunc("GET /meetups/{id}", m.Show)
	mux.HandleFunc("POST /meetups/{id}/rsvp", m.RSVP)
	return a.Middleware(mux), home, svc
}

func do(h http.Handler, method, path string, form url.Values, cookie string) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := browser(httptest.NewRequest(method, path, body))
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func cookieFrom(w *httptest.ResponseRecorder) string {
	for _, c := range w.Result().Cookies() {
		if c.Name == cookieName {
			return c.Value
		}
	}
	return ""
}

func TestAccountFlow(t *testing.T) {
	h, _ := newServer(t)

	// Looking costs nothing: no cookie is set on a page view.
	w := do(h, "GET", "/account", nil, "")
	if cookieFrom(w) != "" || !strings.Contains(w.Body.String(), "/account/start") {
		t.Fatalf("anonymous /account: cookie %q", cookieFrom(w))
	}

	// Bots can't create accounts.
	r := httptest.NewRequest("POST", "/account/start", nil)
	bw := httptest.NewRecorder()
	h.ServeHTTP(bw, r)
	if bw.Code != http.StatusForbidden {
		t.Errorf("bot start: %d", bw.Code)
	}

	w = do(h, "POST", "/account/start", url.Values{}, "")
	cred := cookieFrom(w)
	if cred == "" {
		t.Fatal("no cookie after start")
	}
	w = do(h, "GET", "/account?new=1", nil, cred)
	if !strings.Contains(w.Body.String(), "https://kafumu.test/auth/link?k=") {
		t.Fatalf("magic link missing:\n%s", w.Body.String())
	}

	// The magic link signs in a fresh device.
	w = do(h, "GET", "/auth/link?k="+url.QueryEscape(cred), nil, "")
	if cookieFrom(w) != cred {
		t.Errorf("magic link cookie = %q", cookieFrom(w))
	}

	do(h, "POST", "/account/name", url.Values{"username": {"Joop"}}, cred)
	if w = do(h, "GET", "/account", nil, cred); !strings.Contains(w.Body.String(), "@joop") {
		t.Error("username not shown")
	}

	do(h, "POST", "/account/delete", url.Values{"confirm": {"yes"}}, cred)
	if w = do(h, "GET", "/auth/link?k="+url.QueryEscape(cred), nil, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("deleted account link: %d", w.Code)
	}
}
