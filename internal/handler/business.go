package handler

import (
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/locale"
)

// Businesses serves /business: start a business account (first month free)
// and manage its managers. All a business gets today is hosting meetups
// under its own name; the page promises nothing more.
type Businesses struct {
	Home     *Home
	Accounts *Accounts
	Store    *business.Store
}

type businessPage struct {
	page
	Mine    []*business.Business
	Names   map[string]string // manager user id → username
	Kinds   []string
	Now     time.Time
	Created bool
}

func (h *Businesses) Show(w http.ResponseWriter, r *http.Request) {
	p := businessPage{page: h.Home.newPage(r, ""), Kinds: business.Kinds, Now: time.Now(), Names: map[string]string{}, Created: r.URL.Query().Get("new") == "1"}
	p.Title, p.Tab = locale.T(p.Lang, "biz.title"), "account"
	if u := p.User; u != nil {
		p.Mine, _ = h.Store.ForUser(r.Context(), u.ID)
		for _, b := range p.Mine {
			for _, id := range b.Managers {
				if m, err := h.Accounts.Svc.ByID(r.Context(), id); err == nil && m != nil {
					p.Names[id] = m.Username
				}
			}
		}
	}
	h.Home.render(w, "business.html", p)
}

func (h *Businesses) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || IsBot(r) {
		http.Redirect(w, r, "/account?next=/business", http.StatusSeeOther)
		return
	}
	if mine, _ := h.Store.ForUser(r.Context(), u.ID); len(mine) >= 5 {
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	b, err := h.Store.Create(r.Context(), r.FormValue("name"), r.FormValue("kind"), r.FormValue("contact"), u.ID, time.Now())
	if err != nil {
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	setAs(w, b.ID) // straight into using it (Joop)
	log.Printf("business: created by %s", u.ID[:8])
	http.Redirect(w, r, "/business?new=1", http.StatusSeeOther)
}

// Managers handles POST /business/{id}/managers: add=<username> or remove=<user id>.
func (h *Businesses) Managers(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	b, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if u == nil || err != nil || !b.Manages(u.ID) {
		http.NotFound(w, r)
		return
	}
	if name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("add")), "@")); name != "" {
		if id, err := h.Accounts.Svc.Store.LookupUsername(r.Context(), name); err == nil && id != "" && !b.Manages(id) && len(b.Managers) < 20 {
			b.Managers = append(b.Managers, id)
		}
	}
	if rm := r.FormValue("remove"); rm != "" && rm != u.ID {
		b.Managers = slices.DeleteFunc(b.Managers, func(x string) bool { return x == rm })
	}
	_ = h.Store.Save(r.Context(), b)
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, "/business", http.StatusSeeOther)
}
