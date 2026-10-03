package box

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const kind = "Box"

// entity is one box: all pending messages in a single noindex blob, so a
// poll is one Get. ExpiresAt is indexed for a Datastore TTL policy.
type entity struct {
	Msgs      []byte    `datastore:"msgs,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"`
}

// DatastoreStore keeps boxes in Datastore.
type DatastoreStore struct{ DB *datastore.Client }

func (s *DatastoreStore) List(ctx context.Context, id string) ([]Message, error) {
	var e entity
	if err := s.DB.Get(ctx, datastore.NameKey(kind, id, nil), &e); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return nil, nil
		}
		return nil, err
	}
	var ms []Message
	if err := json.Unmarshal(e.Msgs, &ms); err != nil {
		return nil, err
	}
	return live(ms, time.Now()), nil
}

func (s *DatastoreStore) Append(ctx context.Context, id string, m Message) error {
	return s.update(ctx, id, func(ms []Message) ([]Message, error) {
		if len(ms) >= MaxMessages {
			return nil, ErrFull
		}
		return append(ms, m), nil
	})
}

func (s *DatastoreStore) Ack(ctx context.Context, id string, ids []string) error {
	return s.update(ctx, id, func(ms []Message) ([]Message, error) { return remove(ms, ids), nil })
}

func (s *DatastoreStore) update(ctx context.Context, id string, fn func([]Message) ([]Message, error)) error {
	k := datastore.NameKey(kind, id, nil)
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var e entity
		var ms []Message
		if err := tx.Get(k, &e); err == nil {
			if err := json.Unmarshal(e.Msgs, &ms); err != nil {
				return err
			}
		} else if !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		ms, err := fn(live(ms, time.Now()))
		if err != nil {
			return err
		}
		if len(ms) == 0 {
			return tx.Delete(k)
		}
		raw, err := json.Marshal(ms)
		if err != nil {
			return err
		}
		_, err = tx.Put(k, &entity{Msgs: raw, ExpiresAt: time.Now().Add(TTL)})
		return err
	})
	return err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu    sync.Mutex
	boxes map[string][]Message
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{boxes: map[string][]Message{}} }

func (s *MemoryStore) List(_ context.Context, id string) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message(nil), live(s.boxes[id], time.Now())...), nil
}

func (s *MemoryStore) Append(_ context.Context, id string, m Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms := live(s.boxes[id], time.Now())
	if len(ms) >= MaxMessages {
		return ErrFull
	}
	s.boxes[id] = append(ms, m)
	return nil
}

func (s *MemoryStore) Ack(_ context.Context, id string, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ms := remove(s.boxes[id], ids); len(ms) > 0 {
		s.boxes[id] = ms
	} else {
		delete(s.boxes, id)
	}
	return nil
}

// CachedStore puts a cache in front of a Store. Polls of empty or unchanged
// boxes — almost all of them — are answered from the cache; every write
// drops the cached copy so the next read sees it.
type CachedStore struct {
	Store Store
	Cache interface {
		Get(ctx context.Context, key string) ([]byte, bool)
		Set(ctx context.Context, key string, val []byte, ttl time.Duration)
		Delete(ctx context.Context, key string)
	}
}

func (s *CachedStore) List(ctx context.Context, id string) ([]Message, error) {
	if b, ok := s.Cache.Get(ctx, "box:"+id); ok {
		var ms []Message
		if json.Unmarshal(b, &ms) == nil {
			return live(ms, time.Now()), nil
		}
	}
	ms, err := s.Store.List(ctx, id)
	if err != nil {
		return nil, err
	}
	if b, err := json.Marshal(ms); err == nil {
		s.Cache.Set(ctx, "box:"+id, b, time.Hour)
	}
	return ms, nil
}

func (s *CachedStore) Append(ctx context.Context, id string, m Message) error {
	err := s.Store.Append(ctx, id, m)
	s.Cache.Delete(ctx, "box:"+id)
	return err
}

func (s *CachedStore) Ack(ctx context.Context, id string, ids []string) error {
	err := s.Store.Ack(ctx, id, ids)
	s.Cache.Delete(ctx, "box:"+id)
	return err
}
