package box

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Handler serves the mailbox API under /api/box/. Requests carry no
// cookies (clients use credentials: 'omit'), so the server cannot link a
// box to an account; unknown and empty boxes look the same.
type Handler struct {
	Store   Store
	limiter *limiter
}

// The per-IP limit is generous on purpose: at a conference thousands of
// people share a few NAT'd venue addresses. Memcache absorbs the reads;
// per-box caps (size, count) bound the writes.
func NewHandler(s Store) *Handler { return &Handler{Store: s, limiter: newLimiter(6000, time.Minute)} }

// Get handles GET /api/box/{id}: pending messages, oldest first.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.check(w, r)
	if !ok {
		return
	}
	ms, err := h.Store.List(r.Context(), id)
	if err != nil {
		log.Printf("box: list: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if ms == nil {
		ms = []Message{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]any{"messages": ms})
}

// Post handles POST /api/box/{id} with an opaque body (base64 ciphertext).
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	id, ok := h.check(w, r)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxMessage+1))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	m, err := NewMessage(strings.TrimSpace(string(body)))
	if err != nil || m.Data == "" {
		http.Error(w, "message empty or too big", http.StatusRequestEntityTooLarge)
		return
	}
	switch err := h.Store.Append(r.Context(), id, m); {
	case errors.Is(err, ErrFull):
		http.Error(w, "box full", http.StatusTooManyRequests)
	case err != nil:
		log.Printf("box: append: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// Ack handles POST /api/box/{id}/ack with a JSON list of message ids.
func (h *Handler) Ack(w http.ResponseWriter, r *http.Request) {
	id, ok := h.check(w, r)
	if !ok {
		return
	}
	var ids []string
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&ids); err != nil || len(ids) > MaxMessages {
		http.Error(w, "want a JSON list of message ids", http.StatusBadRequest)
		return
	}
	if err := h.Store.Ack(r.Context(), id, ids); err != nil {
		log.Printf("box: ack: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) check(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !ValidID(id) {
		http.Error(w, "bad box id", http.StatusBadRequest)
		return "", false
	}
	if !h.limiter.allow(clientIP(r)) {
		w.Header().Set("Retry-After", "30")
		http.Error(w, "slow down", http.StatusTooManyRequests)
		return "", false
	}
	return id, true
}

func clientIP(r *http.Request) string {
	// App Engine puts the real client first in X-Forwarded-For.
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

// limiter is a per-instance fixed-window counter per IP. Crude, but enough
// to stop one client from hammering Datastore through polling.
type limiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	start  time.Time
	counts map[string]int
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, start: time.Now(), counts: map[string]int{}}
}

func (l *limiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if time.Since(l.start) > l.window {
		l.start, l.counts = time.Now(), map[string]int{}
	}
	l.counts[ip]++
	return l.counts[ip] <= l.max
}
