package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/vault"
)

// Business sync (75c, server mode): only managers reach the business's key
// and vault; off means no vault at all; switching to server mode gives a
// key the managers' devices fetch.
func TestBusinessSyncServerMode(t *testing.T) {
	_, home, svc := newServerWithMeetups(t)
	a := &Accounts{Home: home, Svc: svc, Vault: vault.NewMemoryStore()}
	bs := &Businesses{Home: home, Accounts: a, Store: business.New(nil)}
	home.Biz = bs.Store
	mux := http.NewServeMux()
	mux.HandleFunc("POST /account/start", a.Start)
	mux.HandleFunc("POST /business", bs.Create)
	mux.HandleFunc("POST /business/{id}/sync", bs.Sync)
	mux.HandleFunc("GET /api/business/{id}/vault", bs.VaultAPI)
	mux.HandleFunc("PUT /api/business/{id}/vault", bs.VaultAPI)
	mux.HandleFunc("GET /api/business/{id}/key", bs.KeyAPI)
	h := a.Middleware(mux)

	owner := cookieFrom(do(h, "POST", "/account/start", url.Values{}, ""))
	other := cookieFrom(do(h, "POST", "/account/start", url.Values{}, ""))
	if owner == "" || other == "" {
		t.Fatal("no accounts")
	}
	do(h, "POST", "/business", url.Values{"name": {"Café Teste"}, "kind": {"cafe"}}, owner)
	all, _ := bs.Store.All(context.Background())
	if len(all) != 1 {
		t.Fatalf("businesses: %d", len(all))
	}
	id := all[0].ID

	if w := do(h, "GET", "/api/business/"+id+"/vault", nil, owner); w.Code != http.StatusNotFound {
		t.Fatalf("vault with sync off: %d", w.Code)
	}
	if w := do(h, "POST", "/business/"+id+"/sync", url.Values{"mode": {"server"}}, other); w.Code != http.StatusNotFound {
		t.Fatalf("a non-manager switched sync on: %d", w.Code)
	}
	do(h, "POST", "/business/"+id+"/sync", url.Values{"mode": {"server"}}, owner)
	w := do(h, "GET", "/api/business/"+id+"/key", nil, owner)
	var k struct{ Key string }
	json.Unmarshal(w.Body.Bytes(), &k)
	if w.Code != http.StatusOK || len(k.Key) != 44 {
		t.Fatalf("manager's key: %d %q", w.Code, w.Body)
	}
	for _, p := range []string{"/key", "/vault"} {
		if w := do(h, "GET", "/api/business/"+id+p, nil, other); w.Code != http.StatusNotFound {
			t.Fatalf("non-manager GET %s: %d", p, w.Code)
		}
	}
	r := browser(httpRequest("PUT", "/api/business/"+id+"/vault", "sealed-blob"))
	r.Header.Set("If-Match", "0")
	r.AddCookie(&http.Cookie{Name: cookieName, Value: owner})
	rec := newRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("manager PUT: %d %s", rec.Code, rec.Body)
	}
	if w := do(h, "GET", "/api/business/"+id+"/vault", nil, owner); !strings.Contains(w.Body.String(), "sealed-blob") {
		t.Fatalf("manager GET: %s", w.Body)
	}
	// Turning it off: the server forgets the key.
	do(h, "POST", "/business/"+id+"/sync", url.Values{"mode": {"off"}}, owner)
	if w := do(h, "GET", "/api/business/"+id+"/key", nil, owner); w.Code != http.StatusNotFound {
		t.Fatalf("key after sync off: %d", w.Code)
	}
}

func httpRequest(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, strings.NewReader(body))
}

func newRecorder() *httptest.ResponseRecorder { return httptest.NewRecorder() }
