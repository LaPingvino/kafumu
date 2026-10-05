package bsky

import (
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
