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
	"golang.org/x/crypto/argon2"
	"math"
	"math/bits"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/LaPingvino/kafumu/internal/geo"
)

const (
	BaseBits  = 4 // 16 Argon2id attempts ≈ 1 s on a phone: cheap once, expensive in bulk
	MaxBits   = 14
	BaseTTL   = time.Hour
	MaxTTL    = 7 * 24 * time.Hour // as in eolnpoc
	Window    = 10 * time.Minute
	MaxText   = 500
	MaxRaw    = 4000 // a chat line is ciphertext inside base64: room for 500 characters
	PerBundle = 50
	busyPer   = 30 // messages per hour per doubling of difficulty
	burstPer  = 5  // messages per 10 minutes per doubling: bursts get dear fast
)

var (
	ErrFormat = errors.New("oln: not an OLN v2 message (v2;nonce;YYYYMMDDhhmmss;base64;keywords)")
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
	// Asks: a question's subjects (its tags minus #geo, #lang…, #re…, #ask),
	// indexed so people with those interests can find it from further away.
	Asks []string `datastore:"asks" json:"-"`
	// Pair: a private message's pair tag (indexed); empty for public ones.
	Pair string `datastore:"pair" json:"-"`
	// Author: the username that posted it, vouched for by this node (the
	// poster chose to show it); empty for anonymous messages.
	Author string `datastore:"author,noindex" json:"author,omitempty"`
}

// Proof of work, v2 (memory-hard, so a GPU gains little over a phone):
// the leading zero bits of Argon2id(line, salt "OLN-v2-proofwork",
// 1 pass, 4 MiB, 1 lane, 32 bytes). Only lines starting "v2;" count.
var workSalt = []byte("OLN-v2-proofwork")

const (
	workMemKiB = 4096
	workPasses = 1
)

// workSlots bounds concurrent Argon2id checks (4 MiB each) on a small instance.
var workSlots = make(chan struct{}, 4)

// Bits counts the leading zero bits of the line's v2 proof of work (0 for
// anything that isn't a v2 line).
func Bits(raw string) int {
	if !strings.HasPrefix(raw, "v2;") {
		return 0
	}
	workSlots <- struct{}{}
	h := argon2.IDKey([]byte(raw), workSalt, workPasses, workMemKiB, 1, 32)
	<-workSlots
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
	if !strings.HasPrefix(raw, "v2;") {
		return nil, ErrFormat
	}
	parts := strings.SplitN(strings.TrimPrefix(raw, "v2;"), ";", 4)
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
	// A private message (chat between two contacts) has no place: its only
	// keyword is the pair's tag, #p<32 hex>, and its text is ciphertext.
	if len(cells) == 0 && len(tags) == 1 && pairTag.MatchString(tags[0]) {
		return &Note{ID: ID(raw), Raw: raw, Text: text, Tags: tags, Pair: tags[0], Bits: Bits(raw), At: at}, nil
	}
	if len(cells) != 1 {
		return nil, ErrPlace
	}
	n := &Note{ID: ID(raw), Raw: raw, Text: text, Cell: cells[0], Tags: tags, Bits: Bits(raw), At: at}
	if slices.Contains(tags, "ask") {
		for _, t := range tags {
			if t != "ask" && !strings.HasPrefix(t, "geo") && !strings.HasPrefix(t, "lang") && !reTag.MatchString(t) {
				n.Asks = append(n.Asks, t)
			}
		}
	}
	return n, nil
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

// Required is the difficulty for a cell given its messages in the last hour
// and the last ten minutes: whichever is busier sets the price, so a burst
// (a bot batching messages) doubles the work every few messages.
func Required(lastHour, last10 int) int {
	hour := math.Log2(1 + float64(lastHour)/busyPer)
	burst := math.Log2(1 + float64(last10)/burstPer)
	return min(BaseBits+int(math.Floor(math.Max(hour, burst))), MaxBits)
}

// Priority ranks notes as eolnpoc does, minus hops: work plus remaining life.
func Priority(n *Note, now time.Time) float64 {
	life := n.ExpiresAt.Sub(n.At)
	left := 0.0
	if life > 0 {
		left = math.Max(0, float64(n.ExpiresAt.Sub(now))/float64(life))
	}
	p := float64(n.Bits)*50 + left*100
	if n.Author != "" {
		p += 150 // standing behind it with your name counts for about three bits
	}
	return p
}

// Store persists notes.
type Store interface {
	Get(ctx context.Context, id string) (*Note, error)
	Put(ctx context.Context, n *Note) error
	InCells(ctx context.Context, cells []string, now time.Time) ([]*Note, error)
	Hidden(ctx context.Context) (map[string]bool, error)
	Hide(ctx context.Context, id string) error
	// AskedAbout returns live questions with subject tag (any distance).
	AskedAbout(ctx context.Context, tag string, now time.Time) ([]*Note, error)
	// ByPair returns live private messages under a pair tag.
	ByPair(ctx context.Context, tag string, now time.Time) ([]*Note, error)
}

var reTag = regexp.MustCompile(`^re[0-9a-f]{10}$`)

var pairTag = regexp.MustCompile(`^p[0-9a-f]{32}$`)

// PairTTL: a private message waits a week for the other side, at base work
// (a phone shouldn't mine for half a minute per chat line).
const PairTTL = 7 * 24 * time.Hour

// ForPair returns the live private messages under a pair tag, oldest first.
func (s *Service) ForPair(ctx context.Context, tag string) ([]*Note, error) {
	if !pairTag.MatchString(tag) {
		return nil, ErrFormat
	}
	ns, err := s.Store.ByPair(ctx, tag, s.Now())
	if err != nil {
		return nil, err
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].At.Before(ns[j].At) })
	return ns, nil
}

