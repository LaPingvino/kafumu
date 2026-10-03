// Package meetup holds meetups for people without an ATproto account: the
// fallback-records layer (VISION.md §4). Every meetup expires a day after it
// ends; lists are read per cell and cached, never queried per request.
package meetup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LaPingvino/kafumu/internal/geo"
)

const (
	MaxTitle    = 100
	MaxText     = 1000
	MaxTags     = 8
	MaxActive   = 10 // upcoming meetups per author
	MaxDuration = 3 * 24 * time.Hour
	Grace       = 24 * time.Hour // kept this long after the end
)

// Meetup is one gathering. RSVPs are account ids, never shown to anyone.
type Meetup struct {
	ID         string `datastore:"-" json:"id"`
	AuthorID   string `datastore:"author_id" json:"-"`
	AuthorName string `datastore:"author_name,noindex" json:"author,omitempty"`
	// Via names the public calendar an imported meetup came from.
	Via       string    `datastore:"via,noindex" json:"via,omitempty"`
	Title     string    `datastore:"title,noindex" json:"title"`
	Text      string    `datastore:"text,noindex" json:"text,omitempty"`
	StartAt   time.Time `datastore:"start_at,noindex" json:"start"`
	EndAt     time.Time `datastore:"end_at,noindex" json:"end"`
	Venue     string    `datastore:"venue,noindex" json:"venue,omitempty"`
	Link      string    `datastore:"link,noindex" json:"link,omitempty"`
	Cell      string    `datastore:"cell" json:"cell"`
	Tags      []string  `datastore:"tags,noindex" json:"tags,omitempty"`
	RSVPs     []string  `datastore:"rsvps,noindex" json:"-"`
	Going     int       `datastore:"-" json:"going"`
	CreatedAt time.Time `datastore:"created_at,noindex" json:"-"`
	// ExpiresAt drives the Datastore TTL policy and the live filter.
	ExpiresAt time.Time `datastore:"expires_at" json:"-"`
}

var (
	ErrNotFound = errors.New("meetup: not found")
	ErrInvalid  = errors.New("meetup: invalid")
	ErrTooMany  = errors.New("meetup: too many upcoming meetups")
)

// Store persists meetups.
type Store interface {
	Get(ctx context.Context, id string) (*Meetup, error)
	Put(ctx context.Context, m *Meetup) error
	// InCells returns unexpired meetups in any of cells (≤ 30).
	InCells(ctx context.Context, cells []string, now time.Time) ([]*Meetup, error)
	// ActiveBy counts unexpired meetups by an author.
	ActiveBy(ctx context.Context, authorID string, now time.Time) (int, error)
	// Update runs fn on the stored meetup in a transaction.
	Update(ctx context.Context, id string, fn func(*Meetup) error) (*Meetup, error)
	Delete(ctx context.Context, id string) error
}

// Validate cleans m and checks it; it sets defaults and ExpiresAt.
func (m *Meetup) Validate(now time.Time) error {
	m.Title = clip(strings.TrimSpace(m.Title), MaxTitle)
	m.Text = clip(strings.TrimSpace(m.Text), MaxText)
	m.Venue = clip(strings.TrimSpace(m.Venue), 200)
	m.Link = strings.TrimSpace(m.Link)
	m.Cell = strings.ToLower(m.Cell)
	if m.Title == "" || !geo.Valid(m.Cell) || m.StartAt.IsZero() {
		return ErrInvalid
	}
	if m.Link != "" && !(strings.HasPrefix(m.Link, "https://") || strings.HasPrefix(m.Link, "http://")) || len(m.Link) > 500 {
		return ErrInvalid
	}
	if m.EndAt.IsZero() || !m.EndAt.After(m.StartAt) {
		m.EndAt = m.StartAt.Add(2 * time.Hour)
	}
	if m.EndAt.Sub(m.StartAt) > MaxDuration || m.EndAt.Before(now) || m.StartAt.After(now.Add(365*24*time.Hour)) {
		return ErrInvalid
	}
	var tags []string
	seen := map[string]bool{}
	for _, t := range m.Tags {
		t = strings.ToLower(strings.Trim(strings.TrimSpace(t), "#"))
		if t != "" && len(t) <= 40 && !seen[t] && len(tags) < MaxTags {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	m.Tags = tags
	m.ExpiresAt = m.EndAt.Add(Grace)
	return nil
}

func clip(s string, n int) string {
	for len(s) > n || !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s[:min(len(s), n)], "")
	}
	return s
}

