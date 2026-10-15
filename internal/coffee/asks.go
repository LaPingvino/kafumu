package coffee

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

// LinkAsk is a request to link two groups by paying across (96n, Joop: "first
// person tries a cross-group payment, it starts linking"): the payer in
// FromGid asks the payee in ToGid; accepting links the groups and sends the
// payment. Expiring (a week), like every server entity (VISION rule 3).
type LinkAsk struct {
	ID        string    `datastore:"-"`
	FromGid   string    `datastore:"from_gid,noindex"`
	ToGid     string    `datastore:"to_gid,noindex"`
	FromUser  string    `datastore:"from_user,noindex"`
	ToUser    string    `datastore:"to_user,noindex"`
	Amount    int64     `datastore:"amount,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"`
}

// AskTTL is how long a link request waits for an answer.
const AskTTL = 7 * 24 * time.Hour

// ErrNoAsk: the request is gone or expired.
var ErrNoAsk = errors.New("coffeebucks: no such request")

// Asks stores link requests.
type Asks interface {
	Put(ctx context.Context, a *LinkAsk) error // sets a.ID when empty
	Get(ctx context.Context, id string) (*LinkAsk, error)
	Delete(ctx context.Context, id string) error
}

func newAskID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// DatastoreAsks keeps requests as LinkAsk entities (purged once expired).
type DatastoreAsks struct{ DB *datastore.Client }

func askKey(id string) *datastore.Key { return datastore.NameKey("LinkAsk", id, nil) }

func (s *DatastoreAsks) Put(ctx context.Context, a *LinkAsk) error {
	if a.ID == "" {
		a.ID = newAskID()
	}
	_, err := s.DB.Put(ctx, askKey(a.ID), a)
	return err
}

func (s *DatastoreAsks) Get(ctx context.Context, id string) (*LinkAsk, error) {
	var a LinkAsk
	if err := s.DB.Get(ctx, askKey(id), &a); err == datastore.ErrNoSuchEntity {
		return nil, ErrNoAsk
	} else if err != nil {
		return nil, err
	}
	if time.Now().After(a.ExpiresAt) {
		return nil, ErrNoAsk
	}
	a.ID = id
	return &a, nil
}

func (s *DatastoreAsks) Delete(ctx context.Context, id string) error {
	return s.DB.Delete(ctx, askKey(id))
}

// MemoryAsks is Asks in memory (tests, local runs).
type MemoryAsks struct {
	mu sync.Mutex
	m  map[string]LinkAsk
}

func (s *MemoryAsks) Put(_ context.Context, a *LinkAsk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m == nil {
		s.m = map[string]LinkAsk{}
	}
	if a.ID == "" {
		a.ID = newAskID()
	}
	s.m[a.ID] = *a
	return nil
}

func (s *MemoryAsks) Get(_ context.Context, id string) (*LinkAsk, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.m[id]
	if !ok || time.Now().After(a.ExpiresAt) {
		return nil, ErrNoAsk
	}
	return &a, nil
}

func (s *MemoryAsks) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	return nil
}