// MaxAskTags bounds one /api/asks request.
const MaxAskTags = 8

// Asks returns live, unhidden questions about any of tags, newest first,
// each tag's list cached a minute per instance.
func (s *Service) Asks(ctx context.Context, tags []string) ([]*Note, error) {
	now := s.Now()
	seen, hidden := map[string]bool{}, s.hiddenSet(ctx)
	var out []*Note
	for i, t := range tags {
		if i == MaxAskTags {
			break
		}
		s.mu.Lock()
		e, ok := s.asks[t]
		s.mu.Unlock()
		if !ok || now.Sub(e.at) > time.Minute {
			ns, err := s.Store.AskedAbout(ctx, t, now)
			if err != nil {
				return nil, err
			}
			e = cellEntry{ns: ns, at: now}
			s.mu.Lock()
			if s.asks == nil || len(s.asks) > 5000 {
				s.asks = map[string]cellEntry{}
			}
			s.asks[t] = e
			s.mu.Unlock()
		}
		for _, n := range e.ns {
			if !seen[n.ID] && n.ExpiresAt.After(now) && !hidden[n.ID] {
				seen[n.ID] = true
				out = append(out, n)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}

// Service accepts notes and serves them per cell through a short cache.
type Service struct {
	Store Store
	Now   func() time.Time

	mu     sync.Mutex
	cells  map[string]cellEntry
	hidden map[string]bool
	hidAt  time.Time
	asks   map[string]cellEntry

	// AuthorFor, if set, names the poster of a request who asked to post
	// under their name (signed in, named); "" otherwise.
	AuthorFor func(r *http.Request) string
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
	now := s.Now()
	hour, ten := 0, 0
	for _, x := range ns {
		if x.At.After(now.Add(-time.Hour)) {
			hour++
		}
		if x.At.After(now.Add(-10 * time.Minute)) {
			ten++
		}
	}
	return Required(hour, ten)
}

// Post verifies and stores a raw message; an identical message is accepted
// again without being stored twice.
func (s *Service) Post(ctx context.Context, raw string) (*Note, error) { return s.PostAs(ctx, raw, "") }

// PostAs is Post with an author this node vouches for ("" = anonymous).
// Anonymous public messages live half as long (Joop).
func (s *Service) PostAs(ctx context.Context, raw, author string) (*Note, error) {
	now := s.Now().UTC()
	n, err := Parse(strings.TrimSpace(raw), now)
	if err != nil {
		return nil, err
	}
	if old, err := s.Store.Get(ctx, n.ID); err == nil {
		return old, nil
	}
	if n.Pair != "" {
		if n.Bits < BaseBits {
			return nil, ErrWork
		}
		n.ExpiresAt = n.At.Add(PairTTL)
		return n, s.Store.Put(ctx, n)
	}
	req := s.RequiredFor(ctx, n.Cell)
	if n.Bits < req {
		return nil, ErrWork
	}
	n.Author = author
	life := TTL(n.Bits, req)
	if author == "" {
		life /= 2
	}
	n.ExpiresAt = n.At.Add(life)
	if err := s.Store.Put(ctx, n); err != nil {
		return nil, err
	}
	s.mu.Lock()
	delete(s.cells, n.Cell)
	for _, t := range n.Asks {
		delete(s.asks, t)
	}
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
