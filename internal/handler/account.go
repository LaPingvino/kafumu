package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/handle"
	"github.com/LaPingvino/kafumu/internal/vault"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/langs"
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
	// Prices records public-inbox prices for the mailbox to enforce.
	Prices account.InboxPrices
	// Cache holds short-lived move requests between your own devices.
	Cache cache.Cache
	// Vault holds each account's encrypted cards and contacts (device sync).
	Vault vault.Store
	// Handles maps usernames to long-lived connect codes (kafumu.com/@name).
	Handles *handle.Store
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
	MyLangs  []myLang
	Visible  bool
	MagicURL string
	Next     string
	New      bool
	Error    string
	// Findable: the /findable tab (public profile and inbox only).
	Findable bool
}

// Show handles GET /account.
func (a *Accounts) Show(w http.ResponseWriter, r *http.Request) { a.show(w, r, false) }

// ShowFindable handles GET /findable: being findable (public profile,
// languages, interests, public inbox) as its own tab.
func (a *Accounts) ShowFindable(w http.ResponseWriter, r *http.Request) { a.show(w, r, true) }

func (a *Accounts) show(w http.ResponseWriter, r *http.Request, findable bool) {
	p := accountPage{page: a.Home.newPage(r, ""), Findable: findable}
	p.Title, p.Tab = locale.T(p.Lang, "account.title"), "account"
	if findable {
		p.Title, p.Tab = locale.T(p.Lang, "profile.title"), "findable"
	}
	if u := p.User; u != nil {
		p.MagicURL = a.Home.Cfg.Origin + "/auth/link?k=" + template.URLQueryEscaper(mustCookie(r))
	}
	p.New = r.URL.Query().Get("new") == "1"
	if u := p.User; u != nil {
		p.Visible = u.Visible(time.Now())
		for _, l := range u.Langs {
			code, lvl, _ := strings.Cut(l, "/")
			p.MyLangs = append(p.MyLangs, myLang{code, langs.Names[code], lvl})
		}
	}
	p.Next = localPath(r.URL.Query().Get("next"))
	if findable && p.User == nil {
		p.Next = "/findable" // create an account, then come straight back
	}
	switch r.URL.Query().Get("err") {
	case "taken":
		p.Error = locale.T(p.Lang, "account.err_taken")
	case "invalid":
		p.Error = locale.T(p.Lang, "account.err_invalid")
	}
	a.Home.render(w, "account.html", p)
}

type myLang struct{ Code, Name, Level string }

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
		if u.DID != "" && a.Home.ATproto != nil {
			_ = a.Home.ATproto.Disconnect(r.Context(), u.DID, u.ATSession)
		}
		if u.InboxBox != "" && a.Prices != nil {
			_ = a.Prices.Delete(r.Context(), u.InboxBox)
		}
		if a.Vault != nil {
			_ = a.Vault.Delete(r.Context(), u.ID)
		}
		if a.Handles != nil && u.Username != "" {
			_ = a.Handles.Delete(r.Context(), u.Username)
		}
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

// SetProfile handles POST /account/profile: the opt-in public profile.
func (a *Accounts) SetProfile(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Redirect(w, r, "/account", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	var ls []string
	for _, code := range r.Form["lang"] {
		if lvl := r.FormValue("level_" + code); lvl != "" {
			ls = append(ls, code+"/"+lvl)
		}
	}
	hours, _ := strconv.Atoi(r.FormValue("visible_hours"))
	tags := strings.FieldsFunc(r.FormValue("tags"), func(c rune) bool { return c == ',' })
	if err := a.Svc.SetProfile(r.Context(), u, r.FormValue("cell"), r.FormValue("bio"), r.FormValue("where"), ls, tags, time.Duration(hours)*time.Hour); err != nil {
		log.Printf("account: profile: %v", err)
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/findable", http.StatusSeeOther)
}

// LinkJSON handles GET /account/link.json: this device's sign-in link on
// the canonical origin, for moving to it. Same-origin only (cookie).
func (a *Accounts) LinkJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if UserFrom(r.Context()) == nil {
		w.Write([]byte("{}"))
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"link": a.Home.Cfg.Origin + "/auth/link?k=" + template.URLQueryEscaper(mustCookie(r))})
}

