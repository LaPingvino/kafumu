package oln

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Two linked nodes: A pulls B's area. B's fresh line and an older one
// (relayed into B earlier, past the posting window) arrive at A marked as
// coming from B; nothing twice; an expired or future-dated line never.
func TestPullFromPeer(t *testing.T) {
	ctx, now := context.Background(), time.Now().UTC()
	b := NewService(NewMemoryStore())
	srv := httptest.NewServer(b.Export("http://b.example", "B"))
	defer srv.Close()
	if _, err := b.Post(ctx, mine(BaseBits, now, "Coffee at the market?", "#geo8ccgmw")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := b.Relay(ctx, mine(BaseBits+2, now.Add(-20*time.Minute), "Concert tonight", "#geo8ccgmw"), "http://c.example"); err != nil || !ok {
		t.Fatalf("relay into B: %v", err)
	}
	if _, _, err := b.Relay(ctx, mineExact(BaseBits, now.Add(-3*time.Hour), "Old news", "#geo8ccgmw"), "http://c.example"); err != ErrExpired {
		t.Fatalf("expired relay: %v", err)
	}
	if _, _, err := b.Relay(ctx, mine(BaseBits, now.Add(time.Hour), "From the future", "#geo8ccgmw"), "http://c.example"); err != ErrClock {
		t.Fatalf("future relay: %v", err)
	}

	a := NewService(NewMemoryStore())
	res := a.Pull(ctx, http.DefaultClient, srv.URL, []string{"8ccgmw"})
	if res.Err != "" || res.Seen != 2 || res.New != 2 {
		t.Fatalf("pull = %+v", res)
	}
	got, _ := a.InCells(ctx, []string{"8ccgmw"})
	if len(got) != 2 || got[0].Via != srv.URL {
		t.Fatalf("A has %+v", got)
	}
	if again := a.Pull(ctx, http.DefaultClient, srv.URL, []string{"8ccgmw"}); again.New != 0 {
		t.Fatalf("second pull took %d again", again.New)
	}
	if bad := a.Pull(ctx, http.DefaultClient, "ftp://nope", []string{"8ccgmw"}); bad.Err == "" {
		t.Fatal("bad peer address accepted")
	}
}
