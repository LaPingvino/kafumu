package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/coffee"
	"github.com/LaPingvino/kafumu/internal/config"
)

// fakeLP stands in for the lp service's admin API: it records groups and
// members and checks the token.
type fakeLP struct {
	mu      sync.Mutex
	groups  map[string][]string
	created []string
}

func (f *fakeLP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer tok" {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var in map[string]string
	json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/lp/admin/group":
		f.groups[in["gid"]] = nil
		f.created = append(f.created, in["gid"])
	case "/lp/admin/member":
		for _, m := range f.groups[in["gid"]] {
			if m == in["member"] {
				http.Error(w, `{"error":"member exists"}`, 400)
				return
			}
		}
		f.groups[in["gid"]] = append(f.groups[in["gid"]], in["member"])
	}
	w.Write([]byte("{}"))
}

func TestCoffeeStartInviteJoin(t *testing.T) {
	lp := &fakeLP{groups: map[string][]string{}}
	srv := httptest.NewServer(lp)
	defer srv.Close()
	svc := account.NewService(account.NewMemoryStore())
	c := &Coffee{Home: &Home{Cfg: &config.Config{Origin: "https://kafumu.test"}}, Accounts: svc,
		C: &coffee.Client{Base: srv.URL, Token: "tok", Key: []byte("0123456789abcdef0123456789abcdef")}}
	ctx := context.Background()
	user := func(name, role string) *account.User {
		u, _, err := svc.Create(ctx, "en")
		if err != nil {
			t.Fatal(err)
		}
		if name != "" {
			if err := svc.SetUsername(ctx, u, name); err != nil {
				t.Fatal(err)
			}
		}
		svc.Update(ctx, u, func(x *account.User) { x.Role = role })
		return u
	}
	as := func(u *account.User, method, path string, form url.Values, h http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if u != nil {
			r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
		}
		w := httptest.NewRecorder()
		h(w, r)
		return w
	}
	ana, bea, anon := user("ana", "host"), user("bea", ""), user("", "")
	// Starting a group: a named account with a passkey or a role.
	if w := as(bea, "POST", "/coffee/group", url.Values{"name": {"Bea's"}}, c.StartGroup); w.Code != 403 {
		t.Fatalf("bea (no passkey, no role) started a group: %d", w.Code)
	}
	w := as(ana, "POST", "/coffee/group", url.Values{"name": {"Barreiro Coffee!"}}, c.StartGroup)
	if w.Code != 303 || len(lp.created) != 1 || !strings.HasPrefix(lp.created[0], "barreiro-coffee-") {
		t.Fatalf("start: %d %v", w.Code, lp.created)
	}
	gid := lp.created[0]
	if len(ana.Groups) != 1 || ana.Groups[0] != gid || len(lp.groups[gid]) != 1 || !strings.HasPrefix(lp.groups[gid][0], "u") || strings.Contains(lp.groups[gid][0], "ana") {
		t.Fatalf("ana's membership: %v / %v", ana.Groups, lp.groups[gid])
	}
	// An invite: only for members; it carries who vouched.
	if w := as(bea, "GET", "/coffee/invite?g="+gid, nil, c.Invite); w.Code != 403 {
		t.Fatal("a non-member got an invite link")
	}
	w = as(ana, "GET", "/coffee/invite?g="+gid, nil, c.Invite)
	var inv struct{ Link string }
	json.NewDecoder(w.Body).Decode(&inv)
	u, _ := url.Parse(inv.Link)
	q := u.Query()
	if u.Path != "/coffee/join" || q.Get("g") != gid || q.Get("by") != lp.groups[gid][0] {
		t.Fatalf("invite link %q", inv.Link)
	}
	// Joining: a forged token fails; signed out goes to sign in first; the real one works.
	bad := url.Values{"g": {gid}, "by": {q.Get("by")}, "t": {"0000000000"}}
	if w := as(bea, "POST", "/coffee/join", bad, c.Join); w.Code != 403 {
		t.Fatal("a forged invite worked")
	}
	if w := as(anon, "POST", "/coffee/join", q, c.Join); w.Code != 303 || !strings.HasPrefix(w.Header().Get("Location"), "/account?next=") {
		t.Fatalf("nameless: %d %s", w.Code, w.Header().Get("Location"))
	}
	if w := as(ana, "POST", "/coffee/join", q, c.Join); w.Code != 303 || len(lp.groups[gid]) != 1 {
		t.Fatal("ana joined her own group twice")
	}
	if w := as(bea, "POST", "/coffee/join", q, c.Join); w.Code != 303 || len(bea.Groups) != 1 || len(lp.groups[gid]) != 2 {
		t.Fatalf("bea's join: %d %v %v", w.Code, bea.Groups, lp.groups[gid])
	}
	// Member ids: stable per account, different between accounts.
	a1, _ := c.C.MemberID(ctx, ana.ID)
	a2, _ := c.C.MemberID(ctx, ana.ID)
	b1, _ := c.C.MemberID(ctx, bea.ID)
	if a1 != a2 || a1 == b1 || len(a1) != 17 {
		t.Fatalf("member ids %s %s %s", a1, a2, b1)
	}
}
