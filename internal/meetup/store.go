package meetup

import (
	"context"
	"errors"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const kind = "Meetup"

// DatastoreStore keeps meetups in Datastore. Queries filter on cell and
// expires_at; the composite index lives in index.yaml.
type DatastoreStore struct{ DB *datastore.Client }

func (s *DatastoreStore) key(id string) *datastore.Key { return datastore.NameKey(kind, id, nil) }

func (s *DatastoreStore) Get(ctx context.Context, id string) (*Meetup, error) {
	var m Meetup
	if err := s.DB.Get(ctx, s.key(id), &m); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.ID = id
	return &m, nil
}

func (s *DatastoreStore) Put(ctx context.Context, m *Meetup) error {
	_, err := s.DB.Put(ctx, s.key(m.ID), m)
	return err
}

func (s *DatastoreStore) Delete(ctx context.Context, id string) error {
	return s.DB.Delete(ctx, s.key(id))
}

// InCells queries in chunks of 30 cells (Datastore's limit for "in"):
// areas widen to many cells where little happens.
func (s *DatastoreStore) InCells(ctx context.Context, cells []string, now time.Time) ([]*Meetup, error) {
	var out []*Meetup
	for i := 0; i < len(cells); i += 30 {
		part, err := s.inCells(ctx, cells[i:min(i+30, len(cells))], now)
		if err != nil {
			return out, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func (s *DatastoreStore) inCells(ctx context.Context, cells []string, now time.Time) ([]*Meetup, error) {
	vals := make([]any, len(cells))
	for i, c := range cells {
		vals[i] = c
	}
	q := datastore.NewQuery(kind).FilterField("cell", "in", vals).FilterField("expires_at", ">", now).Limit(300)
	var ms []*Meetup
	keys, err := s.DB.GetAll(ctx, q, &ms)
	if err != nil {
		return nil, err
	}
	for i, k := range keys {
		ms[i].ID = k.Name
	}
	return ms, nil
}

func (s *DatastoreStore) ActiveBy(ctx context.Context, authorID string, now time.Time) (int, error) {
	q := datastore.NewQuery(kind).FilterField("author_id", "=", authorID).FilterField("expires_at", ">", now).KeysOnly().Limit(MaxActive + 1)
	keys, err := s.DB.GetAll(ctx, q, nil)
	return len(keys), err
}

func (s *DatastoreStore) Update(ctx context.Context, id string, fn func(*Meetup) error) (*Meetup, error) {
	var out Meetup
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var m Meetup
		if err := tx.Get(s.key(id), &m); err != nil {
			if errors.Is(err, datastore.ErrNoSuchEntity) {
				return ErrNotFound
			}
			return err
		}
		if err := fn(&m); err != nil {
			return err
		}
		_, err := tx.Put(s.key(id), &m)
		out = m
		return err
	})
	out.ID = id
	return &out, err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu sync.Mutex
	ms map[string]Meetup
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{ms: map[string]Meetup{}} }

func (s *MemoryStore) Get(_ context.Context, id string) (*Meetup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.ms[id]
	if !ok {
		return nil, ErrNotFound
	}
	m.RSVPs = append([]string(nil), m.RSVPs...)
	return &m, nil
}

func (s *MemoryStore) Put(_ context.Context, m *Meetup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *m
	c.RSVPs = append([]string(nil), m.RSVPs...)
	s.ms[m.ID] = c
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ms, id)
	return nil
}

func (s *MemoryStore) InCells(_ context.Context, cells []string, now time.Time) ([]*Meetup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, c := range cells {
		want[c] = true
	}
	var out []*Meetup
	for _, m := range s.ms {
		if want[m.Cell] && m.ExpiresAt.After(now) {
			c := m
			out = append(out, &c)
		}
	}
	return out, nil
}

func (s *MemoryStore) ActiveBy(_ context.Context, authorID string, now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, m := range s.ms {
		if m.AuthorID == authorID && m.ExpiresAt.After(now) {
			n++
		}
	}
	return n, nil
}

func (s *MemoryStore) Update(ctx context.Context, id string, fn func(*Meetup) error) (*Meetup, error) {
	m, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := fn(m); err != nil {
		return nil, err
	}
	return m, s.Put(ctx, m)
}
