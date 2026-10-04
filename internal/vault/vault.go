// Package vault keeps one encrypted blob per account: your cards and
// contacts, synced between your own devices. The key never reaches the
// server (it is made on your first device and handed to the others through
// the account-bound move), so a vault means nothing without it: foundation
// rule 3. It is deleted with the account.
package vault

import (
	"context"
	"errors"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

// MaxBytes stays well under Datastore's 1 MB entity limit.
const MaxBytes = 900 << 10

const kind = "Vault"

var (
	ErrConflict = errors.New("vault: version conflict")
	ErrTooBig   = errors.New("vault: too big")
)

type Vault struct {
	Data      []byte    `datastore:"data,noindex"`
	Version   int       `datastore:"version,noindex"`
	UpdatedAt time.Time `datastore:"updated_at,noindex"`
}

type Store interface {
	Get(ctx context.Context, user string) (*Vault, error) // nil, nil if none
	// Put stores data if the current version is want, returning the new one.
	Put(ctx context.Context, user string, want int, data []byte, now time.Time) (int, error)
	Delete(ctx context.Context, user string) error
}

func Key(user string) *datastore.Key { return datastore.NameKey(kind, user, nil) }

type DatastoreStore struct{ DB *datastore.Client }

func (s *DatastoreStore) Get(ctx context.Context, user string) (*Vault, error) {
	var v Vault
	if err := s.DB.Get(ctx, Key(user), &v); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (s *DatastoreStore) Put(ctx context.Context, user string, want int, data []byte, now time.Time) (int, error) {
	if len(data) > MaxBytes {
		return 0, ErrTooBig
	}
	var next int
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var v Vault
		if err := tx.Get(Key(user), &v); err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		if v.Version != want {
			return ErrConflict
		}
		next = v.Version + 1
		_, err := tx.Put(Key(user), &Vault{Data: data, Version: next, UpdatedAt: now})
		return err
	})
	return next, err
}

func (s *DatastoreStore) Delete(ctx context.Context, user string) error {
	return s.DB.Delete(ctx, Key(user))
}

type MemoryStore struct {
	mu sync.Mutex
	m  map[string]Vault
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string]Vault{}} }

func (s *MemoryStore) Get(_ context.Context, user string) (*Vault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.m[user]; ok {
		return &v, nil
	}
	return nil, nil
}

func (s *MemoryStore) Put(_ context.Context, user string, want int, data []byte, now time.Time) (int, error) {
	if len(data) > MaxBytes {
		return 0, ErrTooBig
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[user].Version != want {
		return 0, ErrConflict
	}
	v := Vault{Data: data, Version: want + 1, UpdatedAt: now}
	s.m[user] = v
	return v.Version, nil
}

func (s *MemoryStore) Delete(_ context.Context, user string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, user)
	return nil
}
