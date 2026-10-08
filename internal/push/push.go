// Package push wakes a phone when a signal lands in one of its mailboxes.
// The server learns which push endpoint watches which random mailbox ids —
// nothing else — and the notification carries no content: the app opens,
// reads the mailbox and decrypts on the device (VISION.md, signalling).
package push

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/kv"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	webpush "github.com/SherClockHolmes/webpush-go"
)

const (
	subKind   = "PushSub"
	maxBoxes  = 300
	subTTL    = 90 * 24 * time.Hour
	configKey = "vapid"
)

// boxRE: a mailbox id, or an OLN pair tag (chat lines, answers; 77f).
var boxRE = regexp.MustCompile(`^([0-9a-f]{64}|p[0-9a-f]{32})$`)

// Sub is one browser's push subscription and the inboxes it watches.
type Sub struct {
	Endpoint  string    `datastore:"endpoint,noindex" json:"endpoint"`
	Auth      string    `datastore:"auth,noindex" json:"-"`
	P256dh    string    `datastore:"p256dh,noindex" json:"-"`
	Boxes     []string  `datastore:"boxes" json:"-"`
	Lang      string    `datastore:"lang,noindex" json:"-"`
	ExpiresAt time.Time `datastore:"expires_at" json:"-"`
}

// Store keeps subscriptions and the server's VAPID keys.
type Store interface {
	Put(ctx context.Context, s *Sub) error
	Delete(ctx context.Context, endpoint string) error
	ForBox(ctx context.Context, box string) ([]*Sub, error)
	// Keys returns the VAPID key pair, creating it on first use.
	Keys(ctx context.Context) (priv, pub string, err error)
}

func subID(endpoint string) string {
	h := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(h[:])
}

// DatastoreStore keeps subscriptions in Datastore; the VAPID private key
// is generated on the server and stored there, never in the repository.
type DatastoreStore struct{ DB *datastore.Client }

type vapid struct {
	Private string `datastore:"private,noindex"`
	Public  string `datastore:"public,noindex"`
}

func (s *DatastoreStore) Put(ctx context.Context, sub *Sub) error {
	_, err := s.DB.Put(ctx, datastore.NameKey(subKind, subID(sub.Endpoint), nil), sub)
	return err
}

func (s *DatastoreStore) Delete(ctx context.Context, endpoint string) error {
	return s.DB.Delete(ctx, datastore.NameKey(subKind, subID(endpoint), nil))
}

func (s *DatastoreStore) ForBox(ctx context.Context, box string) ([]*Sub, error) {
	var subs []*Sub
	_, err := s.DB.GetAll(ctx, datastore.NewQuery(subKind).FilterField("boxes", "=", box).Limit(5), &subs)
	return subs, err
}

func (s *DatastoreStore) Keys(ctx context.Context) (string, string, error) {
	k := datastore.NameKey("Config", configKey, nil)
	var v vapid
	_, err := s.DB.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		if err := tx.Get(k, &v); err == nil {
			return nil
		} else if !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		priv, pub, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return err
		}
		v = vapid{priv, pub}
		_, err = tx.Put(k, &v)
		return err
	})
	return v.Private, v.Public, err
}

// MemoryStore is for local runs and tests.
type MemoryStore struct {
	mu        sync.Mutex
	subs      map[string]*Sub
	priv, pub string
}

// NewMemoryStore keeps subscriptions and the VAPID keys in memory;
// self-hosted (kv set) they persist: new keys on every restart would
// silently break every phone's subscription.
func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{subs: map[string]*Sub{}}
	kv.Load("PushSub", s.subs)
	keys := map[string]string{}
	kv.Load("PushKeys", keys)
	s.priv, s.pub = keys["priv"], keys["pub"]
	return s
}

func (s *MemoryStore) Put(_ context.Context, sub *Sub) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *sub
	s.subs[subID(sub.Endpoint)] = &c
	kv.Save("PushSub", subID(sub.Endpoint), &c)
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, endpoint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, subID(endpoint))
	kv.Delete("PushSub", subID(endpoint))
	return nil
}

