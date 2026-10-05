package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/LaPingvino/kafumu/internal/business"
)

// Acting as a business (Joop: "switch to it… under the username button…
// automatically switch after creating it"). The choice is a cookie; each
// page checks it against the business (cached a minute, so no Datastore
// read per view) and that you still manage it. Acting changes the name on
// the username button, the default host for new meetups, and the nudge bar.
const asCookie = "kafumu_as"

type bizCache struct {
	mu sync.Mutex
	m  map[string]bizEntry
}

type bizEntry struct {
	b  *business.Business
	at time.Time
}

func (h *Home) acting(ctx context.Context, r *http.Request, userID string) *business.Business {
	c, err := r.Cookie(asCookie)
	if err != nil || c.Value == "" || h.Biz == nil {
		return nil
	}
	b := h.bizByID(ctx, c.Value)
	if b == nil || !b.Manages(userID) {
		return nil
	}
	return b
}

// bizByID is a business, cached a minute (nil if there is none).
func (h *Home) bizByID(ctx context.Context, id string) *business.Business {
	if h.Biz == nil || id == "" {
		return nil
	}
	h.bizc.mu.Lock()
	e, ok := h.bizc.m[id]
	h.bizc.mu.Unlock()
	if !ok || time.Since(e.at) > time.Minute {
		b, err := h.Biz.Get(ctx, id)
		if err != nil {
			b = nil
		}
		e = bizEntry{b, time.Now()}
		h.bizc.mu.Lock()
		if h.bizc.m == nil || len(h.bizc.m) > 1000 {
			h.bizc.m = map[string]bizEntry{}
		}
		h.bizc.m[id] = e
		h.bizc.mu.Unlock()
	}
	return e.b
}

// forgetBiz drops a business from the cache (after it changed).
func (h *Home) forgetBiz(id string) {
	h.bizc.mu.Lock()
	delete(h.bizc.m, id)
	h.bizc.mu.Unlock()
}

func setAs(w http.ResponseWriter, id string) {
	c := &http.Cookie{Name: asCookie, Value: id, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
		Expires: time.Now().Add(400 * 24 * time.Hour)}
	if id == "" {
		c.MaxAge, c.Expires = -1, time.Time{}
	}
	http.SetCookie(w, c)
}

// Use handles POST /account/as: id=<business id> to act as it, id="" for yourself.
func (h *Businesses) Use(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || IsBot(r) {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	id := r.FormValue("id")
	if id != "" {
		b, err := h.Store.Get(r.Context(), id)
		if err != nil || !b.Manages(u.ID) {
			http.Redirect(w, r, "/account", http.StatusSeeOther)
			return
		}
	}
	setAs(w, id)
	next := localPath(r.FormValue("next"))
	if next == "" {
		next = "/account"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// ActingAs is the business the request's user acts as, or nil.
func (h *Home) ActingAs(r *http.Request) *business.Business {
	u := UserFrom(r.Context())
	if u == nil {
		return nil
	}
	return h.acting(r.Context(), r, u.ID)
}
