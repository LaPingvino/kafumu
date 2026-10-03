// Package account is Kafumu's minimal account: a credential and whatever the
// user chooses to publish. Adapted from esperanto-kurso's auth, with three
// changes: accounts are created only on an explicit action (never on a page
// view), the cookie carries "id.token" so a login is one Get by key, and only
// a hash of the token is stored.
package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

// User is the whole server-side account.
type User struct {
	ID        string `datastore:"-"`
	TokenHash string `datastore:"token_hash,noindex"`
	// Sessions are extra token hashes from passkey sign-ins (newest last,
	// at most five), so signing in elsewhere never invalidates the link.
	Sessions  []string  `datastore:"sessions,noindex"`
	Username  string    `datastore:"username"`
	Role      string    `datastore:"role,noindex"` // "" or "admin"
	Lang      string    `datastore:"lang,noindex"`
	Passkeys  []byte    `datastore:"passkeys,noindex"`  // JSON []webauthn.Credential
	KeepDays  int       `datastore:"keep_days,noindex"` // 0 = default retention
	CreatedAt time.Time `datastore:"created_at,noindex"`
	// LastSeenAt drives inactivity cleanup; written at most hourly.
	LastSeenAt time.Time `datastore:"last_seen_at"`

	// Public profile: shown only while VisibleUntil is in the future, and
	// only in Cell's bundle. Discoverability is opt-in and time-boxed.
	Cell         string    `datastore:"cell"`
	VisibleUntil time.Time `datastore:"visible_until"`
	Bio          string    `datastore:"bio,noindex"`
	Where        string    `datastore:"where,noindex"` // "booth A23", "blue hat"
	Langs        []string  `datastore:"langs,noindex"` // "epo/native", "por/learning"
	Tags         []string  `datastore:"tags,noindex"`
}

// Visible reports whether u is discoverable at now.
func (u *User) Visible(now time.Time) bool {
	return u.Username != "" && u.Cell != "" && now.Before(u.VisibleUntil)
}

// Person is what others see of a discoverable user: no id, nothing linkable.
type Person struct {
	Name  string   `json:"name"`
	Bio   string   `json:"bio,omitempty"`
	Where string   `json:"where,omitempty"`
	Langs []string `json:"langs,omitempty"`
	Tags  []string `json:"tags,omitempty"`
	Cell  string   `json:"cell"`
}

// Public returns u's public view.
func (u *User) Public() Person {
	return Person{Name: u.Username, Bio: u.Bio, Where: u.Where, Langs: u.Langs, Tags: u.Tags, Cell: u.Cell}
}

// Named reports whether the user picked a username.
func (u *User) Named() bool { return u.Username != "" }

// ErrNotFound is returned for unknown or deleted users.
var ErrNotFound = errors.New("account: not found")

// ErrTaken is returned when a username is already in use.
var ErrTaken = errors.New("account: username taken")

