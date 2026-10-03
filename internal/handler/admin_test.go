package handler

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