func (s *MemoryStore) ForBox(_ context.Context, box string) ([]*Sub, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Sub
	for _, sub := range s.subs {
		for _, b := range sub.Boxes {
			if b == box {
				c := *sub
				out = append(out, &c)
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) Keys(context.Context) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.priv == "" {
		var err error
		if s.priv, s.pub, err = webpush.GenerateVAPIDKeys(); err != nil {
			return "", "", err
		}
		kv.Save("PushKeys", "priv", s.priv)
		kv.Save("PushKeys", "pub", s.pub)
	}
	return s.priv, s.pub, nil
}

// Service sends notifications and serves the subscription API.
type Service struct {
	Store   Store
	Contact string // VAPID subject: a URL or mailto: for push services to reach us
	Client  webpush.HTTPClient
	// Text returns the notification line in a language; it never names
	// anyone — the app decrypts and shows who when opened.
	Text func(lang string) string
	// MsgText: the line for a chat line or an answer (an OLN pair tag).
	MsgText func(lang string) string
}

// Notify wakes every device watching box. Gone subscriptions are removed.
func (s *Service) Notify(ctx context.Context, box string) {
	subs, err := s.Store.ForBox(ctx, box)
	if err != nil || len(subs) == 0 {
		return
	}
	priv, pub, err := s.Store.Keys(ctx)
	if err != nil {
		log.Printf("push: keys: %v", err)
		return
	}
	for _, sub := range subs {
		text, url := "☕", "/contacts"
		if s.Text != nil {
			text = s.Text(sub.Lang)
		}
		if strings.HasPrefix(box, "p") { // a chat line or an answer: Activity has it
			url = "/activity"
			if s.MsgText != nil {
				text = s.MsgText(sub.Lang)
			}
		}
		payload, _ := json.Marshal(map[string]string{"t": "signal", "text": text, "url": url})
		resp, err := webpush.SendNotificationWithContext(ctx, payload,
			&webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{Auth: sub.Auth, P256dh: sub.P256dh}},
			&webpush.Options{HTTPClient: s.Client, Subscriber: s.Contact, VAPIDPublicKey: pub, VAPIDPrivateKey: priv,
				TTL: 24 * 3600, Urgency: webpush.UrgencyHigh, Topic: "kafumu-signal"})
		if err != nil {
			log.Printf("push: send: %v", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound {
			_ = s.Store.Delete(ctx, sub.Endpoint)
		}
	}
}

// Key handles GET /api/push/key: the VAPID public key for subscribing.
func (s *Service) Key(w http.ResponseWriter, r *http.Request) {
	_, pub, err := s.Store.Keys(r.Context())
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"publicKey": pub})
}

// Subscribe handles POST /api/push/subscribe {subscription, boxes}. The
// boxes are the device's own inboxes; a later call replaces the list.
func (s *Service) Subscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Subscription webpush.Subscription `json:"subscription"`
		Boxes        []string             `json:"boxes"`
		Lang         string               `json:"lang"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil ||
		in.Subscription.Endpoint == "" || len(in.Subscription.Endpoint) > 1000 || len(in.Boxes) > maxBoxes {
		http.Error(w, "bad subscription", http.StatusBadRequest)
		return
	}
	var boxes []string
	for _, b := range in.Boxes {
		if boxRE.MatchString(b) {
			boxes = append(boxes, b)
		}
	}
	sub := &Sub{Endpoint: in.Subscription.Endpoint, Auth: in.Subscription.Keys.Auth, P256dh: in.Subscription.Keys.P256dh,
		Boxes: boxes, Lang: in.Lang, ExpiresAt: time.Now().Add(subTTL)}
	if len(sub.Lang) > 8 {
		sub.Lang = ""
	}
	if err := s.Store.Put(r.Context(), sub); err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unsubscribe handles POST /api/push/unsubscribe {endpoint}.
func (s *Service) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Endpoint string `json:"endpoint"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in) == nil && in.Endpoint != "" {
		_ = s.Store.Delete(r.Context(), in.Endpoint)
	}
	w.WriteHeader(http.StatusNoContent)
}
