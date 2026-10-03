// Package oln is Kafumu's node in Joop's Open Location Network
// (github.com/LaPingvino/eolnpoc): ephemeral public messages with no
// account, paid for with proof of work. The raw message is byte-compatible
// with eolnpoc —
//
//	<nonce>;<YYYYMMDDhhmmss>;<base64url(message)>;<keywords>
//
// — and is valid when its SHA-1 has enough leading zero bits. The time is
// UTC and must be within ±10 minutes of ours, so work can't be stockpiled;
// lifetime doubles with every bit above what the cell requires (VISION.md,
// "Local messages").
package oln

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math"
	"math/bits"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LaPingvino/kafumu/internal/geo"
)

const (
	BaseBits  = 14 // ≈ 0.4 s on a phone
	MaxBits   = 22
	BaseTTL   = time.Hour
	MaxTTL    = 7 * 24 * time.Hour // as in eolnpoc
	Window    = 10 * time.Minute
	MaxText   = 500
	MaxRaw    = 2000
	PerBundle = 50
	busyPer   = 30 // messages per hour per doubling of difficulty
)

var (
	ErrFormat = errors.New("oln: not an OLN message (nonce;YYYYMMDDhhmmss;base64;keywords)")
	ErrClock  = errors.New("oln: time outside the ±10 minute window")
	ErrPlace  = errors.New("oln: needs exactly one #geo cell in the keywords")
	ErrWork   = errors.New("oln: not enough proof of work")
)

// Note is one stored message.
type Note struct {
	ID        string    `datastore:"-" json:"id"`
	Raw       string    `datastore:"raw,noindex" json:"raw"`
	Text      string    `datastore:"text,noindex" json:"text"`
	Cell      string    `datastore:"cell" json:"cell"`
	Tags      []string  `datastore:"tags,noindex" json:"tags,omitempty"`
	Bits      int       `datastore:"bits,noindex" json:"bits"`
	At        time.Time `datastore:"at,noindex" json:"at"`
	ExpiresAt time.Time `datastore:"expires_at" json:"expires"`
}

// Bits counts the leading zero bits of SHA-1(raw).
func Bits(raw string) int {
	h := sha1.Sum([]byte(raw))
	n := 0
	for _, b := range h {
		if b == 0 {
			n += 8
			continue
		}
		return n + bits.LeadingZeros8(b)
	}
	return n
}

// ID is the hex SHA-1 of the raw message.
func ID(raw string) string {
	h := sha1.Sum([]byte(raw))
	return hex.EncodeToString(h[:])
}

// Parse checks the format and returns the note (without TTL) or an error.
func Parse(raw string, now time.Time) (*Note, error) {
	if len(raw) > MaxRaw {
		return nil, ErrFormat
	}
	parts := strings.SplitN(raw, ";", 4)
	if len(parts) != 4 || parts[0] == "" {
		return nil, ErrFormat
	}
	at, err := time.Parse("20060102150405", parts[1])
	if err != nil {
		return nil, ErrFormat
	}
	if d := now.Sub(at); d > Window || d < -Window {
		return nil, ErrClock
	}
	msg, err := base64.URLEncoding.DecodeString(parts[2])
	if err != nil {
		if msg, err = base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
			return nil, ErrFormat
		}
	}
	text := strings.TrimSpace(string(msg))
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxText {
		return nil, ErrFormat
	}
	var cells, tags []string
	seen := map[string]bool{}
	for _, f := range strings.Fields(strings.ToLower(parts[3])) {
		if !strings.HasPrefix(f, "#") || len(f) > 42 || seen[f] {
			continue
		}
		seen[f] = true
		t := f[1:]
		if strings.HasPrefix(t, "geo") && geo.Valid(t[3:]) {
			cells = append(cells, t[3:])
		}
		if len(tags) < 12 {
			tags = append(tags, t)
		}
	}
	if len(cells) != 1 {
		return nil, ErrPlace
	}
	return &Note{ID: ID(raw), Raw: raw, Text: text, Cell: cells[0], Tags: tags, Bits: Bits(raw), At: at}, nil
}

// TTL is the lifetime work buys: BaseTTL at the required bits, doubling per
// extra bit, capped at MaxTTL.
func TTL(bitsDone, required int) time.Duration {
	extra := bitsDone - required
	if extra < 0 {
		return 0
	}
	if extra > 20 {
		return MaxTTL
	}
	return min(BaseTTL<<extra, MaxTTL)
}

