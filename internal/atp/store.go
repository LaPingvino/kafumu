// Package atp connects a Kafumu account to the person's own ATproto
// account (Bluesky or any PDS) with OAuth, so posts, events and RSVPs are
// written to their PDS — public data that is theirs (VISION.md, tier 2).
package atp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/kv"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"github.com/bluesky-social/indigo/atproto/syntax"
)

const (
	sessionKind = "ATSession"
	requestKind = "ATAuthRequest"
	sessionTTL  = 90 * 24 * time.Hour
	requestTTL  = 15 * time.Minute
)

type blob struct {
	JSON      []byte    `datastore:"json,noindex"`
	ExpiresAt time.Time `datastore:"expires_at"` // the daily purge removes expired ones
}

// DatastoreStore implements oauth.ClientAuthStore. Sessions hold the
// tokens Kafumu needs to write to your PDS on your behalf; they're deleted
// when you disconnect, delete your account, or after 90 idle days.
type DatastoreStore struct{ DB *datastore.Client }

func sessKey(did syntax.DID, id string) *datastore.Key {
	return datastore.NameKey(sessionKind, did.String()+"|"+id, nil)
}

func (s *DatastoreStore) get(ctx context.Context, k *datastore.Key, v any) error {
	var b blob
	if err := s.DB.Get(ctx, k, &b); err != nil {
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return errors.New("atp: not found")
		}
		return err
	}
	if time.Now().After(b.ExpiresAt) {
		return errors.New("atp: expired")
	}
	return json.Unmarshal(b.JSON, v)
}

func (s *DatastoreStore) put(ctx context.Context, k *datastore.Key, v any, ttl time.Duration) error {
	j, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Put(ctx, k, &blob{JSON: j, ExpiresAt: time.Now().Add(ttl)})
	return err
}

func (s *DatastoreStore) GetSession(ctx context.Context, did syntax.DID, id string) (*oauth.ClientSessionData, error) {
	var d oauth.ClientSessionData
	return &d, s.get(ctx, sessKey(did, id), &d)
}

func (s *DatastoreStore) SaveSession(ctx context.Context, d oauth.ClientSessionData) error {
	return s.put(ctx, sessKey(d.AccountDID, d.SessionID), d, sessionTTL)
}

func (s *DatastoreStore) DeleteSession(ctx context.Context, did syntax.DID, id string) error {
	return s.DB.Delete(ctx, sessKey(did, id))
}

func (s *DatastoreStore) GetAuthRequestInfo(ctx context.Context, state string) (*oauth.AuthRequestData, error) {
	var d oauth.AuthRequestData
	return &d, s.get(ctx, datastore.NameKey(requestKind, state, nil), &d)
}

func (s *DatastoreStore) SaveAuthRequestInfo(ctx context.Context, d oauth.AuthRequestData) error {
	return s.put(ctx, datastore.NameKey(requestKind, d.State, nil), d, requestTTL)
}

func (s *DatastoreStore) DeleteAuthRequestInfo(ctx context.Context, state string) error {
	return s.DB.Delete(ctx, datastore.NameKey(requestKind, state, nil))
}

var _ oauth.ClientAuthStore = (*DatastoreStore)(nil)

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

// NewMemoryStore keeps sessions in memory; self-hosted (kv set) they also
// persist, so people stay connected to Bluesky across restarts.
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{m: map[string][]byte{}}
	kv.Load("ATSession", s.m)
	return s
}

func (s *MemoryStore) load(k string, v any) error {
	s.mu.Lock()
	b, ok := s.m[k]
	s.mu.Unlock()
	if !ok {
		return errors.New("atp: not found")
	}
	return json.Unmarshal(b, v)
}

func (s *MemoryStore) save(k string, v any) error {
	b, err := json.Marshal(v)
	s.mu.Lock()
	s.m[k] = b
	s.mu.Unlock()
	kv.Save("ATSession", k, b)
	return err
}

func (s *MemoryStore) drop(k string) error {
	s.mu.Lock()
	delete(s.m, k)
	s.mu.Unlock()
	kv.Delete("ATSession", k)
	return nil
}

func (s *MemoryStore) GetSession(_ context.Context, did syntax.DID, id string) (*oauth.ClientSessionData, error) {
	var d oauth.ClientSessionData
	return &d, s.load("s|"+did.String()+"|"+id, &d)
}
func (s *MemoryStore) SaveSession(_ context.Context, d oauth.ClientSessionData) error {
	return s.save("s|"+d.AccountDID.String()+"|"+d.SessionID, d)
}
func (s *MemoryStore) DeleteSession(_ context.Context, did syntax.DID, id string) error {
	return s.drop("s|" + did.String() + "|" + id)
}
func (s *MemoryStore) GetAuthRequestInfo(_ context.Context, state string) (*oauth.AuthRequestData, error) {
	var d oauth.AuthRequestData
	return &d, s.load("r|"+state, &d)
}
func (s *MemoryStore) SaveAuthRequestInfo(_ context.Context, d oauth.AuthRequestData) error {
	return s.save("r|"+d.State, d)
}
func (s *MemoryStore) DeleteAuthRequestInfo(_ context.Context, state string) error {
	return s.drop("r|" + state)
}
