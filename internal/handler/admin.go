package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/report"
	"html/template"
	"log"
	"net/http"
	"slices"
	"sort"
	"strings"
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
	Reports  *report.Service
	// Jobs are the cron jobs, runnable by hand: name → run.
	Jobs map[string]func(ctx context.Context) string
}

// Initial handles GET /admin/initial.
func (a *Admin) Initial(w http.ResponseWriter, r *http.Request) {
	if !cache.OnAppEngine() || !gaeuser.IsAdmin(r.Context()) {
		http.Error(w, "only for the App Engine project's admins", http.StatusForbidden)
		return
	}
	u := UserFrom(r.Context())
	if u == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><meta charset="utf-8"><title>Admin setup</title>
<h1>Admin setup</h1><p>You're an App Engine admin. Now sign in to Kafumu on this browser —
with your passkey or sign-in link at <a href="/account">/account</a> — and come back to
<a href="/admin/initial">/admin/initial</a>.</p>`)
		return
	}
	if u.Role != "admin" {
		u.Role = "admin"
		if err := a.Accounts.Svc.Save(r.Context(), u); err != nil {
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
	// Full is an admin (accounts, jobs); a moderator sees only the queue.
	Full bool
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
	if a.DB != nil {
		now := time.Now()
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
		p.Areas = a.areas(ctx)
	}
	a.Home.render(w, "admin.html", p)
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
		q = datastore.NewQuery("User").FilterField("username", ">=", s).FilterField("username", "<", s+"\uffff").Order("username").Limit(50)
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
	case "role", "rename", "unname", "keep", "delete-user":
		res = a.userAction(r, id)
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
		u.Role = role
		if err := a.Accounts.Svc.Save(ctx, u); err != nil {
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
		if u.KeepDays == -1 {
			u.KeepDays = 0
		} else {
			u.KeepDays = -1
		}
		if err := a.Accounts.Svc.Save(ctx, u); err != nil {
			return "save failed: " + err.Error()
		}
		return who + map[bool]string{true: ": kept forever", false: ": normal retention"}[u.KeepDays == -1]
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
