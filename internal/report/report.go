// Package report lets anyone flag a card (local message, meetup, person,
// Bluesky post) for the moderators, paying a little proof of work instead
// of needing an account. Reports expire after 30 days. A moderator can
// hide what was reported: hidden items stay out of bundles for 90 days.
package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/pow"
)

const (
	TTL     = 30 * 24 * time.Hour
	HideTTL = 90 * 24 * time.Hour
	kind    = "Report"
	hidKind = "Hidden"
)

var (
	Kinds   = []string{"note", "meetup", "person", "post"}
	Reasons = []string{"spam", "scam", "harassment", "wrong-place", "other"}
)

type Report struct {
	Kind      string    `datastore:"kind" json:"kind"`
	Item      string    `datastore:"item" json:"item"`
	Reason    string    `datastore:"reason,noindex" json:"reason"`
	Snippet   string    `datastore:"snippet,noindex" json:"snippet"`
	Cell      string    `datastore:"cell,noindex" json:"cell"`
	At        time.Time `datastore:"at" json:"at"`
	ExpiresAt time.Time `datastore:"expires_at" json:"-"`
}

type hidden struct {
	ExpiresAt time.Time `datastore:"expires_at"`
}

// Item is one reported thing in the queue, with its reports.
type Item struct {
	Kind, Item, Snippet, Cell string
	Reasons                   map[string]int
	Count                     int
	Last                      time.Time
}

type Service struct {
	DB *datastore.Client

	mu      sync.Mutex
	mem     map[string]Report    // without Datastore
	hid     map[string]time.Time // kind|item → until
	hidAt   time.Time            // when hid was loaded
	memHide map[string]time.Time
}

func New(db *datastore.Client) *Service {
	return &Service{DB: db, mem: map[string]Report{}, memHide: map[string]time.Time{}}
}

func valid(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Handle is POST /api/report {kind, item, reason, snippet, cell}, stamped.
func (s *Service) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<10))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	stamp := r.Header.Get("X-Kafumu-Work")
	if err := pow.Check(stamp, body, "report", time.Now()); err != nil {
		http.Error(w, "work: "+err.Error(), http.StatusPaymentRequired)
		return
	}
	var rep Report
	if json.Unmarshal(body, &rep) != nil || !valid(Kinds, rep.Kind) || !valid(Reasons, rep.Reason) || rep.Item == "" || len(rep.Item) > 300 {
		http.Error(w, "want {kind, item, reason}", http.StatusBadRequest)
		return
	}
	if !geo.Valid(rep.Cell) {
		rep.Cell = ""
	}
	if len(rep.Snippet) > 200 {
		rep.Snippet = rep.Snippet[:200]
	}
	rep.At = time.Now()
	rep.ExpiresAt = rep.At.Add(TTL)
	// Keyed by the stamp: replaying the same request counts once.
	sum := sha256.Sum256([]byte(stamp + "|" + string(body)))
	if err := s.put(r.Context(), hex.EncodeToString(sum[:12]), rep); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) put(ctx context.Context, id string, rep Report) error {
	if s.DB == nil {
		s.mu.Lock()
		s.mem[id] = rep
		s.mu.Unlock()
		return nil
	}
	_, err := s.DB.Put(ctx, datastore.NameKey(kind, id, nil), &rep)
	return err
}

// Queue groups live reports by item, most reported (then newest) first.
func (s *Service) Queue(ctx context.Context, now time.Time) ([]Item, error) {
	var reps []Report
	if s.DB == nil {
		s.mu.Lock()
		for _, r := range s.mem {
			reps = append(reps, r)
		}
		s.mu.Unlock()
	} else {
		q := datastore.NewQuery(kind).FilterField("expires_at", ">", now).Limit(500)
		if _, err := s.DB.GetAll(ctx, q, &reps); err != nil {
			return nil, err
		}
	}
	by := map[string]*Item{}
	for _, r := range reps {
		if !r.ExpiresAt.After(now) || s.Hidden(ctx, r.Kind, r.Item) {
			continue
		}
		k := r.Kind + "|" + r.Item
		it := by[k]
		if it == nil {
			it = &Item{Kind: r.Kind, Item: r.Item, Reasons: map[string]int{}}
			by[k] = it
		}
		it.Count++
		it.Reasons[r.Reason]++
		if r.At.After(it.Last) {
			it.Last, it.Snippet, it.Cell = r.At, r.Snippet, r.Cell
		}
	}
	out := make([]Item, 0, len(by))
	for _, it := range by {
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Last.After(out[j].Last)
	})
	return out, nil
}

// Dismiss drops all reports about an item.
func (s *Service) Dismiss(ctx context.Context, kindName, item string) error {
	if s.DB == nil {
		s.mu.Lock()
		for id, r := range s.mem {
			if r.Kind == kindName && r.Item == item {
				delete(s.mem, id)
			}
		}
		s.mu.Unlock()
		return nil
	}
	q := datastore.NewQuery(kind).FilterField("item", "=", item).KeysOnly()
	keys, err := s.DB.GetAll(ctx, q, nil)
	if err != nil {
		return err
	}
	return s.DB.DeleteMulti(ctx, keys)
}

// Hide keeps an item out of bundles (posts, people) for HideTTL and clears
// its reports.
func (s *Service) Hide(ctx context.Context, kindName, item string) error {
	k := kindName + "|" + item
	until := time.Now().Add(HideTTL)
	if s.DB == nil {
		s.mu.Lock()
		s.memHide[k] = until
		s.hidAt = time.Time{}
		s.mu.Unlock()
	} else if _, err := s.DB.Put(ctx, datastore.NameKey(hidKind, hideKey(k), nil), &hidden{ExpiresAt: until}); err != nil {
		return err
	}
	s.mu.Lock()
	if s.hid != nil {
		s.hid[k] = until
	}
	s.mu.Unlock()
	return s.Dismiss(ctx, kindName, item)
}

func hideKey(k string) string { sum := sha256.Sum256([]byte(k)); return hex.EncodeToString(sum[:16]) }

// Hidden says whether a moderator hid this item. The hidden set is loaded
// at most once a minute per instance (few entities, one query).
func (s *Service) Hidden(ctx context.Context, kindName, item string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if s.hid == nil || now.Sub(s.hidAt) > time.Minute {
		s.hid = map[string]time.Time{}
		if s.DB == nil {
			for k, v := range s.memHide {
				s.hid[k] = v
			}
		}
		s.hidAt = now
		if s.DB != nil {
			s.mu.Unlock()
			got := loadHidden(ctx, s.DB, now)
			s.mu.Lock()
			for k, v := range got {
				s.hid[k] = v
			}
		}
	}
	until, ok := s.hid[kindName+"|"+item]
	if !ok {
		until, ok = s.hid[hideKey(kindName+"|"+item)]
	}
	return ok && until.After(now)
}

func loadHidden(ctx context.Context, db *datastore.Client, now time.Time) map[string]time.Time {
	var hs []hidden
	keys, err := db.GetAll(ctx, datastore.NewQuery(hidKind).FilterField("expires_at", ">", now).Limit(5000), &hs)
	out := map[string]time.Time{}
	if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
		return out
	}
	for i, k := range keys {
		out[k.Name] = hs[i].ExpiresAt
	}
	return out
}

// ShortKind turns a kind into a label for the queue.
func ShortKind(k string) string { return strings.ReplaceAll(k, "-", " ") }
