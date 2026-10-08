package handler

import (
	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/business"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/atp"
	"github.com/LaPingvino/kafumu/internal/geo"
)

// ATproto connects a Kafumu account to the person's own Bluesky/ATproto
// account. Switched on with KAFUMU_ATPROTO=1 once Kafumu serves from its
// final domain (the client metadata URL is the client's identity).
type ATproto struct {
	Accounts *Accounts
	Svc      *atp.Service
}

// atForCookie: the business an OAuth round trip connects (76d).
const atForCookie = "kafumu_at_for"

// Login handles POST /oauth/login {handle}.
func (h *ATproto) Login(w http.ResponseWriter, r *http.Request) {
	// Acting as a business: the account being connected is the business's
	// (remembered for the round trip; checked again on the way back).
	if b := h.Accounts.Home.ActingAs(r); b != nil {
		http.SetCookie(w, &http.Cookie{Name: atForCookie, Value: b.ID, Path: "/oauth", MaxAge: 600, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	} else {
		http.SetCookie(w, &http.Cookie{Name: atForCookie, Path: "/oauth", MaxAge: -1})
	}
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
	if c, err := r.Cookie(atForCookie); err == nil && c.Value != "" && u != nil && h.Accounts.Home.Biz != nil {
		http.SetCookie(w, &http.Cookie{Name: atForCookie, Path: "/oauth", MaxAge: -1})
		b, err := h.Accounts.Home.Biz.Get(r.Context(), c.Value)
		if err != nil || !h.Accounts.Home.managesBiz(r.Context(), b, u.ID) {
			_ = h.Svc.Disconnect(r.Context(), did, sid)
			http.Redirect(w, r, "/account?err=atproto#atproto", http.StatusSeeOther)
			return
		}
		if b.DID != "" && b.ATSession != "" && (b.DID != did || b.ATSession != sid) {
			_ = h.Svc.Disconnect(r.Context(), b.DID, b.ATSession)
		}
		handle := atp.Handle(r.Context(), did)
		if _, err := h.Accounts.Home.Biz.Update(r.Context(), b.ID, func(x *business.Business) error {
			x.DID, x.ATSession, x.ATHandle = did, sid, handle
			return nil
		}); err != nil {
			http.Error(w, "could not save", http.StatusInternalServerError)
			return
		}
		h.Accounts.Home.forgetBiz(b.ID)
		http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
		return
	}
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
	handle := atp.Handle(r.Context(), did)
	if err := h.Accounts.Svc.Update(r.Context(), u, func(x *account.User) { x.DID, x.ATSession, x.ATHandle = did, sid, handle }); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
}

// Disconnect handles POST /oauth/disconnect.
func (h *ATproto) Disconnect(w http.ResponseWriter, r *http.Request) {
	if b := h.Accounts.Home.ActingAs(r); b != nil { // the business's account
		if b.DID != "" {
			if err := h.Svc.Disconnect(r.Context(), b.DID, b.ATSession); err != nil {
				log.Printf("atproto: disconnect business: %v", err)
			}
			_, _ = h.Accounts.Home.Biz.Update(r.Context(), b.ID, func(x *business.Business) error {
				x.DID, x.ATSession, x.ATHandle = "", "", "" // b is a cached copy: change only these
				return nil
			})
			h.Accounts.Home.forgetBiz(b.ID)
		}
		http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
		return
	}
	if u := UserFrom(r.Context()); u != nil && u.DID != "" {
		if err := h.Svc.Disconnect(r.Context(), u.DID, u.ATSession); err != nil {
			log.Printf("atproto: disconnect: %v", err)
		}
		_ = h.Accounts.Svc.Update(r.Context(), u, func(x *account.User) { x.DID, x.ATSession, x.ATHandle = "", "", "" })
	}
	http.Redirect(w, r, "/account#atproto", http.StatusSeeOther)
}

// Post handles POST /post {text, cell}: a post in your own Bluesky account,
// with the cell's #geo tag added if you didn't type it.
func (h *ATproto) Post(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	cell := strings.ToLower(r.FormValue("cell"))
	back := "/"
	if geo.Valid(cell) {
		back = "/?cell=" + cell
	}
	text := strings.TrimSpace(r.FormValue("text"))
	did, session := bskyAccount(u, h.Accounts.Home.ActingAs(r)) // as the business: its account, or none
	if u == nil || did == "" || text == "" || len([]rune(text)) > 300 {
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	if geo.Valid(cell) && !strings.Contains(strings.ToLower(text), "#geo"+cell) {
		text += "\n\n#geo" + cell
	}
	lang := h.Accounts.Home.newPage(r, "").Lang
	uri, _, err := h.Svc.CreateRecord(r.Context(), did, session, "app.bsky.feed.post", atp.PostRecord(text, lang, time.Now()))
	if err != nil {
		log.Printf("atproto: post: %v", err)
		http.Redirect(w, r, back+sep(back)+"posted=0", http.StatusSeeOther)
		return
	}
	// The device remembers it (77a), to show replies and likes in Activity.
	first, _, _ := strings.Cut(text, "\n")
	if len([]rune(first)) > 80 {
		first = string([]rune(first)[:80])
	}
	http.Redirect(w, r, back+sep(back)+"posted=1&at="+url.QueryEscape(uri)+"&t="+url.QueryEscape(first), http.StatusSeeOther)
}

// sep: "?" or "&", whichever joins one more query parameter to u.
func sep(u string) string {
	if strings.Contains(u, "?") {
		return "&"
	}
	return "?"
}
