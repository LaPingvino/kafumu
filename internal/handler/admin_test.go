package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
	u := &account.User{ID: "0123456789abcdef", Username: "joop", Role: "admin", KeepDays: -1, ATHandle: "joop.example", Cell: "8ccgmw"}
	p := adminPage{page: home.newPage(httptest.NewRequest("GET", "/admin", nil), "Admin"), Roles: adminRoles,
		Stats: []stat{{"Accounts", 3, "all"}, {"Push", -1, ""}}, Users: []adminUser{{User: u, Passkeys: 2, Synced: true}}, Groups: []userGroup{{Cell: "8ccgmw", Users: []adminUser{{User: u, Passkeys: 2, Synced: true}}}},
		Areas: []areaCount{{"8ccgmw", 4}}, Jobs: []string{"feeds"}, Full: true,
		Queue: []report.Item{{Kind: "post", Item: "at://did:plc:x/app.bsky.feed.post/abc", Snippet: "buy now", Reasons: map[string]int{"spam": 2}, Count: 2}}}
	w := httptest.NewRecorder()
	home.render(w, "admin.html", p)
	body := w.Body.String()
	for _, want := range []string{"@joop", "🔑 2", "🔄 synced", "#geo8ccgmw · 1", "Accounts by area", `name="ids"`, "Apply to selected", "buy now", "spam 2", "https://bsky.app/profile/did:plc:x/post/abc", "kept forever", `value="moderator"`, "Set role", ">3<"} {
		if !strings.Contains(body, want) {
			t.Errorf("admin page lacks %q", want)
		}
	}
}
