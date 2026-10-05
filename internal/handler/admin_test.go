package handler

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/business"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/report"
)

func TestAdminGate(t *testing.T) {
	h, home, svc := newServerWithMeetups(t)
	ran := ""
	a := &Admin{Home: home, Accounts: &Accounts{Home: home, Svc: svc}, Meetups: &Meetups{Home: home, Svc: home.Meetups},
		Jobs: map[string]func(context.Context) string{"purge": func(context.Context) string { ran = "purge"; return "ok" }}}
	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("GET /admin/initial", a.Initial)
	mux.HandleFunc("GET /admin", a.Show)
	mux.HandleFunc("POST /admin/action", a.Action)
	root := a.Accounts.Middleware(mux)

	cred := cookieFrom(do(h, "POST", "/account/start", url.Values{}, ""))
	if w := do(root, "GET", "/admin", nil, cred); w.Code != http.StatusNotFound {
		t.Errorf("non-admin /admin: %d", w.Code)
	}
	if w := do(root, "GET", "/admin/initial", nil, cred); w.Code != http.StatusForbidden {
		t.Errorf("/admin/initial outside App Engine: %d", w.Code)
	}
	u, _ := svc.Resolve(context.Background(), cred)
	u.Role = "admin"
	svc.Save(context.Background(), u)
	if w := do(root, "GET", "/admin", nil, cred); w.Code != 200 || !strings.Contains(w.Body.String(), "Run a job now") {
		t.Errorf("admin /admin: %d", w.Code)
	}
	w := do(root, "POST", "/admin/action", url.Values{"do": {"run"}, "job": {"purge"}}, cred)
	if ran != "purge" || !strings.Contains(w.Header().Get("Location"), "purge") {
		t.Errorf("job not run: %q %q", ran, w.Header().Get("Location"))
	}
}

// The admin page renders with numbers and accounts in it (its data only
// exists in production, so check the template here).
func TestAdminPageRenders(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	u := &account.User{ID: "0123456789abcdef", Username: "joop", Role: "admin", KeepDays: -1, ATHandle: "joop.example", Cell: "8ccgmw", PatronUntil: time.Date(2099, 1, 2, 0, 0, 0, 0, time.UTC)}
	p := adminPage{page: home.newPage(httptest.NewRequest("GET", "/admin", nil), "Admin"), Roles: adminRoles,
		Stats: []stat{{"Accounts", 3, "all"}, {"Push", -1, ""}}, Users: []adminUser{{User: u, Passkeys: 2, Synced: true}}, Groups: []userGroup{{Cell: "8ccgmw", Users: []adminUser{{User: u, Passkeys: 2, Synced: true}}}},
		Areas: []areaCount{{"8ccgmw", 4}}, Jobs: []string{"feeds"}, Full: true,
		Queue: []report.Item{{Kind: "post", Item: "at://did:plc:x/app.bsky.feed.post/abc", Snippet: "buy now", Reasons: map[string]int{"spam": 2}, Count: 2}},
		Businesses: []*business.Business{
			{ID: "b1", Name: "Café Futuro", Kind: "cafe", Status: "active", PaidUntil: time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC).Add(24 * time.Hour), AfterPaid: "stop"},
			{ID: "b2", Name: "Café Passado", Kind: "cafe", Status: "active", PaidUntil: time.Now().AddDate(0, 0, -3), AfterPaid: "stay"}},
		OLN: &olnStats{Total: 3, Live: 2, Expired: 1, Hidden: 1, Messages: []adminNote{
			{ID: "n1", Text: "Fresh croissants", Cell: "8ccgmw", Biz: "Café Futuro", By: "joop", Bits: 5, At: time.Now(), ExpiresAt: time.Now().Add(time.Hour)},
			{ID: "n2", Text: "Cheap watches", Cell: "8ccgmw", Bits: 4, At: time.Now(), ExpiresAt: time.Now().Add(time.Hour), Hidden: true}}}}
	w := httptest.NewRecorder()
	home.render(w, "admin.html", p)
	body := w.Body.String()
	for _, want := range []string{"@joop", "🔑 2", "🔄 synced", "#geo8ccgmw · 1", "Accounts by area", `name="ids"`, "Apply to selected", "buy now", "spam 2", "https://bsky.app/profile/did:plc:x/post/abc", "kept forever", `value="moderator"`, "Set role", ">3<", "Café Futuro", "· paid until ", `name="paid_until" value="2099-12-31"`, "paid until 31 Dec 2099", "paid period over (still live): contact?", `value="stay" selected`, "1 expired, 1 hidden", "Live public messages (2", "🪽 Café Futuro (by @joop)", "Fresh croissants", `value="oln-delete"`, `<span class="badge warn">hidden</span> Cheap watches`, "patron until 1 Jan 2099", `name="until" value="2099-01-01"`} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page lacks %q", want)
		}
	}
}

// Footer: the maker shows only while their link is live; admin edits apply.
func TestFooterMaker(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	home.Cfg.Maker, home.Cfg.Contact = "lapingvino", "https://example.org/issues"
	live := false
	home.MakerLive = func(context.Context, string) bool { return live }
	if m, c, _ := home.footer(context.Background()); m != "" || c != "https://example.org/issues" {
		t.Fatalf("not live: maker %q contact %q", m, c)
	}
	live = true
	home.SaveFooter(context.Background(), footerSettings{Maker: "lapingvino", ContactURL: "mailto:j@example.org", ContactText: "Write me"})
	if m, c, txt := home.footer(context.Background()); m != "lapingvino" || c != "mailto:j@example.org" || txt != "Write me" {
		t.Fatalf("live: %q %q %q", m, c, txt)
	}
}
