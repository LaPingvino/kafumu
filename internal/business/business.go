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
	"github.com/LaPingvino/kafumu/internal/kv"
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

	SyncServer  = "server"
	SyncPrivate = "private"

	AfterStop = "stop"
	AfterStay = "stay"
)

var (
	Kinds       = []string{"cafe", "venue", "organiser", "company", "community"}
	ErrInvalid  = errors.New("business: name and kind needed")
	ErrNotFound = errors.New("business: not found")
)

type Business struct {
	ID      string `datastore:"-" json:"id"`
	Name    string `datastore:"name,noindex" json:"name"`
	Kind    string `datastore:"kind,noindex" json:"kind"`
	Contact string `datastore:"contact,noindex" json:"contact,omitempty"`
	// Username: the business's own kafumu.com/@name, claimed in the same
	// registry as people's (as "biz:<id>"), so a name is one or the other.
	Username string `datastore:"username,noindex" json:"username,omitempty"`
	// SyncMode: how the business's card and contacts sync between its
	// managers' devices: "" (off: each device on its own), SyncServer (the
	// server holds the key, SyncKey) or SyncPrivate (the key only on
	// managers' devices). Managers choose.
	SyncMode string `datastore:"sync_mode,noindex" json:"-"`
	SyncKey  string `datastore:"sync_key,noindex" json:"-"`
	// KeyReqs (private mode): managers' devices asking for the key, and the
	// key wrapped for them by another manager's device. The server holds
	// public keys and ciphertext only.
	KeyReqs []KeyReq `datastore:"key_reqs,noindex" json:"-"`
	// Findable (76c): like a person's public profile, in one area for a
	// while. VisibleUntil is indexed: findable businesses are loaded
	// together (few), not queried per area.
	Cell         string    `datastore:"cell,noindex" json:"-"`
	VisibleUntil time.Time `datastore:"visible_until" json:"-"`
	Bio          string    `datastore:"bio,noindex" json:"-"`
	Where        string    `datastore:"where,noindex" json:"-"`
	Langs        []string  `datastore:"langs,noindex" json:"-"`
	Tags         []string  `datastore:"tags,noindex" json:"-"`
	// A public inbox (76c-2), like a person's: messages to the business,
	// read by its managers' devices (the key travels in the business vault).
	InboxBox  string    `datastore:"inbox_box,noindex" json:"-"`
	InboxPub  string    `datastore:"inbox_pub,noindex" json:"-"`
	InboxBits int       `datastore:"inbox_bits,noindex" json:"-"`
	Managers  []string  `datastore:"managers" json:"-"` // user ids (indexed: "mine")
	CreatedBy string    `datastore:"created_by,noindex" json:"-"`
	CreatedAt time.Time `datastore:"created_at" json:"-"`
	TrialEnds time.Time `datastore:"trial_ends,noindex" json:"-"`
	Status    string    `datastore:"status,noindex" json:"status"`
	// PaidUntil (active accounts): the end of what was paid for; AfterPaid
	// says what happens after it: AfterStop (no longer live, the default)
	// or AfterStay (stays live: invoiced later, a friend, a partner…).
	PaidUntil time.Time `datastore:"paid_until,noindex" json:"-"`
	AfterPaid string    `datastore:"after_paid,noindex" json:"-"`
	Note      string    `datastore:"note,noindex" json:"-"` // admin's own note
}

// KeyReq is one device's request for the business key (private mode).
type KeyReq struct {
	UserID  string    `datastore:"user_id,noindex"`
	Pub     string    `datastore:"pub,noindex"`     // the device's one-off ECDH public key (raw, base64)
	Wrapped string    `datastore:"wrapped,noindex"` // the business key, sealed to Pub by a manager's device
	From    string    `datastore:"from,noindex"`    // who sent it
	At      time.Time `datastore:"at,noindex"`
}

// Live: may host as the business (in its trial, or active).
func (b *Business) Live(now time.Time) bool {
	if b.Status == StatusActive {
		return b.PaidUntil.IsZero() || now.Before(b.PaidUntil) || b.AfterPaid == AfterStay
	}
	return b.Status == StatusTrial && now.Before(b.TrialEnds)
}

// PaidDay is the last day paid for (PaidUntil is the start of the day
// after it, in UTC; this keeps templates free of that arithmetic).
func (b *Business) PaidDay() time.Time { return b.PaidUntil.UTC().Add(-time.Hour) }

// PaidOver: active, but past the paid-until date (admin follow-up; still
// live only if AfterPaid is AfterStay).
func (b *Business) PaidOver(now time.Time) bool {
	return b.Status == StatusActive && !b.PaidUntil.IsZero() && !now.Before(b.PaidUntil)
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
	DB         *datastore.Client
	mu         sync.Mutex
	mem        map[string]Business
	findable   []*Business
	findableAt time.Time
}

func New(db *datastore.Client) *Store {
	s := &Store{DB: db, mem: map[string]Business{}}
	if db == nil {
		kv.Load(kind, s.mem) // self-hosted: kept across restarts
	}
	return s
}

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
		kv.Save(kind, b.ID, b)
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

// Findable lists the businesses findable at now: one query (few), kept a
// minute per instance; the bundle filters them by area.
func (s *Store) Findable(ctx context.Context, now time.Time) []*Business {
	s.mu.Lock()
	if s.findable != nil && now.Sub(s.findableAt) < time.Minute {
		out := s.findable
		s.mu.Unlock()
		return out
	}
	s.mu.Unlock()
	out := []*Business{}
	if s.DB == nil {
		s.mu.Lock()
		for _, b := range s.mem {
			if b.VisibleUntil.After(now) {
				c := b
				out = append(out, &c)
			}
		}
		s.mu.Unlock()
	} else {
		bs, err := s.query(ctx, datastore.NewQuery(kind).FilterField("visible_until", ">", now).Limit(500))
		if err != nil {
			return nil
		}
		out = bs
	}
	s.mu.Lock()
	s.findable, s.findableAt = out, now
	s.mu.Unlock()
	return out
}

// ForgetFindable drops the cached list (after a business changed it).
func (s *Store) ForgetFindable() {
	s.mu.Lock()
	s.findable = nil
	s.mu.Unlock()
}

// Delete removes a business (its last manager left).
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	delete(s.mem, id)
	s.findable = nil
	s.mu.Unlock()
	if s.DB == nil {
		kv.Delete(kind, id)
		return nil
	}
	return s.DB.Delete(ctx, datastore.NameKey(kind, id, nil))
}
