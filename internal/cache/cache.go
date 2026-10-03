// Package cache is a small key-value cache: App Engine memcache in
// production (free, shared by all instances), an in-process map locally.
// It sits in front of Datastore for the hot, mostly-empty reads — mailbox
// polls and friend slots — that would otherwise eat the free read quota.
package cache

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"google.golang.org/appengine/v2"
	"google.golang.org/appengine/v2/memcache"
)

// Cache stores small values under string keys.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration)
	Delete(ctx context.Context, key string)
}

// OnAppEngine reports whether bundled services (memcache) are available.
func OnAppEngine() bool { return os.Getenv("GAE_ENV") == "standard" }

// New returns memcache on App Engine and a memory cache elsewhere.
func New() Cache {
	if OnAppEngine() {
		return memcacheCache{}
	}
	return NewMemory()
}

// Middleware gives each request an App Engine context, which memcache
// needs. Outside App Engine it does nothing.
func Middleware(next http.Handler) http.Handler {
	if !OnAppEngine() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(appengine.NewContext(r)))
	})
}

type memcacheCache struct{}

func (memcacheCache) Get(ctx context.Context, key string) ([]byte, bool) {
	it, err := memcache.Get(ctx, key)
	if err != nil {
		return nil, false
	}
	return it.Value, true
}

func (memcacheCache) Set(ctx context.Context, key string, val []byte, ttl time.Duration) {
	_ = memcache.Set(ctx, &memcache.Item{Key: key, Value: val, Expiration: ttl})
}

func (memcacheCache) Delete(ctx context.Context, key string) { _ = memcache.Delete(ctx, key) }

// Memory is an in-process cache for local runs and tests.
type Memory struct {
	mu sync.Mutex
	m  map[string]memItem
}

type memItem struct {
	v   []byte
	exp time.Time
}

func NewMemory() *Memory { return &Memory{m: map[string]memItem{}} }

func (c *Memory) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	it, ok := c.m[key]
	if !ok || time.Now().After(it.exp) {
		return nil, false
	}
	return it.v, true
}

func (c *Memory) Set(_ context.Context, key string, val []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) > 50000 {
		c.m = map[string]memItem{}
	}
	c.m[key] = memItem{v: append([]byte(nil), val...), exp: time.Now().Add(ttl)}
}

func (c *Memory) Delete(_ context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
}
