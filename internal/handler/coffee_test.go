package handler

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
	paid    []map[string]any
	links   [][2]string
	sent    []map[string]any
	left    []map[string]any
	lastOne bool
	renamed []map[string]any
}

func (f *fakeLP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/lp/g/") { // public: a group's identity document
		w.Write([]byte(`{"node":{"name":"Barreiro Coffee!"},"currency":{"name":"Bica","symbol":"B$"}}`))
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		http.Error(w, `{"error":"unauthorized"}`, 401)
		return
	}
	var raw map[string]any
	json.NewDecoder(r.Body).Decode(&raw)
	in := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			in[k] = s
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/lp/admin/group":
		f.groups[in["gid"]] = nil
		f.created = append(f.created, in["gid"])
	case "/lp/admin/rename":
		f.renamed = append(f.renamed, raw)
	case "/lp/admin/leave", "/lp/admin/move":
		raw["path"] = r.URL.Path
		f.left = append(f.left, raw)
	case "/lp/admin/link":
		f.links = append(f.links, [2]string{in["a"], in["b"]})
	case "/lp/admin/links":
		var out []map[string]string
		for _, l := range f.links {
			for i := 0; i < 2; i++ {
				if l[i] == r.FormValue("gid") {
					out = append(out, map[string]string{"gid": l[1-i], "name": "Other"})
				}
			}
		}
		json.NewEncoder(w).Encode(out)
		return
	case "/lp/admin/quote":
		w.Write([]byte(`{"amount":2000000,"dst":1960784}`))
		return
	case "/lp/admin/transfer":
		f.sent = append(f.sent, raw)
		w.Write([]byte(`{"id":"t1"}`))
		return
	case "/lp/admin/pay":
		if raw["amount"].(float64) > 10_000_000 {
			http.Error(w, `{"error":"insufficient balance"}`, 400)
			return
		}
		f.paid = append(f.paid, raw)
	case "/lp/admin/balance":
		if f.lastOne {
			w.Write([]byte(`{"member":"x","active":true,"balance":12345678,"current_ud":1000000,"members":1}`))
		} else {
			w.Write([]byte(`{"member":"x","active":true,"balance":12345678,"current_ud":1000000,"members":2}`))
		}
		return
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
	tmpl := template.Must(template.New("").Funcs(Funcs).ParseGlob("../../templates/*.html"))
	c := &Coffee{Home: &Home{Cfg: &config.Config{Brand: "Kafumu", Origin: "https://kafumu.test"}, Tmpl: tmpl}, Accounts: svc,
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
	// The pages: bea's wallet (balance, dividend, days); the join page for a
	// valid invite names the group, a forged one says it isn't valid.
	if w := as(bea, "GET", "/coffee", nil, c.Show); w.Code != 200 || !strings.Contains(w.Body.String(), "12.34 B$") || !strings.Contains(w.Body.String(), "Bica") || !strings.Contains(w.Body.String(), "Barreiro Coffee!") || !strings.Contains(w.Body.String(), "≈ 12.3 days") {
		t.Fatalf("wallet page: %d %s", w.Code, w.Body.String())
	}
	if w := as(anon, "GET", "/coffee/join?"+q.Encode(), nil, c.ShowJoin); !strings.Contains(w.Body.String(), "Join Barreiro Coffee!") || !strings.Contains(w.Body.String(), "/account?next=") {
		t.Fatalf("join page: %s", w.Body.String())
	}
	if w := as(anon, "GET", "/coffee/join?"+bad.Encode(), nil, c.ShowJoin); !strings.Contains(w.Body.String(), "This invite link isn") {
		t.Fatalf("forged join page: %s", w.Body.String())
	}
	// Paying @name in the group: the ledger gets two member ids, never names.
	pay := func(u *account.User, to, amount string) string {
		w := as(u, "POST", "/coffee/pay", url.Values{"g": {gid}, "to": {to}, "amount": {amount}}, c.Pay)
		loc, _ := url.Parse(w.Header().Get("Location"))
		return loc.Query().Get("m")
	}
	if m := pay(ana, "@Bea", "2,50"); m != "paid" || len(lp.paid) != 1 || lp.paid[0]["amount"].(float64) != 2_500_000 || lp.paid[0]["to"] == "bea" {
		t.Fatalf("pay @bea: %s %v", m, lp.paid)
	}
	for _, c := range []struct{ to, amount, want string }{{"nobody", "1", "not_member"}, {"ana", "1", "self"}, {"bea", "two", "bad_amount"}, {"bea", "50", "funds"}} {
		if m := pay(ana, c.to, c.amount); m != c.want {
			t.Errorf("pay %s %s: %s, want %s", c.to, c.amount, m, c.want)
		}
	}
	if w := as(anon, "POST", "/coffee/pay", url.Values{"g": {gid}, "to": {"bea"}, "amount": {"1"}}, c.Pay); w.Code != 403 {
		t.Fatal("a non-member paid from the group")
	}
	// Across groups: ana (in both) links them; bea pays @dan in the other
	// group: the confirm page shows the quote, then the transfer goes out.
	w = as(ana, "POST", "/coffee/group", url.Values{"name": {"Arnhem"}}, c.StartGroup)
	gid2 := lp.created[1]
	dan := user("dan", "")
	w = as(ana, "GET", "/coffee/invite?g="+gid2, nil, c.Invite)
	json.NewDecoder(w.Body).Decode(&inv)
	u2, _ := url.Parse(inv.Link)
	as(dan, "POST", "/coffee/join", u2.Query(), c.Join)
	if w := as(bea, "POST", "/coffee/link", url.Values{"a": {gid}, "b": {gid2}}, c.Link); w.Code != 403 {
		t.Fatal("bea linked a group she isn't in")
	}
	if w := as(ana, "POST", "/coffee/link", url.Values{"a": {gid}, "b": {gid2}}, c.Link); w.Code != 303 || len(lp.links) != 1 {
		t.Fatalf("ana's link: %d %v", w.Code, lp.links)
	}
	w = as(bea, "POST", "/coffee/pay", url.Values{"g": {gid}, "to": {"dan"}, "amount": {"2"}}, c.Pay)
	loc, _ := url.Parse(w.Header().Get("Location"))
	if loc.Path != "/coffee/confirm" || loc.Query().Get("tg") != gid2 {
		t.Fatalf("paying across should confirm first: %s", w.Header().Get("Location"))
	}
	w = as(bea, "GET", loc.String(), nil, c.Confirm)
	if !strings.Contains(w.Body.String(), "2.00 B$") || !strings.Contains(w.Body.String(), "1.96 B$") || !strings.Contains(w.Body.String(), "@dan receives") {
		t.Fatalf("confirm page: %s", w.Body.String())
	}
	w = as(bea, "POST", "/coffee/across", loc.Query(), c.Across)
	if m, _ := url.Parse(w.Header().Get("Location")); m.Query().Get("m") != "sent" || len(lp.sent) != 1 || lp.sent[0]["to_gid"] != gid2 {
		t.Fatalf("across: %s %v", w.Header().Get("Location"), lp.sent)
	}
	if w := as(dan, "POST", "/coffee/across", loc.Query(), c.Across); w.Code != 400 {
		t.Fatal("someone outside the payer's group sent the payment")
	}
	// Renaming: any member; strangers can't.
	if w := as(dan, "POST", "/coffee/rename", url.Values{"g": {gid}, "symbol": {"X$"}}, c.Rename); w.Code != 403 {
		t.Fatal("a non-member renamed the group")
	}
	if w := as(bea, "POST", "/coffee/rename", url.Values{"g": {gid}, "symbol": {"X$"}}, c.Rename); w.Code != 303 || len(lp.renamed) != 1 || lp.renamed[0]["symbol"] != "X$" {
		t.Fatalf("rename: %d %v", w.Code, lp.renamed)
	}
	// Leaving: the page; the box must be ticked; give needs a member; share
	// and move call the service and take the group off the account.
	if w := as(bea, "GET", "/coffee/leave?g="+gid, nil, c.ShowLeave); !strings.Contains(w.Body.String(), "Leave Barreiro Coffee!") {
		t.Fatalf("leave page: %s", w.Body.String())
	}
	leave := func(u *account.User, form url.Values) string {
		w := as(u, "POST", "/coffee/leave", form, c.Leave)
		loc, _ := url.Parse(w.Header().Get("Location"))
		return loc.Path + "?" + loc.Query().Get("m")
	}
	if m := leave(bea, url.Values{"g": {gid}, "mode": {"share"}}); m != "/coffee/leave?sure" {
		t.Fatalf("no tick: %s", m)
	}
	lp.lastOne = true // the fake now says one active member: give and share are refused
	if m := leave(bea, url.Values{"g": {gid}, "mode": {"share"}, "sure": {"1"}}); m != "/coffee/leave?last" {
		t.Fatalf("the last member shared out: %s", m)
	}
	if w := as(bea, "GET", "/coffee/leave?g="+gid, nil, c.ShowLeave); !strings.Contains(w.Body.String(), "You're the last member") || strings.Contains(w.Body.String(), `value="share"`) {
		t.Fatal("the last member's leave page offers sharing out")
	}
	lp.lastOne = false
	if m := leave(bea, url.Values{"g": {gid}, "mode": {"give"}, "to": {"dan"}, "sure": {"1"}}); m != "/coffee/leave?give_bad" {
		t.Fatalf("give to a non-member: %s", m)
	}
	if m := leave(bea, url.Values{"g": {gid}, "mode": {"share"}, "sure": {"1"}}); m != "/coffee?left" || slices.Contains(bea.Groups, gid) || lp.left[len(lp.left)-1]["mode"] != "redistribute" {
		t.Fatalf("share: %s %v %v", m, bea.Groups, lp.left)
	}
	if m := leave(ana, url.Values{"g": {gid2}, "mode": {"move"}, "tg": {gid}, "sure": {"1"}}); m != "/coffee?left" || slices.Contains(ana.Groups, gid2) {
		t.Fatalf("move: %s %v", m, ana.Groups)
	}
	if mv := lp.left[len(lp.left)-1]; mv["path"] != "/lp/admin/move" || mv["to_gid"] != gid || mv["member"] != mv["to"] {
		t.Fatalf("move call: %v", mv)
	}
	// Deleting an account destroys its balances in every group (96h4).
	before := len(lp.left)
	acc := &Accounts{Home: c.Home, Svc: svc, Coffee: c.C}
	as(dan, "POST", "/account/delete", url.Values{"confirm": {"yes"}}, acc.Delete)
	if len(lp.left) != before+len(dan.Groups) || lp.left[len(lp.left)-1]["mode"] != "destroy" {
		t.Fatalf("delete: %v", lp.left[before:])
	}
	// Linking from a payment (96n): eli is in a group not linked to ana's;
	// paying @eli offers a link request; eli accepts: link + payment.
	c.Asks = &coffee.MemoryAsks{}
	eli := user("eli", "host")
	w = as(eli, "POST", "/coffee/group", url.Values{"name": {"Setúbal"}}, c.StartGroup)
	gid3 := lp.created[len(lp.created)-1]
	w = as(ana, "POST", "/coffee/pay", url.Values{"g": {gid}, "to": {"eli"}, "amount": {"1"}}, c.Pay)
	lpURL, _ := url.Parse(w.Header().Get("Location"))
	if lpURL.Path != "/coffee/linkpay" || lpURL.Query().Get("tg") != gid3 {
		t.Fatalf("paying across unlinked groups: %s", w.Header().Get("Location"))
	}
	w = as(ana, "POST", "/coffee/linkask", lpURL.Query(), c.LinkAsk)
	eli, _ = svc.ByID(ctx, eli.ID) // the handler updated the stored account
	if len(eli.CoffeeAsks) != 1 {
		t.Fatalf("eli got no request: %d %v", w.Code, eli.CoffeeAsks)
	}
	askID := eli.CoffeeAsks[0]
	if w := as(ana, "POST", "/coffee/linkaccept", url.Values{"id": {askID}}, c.LinkAccept); !strings.Contains(w.Header().Get("Location"), "ask_gone") {
		t.Fatal("the payer accepted their own request")
	}
	links0, sent0 := len(lp.links), len(lp.sent)
	w = as(eli, "POST", "/coffee/linkaccept", url.Values{"id": {askID}}, c.LinkAccept)
	if !strings.Contains(w.Header().Get("Location"), "m=accepted") || len(lp.links) != links0+1 || len(lp.sent) != sent0+1 || lp.sent[len(lp.sent)-1]["to_gid"] != gid3 {
		t.Fatalf("accept: %s links %d sent %d", w.Header().Get("Location"), len(lp.links)-links0, len(lp.sent)-sent0)
	}
	if len(eli.CoffeeAsks) != 0 {
		t.Fatal("the request stayed")
	}
	if w := as(eli, "POST", "/coffee/linkaccept", url.Values{"id": {askID}}, c.LinkAccept); !strings.Contains(w.Header().Get("Location"), "ask_gone") {
		t.Fatal("a request was accepted twice")
	}
	// Member ids: stable per account, different between accounts.
	a1, _ := c.C.MemberID(ctx, ana.ID)
	a2, _ := c.C.MemberID(ctx, ana.ID)
	b1, _ := c.C.MemberID(ctx, bea.ID)
	if a1 != a2 || a1 == b1 || len(a1) != 17 {
		t.Fatalf("member ids %s %s %s", a1, a2, b1)
	}
}
