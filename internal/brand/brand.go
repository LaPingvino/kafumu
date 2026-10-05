// Package brand: one Kafumu, several faces (LOOP-STATE 46). A brand is
// chosen by the host a request comes in on (bahais.in, localprayers.net):
// its own name, tagline, accent colour, wording of the main button and
// the tags Around starts from. Underneath it's the same network: #geo
// cells, local messages, connect codes. Brands are set up by the admin, as
// a paid service; forks (self-hosting + linked nodes) are the free route.
package brand

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/kv"
)

const kind = "Brand"

type Brand struct {
	Host    string   `datastore:"-" json:"host"`
	Name    string   `datastore:"name,noindex" json:"name"`
	Tagline string   `datastore:"tagline,noindex" json:"tagline,omitempty"`
	Accent  string   `datastore:"accent,noindex" json:"accent,omitempty"` // #rrggbb
	Button  string   `datastore:"button,noindex" json:"button,omitempty"` // the main button, e.g. "🙏 Who wants to pray with me?"
	Tags    []string `datastore:"tags,noindex" json:"tags,omitempty"`     // Around starts from the first
	// Admins (46b): user ids that manage this brand's wording and tags.
	Admins    []string  `datastore:"admins,noindex" json:"-"`
	PaidUntil time.Time `datastore:"paid_until,noindex" json:"-"`
	CreatedAt time.Time `datastore:"created_at,noindex" json:"-"`
}

var (
	ErrInvalid = errors.New("brand: invalid")
	hexColour  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	hostRE     = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
)

// CleanHost lowercases a host and drops the port and a leading "www.".
func CleanHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	if i := strings.LastIndex(h, ":"); i > 0 && !strings.Contains(h[i:], "]") {
		h = h[:i]
	}
	return strings.TrimPrefix(h, "www.")
}

// Clean checks a brand and tidies its fields.
func (b *Brand) Clean() error {
	b.Host = CleanHost(b.Host)
	b.Name, b.Tagline, b.Button = clip(b.Name, 40), clip(b.Tagline, 160), clip(b.Button, 60)
	if b.Accent != "" && !hexColour.MatchString(b.Accent) {
		b.Accent = ""
	}
	var tags []string
	for _, t := range b.Tags {
		t = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t != "" && len(t) <= 30 && !strings.ContainsAny(t, " /?&#") && len(tags) < 8 {
			tags = append(tags, t)
		}
	}
	b.Tags = tags
	if !hostRE.MatchString(b.Host) || b.Name == "" {
		return ErrInvalid
	}
	return nil
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// Store keeps brands in Datastore (or memory, persisted through kv when
// self-hosted). All brands are few: they're loaded together and kept a
// minute, so choosing the brand of a request costs nothing.
type Store struct {
	DB  *datastore.Client
	mu  sync.Mutex
	mem map[string]Brand
	all map[string]Brand
	at  time.Time
}

func New(db *datastore.Client) *Store {
	s := &Store{DB: db, mem: map[string]Brand{}}
	if db == nil {
		kv.Load(kind, s.mem)
	}
	return s
}

func (s *Store) load(ctx context.Context) map[string]Brand {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.all != nil && time.Since(s.at) < time.Minute {
		return s.all
	}
	m := map[string]Brand{}
	if s.DB == nil {
		for h, b := range s.mem {
			m[h] = b
		}
	} else {
		var bs []Brand
		keys, err := s.DB.GetAll(ctx, datastore.NewQuery(kind).Limit(200), &bs)
		if err != nil && s.all != nil {
			return s.all // keep the last good list
		}
		for i, k := range keys {
			bs[i].Host = k.Name
			m[k.Name] = bs[i]
		}
	}
	s.all, s.at = m, time.Now()
	return m
}

// For is the brand of a request's host, or nil (plain Kafumu).
func (s *Store) For(ctx context.Context, host string) *Brand {
	if s == nil {
		return nil
	}
	if b, ok := s.load(ctx)[CleanHost(host)]; ok {
		return &b
	}
	return nil
}

// All lists the brands, by host.
func (s *Store) All(ctx context.Context) []Brand {
	var out []Brand
	for _, b := range s.load(ctx) {
		out = append(out, b)
	}
	return out
}

func (s *Store) Save(ctx context.Context, b *Brand) error {
	if err := b.Clean(); err != nil {
		return err
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}
	if s.DB == nil {
		s.mu.Lock()
		s.mem[b.Host] = *b
		s.mu.Unlock()
		kv.Save(kind, b.Host, b)
	} else if _, err := s.DB.Put(ctx, datastore.NameKey(kind, b.Host, nil), b); err != nil {
		return err
	}
	s.forget()
	return nil
}

func (s *Store) Delete(ctx context.Context, host string) error {
	host = CleanHost(host)
	if s.DB == nil {
		s.mu.Lock()
		delete(s.mem, host)
		s.mu.Unlock()
		kv.Delete(kind, host)
	} else if err := s.DB.Delete(ctx, datastore.NameKey(kind, host, nil)); err != nil {
		return err
	}
	s.forget()
	return nil
}

func (s *Store) forget() {
	s.mu.Lock()
	s.all = nil
	s.mu.Unlock()
}
