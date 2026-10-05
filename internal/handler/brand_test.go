package handler

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/account"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/brand"
)

// A brand's host shows its own name, colour, main button, tagline and
// starting tag; the plain host shows Kafumu.
func TestBrandPage(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	home.Brands = brand.New(nil)
	home.Brands.Save(context.Background(), &brand.Brand{Host: "bahais.test", Name: "Bahá'í Local", Tagline: "Pray and serve together, near you",
		Accent: "#3a5f8a", Button: "🙏 Who wants to pray with me?", Tags: []string{"bahai"}})
	get := func(host string) string {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = host
		r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
		r.Header.Set("Accept-Language", "en")
		w := httptest.NewRecorder()
		home.ShowHome(w, r)
		return w.Body.String()
	}
	b := get("bahais.test")
	for _, want := range []string{"<title>Bahá&#39;í Local</title>", "--accent: #3a5f8a", "🙏 Who wants to pray with me?", "Pray and serve together, near you", `data-brand-tag="bahai"`} {
		if !strings.Contains(b, want) {
			t.Errorf("brand page lacks %q", want)
		}
	}
	if p := get("kafumu.test"); strings.Contains(p, "--accent: #3a5f8a") || strings.Contains(p, "pray with me") || !strings.Contains(p, "Kafumu") {
		t.Error("plain host shows the brand")
	}
}

// Brand admins edit their brand on its own host; nobody else can.
func TestBrandAdmin(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	home.Brands = brand.New(nil)
	home.Brands.Save(context.Background(), &brand.Brand{Host: "bahais.test", Name: "Bahá'í Local", Admins: []string{"admin1"}})
	call := func(method, host, body string, u *account.User) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/brand", strings.NewReader(body))
		r.Host = host
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if u != nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		}
		w := httptest.NewRecorder()
		home.BrandPage(w, r)
		return w
	}
	admin, other := &account.User{ID: "admin1", Username: "ana"}, &account.User{ID: "other", Username: "bo"}
	for _, c := range []struct {
		host string
		u    *account.User
	}{{"bahais.test", other}, {"bahais.test", nil}, {"kafumu.test", admin}} {
		if w := call("GET", c.host, "", c.u); w.Code != 404 {
			t.Fatalf("%s as %v: %d", c.host, c.u, w.Code)
		}
	}
	if w := call("GET", "bahais.test", "", admin); w.Code != 200 || !strings.Contains(w.Body.String(), `name="tagline"`) {
		t.Fatalf("admin GET: %d", w.Code)
	}
	if w := call("POST", "bahais.test", "name=Bah%C3%A1%27%C3%AD+Local&tagline=Pray+together&button=%F0%9F%99%8F+Pray+with+me%3F&accent=%233a5f8a&tags=bahai+prayer", admin); w.Code != 303 {
		t.Fatalf("admin POST: %d %s", w.Code, w.Body)
	}
	b := home.Brands.For(context.Background(), "bahais.test")
	if b.Tagline != "Pray together" || b.Button != "🙏 Pray with me?" || len(b.Tags) != 2 || b.Admins[0] != "admin1" {
		t.Fatalf("after save: %+v", b)
	}
	r := httptest.NewRequest("GET", "/account", nil)
	r.Host = "bahais.test"
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, admin))
	if !home.newPage(r, "").BrandAdmin {
		t.Fatal("account page won't show the brand link")
	}
}
