package bsky

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Queued tags are searched once each in the background, then answered from
// the cache; duplicates and fresh tags aren't queued; the worker stops.
func TestTrickle(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`{"posts":[]}`))
	}))
	defer srv.Close()
	old := AppViews
	AppViews = []string{srv.URL}
	defer func() { AppViews = old }()

	c := NewClient()
	c.TrickleEvery = 10 * time.Millisecond
	for _, tag := range []string{"geo8ccgmw", "geo8ccgmx", "#geo8ccgmw"} {
		c.Later(tag, 25)
	}
	if n := c.QueueLen(); n != 2 {
		t.Fatalf("queued %d, want 2 (one duplicate)", n)
	}
	for end := time.Now().Add(3 * time.Second); c.QueueLen() > 0 && time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
	}
	time.Sleep(50 * time.Millisecond)
	if _, ok := c.Cached("geo8ccgmx", 25); !ok || hits.Load() != 2 {
		t.Fatalf("after the trickle: cached %v, upstream hits %d", ok, hits.Load())
	}
	c.Later("geo8ccgmw", 25) // fresh in the cache: not queued again
	if c.QueueLen() != 0 {
		t.Fatal("a fresh tag was queued")
	}
}

// A host that refuses is tried once; then the one that answered goes first.
func TestGoodHostFirst(t *testing.T) {
	var bad, good atomic.Int32
	no := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { bad.Add(1); w.WriteHeader(403) }))
	yes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { good.Add(1); w.Write([]byte(`{"posts":[]}`)) }))
	defer no.Close()
	defer yes.Close()
	old := AppViews
	AppViews = []string{no.URL, yes.URL}
	defer func() { AppViews = old }()
	c := NewClient()
	for _, tag := range []string{"a1", "a2", "a3"} {
		c.SearchTag(context.Background(), tag, 5)
	}
	if bad.Load() != 1 || good.Load() != 3 {
		t.Fatalf("refusing host asked %d times, good one %d", bad.Load(), good.Load())
	}
}
