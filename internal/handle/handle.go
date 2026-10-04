// Package handle gives a named account a long-lived link: kafumu.com/@name
// leads to a working connect code for that person. The server keeps only
// the code's public half (not secret: it's what a QR shows) under the
// username; the private key stays on the owner's device. A handle lives
// 90 days and is renewed whenever its owner opens the app.
package handle

import (
	"context"
	"regexp"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const (
	TTL  = 90 * 24 * time.Hour
	kind = "Handle"
)

var PayloadRE = regexp.MustCompile(`^v1\.[A-Za-z0-9_-]{80,100}$`)

type entry struct {
	Payload   string    `datastore:"payload,noindex"`
	UserID    string    `datastore:"user_id,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"`
}

// Store keeps handles in Datastore, or memory when DB is nil.
type Store struct {
	DB  *datastore.Client
	mu  sync.Mutex
	mem map[string]entry
}

func New(db *datastore.Client) *Store { return &Store{DB: db, mem: map[string]entry{}} }

// Set points name at payload for userID, for TTL from now.
func (s *Store) Set(ctx context.Context, name, userID, payload string, now time.Time) error {
	e := entry{Payload: payload, UserID: userID, ExpiresAt: now.Add(TTL)}
	if s.DB == nil {
		s.mu.Lock()
		s.mem[name] = e
		s.mu.Unlock()
		return nil
	}
	_, err := s.DB.Put(ctx, datastore.NameKey(kind, name, nil), &e)
	return err
}

// Get returns the live payload for name, if any.
func (s *Store) Get(ctx context.Context, name string, now time.Time) (string, bool) {
	var e entry
	if s.DB == nil {
		s.mu.Lock()
		e = s.mem[name]
		s.mu.Unlock()
	} else if err := s.DB.Get(ctx, datastore.NameKey(kind, name, nil), &e); err != nil {
		return "", false
	}
	return e.Payload, e.Payload != "" && now.Before(e.ExpiresAt)
}

// Delete removes name (account deleted or renamed).
func (s *Store) Delete(ctx context.Context, name string) error {
	if s.DB == nil {
		s.mu.Lock()
		delete(s.mem, name)
		s.mu.Unlock()
		return nil
	}
	return s.DB.Delete(ctx, datastore.NameKey(kind, name, nil))
}
