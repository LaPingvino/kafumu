package handler

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/account"
	"log"
	"net/http"
	"net/url"
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
	case "paid":
		p.Error = locale.T(p.Lang, "biz.paid_feature")
	}
	p.Title, p.Tab = locale.T(p.Lang, "biz.title"), "account"
	if u := p.User; u != nil {
		p.Mine, _ = h.Home.bizFor(r.Context(), u.ID)
		for _, b := range p.Mine {
			for _, id := range b.Managers {
				p.Names[id] = strings.TrimPrefix(h.Home.ownerName(r.Context(), h.Accounts.Svc, id), "@")
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
	if u == nil || err != nil || !h.Home.managesBiz(r.Context(), b, u.ID) {
		http.NotFound(w, r)
		return
	}
	add := ""
	if name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("add")), "@")); name != "" && !b.Live(time.Now()) {
		http.Redirect(w, r, "/business?err=paid", http.StatusSeeOther) // more managers: an office tool (45b)
		return
	} else if name != "" {
		if id, err := h.Accounts.Svc.Store.LookupUsername(r.Context(), name); err == nil && id != "" && id != "biz:"+b.ID {
			add = id
		}
	}
	rm := r.FormValue("remove")
	_, _ = h.Store.Update(r.Context(), b.ID, func(x *business.Business) error {
		if add != "" && !x.Manages(add) && len(x.Managers) < 20 {
			x.Managers = append(x.Managers, add)
		}
		if rm != "" && rm != u.ID {
			x.Managers = slices.DeleteFunc(x.Managers, func(m string) bool { return m == rm })
		}
		return nil
	})
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, "/business", http.StatusSeeOther)
}