// NewID returns a random meetup id.
func NewID() string {
	b := make([]byte, 9)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Service caches meetups per cell for a short while: a busy cell costs one
// query per instance per TTL, however many people open the app.
type Service struct {
	Store Store
	TTL   time.Duration
	Now   func() time.Time

	mu    sync.Mutex
	cells map[string]cellEntry
}

type cellEntry struct {
	ms []*Meetup
	at time.Time
}

func NewService(s Store) *Service {
	return &Service{Store: s, TTL: time.Minute, Now: time.Now, cells: map[string]cellEntry{}}
}

// Create validates and stores a new meetup by author.
func (s *Service) Create(ctx context.Context, m *Meetup, authorID, authorName string) error {
	now := s.Now()
	if err := m.Validate(now); err != nil {
		return err
	}
	n, err := s.Store.ActiveBy(ctx, authorID, now)
	if err != nil {
		return err
	}
	if n >= MaxActive {
		return ErrTooMany
	}
	m.ID, m.AuthorID, m.AuthorName, m.CreatedAt = NewID(), authorID, authorName, now
	m.RSVPs = []string{authorID}
	if err := s.Store.Put(ctx, m); err != nil {
		return err
	}
	s.forget(m.Cell)
	return nil
}

// Toggle adds or removes userID's RSVP and reports whether they're going.
func (s *Service) Toggle(ctx context.Context, id, userID string) (bool, *Meetup, error) {
	going := false
	m, err := s.Store.Update(ctx, id, func(m *Meetup) error {
		for i, u := range m.RSVPs {
			if u == userID {
				m.RSVPs = append(m.RSVPs[:i], m.RSVPs[i+1:]...)
				return nil
			}
		}
		if len(m.RSVPs) >= 1000 {
			return ErrInvalid
		}
		m.RSVPs = append(m.RSVPs, userID)
		going = true
		return nil
	})
	if err == nil {
		s.forget(m.Cell)
	}
	return going, m, err
}

// Get returns one meetup with its RSVP count.
func (s *Service) Get(ctx context.Context, id string) (*Meetup, error) {
	m, err := s.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.Now().After(m.ExpiresAt) {
		return nil, ErrNotFound
	}
	m.Going = len(m.RSVPs)
	return m, nil
}

// InCells returns upcoming and running meetups in cells, soonest first,
// from the per-cell cache where possible.
func (s *Service) InCells(ctx context.Context, cells []string) ([]*Meetup, error) {
	now := s.Now()
	var missing []string
	var res []*Meetup
	s.mu.Lock()
	for _, c := range cells {
		if e, ok := s.cells[c]; ok && now.Sub(e.at) < s.TTL {
			res = append(res, e.ms...)
		} else {
			missing = append(missing, c)
		}
	}
	s.mu.Unlock()
	if len(missing) > 0 {
		got, err := s.Store.InCells(ctx, missing, now)
		if err != nil {
			return nil, err
		}
		by := map[string][]*Meetup{}
		for _, m := range got {
			m.Going = len(m.RSVPs)
			by[m.Cell] = append(by[m.Cell], m)
		}
		s.mu.Lock()
		if len(s.cells) > 20000 {
			s.cells = map[string]cellEntry{}
		}
		for _, c := range missing {
			s.cells[c] = cellEntry{ms: by[c], at: now}
			res = append(res, by[c]...)
		}
		s.mu.Unlock()
	}
	live := res[:0:0]
	for _, m := range res {
		if m.EndAt.After(now) {
			live = append(live, m)
		}
	}
	sort.Slice(live, func(i, j int) bool { return live[i].StartAt.Before(live[j].StartAt) })
	return live, nil
}

// Delete removes a meetup; only its author may.
func (s *Service) Delete(ctx context.Context, id, userID string) error {
	m, err := s.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if m.AuthorID != userID {
		return ErrNotFound
	}
	if err := s.Store.Delete(ctx, id); err != nil {
		return err
	}
	s.forget(m.Cell)
	return nil
}

// HasRSVP reports whether userID is going to m.
func (m *Meetup) HasRSVP(userID string) bool {
	for _, u := range m.RSVPs {
		if u == userID {
			return true
		}
	}
	return false
}

// ForgetAll drops the per-cell cache (after a feed sync).
func (s *Service) ForgetAll() {
	s.mu.Lock()
	s.cells = map[string]cellEntry{}
	s.mu.Unlock()
}

func (s *Service) forget(cell string) {
	s.mu.Lock()
	delete(s.cells, cell)
	s.mu.Unlock()
}
