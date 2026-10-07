package handler

import (
	"context"
	"encoding/json"
	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/meetup"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// A brand admin can be a person or a business: a business's managers
// manage the brand whether or not they act as it; others can't, and a
// manager removed from the business loses it at once.
func TestBrandAdminBusiness(t *testing.T) {
	ctx := context.Background()
	_, home, _ := newServerWithMeetups(t)
	home.Brands, home.Biz = brand.New(nil), business.New(nil)
	org, _ := home.Biz.Create(ctx, "Bahá'í Office", "community", "", "mgr1", time.Now())
	home.Brands.Save(ctx, &brand.Brand{Host: "bahais.test", Name: "Bahá'í Local", Admins: []string{"person1", "biz:" + org.ID}})
	page := func(u *account.User) bool {
		r := httptest.NewRequest("GET", "/account", nil)
		r.Host = "bahais.test"
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		return home.newPage(r, "").BrandAdmin
	}
	if !page(&account.User{ID: "person1"}) {
		t.Fatal("a personal brand admin lost access")
	}
	if !page(&account.User{ID: "mgr1"}) {
		t.Fatal("the business's manager can't manage the brand")
	}
	if page(&account.User{ID: "stranger"}) {
		t.Fatal("a stranger manages the brand")
	}
	org.Managers = []string{"mgr2"}
	home.Biz.Save(ctx, org)
	home.forgetBiz(org.ID)
	if page(&account.User{ID: "mgr1"}) || !page(&account.User{ID: "mgr2"}) {
		t.Fatal("brand access doesn't follow the business's managers")
	}
}

// On a brand's own domain (46c): no "we moved" banner, links and passkeys
// belong to that domain; the plain host keeps the configured origin.
func TestBrandDomain(t *testing.T) {
	_, home, svc := newServerWithMeetups(t)
	home.Cfg.Origin, home.Cfg.LegacyOrigins, home.Cfg.Passkeys = "https://kafumu.test", []string{"https://old.test"}, true
	home.Brands = brand.New(nil)
	home.Brands.Save(context.Background(), &brand.Brand{Host: "bahais.test", Name: "Bahá'í Local"})
	req := func(host string) *http.Request {
		r := httptest.NewRequest("GET", "/account", nil)
		r.Host = host
		return r
	}
	if p := home.newPage(req("bahais.test"), ""); p.MovedTo != "" || !p.Passkeys {
		t.Fatalf("brand page: moved %q, passkeys %v", p.MovedTo, p.Passkeys)
	}
	if p := home.newPage(req("old.test"), ""); p.MovedTo == "" {
		t.Fatal("old domain lost its moved banner")
	}
	if o := home.Origin(req("bahais.test")); o != "https://bahais.test" {
		t.Fatalf("brand origin %q", o)
	}
	if o := home.Origin(req("kafumu.test")); o != "https://kafumu.test" {
		t.Fatalf("plain origin %q", o)
	}
	pk := NewPasskeys(&Accounts{Home: home, Svc: svc}, "https://kafumu.test", nil)
	if pk == nil {
		t.Fatal("no passkeys")
	}
	if id := pk.waFor(req("bahais.test")).Config.RPID; id != "bahais.test" {
		t.Fatalf("brand passkeys bound to %q", id)
	}
	if id := pk.waFor(req("kafumu.test")).Config.RPID; id != "kafumu.test" {
		t.Fatalf("plain passkeys bound to %q", id)
	}
}

// A citywide event (placed only by its city) is in the bundle anywhere in
// that city; a precise one at the centre stays local.
func TestCityWideMeetup(t *testing.T) {
	ctx := context.Background()
	_, home, _ := newServerWithMeetups(t)
	lat, lon, km, _ := home.Gaz.LocateArea("Parizo", "FR")
	centre := geo.Cell(lat, lon)
	store := home.Meetups.Store
	for _, m := range []*meetup.Meetup{
		{ID: "citywide", Title: "Parolrondo", Cell: centre, AreaKm: km, StartAt: time.Now().Add(24 * time.Hour), EndAt: time.Now().Add(26 * time.Hour), ExpiresAt: time.Now().Add(48 * time.Hour)},
		{ID: "precise", Title: "At the fountain", Cell: centre, StartAt: time.Now().Add(24 * time.Hour), EndAt: time.Now().Add(26 * time.Hour), ExpiresAt: time.Now().Add(48 * time.Hour)},
	} {
		if err := store.Put(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest("GET", "/bundle?cells="+geo.Cell(48.892, 2.236), nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
	r.Header.Set("Accept-Language", "en")
	w := httptest.NewRecorder()
	home.Bundle(w, r)
	body := w.Body.String()
	if !strings.Contains(body, "Parolrondo") || strings.Contains(body, "At the fountain") {
		t.Fatalf("La Défense bundle: citywide %v, precise %v", strings.Contains(body, "Parolrondo"), strings.Contains(body, "At the fountain"))
	}
}

// Widening a quiet area (wide=1): meetups from rings further out, more
// cells than a normal bundle, and no Bluesky fetch during the request
// (nothing cached here: no posts; the rest is queued for later).
func TestWideBundle(t *testing.T) {
	ctx := context.Background()
	_, home, _ := newServerWithMeetups(t)
	// The device's first widening band: rings 4 and 5 (72 cells).
	cells := geo.Rings("8ccgmw", 5)[len(geo.Rings("8ccgmw", 3)):]
	last := cells[len(cells)-1]
	home.Meetups.Store.Put(ctx, &meetup.Meetup{ID: "far", Title: "Far away kafo", Cell: last, StartAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour), ExpiresAt: time.Now().Add(48 * time.Hour)})
	r := httptest.NewRequest("GET", "/bundle?wide=1&cells="+strings.Join(cells, ","), nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
	r.Header.Set("Accept-Language", "en")
	w := httptest.NewRecorder()
	home.Bundle(w, r)
	body := w.Body.String()
	if len(cells) <= maxBundleCells || !strings.Contains(body, "Far away kafo") || !strings.Contains(body, `"posts":[]`) {
		t.Fatalf("wide bundle (%d cells): %.200s", len(cells), body)
	}
}

// Wide bundles include Bluesky posts already cached for the outer cells
// and queue the rest for the slow background search; never a fetch now.
func TestWideBundlePosts(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	cells := geo.Rings("8ccgmw", 5)[len(geo.Rings("8ccgmw", 3)):]
	home.Bsky.Prime(geo.Tag(cells[0]), 25, []bsky.Post{{URI: "at://x/app.bsky.feed.post/1", Text: "Kafo en la vilaĝo"}})
	r := httptest.NewRequest("GET", "/bundle?wide=1&cells="+strings.Join(cells, ","), nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
	r.Header.Set("Accept-Language", "en")
	w := httptest.NewRecorder()
	home.Bundle(w, r)
	if !strings.Contains(w.Body.String(), "Kafo en la vilaĝo") {
		t.Fatalf("cached post missing: %.200s", w.Body.String())
	}
	if n := home.Bsky.QueueLen(); n < len(cells)-1-5 { // the rest queued (the worker may have taken a few)
		t.Fatalf("queued %d of %d uncached cells", n, len(cells)-1)
	}
}

// The outer rings also look for their towns' tags (#evora…), slowly.
func TestWideBundleTownTags(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	c := geo.Cell(38.571, -7.909) // Évora
	cells := geo.Rings(c, 5)[len(geo.Rings(c, 3)):]
	town := home.Gaz.TownTags(cells, 4)
	if len(town) == 0 {
		t.Fatal("no towns in the outer rings around Évora")
	}
	t.Logf("town tags around Évora: %v", town)
	home.Bsky.Prime(town[0], 25, []bsky.Post{{URI: "at://x/app.bsky.feed.post/2", Text: "Feira no fim de semana"}})
	r := httptest.NewRequest("GET", "/bundle?wide=1&cells="+strings.Join(cells, ","), nil)
	r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
	r.Header.Set("Accept-Language", "pt")
	w := httptest.NewRecorder()
	home.Bundle(w, r)
	if !strings.Contains(w.Body.String(), "Feira no fim de semana") {
		t.Fatalf("town tag #%s not used: %.200s", town[0], w.Body.String())
	}
}

// A meetup's language tag reads as a language, not as "#lang:epo".
func TestTagLabel(t *testing.T) {
	f := Funcs["tagLabel"].(func(string) string)
	if got := f("lang:epo"); got != "🗣 Esperanto" {
		t.Fatalf("lang:epo → %q", got)
	}
	if got := f("esperanto"); got != "#esperanto" {
		t.Fatalf("esperanto → %q", got)
	}
}

// Bluesky writes done as a business go to the business's account, or to
// none, never to the manager's own (76d); otherwise to yours.
func TestBskyAccount(t *testing.T) {
	me := &account.User{ID: "u1", DID: "did:plc:me", ATSession: "s-me"}
	withBsky := &business.Business{ID: "b1", DID: "did:plc:cafe", ATSession: "s-cafe"}
	without := &business.Business{ID: "b2"}
	if d, s := bskyAccount(me, nil); d != "did:plc:me" || s != "s-me" {
		t.Fatalf("as yourself: %q %q", d, s)
	}
	if d, _ := bskyAccount(me, withBsky); d != "did:plc:cafe" {
		t.Fatalf("as a business with Bluesky: %q", d)
	}
	if d, _ := bskyAccount(me, without); d != "" {
		t.Fatalf("as a business without Bluesky it went to %q", d)
	}
}

// /api/meetups (77d): the current state of meetups by id; ones that no
// longer exist are listed as gone.
func TestMeetupsByIDs(t *testing.T) {
	svc := meetup.NewService(meetup.NewMemoryStore())
	m := &meetup.Meetup{Title: "Kafo", Cell: "8ccgmw", StartAt: time.Now().Add(time.Hour), EndAt: time.Now().Add(2 * time.Hour)}
	if err := svc.Create(context.Background(), m, "u1", "host"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Toggle(context.Background(), m.ID, "u2"); err != nil { // host + u2
		t.Fatal(err)
	}
	h := &Meetups{Svc: svc}
	rec := httptest.NewRecorder()
	h.ByIDs(rec, httptest.NewRequest("GET", "/api/meetups?ids="+m.ID+",nope", nil))
	var out struct {
		Meetups []struct {
			ID    string `json:"id"`
			Going int    `json:"going"`
		} `json:"meetups"`
		Gone []string `json:"gone"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err, rec.Body.String())
	}
	if len(out.Meetups) != 1 || out.Meetups[0].ID != m.ID || out.Meetups[0].Going != 2 || len(out.Gone) != 1 || out.Gone[0] != "nope" {
		t.Fatalf("got %+v", out)
	}
	rec = httptest.NewRecorder()
	h.ByIDs(rec, httptest.NewRequest("GET", "/api/meetups", nil))
	if rec.Code != 400 {
		t.Fatalf("no ids: %d", rec.Code)
	}
}
