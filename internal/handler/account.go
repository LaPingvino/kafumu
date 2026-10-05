package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/business"
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
	// BizProfile serves /@name when the name is a business's.
	BizProfile func(w http.ResponseWriter, r *http.Request, id, payload string)
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
	MyLangs []myLang
	// MyBiz: the businesses you manage, to switch to (acting.go).
	MyBiz    []*business.Business
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
		p.MagicURL = a.Home.Origin(r) + "/auth/link?k=" + template.URLQueryEscaper(mustCookie(r))
	}
	p.New = r.URL.Query().Get("new") == "1"
	if b := p.Acting; findable && b != nil && p.User != nil {
		// Acting as a business: the form is the business's profile (76c).
		view := *p.User
		view.Username, view.Bio, view.Where, view.Cell, view.Langs, view.Tags, view.VisibleUntil = b.Username, b.Bio, b.Where, b.Cell, b.Langs, b.Tags, b.VisibleUntil
		view.InboxBox, view.InboxPub, view.InboxBits = b.InboxBox, b.InboxPub, b.InboxBits
		p.User = &view
	}
	if u := p.User; u != nil {
		if a.Home.Biz != nil && !findable {
			p.MyBiz, _ = a.Home.bizFor(r.Context(), u.ID)
		}
		p.Visible = u.Visible(time.Now())
		for _, l := range u.Langs {
			code, lvl, _ := strings.Cut(l, "/")
			name := langs.Names[code]
			if strings.HasPrefix(code, "x:") {
				name = strings.TrimPrefix(code, "x:")
			} else if name == "" {
				name = code
			}
			switch lvl { // the old two levels, read as CEFR
			case "fluent":
				lvl = "C1"
			case "learning":
				lvl = "A2"
			}
			p.MyLangs = append(p.MyLangs, myLang{code, name, lvl})
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
	case "paid":
		p.Error = locale.T(p.Lang, "biz.paid_feature")
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
	if b := a.Home.ActingAs(r); b != nil && a.Home.Biz != nil { // the business you act as becomes findable (76c)
		b.Bio, b.Where, b.Langs, b.Tags = account.CleanProfile(r.FormValue("bio"), r.FormValue("where"), ls, tags)
		b.Cell = strings.ToLower(strings.TrimSpace(r.FormValue("cell")))
		visible := min(time.Duration(hours)*time.Hour, account.MaxVisible)
		b.VisibleUntil = time.Time{}
		if visible > 0 && b.Username != "" && len(b.Cell) == 6 {
			b.VisibleUntil = time.Now().Add(visible)
		}
		if err := a.Home.Biz.Save(r.Context(), b); err != nil {
			http.Error(w, "could not save", http.StatusInternalServerError)
			return
		}
		a.Home.forgetBiz(b.ID)
		a.Home.Biz.ForgetFindable()
		http.Redirect(w, r, "/findable", http.StatusSeeOther)
		return
	}
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
	if b := a.Home.ActingAs(r); b != nil && a.Home.Biz != nil { // the business's inbox (76c-2)
		if box != "" && !account.ValidInbox(box, pub) {
			http.Error(w, "bad inbox", http.StatusBadRequest)
			return
		}
		if b.InboxBox != "" && b.InboxBox != box && a.Prices != nil {
			_ = a.Prices.Delete(r.Context(), b.InboxBox)
		}
		b.InboxBox, b.InboxPub, b.InboxBits = box, pub, 0
		if box != "" {
			b.InboxBits = max(account.MinInboxBits, min(account.MaxInboxBits, bits))
			if a.Prices != nil {
				if err := a.Prices.Set(r.Context(), box, b.InboxBits); err != nil {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
			}
		}
		if err := a.Home.Biz.Save(r.Context(), b); err != nil {
			http.Error(w, "could not save", http.StatusInternalServerError)
			return
		}
		a.Home.forgetBiz(b.ID)
		a.Home.Biz.ForgetFindable()
		w.WriteHeader(http.StatusNoContent)
		return
	}
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
	a.serveVault(w, r, u.ID)
}

// serveVault is GET/PUT of one encrypted vault: yours, or a business's
// ("biz:<id>", for its managers).
func (a *Accounts) serveVault(w http.ResponseWriter, r *http.Request, owner string) {
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
		v, err := a.Vault.Put(r.Context(), owner, want, body, time.Now())
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
	v, err := a.Vault.Get(r.Context(), owner)
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

// handleOwner: whose kafumu.com/@name link a request is about: the
// business you're acting as (named, and live: it's an office tool), or
// you. A business's link is owned as "biz:<id>" (LOOP-STATE 76b).
func (a *Accounts) handleOwner(r *http.Request) (name, owner string, status int) {
	u := UserFrom(r.Context())
	if u == nil || a.Handles == nil || IsBot(r) {
		return "", "", http.StatusUnauthorized
	}
	if b := a.Home.ActingAs(r); b != nil {
		if b.Username == "" {
			return "", "", http.StatusUnauthorized
		}
		if !b.Live(time.Now()) {
			return "", "", http.StatusPaymentRequired
		}
		return b.Username, "biz:" + b.ID, 0
	}
	if u.Username == "" {
		return "", "", http.StatusUnauthorized
	}
	return u.Username, u.ID, 0
}

// SetHandle handles PUT /api/handle {payload}: your (or the business's
// you act as) kafumu.com/@name link now leads to this connect code
// (renewed by the device; 90 days).
func (a *Accounts) SetHandle(w http.ResponseWriter, r *http.Request) {
	name, owner, status := a.handleOwner(r)
	if status != 0 {
		http.Error(w, "a named account is needed", status)
		return
	}
	var in struct {
		Payload string `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&in); err != nil || !handle.PayloadRE.MatchString(in.Payload) {
		http.Error(w, "want {payload: v1.…}", http.StatusBadRequest)
		return
	}
	if err := a.Handles.Set(r.Context(), name, owner, in.Payload, time.Now()); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": a.Home.Origin(r) + "/@" + name})
}

// FollowHandle serves /@name: to that person's connect code, marked as theirs.
func (a *Accounts) FollowHandle(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.ToLower(name)
	if a.Handles == nil || !account.ValidUsername(name) {
		http.NotFound(w, r)
		return
	}
	payload, ok := a.Handles.Get(r.Context(), name, time.Now())
	// A business's name: its public page, with "Connect" when it has a link.
	if id, err := a.Svc.Store.LookupUsername(r.Context(), name); err == nil && strings.HasPrefix(id, "biz:") && a.BizProfile != nil {
		if ok && !IsBot(r) {
			a.countView(r, name)
		}
		a.BizProfile(w, r, strings.TrimPrefix(id, "biz:"), payload)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	if !IsBot(r) {
		a.countView(r, name)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/c?from="+url.QueryEscape(name)+"#"+payload, http.StatusFound)
}

// DeleteHandle handles DELETE /api/handle: your kafumu.com/@name link stops working.
func (a *Accounts) DeleteHandle(w http.ResponseWriter, r *http.Request) {
	name, _, status := a.handleOwner(r)
	if status == http.StatusUnauthorized {
		http.Error(w, "a named account is needed", status)
		return
	}
	if name == "" { // a lapsed business may still switch its link off
		if b := a.Home.ActingAs(r); b != nil {
			name = b.Username
		}
	}
	if err := a.Handles.Delete(r.Context(), name); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Link views: how often kafumu.com/@name was opened today and when last;
// nothing about who. Kept only in the short-lived cache (a day).
type handleViews struct {
	Day  string `json:"day"`
	N    int    `json:"n"`
	Last int64  `json:"last"`
}

func (a *Accounts) countView(r *http.Request, name string) {
	if a.Cache == nil {
		return
	}
	key, now := "hv:"+name, time.Now()
	var v handleViews
	if b, ok := a.Cache.Get(r.Context(), key); ok {
		_ = json.Unmarshal(b, &v)
	}
	if day := now.UTC().Format("2006-01-02"); v.Day != day {
		v = handleViews{Day: day}
	}
	v.N++
	v.Last = now.Unix()
	b, _ := json.Marshal(v)
	a.Cache.Set(r.Context(), key, b, 24*time.Hour)
}

// HandleViews handles GET /api/handle: your link's views today and the last.
func (a *Accounts) HandleViews(w http.ResponseWriter, r *http.Request) {
	name, _, status := a.handleOwner(r)
	if status != 0 {
		http.Error(w, "a named account is needed", status)
		return
	}
	var v handleViews
	if a.Cache != nil {
		if b, ok := a.Cache.Get(r.Context(), "hv:"+name); ok {
			_ = json.Unmarshal(b, &v)
		}
	}
	if v.Day != time.Now().UTC().Format("2006-01-02") {
		v.N = 0
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
