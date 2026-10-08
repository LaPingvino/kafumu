package handler

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/brand"
	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/report"
	"github.com/LaPingvino/kafumu/internal/sqlstore"
	"html/template"
	"log"
	"net/http"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	gaeuser "google.golang.org/appengine/v2/user"

	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// Admin is Joop's control room. Becoming admin needs both App Engine's own
// admin login (app.yaml `login: admin` on /admin/initial, and IsAdmin
// checked again here) and a Kafumu account — typically with a passkey.
type Admin struct {
	Home     *Home
	Accounts *Accounts
	Meetups  *Meetups
	Notes    *oln.Service
	DB       *datastore.Client
	// SQL: the self-hosted store (KAFUMU_SQLITE), when there's no Datastore.
	SQL        *sql.DB
	Reports    *report.Service
	Businesses *business.Store
	Brands     *brand.Store
	// peersMem: linked nodes without Datastore (kept via kv when self-hosted).
	peersMu  sync.Mutex
	peersMem *peerConfig
	// Jobs are the cron jobs, runnable by hand: name → run.
	Jobs map[string]func(ctx context.Context) string
}

// mayBootstrap: who may make themselves admin. On App Engine, the
// project's admins; self-hosted, whoever brings the token the operator
// set in KAFUMU_ADMIN_TOKEN (at least 16 characters; never on App Engine).
func (a *Admin) mayBootstrap(r *http.Request) bool {
	if cache.OnAppEngine() {
		return gaeuser.IsAdmin(r.Context())
	}
	want, got := os.Getenv("KAFUMU_ADMIN_TOKEN"), r.URL.Query().Get("token")
	return len(want) >= 16 && subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
}

