package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/LaPingvino/kafumu/internal/cache"
)

// Passkeys: an optional, faster way back into your account. The magic link
// stays the recovery path. Bound to the canonical domain (RP ID), so they
// are switched on only once Kafumu serves from its final address.
type Passkeys struct {
	Accounts *Accounts
	WA       *webauthn.WebAuthn
	Cache    cache.Cache
}

// NewPasskeys returns nil when origin isn't a usable https origin.
func NewPasskeys(a *Accounts, origin string, c cache.Cache) *Passkeys {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return nil
	}
	origins := []string{origin}
	if u.Scheme == "https" {
		origins = append(origins, "https://www."+u.Hostname())
	}
	wa, err := webauthn.New(&webauthn.Config{RPDisplayName: a.Home.Cfg.Brand, RPID: u.Hostname(), RPOrigins: origins})
	if err != nil {
		log.Printf("passkeys: %v", err)
		return nil
	}
	return &Passkeys{Accounts: a, WA: wa, Cache: c}
}

const sessionTTL = 5 * time.Minute

func (p *Passkeys) keep(r *http.Request, key string, sd *webauthn.SessionData) {
	if b, err := json.Marshal(sd); err == nil {
		p.Cache.Set(r.Context(), "wa:"+key, b, sessionTTL)
	}
}

func (p *Passkeys) take(r *http.Request, key string) (*webauthn.SessionData, bool) {
	b, ok := p.Cache.Get(r.Context(), "wa:"+key)
	if !ok {
		return nil, false
	}
	p.Cache.Delete(r.Context(), "wa:"+key)
	var sd webauthn.SessionData
	return &sd, json.Unmarshal(b, &sd) == nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// RegisterBegin handles POST /auth/passkey/register/begin.
func (p *Passkeys) RegisterBegin(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	// Discoverable (resident) credentials, so "Sign in with a passkey" works
	// without typing a username.
	opts, sd, err := p.WA.BeginRegistration(u, webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.keep(r, "reg:"+u.ID, sd)
	writeJSON(w, opts)
}

// RegisterFinish handles POST /auth/passkey/register/finish.
func (p *Passkeys) RegisterFinish(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Error(w, "sign in first", http.StatusUnauthorized)
		return
	}
	sd, ok := p.take(r, "reg:"+u.ID)
	if !ok {
		http.Error(w, "session expired, try again", http.StatusBadRequest)
		return
	}
	cred, err := p.WA.FinishRegistration(u, *sd, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := p.Accounts.Svc.AddPasskey(r.Context(), u, *cred); err != nil {
		http.Error(w, "could not save the passkey", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

// LoginBegin handles POST /auth/passkey/login/begin (discoverable login).
func (p *Passkeys) LoginBegin(w http.ResponseWriter, r *http.Request) {
	opts, sd, err := p.WA.BeginDiscoverableLogin()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.keep(r, "login:"+sd.Challenge, sd)
	writeJSON(w, opts)
}

// LoginFinish handles POST /auth/passkey/login/finish.
func (p *Passkeys) LoginFinish(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var parsed struct {
		Response struct {
			ClientDataJSON string `json:"clientDataJSON"`
		} `json:"response"`
	}
	json.Unmarshal(body, &parsed)
	var cd struct {
		Challenge string `json:"challenge"`
	}
	if raw, err := base64.RawURLEncoding.DecodeString(parsed.Response.ClientDataJSON); err == nil {
		json.Unmarshal(raw, &cd)
	}
	sd, ok := p.take(r, "login:"+cd.Challenge)
	if !ok {
		http.Error(w, "session expired, try again", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var found string
	lookup := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := p.Accounts.Svc.ByID(r.Context(), string(userHandle))
		if err == nil {
			found = u.ID
		}
		return u, err
	}
	if _, err := p.WA.FinishDiscoverableLogin(lookup, *sd, r); err != nil || found == "" {
		http.Error(w, "that passkey didn't work", http.StatusUnauthorized)
		return
	}
	u, err := p.Accounts.Svc.ByID(r.Context(), found)
	if err != nil {
		http.Error(w, "account not found", http.StatusUnauthorized)
		return
	}
	cred, err := p.Accounts.Svc.NewSession(r.Context(), u)
	if err != nil {
		http.Error(w, "could not sign in", http.StatusInternalServerError)
		return
	}
	setCookie(w, cred)
	writeJSON(w, map[string]string{"status": "ok"})
}
