package oln

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const (
	noteKind   = "Note"
	hiddenKind = "HiddenNote"
)

var errNotFound = errors.New("oln: not found")

// DatastoreStore keeps notes in Datastore; the (cell, expires_at) index is
// in index.yaml, and the daily purge removes expired notes.
type DatastoreStore struct{ DB *datastore.Client }

func (s *DatastoreStore) Get(ctx context.Context, id string) (*Note, error) {
	var n Note
	if err := s.DB.Get(ctx, datastore.NameKey(noteKind, id, nil), &n); err != nil {
		return nil, errNotFound
	}
	n.ID = id
	return &n, nil
}

func (s *DatastoreStore) Put(ctx context.Context, n *Note) error {
	_, err := s.DB.Put(ctx, datastore.NameKey(noteKind, n.ID, nil), n)
	return err
}

// InCells queries in chunks of 30 cells (Datastore's limit for "in"):
// areas widen to many cells where little happens.
func (s *DatastoreStore) InCells(ctx context.Context, cells []string, now time.Time) ([]*Note, error) {
	var out []*Note
	for i := 0; i < len(cells); i += 30 {
		part, err := s.inCells(ctx, cells[i:min(i+30, len(cells))], now)
		if err != nil {
			return out, err
		}
		out = append(out, part...)
	}
	return out, nil
}

func (s *DatastoreStore) inCells(ctx context.Context, cells []string, now time.Time) ([]*Note, error) {
	vals := make([]any, len(cells))
	for i, c := range cells {
		vals[i] = c
	}
	q := datastore.NewQuery(noteKind).FilterField("cell", "in", vals).FilterField("expires_at", ">", now).Limit(500)
	var ns []*Note
	keys, err := s.DB.GetAll(ctx, q, &ns)
	for i, k := range keys {
		ns[i].ID = k.Name
	}
	return ns, err
}

func (s *DatastoreStore) Hidden(ctx context.Context) (map[string]bool, error) {
	keys, err := s.DB.GetAll(ctx, datastore.NewQuery(hiddenKind).KeysOnly().Limit(5000), nil)
	h := map[string]bool{}
	for _, k := range keys {
		h[k.Name] = true
	}
	return h, err
}

func (s *DatastoreStore) Hide(ctx context.Context, id string) error {
	_, err := s.DB.Put(ctx, datastore.NameKey(hiddenKind, id, nil), &struct {
		At time.Time `datastore:"at,noindex"`
	}{time.Now()})
	return err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu     sync.Mutex
	notes  map[string]Note
	hidden map[string]bool
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{notes: map[string]Note{}, hidden: map[string]bool{}}
}

func (s *MemoryStore) Get(_ context.Context, id string) (*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.notes[id]
	if !ok {
		return nil, errNotFound
	}
	return &n, nil
}

func (s *MemoryStore) Put(_ context.Context, n *Note) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes[n.ID] = *n
	return nil
}

func (s *MemoryStore) InCells(_ context.Context, cells []string, now time.Time) ([]*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, c := range cells {
		want[c] = true
	}
	var out []*Note
	for _, n := range s.notes {
		if want[n.Cell] && n.ExpiresAt.After(now) {
			c := n
			out = append(out, &c)
		}
	}
	return out, nil
}

func (s *MemoryStore) Hidden(context.Context) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := map[string]bool{}
	for k := range s.hidden {
		h[k] = true
	}
	return h, nil
}

func (s *MemoryStore) Hide(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hidden[id] = true
	return nil
}

func (s *DatastoreStore) AskedAbout(ctx context.Context, tag string, now time.Time) ([]*Note, error) {
	var ns []*Note
	keys, err := s.DB.GetAll(ctx, datastore.NewQuery(noteKind).FilterField("asks", "=", tag).Limit(50), &ns)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return nil, err
		}
	}
	out := ns[:0]
	for i, n := range ns {
		n.ID = keys[i].Name
		if n.ExpiresAt.After(now) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *MemoryStore) AskedAbout(_ context.Context, tag string, now time.Time) ([]*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Note
	for _, n := range s.notes {
		if n.ExpiresAt.After(now) && slices.Contains(n.Asks, tag) {
			c := n
			out = append(out, &c)
		}
	}
	return out, nil
}

func (s *DatastoreStore) ByPair(ctx context.Context, tag string, now time.Time) ([]*Note, error) {
	var ns []*Note
	keys, err := s.DB.GetAll(ctx, datastore.NewQuery(noteKind).FilterField("pair", "=", tag).Limit(200), &ns)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return nil, err
		}
	}
	out := ns[:0]
	for i, n := range ns {
		n.ID = keys[i].Name
		if n.ExpiresAt.After(now) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *MemoryStore) ByPair(_ context.Context, tag string, now time.Time) ([]*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Note
	for _, n := range s.notes {
		if n.Pair == tag && n.ExpiresAt.After(now) {
			c := n
			out = append(out, &c)
		}
	}
	return out, nil
}

func (s *DatastoreStore) RepliesTo(ctx context.Context, res []string, now time.Time) ([]*Note, error) {
	var ns []*Note
	keys, err := s.DB.GetAll(ctx, datastore.NewQuery(noteKind).FilterField("re", "in", res).Limit(300), &ns)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return nil, err
		}
	}
	out := ns[:0]
	for i, n := range ns {
		n.ID = keys[i].Name
		if n.ExpiresAt.After(now) {
			out = append(out, n)
		}
	}
	return out, nil
}

func (s *MemoryStore) RepliesTo(_ context.Context, res []string, now time.Time) ([]*Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Note
	for _, n := range s.notes {
		if n.Re != "" && slices.Contains(res, n.Re) && n.ExpiresAt.After(now) {
			c := n
			out = append(out, &c)
		}
	}
	return out, nil
}