// Initial handles GET /admin/initial (?token=… when self-hosted).
func (a *Admin) Initial(w http.ResponseWriter, r *http.Request) {
	if !a.mayBootstrap(r) {
		http.Error(w, "only for the App Engine project's admins (or, self-hosted, with KAFUMU_ADMIN_TOKEN)", http.StatusForbidden)
		return
	}
	u := UserFrom(r.Context())
	if u == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><meta charset="utf-8"><title>Admin setup</title>
<h1>Admin setup</h1><p>You're an App Engine admin. Now sign in to Kafumu on this browser —
with your passkey or sign-in link at <a href="/account">/account</a> — and come back to
this same link again.</p>`)
		return
	}
	if u.Role != "admin" {
		if err := a.Accounts.Svc.Update(r.Context(), u, func(x *account.User) { x.Role = "admin" }); err != nil {
			http.Error(w, "could not save", http.StatusInternalServerError)
			return
		}
		log.Printf("admin: %s promoted", u.ID)
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (a *Admin) admin(w http.ResponseWriter, r *http.Request) bool {
	if u := UserFrom(r.Context()); u != nil && (u.Role == "admin" || u.Role == "moderator") {
		return true
	}
	http.NotFound(w, r)
	return false
}

type adminPage struct {
	page
	Stats  []stat
	Result string
	Jobs   []string
	Search string
	Users  []adminUser
	Groups []userGroup
	Areas  []areaCount
	Roles  []string
	Queue  []report.Item
	// Where App Engine thinks this request comes from (the admin's own).
	ReqGeo string
	// Full is an admin (accounts, jobs); a moderator sees only the queue.
	Full bool
	// Footer: the "Contact the maker" settings as stored (for editing).
	Footer footerSettings
	// OLN: what the local-message store holds (inspection).
	OLN *olnStats
	// Businesses: business accounts, trials that ended first ("contact?").
	Businesses []*business.Business
	// Peers: linked OLN nodes this one pulls from.
	Peers []peerEntry
	// Brands: Kafumu's faces on other hosts; BrandAdmins: host → "@a @b".
	Brands      []brand.Brand
	BrandAdmins map[string]string
}

type stat struct {
	Label string
	N     int
	Note  string
}

// userGroup: named accounts in one area (#geo cell; "" = none set).
type userGroup struct {
	Cell  string
	Users []adminUser
}

type areaCount struct {
	Cell string
	N    int
}

type adminUser struct {
	*account.User
	Passkeys int
	Synced   bool
}

// Roles an admin can give: trusted hosts' meetups rank as such, moderators
// work the report queue, admins everything.
var adminRoles = []string{"", "host", "moderator", "admin"}

func (a *Admin) count(ctx context.Context, q *datastore.Query) int {
	res, err := a.DB.RunAggregationQuery(ctx, q.NewAggregationQuery().WithCount("n"))
	if err == nil {
		switch v := res["n"].(type) {
		case int64:
			return int(v)
		case int:
			return v
		}
		log.Printf("admin: count: unexpected %T", res["n"])
	} else {
		log.Printf("admin: count: %v", err)
	}
	// Fallback: count keys (small reads), capped.
	keys, err := a.DB.GetAll(ctx, q.KeysOnly().Limit(10000), nil)
	if err != nil {
		log.Printf("admin: count keys: %v", err)
		return -1
	}
	return len(keys)
}

// Show handles GET /admin: numbers first, then accounts, jobs, moderation.
func (a *Admin) Show(w http.ResponseWriter, r *http.Request) {
	if !a.admin(w, r) {
		return
	}
	ctx := r.Context()
	p := adminPage{page: a.Home.newPage(r, "Admin"), Result: r.URL.Query().Get("r"), Search: strings.TrimSpace(r.URL.Query().Get("s"))}
	p.Tab, p.Roles = "admin", adminRoles
	p.Full = UserFrom(ctx).Role == "admin"
	p.ReqGeo = strings.Join([]string{r.Header.Get("X-Appengine-Country"), r.Header.Get("X-Appengine-Region"),
		r.Header.Get("X-Appengine-City"), r.Header.Get("X-Appengine-Citylatlong")}, " / ")
	if a.Reports != nil {
		if q, err := a.Reports.Queue(ctx, time.Now()); err == nil {
			p.Queue = q
		}
	}
	if !p.Full {
		a.Home.render(w, "admin.html", p)
		return
	}
	for name := range a.Jobs {
		p.Jobs = append(p.Jobs, name)
	}
	sort.Strings(p.Jobs)
	now := time.Now()
	p.Now = now
	if a.DB != nil {
		u := func() *datastore.Query { return datastore.NewQuery("User") }
		live := func(kind string) *datastore.Query {
			return datastore.NewQuery(kind).FilterField("expires_at", ">", now)
		}
		p.Stats = []stat{
			{"Accounts", a.count(ctx, u()), "all, named or not"},
			{"Named", a.count(ctx, u().FilterField("username", ">", "")), "kept a year"},
			{"Active 24 h", a.count(ctx, u().FilterField("last_seen_at", ">", now.Add(-24*time.Hour))), ""},
			{"Active 7 d", a.count(ctx, u().FilterField("last_seen_at", ">", now.Add(-7*24*time.Hour))), ""},
			{"Active 30 d", a.count(ctx, u().FilterField("last_seen_at", ">", now.Add(-30*24*time.Hour))), ""},
			{"Findable now", a.count(ctx, u().FilterField("visible_until", ">", now)), "public profiles"},
			{"Synced", a.count(ctx, datastore.NewQuery("Vault")), "accounts with a device vault"},
			{"Meetups", a.count(ctx, live("Meetup")), "upcoming or running"},
			{"…from feeds", a.count(ctx, live("Meetup").FilterField("author_id", "=", "feed")), "Luma, iCal, Smoke Signal"},
			{"Local messages", a.count(ctx, live("Note")), "OLN, unexpired"},
			{"Mailboxes", a.count(ctx, live("Box")), "pairing, inboxes, moves"},
			{"Push", a.count(ctx, live("PushSub")), "devices with notifications"},
			{"Bluesky linked", a.count(ctx, live("ATSession")), "OAuth sessions"},
		}
		p.Users = a.findUsers(ctx, p.Search)
		p.OLN = a.olnStats(ctx, now)
		p.Areas = a.areas(ctx)
	} else if a.SQL != nil {
		// Self-hosted (SQLite): the same numbers and account search.
		for _, st := range sqlstore.Stats(ctx, a.SQL, now) {
			p.Stats = append(p.Stats, stat{st.Name, st.N, st.Note})
		}
		if u, err := a.Accounts.Svc.ByID(ctx, strings.ToLower(p.Search)); p.Search != "" && err == nil && u != nil {
			p.Users = a.adminUsers(ctx, []*account.User{u})
		} else {
			var us []*account.User
			for _, id := range sqlstore.FindUsers(ctx, a.SQL, p.Search) {
				if u, err := a.Accounts.Svc.ByID(ctx, id); err == nil && u != nil {
					us = append(us, u)
				}
			}
			p.Users = a.adminUsers(ctx, us)
		}
	}
	p.Peers = a.loadPeers(ctx).Peers
	if a.Brands != nil {
		p.Brands = a.Brands.All(ctx)
		sort.Slice(p.Brands, func(i, j int) bool { return p.Brands[i].Host < p.Brands[j].Host })
		p.BrandAdmins = map[string]string{}
		for _, b := range p.Brands {
			var names []string
			for _, id := range b.Admins {
				if n := a.Home.ownerName(ctx, a.Accounts.Svc, id); n != "" {
					names = append(names, n)
				}
			}
			p.BrandAdmins[b.Host] = strings.Join(names, " ")
		}
	}
	a.Home.footer(ctx)
	a.Home.foot.mu.Lock()
	if a.Home.foot.settings != nil {
		p.Footer = *a.Home.foot.settings
	}
	a.Home.foot.mu.Unlock()
	if a.Businesses != nil {
		if bs, err := a.Businesses.All(ctx); err == nil {
			sort.SliceStable(bs, func(i, j int) bool { return bs[i].TrialOver(now) && !bs[j].TrialOver(now) })
			p.Businesses = bs
		}
	}
	by := map[string]int{}
	for _, u := range p.Users {
		if _, ok := by[u.Cell]; !ok {
			by[u.Cell] = len(p.Groups)
			p.Groups = append(p.Groups, userGroup{Cell: u.Cell})
		}
		g := &p.Groups[by[u.Cell]]
		g.Users = append(g.Users, u)
	}
	sort.SliceStable(p.Groups, func(i, j int) bool { return len(p.Groups[i].Users) < len(p.Groups[j].Users) })
	a.Home.render(w, "admin.html", p)
}

// adminUsers adds passkey and sync facts to accounts for the admin list.
func (a *Admin) adminUsers(ctx context.Context, us []*account.User) []adminUser {
	var out []adminUser
	for _, u := range us {
		var pk []json.RawMessage
		_ = json.Unmarshal(u.Passkeys, &pk)
		au := adminUser{User: u, Passkeys: len(pk)}
		if a.Accounts.Vault != nil {
			if v, _ := a.Accounts.Vault.Get(ctx, u.ID); v != nil {
				au.Synced = true
			}
		}
		out = append(out, au)
	}
	return out
}

// findUsers: a name prefix or an account id; with no search, the first
// named accounts alphabetically.
func (a *Admin) findUsers(ctx context.Context, s string) []adminUser {
	var out []adminUser
	add := func(u *account.User) {
		var pk []json.RawMessage
		_ = json.Unmarshal(u.Passkeys, &pk)
		au := adminUser{User: u, Passkeys: len(pk)}
		if a.Accounts.Vault != nil {
			if v, _ := a.Accounts.Vault.Get(ctx, u.ID); v != nil {
				au.Synced = true
			}
		}
		out = append(out, au)
	}
	s = strings.ToLower(s)
	if s != "" {
		if u, err := a.Accounts.Svc.ByID(ctx, s); err == nil && u != nil {
			add(u)
			return out
		}
	}
	q := datastore.NewQuery("User").FilterField("username", ">", "").Order("username").Limit(50)
	if s != "" {
		q = datastore.NewQuery("User").FilterField("username", ">=", s).FilterField("username", "<", s+"\uffff").Order("username").Limit(300)
	}
	keys, err := a.DB.GetAll(ctx, q.KeysOnly(), nil)
	if err != nil {
		log.Printf("admin: users: %v", err)
		return out
	}
	for _, k := range keys {
		if u, err := a.Accounts.Svc.ByID(ctx, k.Name); err == nil && u != nil {
			add(u)
		}
	}
	return out
}

// Action handles POST /admin/action: hide a note, delete a meetup, run a job.
func (a *Admin) Action(w http.ResponseWriter, r *http.Request) {
	if !a.admin(w, r) {
		return
	}
	ctx, id, res := r.Context(), strings.TrimSpace(r.FormValue("id")), ""
	do := r.FormValue("do")
	if UserFrom(ctx).Role != "admin" && do != "hide-note" && do != "delete-meetup" && do != "report-hide" && do != "report-dismiss" {
		http.NotFound(w, r)
		return
	}
	switch do {
	case "report-dismiss":
		if err := a.Reports.Dismiss(ctx, r.FormValue("kind"), id); err != nil {
			res = "dismiss failed: " + err.Error()
		} else {
			res = "dismissed reports on " + id
		}
	case "report-hide":
		// Notes and meetups have their own removal; posts and people are
		// kept out of bundles.
		var err error
		switch r.FormValue("kind") {
		case "note":
			if err = a.Notes.Hide(ctx, id); err == nil {
				err = a.Reports.Dismiss(ctx, "note", id)
			}
		case "meetup":
			if err = a.Meetups.Svc.Store.Delete(ctx, id); err == nil {
				a.Meetups.Svc.ForgetAll()
				err = a.Reports.Dismiss(ctx, "meetup", id)
			}
		default:
			err = a.Reports.Hide(ctx, r.FormValue("kind"), id)
		}
		if err != nil {
			res = "hide failed: " + err.Error()
		} else {
			res = "hid " + r.FormValue("kind") + " " + id
		}
	case "hide-note":
		if err := a.Notes.Hide(ctx, id); err != nil {
			res = "hide failed: " + err.Error()
		} else {
			res = "hidden " + id
		}
	case "delete-meetup":
		if err := a.Meetups.Svc.Store.Delete(ctx, id); err != nil {
			res = "delete failed: " + err.Error()
		} else {
			a.Meetups.Svc.ForgetAll()
			res = "deleted meetup " + id
		}
	case "oln-purge":
		if a.DB == nil || a.Notes == nil {
			res = "no Datastore"
			break
		}
		e, h, err := a.olnPurge(ctx, time.Now())
		res = fmt.Sprintf("oln-purge: deleted %d expired and %d hidden messages", e, h)
		if err != nil {
			res += " (" + err.Error() + ")"
		}
	case "oln-hide":
		if a.Notes == nil {
			res = "no local messages here"
		} else if err := a.Notes.Hide(ctx, id); err != nil {
			res = "hide failed: " + err.Error()
		} else {
			res = "hidden message " + id
		}
	case "oln-delete":
		if a.DB == nil {
			res = "no Datastore"
			break
		}
		if err := a.DB.Delete(ctx, datastore.NameKey("Note", id, nil)); err != nil {
			res = "delete failed: " + err.Error()
		} else {
			a.Notes.ForgetAll()
			res = "deleted message " + id
		}
	case "oln-clean-v1", "oln-reset":
		if a.DB == nil {
			res = "no Datastore"
			break
		}
		if do == "oln-reset" && r.FormValue("confirm") != "RESET" {
			res = "type RESET to confirm"
			break
		}
		n, err := a.olnClean(ctx, do == "oln-reset")
		res = fmt.Sprintf("%s: deleted %d local messages", do, n)
		if err != nil {
			res += " (" + err.Error() + ")"
		}
	case "brand-save":
		if a.Brands == nil {
			res = "no brands here"
			break
		}
		b := &brand.Brand{Host: r.FormValue("host"), Name: r.FormValue("name"), Tagline: r.FormValue("tagline"),
			Accent: r.FormValue("accent"), Button: r.FormValue("button"), Tags: strings.FieldsFunc(r.FormValue("tags"), func(c rune) bool { return c == ',' || c == ' ' })}
		if old := a.Brands.For(ctx, b.Host); old != nil {
			b.Admins, b.PaidUntil, b.CreatedAt = old.Admins, old.PaidUntil, old.CreatedAt
		}
		if err := a.Brands.Save(ctx, b); err != nil {
			res = "brand: a host like bahais.in and a name, please"
		} else {
			res = "brand saved: " + b.Host + " (map the domain to this app on App Engine to make it live)"
		}
	case "brand-admins":
		b := a.Brands.For(ctx, id)
		if b == nil {
			res = "no brand " + id
			break
		}
		b.Admins = nil
		var names []string
		for _, n := range strings.FieldsFunc(r.FormValue("admins"), func(c rune) bool { return c == ',' || c == ' ' }) {
			n = strings.ToLower(strings.TrimPrefix(n, "@"))
			// A person's @username, or a business's @name ("biz:<id>": its managers).
			if uid, err := a.Accounts.Svc.Store.LookupUsername(ctx, n); err == nil && uid != "" && !slices.Contains(b.Admins, uid) {
				b.Admins = append(b.Admins, uid)
				names = append(names, "@"+n)
			}
		}
		if err := a.Brands.Save(ctx, b); err != nil {
			res = "save failed: " + err.Error()
		} else {
			res = b.Host + " admins: " + strings.Join(names, " ")
		}
	case "brand-delete":
		if a.Brands != nil && a.Brands.Delete(ctx, id) == nil {
			res = "brand removed: " + id
		}
	case "peer-add":
		o := peerOrigin(r.FormValue("origin"))
		if o == "" {
			res = "a node address is https://host (http only for localhost)"
			break
		}
		c := a.loadPeers(ctx)
		c.Peers = slices.DeleteFunc(c.Peers, func(p peerEntry) bool { return p.Origin == o })
		c.Peers = append(c.Peers, peerEntry{Origin: o, Cells: cleanCells(r.FormValue("cells"))})
		if err := a.savePeers(ctx, c); err != nil {
			res = "save failed: " + err.Error()
		} else {
			res = "linked " + o
		}
	case "peer-remove":
		c := a.loadPeers(ctx)
		c.Peers = slices.DeleteFunc(c.Peers, func(p peerEntry) bool { return p.Origin == id })
		if err := a.savePeers(ctx, c); err != nil {
			res = "save failed: " + err.Error()
		} else {
			res = "unlinked " + id
		}
	case "oln-pull":
		res = a.PullPeers(ctx)
	case "footer":
		s := footerSettings{Maker: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("maker")), "@")),
			ContactURL: strings.TrimSpace(r.FormValue("contact_url")), ContactText: strings.TrimSpace(r.FormValue("contact_text"))}
		if s.ContactURL != "" && !strings.HasPrefix(s.ContactURL, "https://") && !strings.HasPrefix(s.ContactURL, "mailto:") {
			res = "contact link must start with https:// or mailto:"
			break
		}
		if err := a.Home.SaveFooter(ctx, s); err != nil {
			res = "save failed: " + err.Error()
		} else {
			res = "footer saved"
		}
	case "biz-status":
		status, note := r.FormValue("status"), strings.TrimSpace(r.FormValue("note"))
		paid, after := time.Time{}, business.AfterStop
		if d, err := time.Parse("2006-01-02", r.FormValue("paid_until")); err == nil {
			paid = d.Add(24 * time.Hour) // through the end of that day (UTC)
		}
		if r.FormValue("after") == business.AfterStay {
			after = business.AfterStay
		}
		if !slices.Contains([]string{business.StatusTrial, business.StatusActive, business.StatusPaused, business.StatusEnded}, status) {
			res = "unknown status"
		} else if b, err := a.Businesses.Update(ctx, id, func(x *business.Business) error { // only these fields
			x.Status, x.Note, x.PaidUntil, x.AfterPaid = status, note, paid, after
			return nil
		}); errors.Is(err, business.ErrNotFound) {
			res = "no business " + id
		} else if err != nil {
			res = "save failed: " + err.Error()
		} else {
			res = b.Name + ": " + b.Status
			if !b.PaidUntil.IsZero() {
				res += ", paid until " + b.PaidDay().Format("2 Jan 2006") + " (then " + b.AfterPaid + ")"
			}
		}
	case "role", "rename", "unname", "keep", "delete-user", "patron":
		res = a.userAction(r, id)
	case "bulk":
		res = a.bulkAction(r)
	case "run":
		if job, ok := a.Jobs[r.FormValue("job")]; ok {
			res = r.FormValue("job") + ": " + job(ctx)
		}
	}
	log.Printf("admin: %s", res)
	http.Redirect(w, r, "/admin?r="+template.URLQueryEscaper(res), http.StatusSeeOther)
}

func (a *Admin) userAction(r *http.Request, id string) string {
	ctx := r.Context()
	u, err := a.Accounts.Svc.ByID(ctx, id)
	if err != nil || u == nil {
		return "no account " + id
	}
	who := u.ID[:8]
	if u.Username != "" {
		who = "@" + u.Username
	}
	switch r.FormValue("do") {
	case "role":
		role := r.FormValue("role")
		if !slices.Contains(adminRoles, role) {
			return "unknown role"
		}
		if u.ID == UserFrom(ctx).ID && role != "admin" {
			return "you can't drop your own admin role here"
		}
		if err := a.Accounts.Svc.Update(ctx, u, func(x *account.User) { x.Role = role }); err != nil {
			return "save failed: " + err.Error()
		}
		return who + " is now " + map[bool]string{true: "a regular user", false: role}[role == ""]
	case "rename":
		if err := a.Accounts.Svc.SetUsername(ctx, u, r.FormValue("name")); err != nil {
			return "rename failed: " + err.Error()
		}
		return who + " renamed to @" + u.Username
	case "unname":
		if err := a.Accounts.Svc.ClearUsername(ctx, u); err != nil {
			return "failed: " + err.Error()
		}
		return who + ": name released"
	case "keep":
		if err := a.Accounts.Svc.Update(ctx, u, func(x *account.User) {
			if x.KeepDays == -1 {
				x.KeepDays = 0
			} else {
				x.KeepDays = -1
			}
		}); err != nil {
			return "save failed: " + err.Error()
		}
		return who + map[bool]string{true: ": kept forever", false: ": normal retention"}[u.KeepDays == -1]
	case "patron":
		until := time.Time{}
		if d, err := time.Parse("2006-01-02", r.FormValue("until")); err == nil {
			until = d.Add(24 * time.Hour) // through the end of that day (UTC)
		}
		if err := a.Accounts.Svc.Update(ctx, u, func(x *account.User) { x.PatronUntil = until }); err != nil {
			return "save failed: " + err.Error()
		}
		if u.PatronUntil.IsZero() {
			return who + ": no longer a patron"
		}
		return who + ": patron until " + u.PatronUntil.Add(-time.Hour).Format("2 Jan 2006")
	case "delete-user":
		if u.ID == UserFrom(ctx).ID {
			return "that's you"
		}
		if a.Accounts.Vault != nil {
			_ = a.Accounts.Vault.Delete(ctx, u.ID)
		}
		if err := a.Accounts.Svc.Delete(ctx, u); err != nil {
			return "delete failed: " + err.Error()
		}
		return who + " deleted"
	}
	return ""
}

// areas counts all accounts (named or not) per #geo cell they set, busiest
// first, from a projection on the indexed cell field.
func (a *Admin) areas(ctx context.Context) []areaCount {
	var rows []struct {
		Cell string `datastore:"cell"`
	}
	if _, err := a.DB.GetAll(ctx, datastore.NewQuery("User").Project("cell").Limit(10000), &rows); err != nil {
		log.Printf("admin: areas: %v", err)
		return nil
	}
	n := map[string]int{}
	for _, r := range rows {
		if r.Cell != "" {
			n[r.Cell]++
		}
	}
	out := make([]areaCount, 0, len(n))
	for c, k := range n {
		out = append(out, areaCount{c, k})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N > out[j].N })
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// bulkAction applies one action to the ticked accounts: delete, keep for
// a day (the purge removes them once idle that long), keep forever, or
// back to normal retention. Your own account is skipped.
func (a *Admin) bulkAction(r *http.Request) string {
	ctx, me := r.Context(), UserFrom(r.Context())
	act, n, skipped := r.FormValue("bulk"), 0, 0
	for _, id := range r.Form["ids"] {
		u, err := a.Accounts.Svc.ByID(ctx, id)
		if err != nil || u == nil || u.ID == me.ID {
			skipped++
			continue
		}
		switch act {
		case "delete":
			if a.Accounts.Vault != nil {
				_ = a.Accounts.Vault.Delete(ctx, u.ID)
			}
			err = a.Accounts.Svc.Delete(ctx, u)
		case "day", "keep", "normal":
			keep := map[string]int{"day": 1, "keep": -1, "normal": 0}[act]
			err = a.Accounts.Svc.Update(ctx, u, func(x *account.User) { x.KeepDays = keep })
		default:
			return "unknown bulk action"
		}
		if err != nil {
			skipped++
			continue
		}
		n++
	}
	res := fmt.Sprintf("%s: %d accounts", act, n)
	if skipped > 0 {
		res += fmt.Sprintf(" (%d skipped)", skipped)
	}
	return res
}

// olnStats inspects the Note store: totals by kind and the busiest cells.
type olnStats struct {
	Total, V2, V1, Private, Named, Live int
	// Expired: stored but past their life; Hidden: hidden by moderation.
	// Both are "stored but not live": the purge removes them now.
	Expired, Hidden int
	Cells           []areaCount
	// Messages: the live public ones, newest first (private chat is
	// ciphertext: counted only).
	Messages []adminNote
}

type adminNote struct {
	ID, Text, Cell, Author, By, Biz string
	Bits                            int
	At, ExpiresAt                   time.Time
	Hidden                          bool
}

func (a *Admin) olnStats(ctx context.Context, now time.Time) *olnStats {
	var rows []struct {
		Raw       string    `datastore:"raw,noindex"`
		Text      string    `datastore:"text,noindex"`
		Cell      string    `datastore:"cell"`
		Pair      string    `datastore:"pair"`
		Author    string    `datastore:"author,noindex"`
		By        string    `datastore:"by,noindex"`
		Biz       string    `datastore:"biz,noindex"`
		Bits      int       `datastore:"bits,noindex"`
		At        time.Time `datastore:"at,noindex"`
		ExpiresAt time.Time `datastore:"expires_at"`
	}
	keys, err := a.DB.GetAll(ctx, datastore.NewQuery("Note").Limit(5000), &rows)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			log.Printf("admin: oln stats: %v", err)
			return nil
		}
	}
	hidden := map[string]bool{}
	if a.Notes != nil {
		hidden, _ = a.Notes.Store.Hidden(ctx)
	}
	st, cells := &olnStats{Total: len(rows)}, map[string]int{}
	for i, n := range rows {
		id := keys[i].Name
		live := n.ExpiresAt.After(now)
		if !live {
			st.Expired++
		} else if hidden[id] {
			st.Hidden++
		}
		if live && n.Pair == "" {
			st.Messages = append(st.Messages, adminNote{ID: id, Text: n.Text, Cell: n.Cell, Author: n.Author, By: n.By, Biz: n.Biz,
				Bits: n.Bits, At: n.At, ExpiresAt: n.ExpiresAt, Hidden: hidden[id]})
		}
		if strings.HasPrefix(n.Raw, "v2;") {
			st.V2++
		} else {
			st.V1++
		}
		if n.Pair != "" {
			st.Private++
		} else {
			cells[n.Cell]++
		}
		if n.Author != "" {
			st.Named++
		}
		if n.ExpiresAt.After(now) {
			st.Live++
		}
	}
	for c, k := range cells {
		st.Cells = append(st.Cells, areaCount{c, k})
	}
	sort.Slice(st.Cells, func(i, j int) bool { return st.Cells[i].N > st.Cells[j].N })
	sort.Slice(st.Messages, func(i, j int) bool { return st.Messages[i].At.After(st.Messages[j].At) })
	if len(st.Messages) > 300 {
		st.Messages = st.Messages[:300]
	}
	if len(st.Cells) > 10 {
		st.Cells = st.Cells[:10]
	}
	return st
}

// olnPurge deletes what's stored but not live: expired messages (before
// the Datastore TTL policy gets to them) and hidden ones, with their
// hidden markers.
func (a *Admin) olnPurge(ctx context.Context, now time.Time) (expired, hid int, err error) {
	var rows []struct {
		ExpiresAt time.Time `datastore:"expires_at"`
	}
	keys, err := a.DB.GetAll(ctx, datastore.NewQuery("Note").Project("expires_at").Limit(5000), &rows)
	if err != nil {
		return 0, 0, err
	}
	hidden, _ := a.Notes.Store.Hidden(ctx)
	var del, marks []*datastore.Key
	for i, k := range keys {
		switch {
		case !rows[i].ExpiresAt.After(now):
			del = append(del, k)
			expired++
		case hidden[k.Name]:
			del = append(del, k)
			hid++
		}
	}
	// Every hidden message goes, so every hidden marker is left over.
	for id := range hidden {
		marks = append(marks, datastore.NameKey("HiddenNote", id, nil))
	}
	for _, ks := range [][]*datastore.Key{del, marks} {
		for i := 0; i < len(ks); i += 500 {
			if err := a.DB.DeleteMulti(ctx, ks[i:min(i+500, len(ks))]); err != nil {
				return expired, hid, err
			}
		}
	}
	a.Notes.ForgetAll()
	return expired, hid, nil
}

// olnClean deletes local messages: only v1 leftovers, or (all) every one.
func (a *Admin) olnClean(ctx context.Context, all bool) (int, error) {
	var rows []struct {
		Raw string `datastore:"raw,noindex"`
	}
	keys, err := a.DB.GetAll(ctx, datastore.NewQuery("Note").Limit(5000), &rows)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return 0, err
		}
	}
	var del []*datastore.Key
	for i, k := range keys {
		if all || !strings.HasPrefix(rows[i].Raw, "v2;") {
			del = append(del, k)
		}
	}
	for i := 0; i < len(del); i += 500 {
		if err := a.DB.DeleteMulti(ctx, del[i:min(i+500, len(del))]); err != nil {
			return i, err
		}
	}
	a.Notes.ForgetAll()
	return len(del), nil
}