// SetInbox handles POST /account/inbox {box, pub, bits} — or close=1 — for
// the public inbox, whose keys are made on the device.
func (a *Accounts) SetInbox(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	box, pub := r.FormValue("box"), r.FormValue("pub")
	if r.FormValue("close") == "1" {
		box, pub = "", ""
	}
	bits, _ := strconv.Atoi(r.FormValue("bits"))
	if err := a.Svc.SetInbox(r.Context(), a.Prices, u, box, pub, bits); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Account-bound moves: a new device signed into this account asks for the
// data; another device signed into the same account sees the request and
// sends it end-to-end encrypted to the new device's key. No code to scan,
// so nothing to substitute. Requests live 15 minutes in the cache.

type moveRequest struct {
	Box string `json:"box"`
	Pub string `json:"pub"`
	At  int64  `json:"at"`
}

// MoveRequest handles POST /account/move {box, pub} (new device) and
// GET /account/move (old device: is there a request?) and DELETE.
func (a *Accounts) MoveRequest(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || a.Cache == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	key := "move:" + u.ID
	w.Header().Set("Cache-Control", "no-store")
	switch r.Method {
	case http.MethodPost:
		m := moveRequest{Box: r.FormValue("box"), Pub: r.FormValue("pub"), At: time.Now().Unix()}
		if !boxIDRE.MatchString(m.Box) || len(m.Pub) < 80 || len(m.Pub) > 100 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		b, _ := json.Marshal(m)
		a.Cache.Set(r.Context(), key, b, 15*time.Minute)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		a.Cache.Delete(r.Context(), key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Content-Type", "application/json")
		if b, ok := a.Cache.Get(r.Context(), key); ok {
			w.Write(b)
			return
		}
		w.Write([]byte("{}"))
	}
}

var boxIDRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// VaultAPI handles GET and PUT /api/vault: your own encrypted cards and
// contacts, for your other devices. PUT needs If-Match: <version> and
// answers 409 when another device wrote first (pull, merge, retry).
func (a *Accounts) VaultAPI(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || a.Vault == nil || IsBot(r) {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPut {
		want, err := strconv.Atoi(r.Header.Get("If-Match"))
		if err != nil {
			http.Error(w, "If-Match: version wanted", http.StatusPreconditionRequired)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, vault.MaxBytes+1))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		v, err := a.Vault.Put(r.Context(), u.ID, want, body, time.Now())
		switch {
		case errors.Is(err, vault.ErrConflict):
			http.Error(w, "conflict", http.StatusConflict)
		case errors.Is(err, vault.ErrTooBig):
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
		case err != nil:
			log.Printf("vault: put: %v", err)
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		default:
			json.NewEncoder(w).Encode(map[string]int{"version": v})
		}
		return
	}
	v, err := a.Vault.Get(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	out := struct {
		Version int    `json:"version"`
		Data    string `json:"data,omitempty"`
	}{}
	if v != nil {
		out.Version, out.Data = v.Version, string(v.Data)
	}
	json.NewEncoder(w).Encode(out)
}

// SetHandle handles PUT /api/handle {payload}: your kafumu.com/@name link
// now leads to this connect code (renewed by your device; 90 days).
func (a *Accounts) SetHandle(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || u.Username == "" || a.Handles == nil || IsBot(r) {
		http.Error(w, "a named account is needed", http.StatusUnauthorized)
		return
	}
	var in struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in); err != nil || !handle.PayloadRE.MatchString(in.Payload) {
		http.Error(w, "want {payload: v1.…}", http.StatusBadRequest)
		return
	}
	if err := a.Handles.Set(r.Context(), u.Username, u.ID, in.Payload, time.Now()); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": a.Home.Cfg.Origin + "/@" + u.Username})
}

// FollowHandle serves /@name: to that person's connect code, marked as theirs.
func (a *Accounts) FollowHandle(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.ToLower(name)
	if a.Handles == nil || !account.ValidUsername(name) {
		http.NotFound(w, r)
		return
	}
	payload, ok := a.Handles.Get(r.Context(), name, time.Now())
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/c?from="+url.QueryEscape(name)+"#"+payload, http.StatusFound)
}

// DeleteHandle handles DELETE /api/handle: your kafumu.com/@name link stops working.
func (a *Accounts) DeleteHandle(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil || u.Username == "" || a.Handles == nil {
		http.Error(w, "a named account is needed", http.StatusUnauthorized)
		return
	}
	if err := a.Handles.Delete(r.Context(), u.Username); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
