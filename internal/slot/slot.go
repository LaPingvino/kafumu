// Package slot stores proximity beacons: for each pair and direction, one
// random-keyed slot holding the opaque tokens HMAC(pairKey, role, cell, day)
// for the (cell, day)s its owner checked in from during the last week. The
// friend fetches the slot and compares tokens on the device; the server
// never sees a cell, a day, or a match (VISION.md, friend proximity).
package slot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/pow"
)

const (
	TTL       = 8 * 24 * time.Hour
	MaxTokens = 64
	kind      = "Slot"
)

var (
	idRE    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	tokenRE = regexp.MustCompile(`^[0-9a-f]{32}$`)
)

// Store keeps slots.
type Store interface {
	Get(ctx context.Context, id string) ([]string, error)
	Put(ctx context.Context, id string, tokens []string) error
}

type entity struct {
	Tokens    []string  `datastore:"tokens,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"`
}

// DatastoreStore keeps slots in Datastore, one small entity each.
type DatastoreStore struct{ DB *datastore.Client }

func (s *DatastoreStore) Get(ctx context.Context, id string) ([]string, error) {
	var e entity
	err := s.DB.Get(ctx, datastore.NameKey(kind, id, nil), &e)
	if errors.Is(err, datastore.ErrNoSuchEntity) || err == nil && time.Now().After(e.ExpiresAt) {
		return nil, nil
	}
	return e.Tokens, err
}

func (s *DatastoreStore) Put(ctx context.Context, id string, tokens []string) error {
	_, err := s.DB.Put(ctx, datastore.NameKey(kind, id, nil), &entity{Tokens: tokens, ExpiresAt: time.Now().Add(TTL)})
	return err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string][]string
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string][]string{}} }

func (s *MemoryStore) Get(_ context.Context, id string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[id], nil
}

func (s *MemoryStore) Put(_ context.Context, id string, tokens []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[id] = tokens
	return nil
}

// Handler serves /api/slot/{id}: GET the tokens, PUT a JSON list to replace
// them. No cookies are sent (clients use credentials: 'omit').
type Handler struct {
	Store Store
	mu    sync.Mutex
	start time.Time
	count map[string]int
}

func NewHandler(s Store) *Handler {
	return &Handler{Store: s, start: time.Now(), count: map[string]int{}}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.check(w, r)
	if !ok {
		return
	}
	toks, err := h.Store.Get(r.Context(), id)
	if err != nil {
		log.Printf("slot: get: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if toks == nil {
		toks = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"tokens": toks})
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) {
	id, ok := h.check(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := pow.Check(r.Header.Get("X-Kafumu-Work"), body, "slot"+id, time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusPaymentRequired)
		return
	}
	var toks []string
	if err := json.Unmarshal(body, &toks); err != nil || len(toks) > MaxTokens {
		http.Error(w, "want a JSON list of at most 64 tokens", http.StatusBadRequest)
		return
	}
	for _, t := range toks {
		if !tokenRE.MatchString(t) {
			http.Error(w, "bad token", http.StatusBadRequest)
			return
		}
	}
	if err := h.Store.Put(r.Context(), id, toks); err != nil {
		log.Printf("slot: put: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) check(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !idRE.MatchString(id) {
		http.Error(w, "bad slot id", http.StatusBadRequest)
		return "", false
	}
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	h.mu.Lock()
	if time.Since(h.start) > time.Minute {
		h.start, h.count = time.Now(), map[string]int{}
	}
	h.count[ip]++
	over := h.count[ip] > 6000 // venues share a few NAT'd addresses; see box.NewHandler
	h.mu.Unlock()
	if over {
		http.Error(w, "slow down", http.StatusTooManyRequests)
		return "", false
	}
	return id, true
}

// CachedStore puts a cache in front of a Store; slot writes set the cached
// copy directly, so reads almost never reach Datastore.
type CachedStore struct {
	Store Store
	Cache interface {
		Get(ctx context.Context, key string) ([]byte, bool)
		Set(ctx context.Context, key string, val []byte, ttl time.Duration)
	}
}

func (s *CachedStore) Get(ctx context.Context, id string) ([]string, error) {
	if b, ok := s.Cache.Get(ctx, "slot:"+id); ok {
		var t []string
		if json.Unmarshal(b, &t) == nil {
			return t, nil
		}
	}
	t, err := s.Store.Get(ctx, id)
	if err == nil {
		if t == nil {
			t = []string{}
		}
		if b, err := json.Marshal(t); err == nil {
			s.Cache.Set(ctx, "slot:"+id, b, 6*time.Hour)
		}
	}
	return t, err
}

func (s *CachedStore) Put(ctx context.Context, id string, tokens []string) error {
	if err := s.Store.Put(ctx, id, tokens); err != nil {
		return err
	}
	if b, err := json.Marshal(tokens); err == nil {
		s.Cache.Set(ctx, "slot:"+id, b, 6*time.Hour)
	}
	return nil
}
