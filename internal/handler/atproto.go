package handler

import (
	"log"
	"net/http"

	"github.com/LaPingvino/kafumu/internal/atp"
)

// ATproto connects a Kafumu account to the person's own Bluesky/ATproto
// account. Switched on with KAFUMU_ATPROTO=1 once Kafumu serves from its
// final domain (the client metadata URL is the client's identity).
type ATproto struct {
	Accounts *Accounts
	Svc      *atp.Service
}

// Login handles POST /oauth/login {handle}.
func (h *ATproto) Login(w http.ResponseWriter, r *http.Request) {
	to, err := h.Svc.Start(r.Context(), r.FormValue("handle"))
	if err != nil {
		log.Printf("atproto: start: %v", err)
		http.Redirect(w, r, "/account?err=atproto#atproto", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// Callback handles GET /oauth/callback. Signed-in people get the ATproto
// account linked; others get a fresh Kafumu account with it linked.
func (h *ATproto) Callback(w http.ResponseWriter, r *http.Request) {
	did, sid, err := h.Svc.Finish(r.Context(), r.URL.Query())
	if err != nil {
		log.Printf("atproto: callback: %v", err)
		http.Redirect(w, r, "/account?err=atproto#atproto", http.StatusSeeOther)
		return
	}
	u := UserFrom(r.Context())
	if u == nil {
		nu, cred, err := h.Accounts.Svc.Create(r.Context(), h.Accounts.Home.newPage(r, "").Lang)
		if err != nil {
			http.Error(w, "could not create account", http.StatusInternalServerError)
			return
		}
		setCookie(w, cred)
		u = nu
	}
	if u.DID != "" && u.ATSession != "" && (u.DID != did || u.ATSession != sid) {
		_ = h.Svc.Disconnect(r.Context(), u.DID, u.ATSession)
	}
	u.DID, u.ATSession, u.ATHandle = did, sid, atp.Handle(r.Context(), did)
	if err := h.Accounts.Svc.Save(r.Context(), u); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
}

// Disconnect handles POST /oauth/disconnect.
func (h *ATproto) Disconnect(w http.ResponseWriter, r *http.Request) {
	if u := UserFrom(r.Context()); u != nil && u.DID != "" {
		if err := h.Svc.Disconnect(r.Context(), u.DID, u.ATSession); err != nil {
			log.Printf("atproto: disconnect: %v", err)
		}
		u.DID, u.ATSession, u.ATHandle = "", "", ""
		_ = h.Accounts.Svc.Save(r.Context(), u)
	}
	http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
}
