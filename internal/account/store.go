package account

import (
	"context"
	"errors"
	"sync"

	"cloud.google.com/go/datastore"
)

const (
	userKind     = "User"
	usernameKind = "Username"
)

// DatastoreStore keeps users in Datastore. Usernames are separate entities
// keyed by name, so uniqueness is a transactional Get+Put, not a query.
type DatastoreStore struct{ DB *datastore.Client }

type usernameEntity struct {
	UserID string `datastore:"user_id,noindex"`
}

func (s *DatastoreStore) Get(ctx context.Context, id string) (*User, error) {
	var u User
	if err := s.DB.Get(ctx, datastore.NameKey(userKind, id, nil), &u); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.ID = id
	return &u, nil
}

func (s *DatastoreStore) Put(ctx context.Context, u *User) error {
	_, err := s.DB.Put(ctx, datastore.NameKey(userKind, u.ID, nil), u)
	return err
}

func (s *DatastoreStore) Delete(ctx context.Context, id string) error {
	return s.DB.Delete(ctx, datastore.NameKey(userKind, id, nil))
}

func (s *DatastoreStore) ClaimUsername(ctx context.Context, name, id string) error {
	k := datastore.NameKey(usernameKind, name, nil)
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var e usernameEntity
		err := tx.Get(k, &e)
		if err == nil && e.UserID != id {
			return ErrTaken
		}
		if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		_, err = tx.Put(k, &usernameEntity{UserID: id})
		return err
	})
	return err
}

func (s *DatastoreStore) ReleaseUsername(ctx context.Context, name, id string) error {
	k := datastore.NameKey(usernameKind, name, nil)
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var e usernameEntity
		if err := tx.Get(k, &e); err != nil {
			return nil
		}
		if e.UserID != id {
			return nil
		}
		return tx.Delete(k)
	})
	return err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu    sync.Mutex
	users map[string]User
	names map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: map[string]User{}, names: map[string]string{}}
}

func (s *MemoryStore) Get(_ context.Context, id string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (s *MemoryStore) Put(_ context.Context, u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.users[u.ID] = *u
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return nil
}

func (s *MemoryStore) ClaimUsername(_ context.Context, name, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner, ok := s.names[name]; ok && owner != id {
		return ErrTaken
	}
	s.names[name] = id
	return nil
}

func (s *MemoryStore) ReleaseUsername(_ context.Context, name, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.names[name] == id {
		delete(s.names, name)
	}
	return nil
}
