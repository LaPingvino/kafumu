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
	"errors"
	"strings"
	"sync"
	"time"
)

// User is the whole server-side account.
type User struct {
	ID        string    `datastore:"-"`
	TokenHash string    `datastore:"token_hash,noindex"`
	Username  string    `datastore:"username"`
	Role      string    `datastore:"role,noindex"` // "" or "admin"
	Lang      string    `datastore:"lang,noindex"`
	Passkeys  []byte    `datastore:"passkeys,noindex"`  // JSON []webauthn.Credential
	KeepDays  int       `datastore:"keep_days,noindex"` // 0 = default retention
	CreatedAt time.Time `datastore:"created_at,noindex"`
	// LastSeenAt drives inactivity cleanup; written at most hourly.
	LastSeenAt time.Time `datastore:"last_seen_at"`
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
}

// Service is the account logic on top of a Store, with a small per-instance
// cache so a logged-in page view usually costs no Datastore read.
type Service struct {
	Store Store
	TTL   time.Duration

	mu    sync.Mutex
	cache map[string]cachedUser
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
	if subtle.ConstantTimeCompare([]byte(u.TokenHash), []byte(hash(tok))) != 1 {
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
