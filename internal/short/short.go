// Package short makes connect codes easy to say or type: kafumu.com/j/K7Q2PX
// leads to the full /c#v1.<public key> link for an hour. The server holds
// only the invite's public key, which is not secret (anyone shown the QR
// has it too).
package short

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/kv"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/pow"
)

const (
	alphabet = "23456789CFGHJMPQRVWX" // plus-code letters: no 0/O, 1/I mix-ups
	Length   = 6                      // 64 million codes
	TTL      = time.Hour
	kind     = "ShortCode"
)

var payloadRE = regexp.MustCompile(`^v1\.[A-Za-z0-9_-]{80,100}$`)

type entry struct {
	Payload   string    `datastore:"payload,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"`
}

// Service stores codes in Datastore (or memory when DB is nil).
type Service struct {
	DB  *datastore.Client
	mu  sync.Mutex
	mem map[string]entry
}

func New(db *datastore.Client) *Service {
	s := &Service{DB: db, mem: map[string]entry{}}
	if db == nil {
		kv.Load(kind, s.mem) // self-hosted: kept across restarts
	}
	return s
}

func newCode() string {
	b := make([]byte, Length)
	rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func (s *Service) put(ctx context.Context, code string, e entry) (bool, error) {
	if s.DB == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if old, ok := s.mem[code]; ok && time.Now().Before(old.ExpiresAt) {
			return false, nil
		}
		s.mem[code] = e
		kv.Save(kind, code, e)
		return true, nil
	}
	k := datastore.NameKey(kind, code, nil)
	ok := false
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var old entry
		if err := tx.Get(k, &old); err == nil && time.Now().Before(old.ExpiresAt) {
			return nil // taken
		}
		ok = true
		_, err := tx.Put(k, &e)
		return err
	})
	return ok, err
}

func (s *Service) get(ctx context.Context, code string) (string, bool) {
	var e entry
	if s.DB == nil {
		s.mu.Lock()
		e = s.mem[code]
		s.mu.Unlock()
	} else if err := s.DB.Get(ctx, datastore.NameKey(kind, code, nil), &e); err != nil {
		return "", false
	}
	return e.Payload, e.Payload != "" && time.Now().Before(e.ExpiresAt)
}

// Make handles POST /api/short with {"payload": "v1.…"} and a work stamp.
func (s *Service) Make(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var in struct {
		Payload string `json:"payload"`
	}
	if json.Unmarshal(body, &in) != nil || !payloadRE.MatchString(in.Payload) {
		http.Error(w, "want {payload: v1.…}", http.StatusBadRequest)
		return
	}
	if err := pow.Check(r.Header.Get("X-Kafumu-Work"), body, "short", time.Now()); err != nil {
		http.Error(w, err.Error(), http.StatusPaymentRequired)
		return
	}
	for i := 0; i < 5; i++ {
		code := newCode()
		ok, err := s.put(r.Context(), code, entry{Payload: in.Payload, ExpiresAt: time.Now().Add(TTL)})
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if ok {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"code": code})
			return
		}
	}
	http.Error(w, errors.New("no free code").Error(), http.StatusServiceUnavailable)
}

// Follow handles GET /j/{code}: on to the full connect link.
func (s *Service) Follow(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	p, ok := s.get(r.Context(), code)
	if !ok || len(code) != Length {
		http.Error(w, "this code has expired or doesn't exist — ask for a new one", http.StatusNotFound)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, "/c#"+p, http.StatusFound)
}
