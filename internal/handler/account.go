package handler

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/locale"
)

const cookieName = "k"

type ctxKey struct{}

// UserFrom returns the signed-in user, or nil.
func UserFrom(ctx context.Context) *account.User {
	u, _ := ctx.Value(ctxKey{}).(*account.User)
	return u
}

// Accounts handles sign-in and the account page.
type Accounts struct {
	Home *Home
	Svc  *account.Service
}

// Middleware resolves the "k" cookie. It never creates an account: page views
// by people (or bots) without one cost nothing.
func (a *Accounts) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
			if u, err := a.Svc.Resolve(r.Context(), c.Value); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
			} else if errors.Is(err, account.ErrNotFound) {
				clearCookie(w)
			} else {
				log.Printf("account: resolve: %v", err)
			}
		}
		next.ServeHTTP(w, r)
	})
}

type accountPage struct {
	page
	MagicURL string
	Next     string
	New      bool
	Error    string
}

// Show handles GET /account.
func (a *Accounts) Show(w http.ResponseWriter, r *http.Request) {
	p := accountPage{page: a.Home.newPage(r, "")}
	p.Title, p.Tab = locale.T(p.Lang, "account.title"), "account"
	if u := p.User; u != nil {
		p.MagicURL = a.Home.Cfg.Origin + "/auth/link?k=" + template.URLQueryEscaper(mustCookie(r))
	}
	p.New = r.URL.Query().Get("new") == "1"
	p.Next = localPath(r.URL.Query().Get("next"))
	switch r.URL.Query().Get("err") {
	case "taken":
		p.Error = locale.T(p.Lang, "account.err_taken")
	case "invalid":
		p.Error = locale.T(p.Lang, "account.err_invalid")
	}
	a.Home.render(w, "account.html", p)
}

// Start handles POST /account/start: the one place accounts are created.
func (a *Accounts) Start(w http.ResponseWriter, r *http.Request) {
	if IsBot(r) {
		http.Error(w, "not for robots", http.StatusForbidden)
		return
	}
	if UserFrom(r.Context()) != nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	lang := a.Home.newPage(r, "").Lang
	_, cred, err := a.Svc.Create(r.Context(), lang)
	if err != nil {
		log.Printf("account: create: %v", err)
		http.Error(w, "could not create account", http.StatusInternalServerError)
		return
	}
	setCookie(w, cred)
	next := "/account?new=1"
	if n := localPath(r.FormValue("next")); n != "" {
		next = "/account?new=1&next=" + n
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

// localPath returns p if it is a path on this site, else "".
func localPath(p string) string {
	if strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.ContainsAny(p, "\\\r\n") && len(p) < 200 {
		return p
	}
	return ""
}

// Link handles GET /auth/link?k=…, the magic link: sign in on this device.
func (a *Accounts) Link(w http.ResponseWriter, r *http.Request) {
	cred := r.URL.Query().Get("k")
	if _, err := a.Svc.Resolve(r.Context(), cred); err != nil {
		http.Error(w, "this sign-in link is not valid (any more)", http.StatusUnauthorized)
		return
	}
	setCookie(w, cred)
	http.Redirect(w, r, "/account", http.StatusSeeOther)
}

// SetName handles POST /account/name.
func (a *Accounts) SetName(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	err := a.Svc.SetUsername(r.Context(), u, r.FormValue("username"))
	switch {
	case errors.Is(err, account.ErrTaken):
		http.Redirect(w, r, "/account?err=taken", http.StatusSeeOther)
	case err != nil:
		http.Redirect(w, r, "/account?err=invalid", http.StatusSeeOther)
	default:
		http.Redirect(w, r, "/account", http.StatusSeeOther)
	}
}

// SignOut handles POST /account/signout: forget the account on this device.
func (a *Accounts) SignOut(w http.ResponseWriter, r *http.Request) {
	clearCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Delete handles POST /account/delete.
func (a *Accounts) Delete(w http.ResponseWriter, r *http.Request) {
	if u := UserFrom(r.Context()); u != nil && r.FormValue("confirm") == "yes" {
		if err := a.Svc.Delete(r.Context(), u); err != nil {
			log.Printf("account: delete: %v", err)
			http.Error(w, "could not delete account", http.StatusInternalServerError)
			return
		}
		clearCookie(w)
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func mustCookie(r *http.Request) string {
	c, _ := r.Cookie(cookieName)
	if c == nil {
		return ""
	}
	return c.Value
}

func setCookie(w http.ResponseWriter, cred string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: cred, Path: "/", HttpOnly: true, Secure: true,
		SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(400 * 24 * time.Hour),
	})
}

func clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true})
}
