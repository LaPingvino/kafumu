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
	"fmt"
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
	BaseBits = 4 // 16 Argon2id attempts ≈ 1 s on a phone: cheap once, expensive in bulk
	MaxBits  = 14
	BaseTTL  = time.Hour
	MaxTTL   = 7 * 24 * time.Hour // as in eolnpoc
	Window   = 10 * time.Minute
	MaxText  = 500
	MaxRaw   = 4000 // a chat line is ciphertext inside base64: room for 500 characters
	// MaxPairText bounds a private message's text, which is ciphertext in
	// base64: 500 characters of any script (up to 4 bytes each) plus JSON,
	// IV and tag come to about 2.8 KB. MaxText is for readable text.
	MaxPairText = 3000
	PerBundle   = 50
	busyPer     = 30 // messages per hour per doubling of difficulty
	burstPer    = 5  // messages per 10 minutes per doubling: bursts get dear fast
)

var (
	ErrFormat = errors.New("oln: not an OLN v2 message (v2;nonce;YYYYMMDDhhmmss;base64;keywords)")
	ErrClock  = errors.New("oln: time outside the ±10 minute window")
	ErrPlace  = errors.New("oln: needs exactly one #geo cell in the keywords")
	ErrWork   = errors.New("oln: not enough proof of work")
	ErrRepeat = errors.New("oln: this message is already here")
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
	// Recv is when this node received it. The area's price counts arrivals,
	// not the claimed time: a sender can shift that by up to ten minutes
	// either way (and pre-mine for a future moment), not when it arrives.
	Recv time.Time `datastore:"recv,noindex" json:"-"`
	// Asks: a question's subjects (its tags minus #geo, #lang…, #re…, #ask),
	// indexed so people with those interests can find it from further away.
	Asks []string `datastore:"asks" json:"-"`
	// Re: what a reply or reaction answers (its #re tag without "re"),
	// indexed so the author can find replies from anywhere (77b).
	Re string `datastore:"re" json:"-"`
	// Pair: a private message's pair tag (indexed); empty for public ones.
	Pair string `datastore:"pair" json:"-"`
	// Author: the username that posted it, vouched for by this node (the
	// poster chose to show it); empty for anonymous messages.
	Author string `datastore:"author,noindex" json:"author,omitempty"`
	// By: the manager who posted as a business (Author stays empty then, so
	// the personal name stays out of it; kept for moderation only).
	By string `datastore:"by,noindex" json:"-"`
	// Biz: posted as a business account (by By, its manager); BizLive:
	// the business was in its trial or paid up then (coloured badge, else grey).
	Biz string `datastore:"biz,noindex" json:"biz,omitempty"`
	// Via: the node it was pulled from (a linked peer), "" when posted here.
	Via string `datastore:"via,noindex" json:"via,omitempty"`
	// Patron: posted under the name of a patron of Kafumu (gold wings),
	// as of posting.
	Patron  bool `datastore:"patron,noindex" json:"patron,omitempty"`
	BizLive bool `datastore:"biz_live,noindex" json:"biz_live,omitempty"`
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
func Parse(raw string, now time.Time) (*Note, error) { return parse(raw, now, false) }

// parse is Parse; relay (a line pulled from another node) drops the
// lower bound of the clock window, since relayed lines are older by
// nature, but keeps the upper one: a line dated ahead is still refused.
func parse(raw string, now time.Time, relay bool) (*Note, error) {
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
	if d := now.Sub(at); d < -Window || (!relay && d > Window) || d > MaxTTL {
		return nil, ErrClock
	}
	msg, err := base64.URLEncoding.DecodeString(parts[2])
	if err != nil {
		if msg, err = base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
			return nil, ErrFormat
		}
	}
	text := strings.TrimSpace(string(msg))
	if text == "" || !utf8.ValidString(text) || len(text) > MaxPairText {
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
	if utf8.RuneCountInString(text) > MaxText {
		return nil, ErrFormat // readable text: the long limit is for ciphertext only
	}
	// A reaction needs no place (Joop): "#re<id>" says what it's about, and
	// it's found from that thing wherever it is (77b). The API files such
	// lines under the cell Everywhere, which no area's list includes.
	if len(cells) == 0 && slices.ContainsFunc(tags, reTag.MatchString) {
		cells = []string{Everywhere}
	}
	if len(cells) != 1 {
		return nil, ErrPlace
	}
	n := &Note{ID: ID(raw), Raw: raw, Text: text, Cell: cells[0], Tags: tags, Bits: Bits(raw), At: at}
	for _, t := range tags {
		if reTag.MatchString(t) {
			n.Re = t[2:]
			break
		}
	}
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
	if n.Author != "" || n.Biz != "" {
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
	// RepliesTo returns live public replies and reactions to any of res
	// (#re ids, at most 30: one query).
	RepliesTo(ctx context.Context, res []string, now time.Time) ([]*Note, error)
	// ByPair returns live private messages under a pair tag.
	ByPair(ctx context.Context, tag string, now time.Time) ([]*Note, error)
}

var reTag = regexp.MustCompile(`^re[0-9a-f]{10}$`)

// Everywhere is the cell lines without a #geo are filed under (all zero, as
// a padded plustag: no place in particular). Not a valid area cell, so no
// bundle ever asks for it.
const Everywhere = "000000"

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

// MaxReIDs bounds one /api/oln/re request.
const MaxReIDs = 20

// Replies returns the live, unhidden replies and reactions to any of ids
// (#re ids), oldest first, each id's list cached a minute per instance.
func (s *Service) Replies(ctx context.Context, ids []string) ([]*Note, error) {
	now := s.Now()
	if len(ids) > MaxReIDs {
		ids = ids[:MaxReIDs]
	}
	// The ids not cached (or stale) are read together: one query.
	var stale []string
	s.mu.Lock()
	for _, id := range ids {
		if e, ok := s.res[id]; !ok || now.Sub(e.at) > time.Minute {
			stale = append(stale, id)
		}
	}
	s.mu.Unlock()
	if len(stale) > 0 {
		ns, err := s.Store.RepliesTo(ctx, stale, now)
		if err != nil {
			return nil, err
		}
		by := map[string][]*Note{}
		for _, n := range ns {
			by[n.Re] = append(by[n.Re], n)
		}
		s.mu.Lock()
		if s.res == nil || len(s.res) > 5000 {
			s.res = map[string]cellEntry{}
		}
		for _, id := range stale {
			s.res[id] = cellEntry{ns: by[id], at: now}
		}
		s.mu.Unlock()
	}
	seen, hidden := map[string]bool{}, s.hiddenSet(ctx)
	var out []*Note
	s.mu.Lock()
	for _, id := range ids {
		for _, n := range s.res[id].ns {
			if !seen[n.ID] && n.ExpiresAt.After(now) && !hidden[n.ID] {
				seen[n.ID] = true
				out = append(out, n)
			}
		}
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, nil
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
	// OnPair, if set, runs after a private message lands (chat line or
	// answer), with its pair tag, to wake whoever watches it (77f).
	OnPair func(ctx context.Context, tag string)
	Store  Store
	Now    func() time.Time

	mu     sync.Mutex
	cells  map[string]cellEntry
	hidden map[string]bool
	hidAt  time.Time
	asks   map[string]cellEntry
	res    map[string]cellEntry // replies per #re id (77b)
	// repeats: recently posted texts (normalised) and the cells they went
	// to, for this node's repeat policy (see repeatPrice).
	repeats map[string]map[string]time.Time

	// AuthorFor, if set, names the poster of a request who asked to post
	// under their name (signed in, named); "" otherwise.
	AuthorFor func(r *http.Request) string
	// BizFor: the business a named post is made as (its name, and whether
	// it's live); "" when posting as yourself.
	BizFor func(r *http.Request) (string, bool)
	// PatronFor: whether the request's user is a patron (wings on named posts).
	PatronFor func(r *http.Request) bool
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
		arrived := x.Recv
		if arrived.IsZero() {
			arrived = x.At // stored before Recv existed
		}
		if arrived.After(now.Add(-time.Hour)) {
			hour++
		}
		if arrived.After(now.Add(-10 * time.Minute)) {
			ten++
		}
	}
	return Required(hour, ten)
}

// requiredForRe is the difficulty of answering re right now: the same
// formula as an area, counting recent replies to that one thing.
func (s *Service) requiredForRe(ctx context.Context, re string) int {
	ns, err := s.Store.RepliesTo(ctx, []string{re}, s.Now())
	if err != nil {
		return BaseBits
	}
	now := s.Now()
	hour, ten := 0, 0
	for _, x := range ns {
		if x.Recv.After(now.Add(-time.Hour)) {
			hour++
		}
		if x.Recv.After(now.Add(-10 * time.Minute)) {
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
	n.Recv = now
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
	extra, err := s.repeatPrice(n, now)
	if err != nil {
		return nil, err
	}
	req := s.RequiredFor(ctx, n.Cell) + extra
	if n.Cell == Everywhere && n.Re != "" {
		req = s.requiredForRe(ctx, n.Re) + extra // priced by the thing it answers, not by the whole world
	}
	if n.Bits < req {
		return nil, &NeedError{Need: req}
	}
	n.Author = author
	if author != "" && s.BizFor != nil && ctxReq(ctx) != nil {
		n.Biz, n.BizLive = s.BizFor(ctxReq(ctx))
		if n.Biz != "" {
			n.By, n.Author = author, "" // the business speaks, not the person (Joop)
		}
	}
	if n.Author != "" && s.PatronFor != nil && ctxReq(ctx) != nil {
		n.Patron = s.PatronFor(ctxReq(ctx))
	}
	life := TTL(n.Bits, req)
	if author == "" {
		life /= 2
	}
	// Life runs from the claimed time, but a time in the future buys none.
	n.ExpiresAt = minTime(n.At, now).Add(life)
	if err := s.Store.Put(ctx, n); err != nil {
		return nil, err
	}
	s.rememberRepeat(n, now)
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
		if n.ExpiresAt.After(now) && !hidden[n.ID] && strings.HasPrefix(n.Raw, "v2;") { // v1 (SHA-1) leftovers carry no valid work
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

// NeedError is ErrWork with the work the message needed.
type NeedError struct{ Need int }

func (e *NeedError) Error() string {
	return fmt.Sprintf("oln: not enough proof of work (need %d bits)", e.Need)
}
func (e *NeedError) Is(target error) bool { return target == ErrWork }

// Repeat policy (this node's, not the format's): the same text posted again
// within a day is dropped in the same cell, and costs RepeatBits more per
// other cell it already went to. Reactions and very short texts are exempt
// (many people say "👍").
const (
	RepeatBits   = 4
	repeatWindow = 24 * time.Hour
	repeatMinLen = 12
)

func repeatKey(n *Note) string {
	if n.Pair != "" || len([]rune(n.Text)) < repeatMinLen || slices.ContainsFunc(n.Tags, reTag.MatchString) {
		return ""
	}
	norm := strings.Join(strings.Fields(strings.ToLower(n.Text)), " ")
	h := sha1.Sum([]byte(norm))
	return hex.EncodeToString(h[:12])
}

// repeatPrice returns the extra bits for n, or ErrRepeat when it's already
// in this cell.
func (s *Service) repeatPrice(n *Note, now time.Time) (int, error) {
	k := repeatKey(n)
	if k == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	others := 0
	for cell, at := range s.repeats[k] {
		if now.Sub(at) > repeatWindow {
			continue
		}
		if cell == n.Cell {
			return 0, ErrRepeat
		}
		others++
	}
	return others * RepeatBits, nil
}

func (s *Service) rememberRepeat(n *Note, now time.Time) {
	k := repeatKey(n)
	if k == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repeats == nil || len(s.repeats) > 20000 {
		s.repeats = map[string]map[string]time.Time{}
	}
	if s.repeats[k] == nil {
		s.repeats[k] = map[string]time.Time{}
	}
	s.repeats[k][n.Cell] = now
}

// ForgetAll drops this instance's caches (admin maintenance).
func (s *Service) ForgetAll() {
	s.mu.Lock()
	s.cells, s.asks, s.hidden = map[string]cellEntry{}, map[string]cellEntry{}, nil
	s.mu.Unlock()
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

type reqKey struct{}

// withReq / ctxReq carry the request to PostAs, for BizFor.
func withReq(r *http.Request) context.Context { return context.WithValue(r.Context(), reqKey{}, r) }
func ctxReq(ctx context.Context) *http.Request {
	r, _ := ctx.Value(reqKey{}).(*http.Request)
	return r
}

// ErrExpired: a relayed line whose life (by this node's rules) is over.
var ErrExpired = errors.New("oln: expired")

// Relay takes in a line pulled from a linked node (via). The work and the
// format are checked as for a post; the time only for not being ahead;
// the life is this node's own rule for anonymous lines of that work,
// counted from the line's time, so a relay never extends a life. Area
// prices and repeat rules don't apply: they price speaking here, and the
// line was spoken elsewhere.
func (s *Service) Relay(ctx context.Context, raw, via string) (*Note, bool, error) {
	now := s.Now().UTC()
	n, err := parse(strings.TrimSpace(raw), now, true)
	if err != nil {
		return nil, false, err
	}
	if old, err := s.Store.Get(ctx, n.ID); err == nil {
		return old, false, nil
	}
	if n.Bits < BaseBits {
		return nil, false, ErrWork
	}
	if n.Pair != "" {
		n.ExpiresAt = n.At.Add(PairTTL)
	} else {
		n.ExpiresAt = minTime(n.At, now).Add(TTL(n.Bits, BaseBits) / 2)
	}
	if !n.ExpiresAt.After(now) {
		return nil, false, ErrExpired
	}
	n.Recv, n.Via = now, via
	if err := s.Store.Put(ctx, n); err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	delete(s.cells, n.Cell)
	for _, t := range n.Asks {
		delete(s.asks, t)
	}
	s.mu.Unlock()
	return n, true, nil
}

// RecentCells: the areas people looked at on this instance in the last
// hour (the list cache), for pulling from linked nodes.
func (s *Service) RecentCells(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for c, e := range s.cells {
		if c != "" && now.Sub(e.at) < time.Hour {
			out = append(out, c)
		}
	}
	return out
}
