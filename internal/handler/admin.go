package handler

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
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
	if u := UserFrom(r.Context()); u != nil && u.Role == "admin" {
		return true
	}
	http.NotFound(w, r)
	return false
}

type adminPage struct {
	page
	Counts map[string]int
	Result string
	Jobs   []string
}

// Show handles GET /admin.
func (a *Admin) Show(w http.ResponseWriter, r *http.Request) {
	if !a.admin(w, r) {
		return
	}
	p := adminPage{page: a.Home.newPage(r, "Admin"), Counts: map[string]int{}, Result: r.URL.Query().Get("r")}
	for name := range a.Jobs {
		p.Jobs = append(p.Jobs, name)
	}
	if a.DB != nil {
		now := time.Now()
		for _, k := range []struct{ kind, field string }{{"User", ""}, {"Meetup", "expires_at"}, {"Note", "expires_at"},
			{"Box", "expires_at"}, {"Slot", "expires_at"}, {"PushSub", "expires_at"}, {"ATSession", "expires_at"}} {
			q := datastore.NewQuery(k.kind)
			if k.field != "" {
				q = q.FilterField(k.field, ">", now)
			}
			res, err := a.DB.RunAggregationQuery(r.Context(), q.NewAggregationQuery().WithCount("n"))
			if err == nil {
				if v, ok := res["n"].(int64); ok {
					p.Counts[k.kind] = int(v)
				}
			}
		}
	}
	a.Home.render(w, "admin.html", p)
}

// Action handles POST /admin/action: hide a note, delete a meetup, run a job.
func (a *Admin) Action(w http.ResponseWriter, r *http.Request) {
	if !a.admin(w, r) {
		return
	}
	ctx, id, res := r.Context(), strings.TrimSpace(r.FormValue("id")), ""
	switch r.FormValue("do") {
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
	case "run":
		if job, ok := a.Jobs[r.FormValue("job")]; ok {
			res = r.FormValue("job") + ": " + job(ctx)
		}
	}
	log.Printf("admin: %s", res)
	http.Redirect(w, r, "/admin?r="+template.URLQueryEscaper(res), http.StatusSeeOther)
}
