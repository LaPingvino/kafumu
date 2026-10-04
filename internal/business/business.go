// Package business holds business accounts (Joop's paid model): a café,
// venue, organiser or company with personal accounts as its managers. The
// first month is free; after that Joop gets in touch from the admin panel
// to agree on a contract. Business accounts never buy visibility: what they
// get is to host meetups under their own name.
package business

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const (
	Trial = 30 * 24 * time.Hour
	kind  = "Business"
)

// Status values.
const (
	StatusTrial  = "trial"
	StatusActive = "active"
	StatusPaused = "paused"
	StatusEnded  = "ended"
)

var (
	Kinds       = []string{"cafe", "venue", "organiser", "company", "community"}
	ErrInvalid  = errors.New("business: name and kind needed")
	ErrNotFound = errors.New("business: not found")
)

type Business struct {
	ID        string    `datastore:"-" json:"id"`
	Name      string    `datastore:"name,noindex" json:"name"`
	Kind      string    `datastore:"kind,noindex" json:"kind"`
	Contact   string    `datastore:"contact,noindex" json:"contact,omitempty"`
	Managers  []string  `datastore:"managers" json:"-"` // user ids (indexed: "mine")
	CreatedBy string    `datastore:"created_by,noindex" json:"-"`
	CreatedAt time.Time `datastore:"created_at" json:"-"`
	TrialEnds time.Time `datastore:"trial_ends,noindex" json:"-"`
	Status    string    `datastore:"status,noindex" json:"status"`
	Note      string    `datastore:"note,noindex" json:"-"` // admin's own note
}

// Live: may host as the business (in its trial, or active).
func (b *Business) Live(now time.Time) bool {
	return b.Status == StatusActive || (b.Status == StatusTrial && now.Before(b.TrialEnds))
}

// TrialOver: the free month ended and nobody decided yet (admin follow-up).
func (b *Business) TrialOver(now time.Time) bool {
	return b.Status == StatusTrial && !now.Before(b.TrialEnds)
}

func (b *Business) Manages(userID string) bool { return slices.Contains(b.Managers, userID) }

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Store keeps business accounts in Datastore (or memory when DB is nil).
type Store struct {
	DB  *datastore.Client
	mu  sync.Mutex
	mem map[string]Business
}

func New(db *datastore.Client) *Store { return &Store{DB: db, mem: map[string]Business{}} }

func newID() string { b := make([]byte, 8); rand.Read(b); return hex.EncodeToString(b) }

// Create starts a business with its first manager and a free month.
func (s *Store) Create(ctx context.Context, name, kindName, contact, userID string, now time.Time) (*Business, error) {
	b := &Business{ID: newID(), Name: clip(name, 80), Kind: kindName, Contact: clip(contact, 200), Managers: []string{userID},
		CreatedBy: userID, CreatedAt: now, TrialEnds: now.Add(Trial), Status: StatusTrial}
	if b.Name == "" || !slices.Contains(Kinds, b.Kind) {
		return nil, ErrInvalid
	}
	return b, s.Save(ctx, b)
}

func (s *Store) Save(ctx context.Context, b *Business) error {
	if s.DB == nil {
		s.mu.Lock()
		s.mem[b.ID] = *b
		s.mu.Unlock()
		return nil
	}
	_, err := s.DB.Put(ctx, datastore.NameKey(kind, b.ID, nil), b)
	return err
}

func (s *Store) Get(ctx context.Context, id string) (*Business, error) {
	if s.DB == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		b, ok := s.mem[id]
		if !ok {
			return nil, ErrNotFound
		}
		return &b, nil
	}
	var b Business
	if err := s.DB.Get(ctx, datastore.NameKey(kind, id, nil), &b); err != nil {
		return nil, ErrNotFound
	}
	b.ID = id
	return &b, nil
}

// ForUser lists the businesses userID manages.
func (s *Store) ForUser(ctx context.Context, userID string) ([]*Business, error) {
	if s.DB == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		var out []*Business
		for _, b := range s.mem {
			if b.Manages(userID) {
				c := b
				out = append(out, &c)
			}
		}
		return out, nil
	}
	return s.query(ctx, datastore.NewQuery(kind).FilterField("managers", "=", userID).Limit(50))
}

// All lists business accounts for the admin, newest first.
func (s *Store) All(ctx context.Context) ([]*Business, error) {
	var out []*Business
	if s.DB == nil {
		s.mu.Lock()
		for _, b := range s.mem {
			c := b
			out = append(out, &c)
		}
		s.mu.Unlock()
	} else {
		var err error
		if out, err = s.query(ctx, datastore.NewQuery(kind).Limit(500)); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) query(ctx context.Context, q *datastore.Query) ([]*Business, error) {
	var bs []*Business
	keys, err := s.DB.GetAll(ctx, q, &bs)
	if err != nil {
		if _, ok := err.(*datastore.ErrFieldMismatch); !ok {
			return nil, err
		}
	}
	for i, k := range keys {
		bs[i].ID = k.Name
	}
	return bs, nil
}
