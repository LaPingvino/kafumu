package sqlstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/box"
	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/handle"
	"github.com/LaPingvino/kafumu/internal/kv"
	"github.com/LaPingvino/kafumu/internal/push"
	"github.com/LaPingvino/kafumu/internal/report"
)

// Stores without Datastore keep their data across a restart when kv has a
// SQLite persister: businesses (with the fields their JSON hides) and
// named links, and deletes stick.
func TestKVRestart(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	kv.Default = &KV{DB: db}
	defer func() { kv.Default = nil }()
	ctx, now := context.Background(), time.Now()

	bs := business.New(nil)
	b, err := bs.Create(ctx, "Café Teste", "cafe", "hi@example.com", "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	b.Username, b.Managers = "cafe-teste", append(b.Managers, "u2")
	if err := bs.Save(ctx, b); err != nil {
		t.Fatal(err)
	}
	hs := handle.New(nil)
	hs.Set(ctx, "joop", "u1", "v1.payload", now)
	hs.Set(ctx, "gone", "u3", "v1.other", now)
	hs.Delete(ctx, "gone")

	// "Restart": fresh stores load what was saved.
	got, err := business.New(nil).Get(ctx, b.ID)
	if err != nil || got.Username != "cafe-teste" || len(got.Managers) != 2 || got.CreatedBy != "u1" || !got.TrialEnds.Equal(b.TrialEnds) {
		t.Fatalf("business after restart: %+v, %v", got, err)
	}
	h2 := handle.New(nil)
	if p, ok := h2.Get(ctx, "joop", now); !ok || p != "v1.payload" {
		t.Fatalf("named link after restart: %q %v", p, ok)
	}
	if _, ok := h2.Get(ctx, "gone", now); ok {
		t.Fatal("deleted link came back")
	}

	// Push: the same server keys after a restart (new ones would break
	// every phone's subscription), and the subscriptions themselves.
	ps := push.NewMemoryStore()
	priv, pub, _ := ps.Keys(ctx)
	ps.Put(ctx, &push.Sub{Endpoint: "https://push.example/abc", Boxes: []string{"box1"}})
	ps2 := push.NewMemoryStore()
	if p2, q2, _ := ps2.Keys(ctx); p2 != priv || q2 != pub {
		t.Fatal("push keys changed across a restart")
	}
	if subs, _ := ps2.ForBox(ctx, "box1"); len(subs) != 1 {
		t.Fatalf("push subscriptions after restart: %d", len(subs))
	}
	// An inbox price, and a moderator's hide.
	box.NewPrices(nil).Set(ctx, "inbox9", 7)
	if got := box.NewPrices(nil).Price(ctx, "inbox9"); got != 7 {
		t.Fatalf("inbox price after restart = %d", got)
	}
	report.New(nil).Hide(ctx, "post", "at://x/y")
	if !report.New(nil).Hidden(ctx, "post", "at://x/y") {
		t.Fatal("hidden post visible again after a restart")
	}
}