// Name handles POST /business/{id}/name: the business's own @username.
func (h *Businesses) Name(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	b, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if u == nil || err != nil || !h.Home.managesBiz(r.Context(), b, u.ID) {
		http.NotFound(w, r)
		return
	}
	back := "/business"
	if localPath(r.FormValue("next")) == "/account" {
		back = "/account"
	}
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.FormValue("username")), "@"))
	if name == b.Username {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if !account.ValidUsername(name) {
		http.Redirect(w, r, back+"?err=invalid", http.StatusSeeOther)
		return
	}
	if !b.Live(time.Now()) {
		http.Redirect(w, r, back+"?err=paid", http.StatusSeeOther)
		return
	}
	if err := h.Accounts.Svc.Store.ClaimUsername(r.Context(), name, "biz:"+b.ID); err != nil {
		http.Redirect(w, r, back+"?err=taken", http.StatusSeeOther)
		return
	}
	old := b.Username
	if _, err := h.Store.Update(r.Context(), b.ID, func(x *business.Business) error { x.Username = name; return nil }); err != nil {
		_ = h.Accounts.Svc.Store.ReleaseUsername(r.Context(), name, "biz:"+b.ID)
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if old != "" {
		_ = h.Accounts.Svc.Store.ReleaseUsername(r.Context(), old, "biz:"+b.ID)
	}
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type bizProfilePage struct {
	page
	B    *business.Business
	Link bool // Contact is a web address
	// Connect: the business's named link (a connect code), when it has one
	// and is live.
	Connect string
}

// Profile renders kafumu.com/@name for a business: who it is and how to
// reach it. No query: the name and the business are single cached reads.
func (h *Businesses) Profile(w http.ResponseWriter, r *http.Request, id, payload string) {
	b := h.Home.bizByID(r.Context(), id)
	if b == nil {
		http.NotFound(w, r)
		return
	}
	p := bizProfilePage{page: h.Home.newPage(r, b.Name), B: b,
		Link: strings.HasPrefix(b.Contact, "https://") || strings.HasPrefix(b.Contact, "http://")}
	if payload != "" && b.Live(time.Now()) && b.Username != "" {
		p.Connect = "/c?from=" + url.QueryEscape(b.Username) + "&biz=1#" + payload
	}
	h.Home.render(w, "business_profile.html", p)
}

// bizManaged: the business at {id} if the signed-in user manages it.
// bizSyncing: the business at {id} for one of its managers, with sync
// usable: a lapsed business's sync pauses (devices keep their data).
func (h *Businesses) bizSyncing(w http.ResponseWriter, r *http.Request) (*business.Business, bool) {
	b, ok := h.bizManaged(r)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return nil, false
	}
	if !b.Live(time.Now()) {
		http.Error(w, "paused until the business account is paid", http.StatusPaymentRequired)
		return nil, false
	}
	return b, true
}

func (h *Businesses) bizManaged(r *http.Request) (*business.Business, bool) {
	u := UserFrom(r.Context())
	if u == nil || IsBot(r) {
		return nil, false
	}
	b, err := h.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil || !h.Home.managesBiz(r.Context(), b, u.ID) {
		return nil, false
	}
	return b, true
}

// VaultAPI handles GET/PUT /api/business/{id}/vault: the business's card
// and contacts, encrypted, for its managers' devices (when sync is on).
func (h *Businesses) VaultAPI(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizSyncing(w, r)
	if !ok {
		return
	}
	if b.SyncMode == "" || h.Accounts.Vault == nil {
		http.Error(w, "not synced", http.StatusNotFound)
		return
	}
	h.Accounts.serveVault(w, r, "biz:"+b.ID)
}

// KeyAPI handles GET /api/business/{id}/key: in server mode, the key that
// opens the business vault, for its managers' devices.
func (h *Businesses) KeyAPI(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizSyncing(w, r)
	if !ok {
		return
	}
	if b.SyncMode != business.SyncServer || b.SyncKey == "" {
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
	if mode := r.FormValue("mode"); mode != "off" && mode != b.SyncMode && !b.Live(time.Now()) {
		http.Redirect(w, r, "/business?err=paid", http.StatusSeeOther)
		return
	}
	mode, key, keyReqs := b.SyncMode, b.SyncKey, b.KeyReqs
	switch r.FormValue("mode") {
	case "off":
		mode, key, keyReqs = "", "", nil
	case business.SyncServer:
		if b.SyncMode != business.SyncServer || b.SyncKey == "" {
			key = r.FormValue("key")
			if raw, err := base64.StdEncoding.DecodeString(key); err != nil || len(raw) != 32 {
				raw = make([]byte, 32)
				rand.Read(raw)
				key = base64.StdEncoding.EncodeToString(raw)
				if h.Accounts.Vault != nil {
					_ = h.Accounts.Vault.Delete(r.Context(), "biz:"+b.ID) // sealed with a key nobody gave us
				}
			}
			mode = business.SyncServer
		}
	case business.SyncPrivate:
		mode, key = business.SyncPrivate, ""
	default:
		http.Redirect(w, r, "/business", http.StatusSeeOther)
		return
	}
	// Atomically, so a manager's other device saving the business at the
	// same moment can't put the old mode back (it did, in the browser test).
	if _, err := h.Store.Update(r.Context(), b.ID, func(x *business.Business) error {
		x.SyncMode, x.SyncKey = mode, key
		if mode == "" {
			x.KeyReqs = keyReqs
		}
		return nil
	}); err != nil {
		log.Printf("business: sync mode: %v", err)
	}
	h.Home.forgetBiz(b.ID)
	http.Redirect(w, r, "/business", http.StatusSeeOther)
}

// KeyReqAPI handles the key handover in private mode:
//
//	GET  /api/business/{id}/keyreq?pub=<this device's>: other devices' open
//	     requests (yours included: your second phone is another device),
//	     and this device's own, with the wrapped key once it was sent;
//	POST /api/business/{id}/keyreq: pub=<raw base64>, this device asks;
//	POST /api/business/{id}/keygrant: pub=<the request's>, wrapped=<…>, a
//	     manager's device sends the key sealed to that public key.
//
// Requests older than a week, or from people no longer managers, drop out.
func (h *Businesses) KeyReqAPI(w http.ResponseWriter, r *http.Request) {
	b, ok := h.bizSyncing(w, r)
	if !ok {
		return
	}
	if b.SyncMode != business.SyncPrivate {
		http.Error(w, "no private sync here", http.StatusNotFound)
		return
	}
	u := UserFrom(r.Context())
	now := time.Now()
	b.KeyReqs = slices.DeleteFunc(b.KeyReqs, func(k business.KeyReq) bool {
		return now.Sub(k.At) > 7*24*time.Hour || !h.Home.managesBiz(r.Context(), b, k.UserID)
	})
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method == http.MethodGet:
		type out struct {
			Name    string `json:"name"`
			Pub     string `json:"pub"`
			Wrapped string `json:"wrapped,omitempty"`
			Mine    bool   `json:"mine,omitempty"`
		}
		list, me := []out{}, r.URL.Query().Get("pub")
		for _, k := range b.KeyReqs {
			o := out{Pub: k.Pub, Mine: me != "" && k.Pub == me}
			if o.Mine {
				o.Wrapped = k.Wrapped
			} else if k.Wrapped != "" {
				continue // answered
			}
			if m, err := h.Accounts.Svc.ByID(r.Context(), k.UserID); err == nil && m != nil {
				o.Name = m.Username
			}
			list = append(list, o)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}
	// POST keyreq / keygrant: applied atomically to the current copy, so two
	// devices asking or granting at once don't drop each other's change.
	pub := r.FormValue("pub")
	isReq := strings.HasSuffix(r.URL.Path, "/keyreq")
	if isReq {
		if raw, err := base64.StdEncoding.DecodeString(pub); err != nil || len(raw) != 65 {
			http.Error(w, "pub: a raw P-256 key", http.StatusBadRequest)
			return
		}
	}
	_, err := h.Store.Update(r.Context(), b.ID, func(x *business.Business) error {
		x.KeyReqs = slices.DeleteFunc(x.KeyReqs, func(k business.KeyReq) bool {
			return now.Sub(k.At) > 7*24*time.Hour || !h.Home.managesBiz(r.Context(), x, k.UserID)
		})
		if isReq {
			x.KeyReqs = slices.DeleteFunc(x.KeyReqs, func(k business.KeyReq) bool { return k.Pub == pub && k.UserID == u.ID })
			if r.FormValue("cancel") == "1" { // this device has the key now: withdraw its request
				return nil
			}
			if slices.ContainsFunc(x.KeyReqs, func(k business.KeyReq) bool { return k.Pub == pub }) {
				return &refusal{http.StatusConflict, "taken"}
			}
			mine := 0
			for _, k := range x.KeyReqs {
				if k.UserID == u.ID {
					mine++
				}
			}
			if mine >= 5 {
				return &refusal{http.StatusTooManyRequests, "too many open requests"}
			}
			x.KeyReqs = append(x.KeyReqs, business.KeyReq{UserID: u.ID, Pub: pub, At: now})
			return nil
		}
		wrapped := r.FormValue("wrapped") // keygrant
		i := slices.IndexFunc(x.KeyReqs, func(k business.KeyReq) bool { return k.Pub == pub })
		if i < 0 || wrapped == "" || len(wrapped) > 1000 {
			return &refusal{http.StatusNotFound, "no such request"}
		}
		x.KeyReqs[i].Wrapped, x.KeyReqs[i].From = wrapped, u.ID
		return nil
	})
	var no *refusal
	switch {
	case errors.As(err, &no):
		http.Error(w, no.msg, no.code)
		return
	case err != nil:
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// refusal: a change an atomic update declined, as the HTTP answer to give.
type refusal struct {
	code int
	msg  string
}

func (e *refusal) Error() string { return e.msg }