// Store persists users. Datastore in production, memory locally and in tests.
type Store interface {
	Get(ctx context.Context, id string) (*User, error)
	Put(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
	// ClaimUsername reserves name for id, failing with ErrTaken.
	ClaimUsername(ctx context.Context, name, id string) error
	ReleaseUsername(ctx context.Context, name, id string) error
	// VisibleIn returns users visible in any of cells (≤ 30) at now.
	VisibleIn(ctx context.Context, cells []string, now time.Time) ([]*User, error)
}

// Service is the account logic on top of a Store, with a small per-instance
// cache so a logged-in page view usually costs no Datastore read.
type Service struct {
	Store Store
	TTL   time.Duration

	mu     sync.Mutex
	cache  map[string]cachedUser
	people map[string]peopleEntry
}

type cachedUser struct {
	u  User
	at time.Time
}

func NewService(s Store) *Service {
	return &Service{Store: s, TTL: time.Minute, cache: map[string]cachedUser{}}
}

// Create makes a new anonymous account and returns it with its cookie value
// ("id.token"). The token is shown to the user once, as the magic link.
func (s *Service) Create(ctx context.Context, lang string) (*User, string, error) {
	id, err := random(12)
	if err != nil {
		return nil, "", err
	}
	tok, err := random(24)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	u := &User{ID: id, TokenHash: hash(tok), Lang: lang, CreatedAt: now, LastSeenAt: now}
	if err := s.Store.Put(ctx, u); err != nil {
		return nil, "", err
	}
	s.remember(u)
	return u, id + "." + tok, nil
}

// Resolve returns the user for a cookie/magic-link value, or ErrNotFound.
func (s *Service) Resolve(ctx context.Context, cred string) (*User, error) {
	id, tok, ok := strings.Cut(cred, ".")
	if !ok || id == "" || tok == "" || len(cred) > 128 {
		return nil, ErrNotFound
	}
	u, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	h, ok := []byte(hash(tok)), subtle.ConstantTimeCompare([]byte(u.TokenHash), []byte(hash(tok))) == 1
	for _, s := range u.Sessions {
		ok = ok || subtle.ConstantTimeCompare([]byte(s), h) == 1
	}
	if !ok {
		return nil, ErrNotFound
	}
	if time.Since(u.LastSeenAt) > time.Hour {
		u.LastSeenAt = time.Now()
		_ = s.Save(ctx, u)
	}
	return u, nil
}

// Save writes u and refreshes the local cache.
func (s *Service) Save(ctx context.Context, u *User) error {
	if err := s.Store.Put(ctx, u); err != nil {
		return err
	}
	s.remember(u)
	return nil
}

// SetUsername claims name (3–30 of a-z 0-9 - _, case-insensitive) for u.
func (s *Service) SetUsername(ctx context.Context, u *User, name string) error {
	name = strings.ToLower(strings.TrimSpace(name))
	if !ValidUsername(name) {
		return errors.New("account: username must be 3–30 letters, digits, - or _")
	}
	if name == u.Username {
		return nil
	}
	if err := s.Store.ClaimUsername(ctx, name, u.ID); err != nil {
		return err
	}
	old := u.Username
	u.Username = name
	if err := s.Save(ctx, u); err != nil {
		return err
	}
	if old != "" {
		_ = s.Store.ReleaseUsername(ctx, old, u.ID)
	}
	return nil
}

// Delete removes the account and its username. Everything else Kafumu keeps
// about a user expires on its own.
func (s *Service) Delete(ctx context.Context, u *User) error {
	if u.Username != "" {
		_ = s.Store.ReleaseUsername(ctx, u.Username, u.ID)
	}
	s.mu.Lock()
	delete(s.cache, u.ID)
	s.mu.Unlock()
	return s.Store.Delete(ctx, u.ID)
}

// MaxVisible caps how long someone stays discoverable without renewing.
const MaxVisible = 7 * 24 * time.Hour

// SetProfile validates and saves u's public profile. visibleFor <= 0 hides it.
func (s *Service) SetProfile(ctx context.Context, u *User, cell, bio, where string, langs, tags []string, visibleFor time.Duration) error {
	u.Cell = strings.ToLower(strings.TrimSpace(cell))
	u.Bio = clip(strings.TrimSpace(bio), 160)
	u.Where = clip(strings.TrimSpace(where), 80)
	u.Langs = cleanList(langs, 12, func(l string) bool { return langRE.MatchString(l) })
	u.Tags = cleanList(tags, 12, func(t string) bool { return len(t) <= 40 })
	if visibleFor > MaxVisible {
		visibleFor = MaxVisible
	}
	u.VisibleUntil = time.Time{}
	if visibleFor > 0 && u.Username != "" && len(u.Cell) == 6 {
		u.VisibleUntil = time.Now().Add(visibleFor)
	}
	s.mu.Lock()
	s.people = map[string]peopleEntry{}
	s.mu.Unlock()
	return s.Save(ctx, u)
}

// People returns the discoverable people in cells, via a per-instance
// cache (one query per instance per minute for missing cells).
func (s *Service) People(ctx context.Context, cells []string) ([]Person, error) {
	now := time.Now()
	var out []Person
	var missing []string
	s.mu.Lock()
	if s.people == nil {
		s.people = map[string]peopleEntry{}
	}
	for _, c := range cells {
		if e, ok := s.people[c]; ok && now.Sub(e.at) < time.Minute {
			out = append(out, e.ps...)
		} else {
			missing = append(missing, c)
		}
	}
	s.mu.Unlock()
	if len(missing) > 0 {
		us, err := s.Store.VisibleIn(ctx, missing, now)
		if err != nil {
			return nil, err
		}
		by := map[string][]Person{}
		for _, u := range us {
			if u.Visible(now) {
				by[u.Cell] = append(by[u.Cell], u.Public())
			}
		}
		s.mu.Lock()
		for _, c := range missing {
			s.people[c] = peopleEntry{ps: by[c], at: now}
			out = append(out, by[c]...)
		}
		s.mu.Unlock()
	}
	return out, nil
}

type peopleEntry struct {
	ps []Person
	at time.Time
}

var langRE = regexp.MustCompile(`^[a-z]{3}/(native|fluent|learning)$`)

func cleanList(in []string, max int, ok func(string) bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range in {
		x = strings.ToLower(strings.TrimSpace(x))
		if x != "" && !seen[x] && ok(x) && len(out) < max {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// ValidUsername reports whether name is an acceptable username.
func ValidUsername(name string) bool {
	if len(name) < 3 || len(name) > 30 {
		return false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (s *Service) get(ctx context.Context, id string) (*User, error) {
	s.mu.Lock()
	if c, ok := s.cache[id]; ok && time.Since(c.at) < s.TTL {
		s.mu.Unlock()
		u := c.u
		return &u, nil
	}
	s.mu.Unlock()
	u, err := s.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.remember(u)
	return u, nil
}

func (s *Service) remember(u *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) > 10000 {
		s.cache = map[string]cachedUser{}
	}
	s.cache[u.ID] = cachedUser{u: *u, at: time.Now()}
}

func random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hash(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// WebAuthn (passkeys). The user handle is the account id, so a discoverable
// login finds the account without a username.
func (u *User) WebAuthnID() []byte          { return []byte(u.ID) }
func (u *User) WebAuthnName() string        { return u.label() }
func (u *User) WebAuthnDisplayName() string { return u.label() }
func (u *User) WebAuthnIcon() string        { return "" }
func (u *User) WebAuthnCredentials() []webauthn.Credential {
	var cs []webauthn.Credential
	if len(u.Passkeys) > 0 {
		_ = json.Unmarshal(u.Passkeys, &cs)
	}
	return cs
}

func (u *User) label() string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return "Kafumu " + u.ID[:6]
}

// AddPasskey stores a new credential on u.
func (s *Service) AddPasskey(ctx context.Context, u *User, c webauthn.Credential) error {
	cs := append(u.WebAuthnCredentials(), c)
	b, err := json.Marshal(cs)
	if err != nil {
		return err
	}
	u.Passkeys = b
	return s.Save(ctx, u)
}

// ByID returns a user by id (for passkey logins).
func (s *Service) ByID(ctx context.Context, id string) (*User, error) { return s.get(ctx, id) }

// NewSession returns a cookie value for u with a fresh session token, for
// passkey sign-ins (the link token is stored hashed, so it can't be reused).
func (s *Service) NewSession(ctx context.Context, u *User) (string, error) {
	tok, err := random(24)
	if err != nil {
		return "", err
	}
	u.Sessions = append(u.Sessions, hash(tok))
	if len(u.Sessions) > 5 {
		u.Sessions = u.Sessions[len(u.Sessions)-5:]
	}
	return u.ID + "." + tok, s.Save(ctx, u)
}
