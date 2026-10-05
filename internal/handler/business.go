package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/LaPingvino/kafumu/internal/account"
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
	Created bool
	Error   string
}

func (h *Businesses) Show(w http.ResponseWriter, r *http.Request) {
	p := businessPage{page: h.Home.newPage(r, ""), Kinds: business.Kinds, Names: map[string]string{}, Created: r.URL.Query().Get("new") == "1"}
	switch r.URL.Query().Get("err") {
	case "taken":
		p.Error = locale.T(p.Lang, "account.err_taken")
	case "invalid":
		p.Error = locale.T(p.Lang, "account.err_invalid")
	}
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
		if id, err := h.Accounts.Svc.Store.LookupUsername(r.Context(), name); err == nil && id != "" && !strings.HasPrefix(id, "biz:") && !b.Manages(id) && len(b.Managers) < 20 {
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

// Name handles POST /business/{id}/name: the business's own @username.
func (h *Businesses) Name(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	b, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if u == nil || err != nil || !b.Manages(u.ID) {
		http.NotFound(w, r)
		return
	}
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("username")), "@"))
	if name == b.Username {
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	if !account.ValidUsername(name) {
		http.Redirect(w, r, "/business?err=invalid", http.StatusSeeOther)
		return
	}
	if err := h.Accounts.Svc.Store.ClaimUsername(r.Context(), name, "biz:"+b.ID); err != nil {
		http.Redirect(w, r, "/business?err=taken", http.StatusSeeOther)
		return
	}
	old := b.Username
	b.Username = name
	if err := h.Store.Save(r.Context(), b); err != nil {
		_ = h.Accounts.Svc.Store.ReleaseUsername(r.Context(), name, "biz:"+b.ID)
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	if old != "" {
		_ = h.Accounts.Svc.Store.ReleaseUsername(r.Context(), old, "biz:"+b.ID)
	}
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, "/business", http.StatusSeeOther)
}

type bizProfilePage struct {
	page
	B    *business.Business
	Link bool // Contact is a web address
}

// Profile renders kafumu.com/@name for a business: who it is and how to
// reach it. No query: the name and the business are single cached reads.
func (h *Businesses) Profile(w http.ResponseWriter, r *http.Request, id string) {
	b := h.Home.bizByID(r.Context(), id)
	if b == nil {
		http.NotFound(w, r)
		return
	}
	p := bizProfilePage{page: h.Home.newPage(r, b.Name), B: b,
		Link: strings.HasPrefix(b.Contact, "https://") || strings.HasPrefix(b.Contact, "http://")}
	h.Home.render(w, "business_profile.html", p)
}

// bizManaged: the business at {id} if the signed-in user manages it.
func (h *Businesses) bizManaged(r *http.Request) (*business.Business, bool) {
	u := UserFrom(r.Context())
	if u == nil || IsBot(r) {
		return nil, false
	}
	b, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil || !b.Manages(u.ID) {
		return nil, false
	}
	return b, true
}

// VaultAPI handles GET/PUT /api/business/{id}/vault: the business's card
// and contacts, encrypted, for its managers' devices (when sync is on).
func (h *Businesses) VaultAPI(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizManaged(r)
	if !ok || b.SyncMode == "" || h.Accounts.Vault == nil {
		http.Error(w, "not synced", http.StatusNotFound)
		return
	}
	h.Accounts.serveVault(w, r, "biz:"+b.ID)
}

// KeyAPI handles GET /api/business/{id}/key: in server mode, the key that
// opens the business vault, for its managers' devices.
func (h *Businesses) KeyAPI(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizManaged(r)
	if !ok || b.SyncMode != business.SyncServer || b.SyncKey == "" {
		http.Error(w, "no key here", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"key": b.SyncKey})
}

// Sync handles POST /business/{id}/sync: mode=off|server|private. Going to
// server mode takes the key a device already has (key=…), so the vault
// stays readable; without one, it starts a fresh key and vault. Leaving
// server mode, the server forgets the key.
func (h *Businesses) Sync(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizManaged(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.FormValue("mode") {
	case "off":
		b.SyncMode, b.SyncKey = "", ""
	case business.SyncServer:
		if b.SyncMode != business.SyncServer || b.SyncKey == "" {
			key := r.FormValue("key")
			if raw, err := base64.StdEncoding.DecodeString(key); err != nil || len(raw) != 32 {
				raw = make([]byte, 32)
				rand.Read(raw)
				key = base64.StdEncoding.EncodeToString(raw)
				if h.Accounts.Vault != nil {
					_ = h.Accounts.Vault.Delete(r.Context(), "biz:"+b.ID) // sealed with a key nobody gave us
				}
			}
			b.SyncMode, b.SyncKey = business.SyncServer, key
		}
	case business.SyncPrivate:
		b.SyncMode, b.SyncKey = business.SyncPrivate, ""
	default:
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	if err := h.Store.Save(r.Context(), b); err != nil {
		log.Printf("business: sync mode: %v", err)
	}
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, "/business", http.StatusSeeOther)
}