// Required is the difficulty for a cell given its messages in the last hour.
func Required(lastHour int) int {
	r := BaseBits + int(math.Floor(math.Log2(1+float64(lastHour)/busyPer)))
	return min(r, MaxBits)
}

// Priority ranks notes as eolnpoc does, minus hops: work plus remaining life.
func Priority(n *Note, now time.Time) float64 {
	life := n.ExpiresAt.Sub(n.At)
	left := 0.0
	if life > 0 {
		left = math.Max(0, float64(n.ExpiresAt.Sub(now))/float64(life))
	}
	return float64(n.Bits)*50 + left*100
}

// Store persists notes.
type Store interface {
	Get(ctx context.Context, id string) (*Note, error)
	Put(ctx context.Context, n *Note) error
	InCells(ctx context.Context, cells []string, now time.Time) ([]*Note, error)
	Hidden(ctx context.Context) (map[string]bool, error)
	Hide(ctx context.Context, id string) error
}

// Service accepts notes and serves them per cell through a short cache.
type Service struct {
	Store Store
	Now   func() time.Time

	mu     sync.Mutex
	cells  map[string]cellEntry
	hidden map[string]bool
	hidAt  time.Time
}

type cellEntry struct {
	ns []*Note
	at time.Time
}

func NewService(s Store) *Service {
	return &Service{Store: s, Now: time.Now, cells: map[string]cellEntry{}}
}

// RequiredFor is the difficulty in cell right now.
func (s *Service) RequiredFor(ctx context.Context, cell string) int {
	ns, err := s.InCells(ctx, []string{cell})
	if err != nil {
		return BaseBits
	}
	hourAgo, n := s.Now().Add(-time.Hour), 0
	for _, x := range ns {
		if x.At.After(hourAgo) {
			n++
		}
	}
	return Required(n)
}

// Post verifies and stores a raw message; an identical message is accepted
// again without being stored twice.
func (s *Service) Post(ctx context.Context, raw string) (*Note, error) {
	now := s.Now().UTC()
	n, err := Parse(strings.TrimSpace(raw), now)
	if err != nil {
		return nil, err
	}
	if old, err := s.Store.Get(ctx, n.ID); err == nil {
		return old, nil
	}
	req := s.RequiredFor(ctx, n.Cell)
	if n.Bits < req {
		return nil, ErrWork
	}
	n.ExpiresAt = n.At.Add(TTL(n.Bits, req))
	if err := s.Store.Put(ctx, n); err != nil {
		return nil, err
	}
	s.mu.Lock()
	delete(s.cells, n.Cell)
	s.mu.Unlock()
	return n, nil
}

// InCells returns live, unhidden notes in cells, highest priority first,
// from a one-minute per-instance cache.
func (s *Service) InCells(ctx context.Context, cells []string) ([]*Note, error) {
	now := s.Now()
	var missing []string
	var out []*Note
	s.mu.Lock()
	for _, c := range cells {
		if e, ok := s.cells[c]; ok && now.Sub(e.at) < time.Minute {
			out = append(out, e.ns...)
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
		by := map[string][]*Note{}
		for _, n := range got {
			by[n.Cell] = append(by[n.Cell], n)
		}
		s.mu.Lock()
		if len(s.cells) > 20000 {
			s.cells = map[string]cellEntry{}
		}
		for _, c := range missing {
			s.cells[c] = cellEntry{ns: by[c], at: now}
			out = append(out, by[c]...)
		}
		s.mu.Unlock()
	}
	hidden := s.hiddenSet(ctx)
	live := out[:0:0]
	for _, n := range out {
		if n.ExpiresAt.After(now) && !hidden[n.ID] {
			live = append(live, n)
		}
	}
	sort.Slice(live, func(i, j int) bool { return Priority(live[i], now) > Priority(live[j], now) })
	return live, nil
}

func (s *Service) hiddenSet(ctx context.Context) map[string]bool {
	s.mu.Lock()
	if s.hidden != nil && time.Since(s.hidAt) < 5*time.Minute {
		h := s.hidden
		s.mu.Unlock()
		return h
	}
	s.mu.Unlock()
	h, err := s.Store.Hidden(ctx)
	if err != nil {
		h = map[string]bool{}
	}
	s.mu.Lock()
	s.hidden, s.hidAt = h, time.Now()
	s.mu.Unlock()
	return h
}

// Hide removes a note from every list (admin).
func (s *Service) Hide(ctx context.Context, id string) error {
	if err := s.Store.Hide(ctx, id); err != nil {
		return err
	}
	s.mu.Lock()
	s.hidden = nil
	s.mu.Unlock()
	return nil
}
