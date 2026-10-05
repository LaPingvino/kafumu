package sqlstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/box"
)

// Mailboxes on SQLite: append, list in order, ack, the per-box limit, the
// week's life, and survival across reopening; slots likewise.
func TestBoxesAndSlotsOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	b := &Boxes{DB: db}
	m1, _ := box.NewMessage("hello")
	m2, _ := box.NewMessage("card")
	m1.At, m2.At = time.Now().Add(-2*time.Second), time.Now().Add(-time.Second)
	old, _ := box.NewMessage("last week")
	old.At = time.Now().Add(-box.TTL - time.Hour)
	for _, m := range []box.Message{m1, m2, old} {
		if err := b.Append(ctx, "inbox1", m); err != nil {
			t.Fatal(err)
		}
	}
	ms, _ := b.List(ctx, "inbox1")
	if len(ms) != 2 || ms[0].Data != "hello" || ms[1].Data != "card" {
		t.Fatalf("list = %+v", ms)
	}
	if err := b.Ack(ctx, "inbox1", []string{m1.ID, "unknown"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < box.MaxMessages-1; i++ {
		m, _ := box.NewMessage(fmt.Sprint(i))
		if err := b.Append(ctx, "inbox1", m); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	extra, _ := box.NewMessage("one too many")
	if err := b.Append(ctx, "inbox1", extra); !errors.Is(err, box.ErrFull) {
		t.Fatalf("full box: %v", err)
	}
	s := &Slots{DB: db}
	if err := s.Put(ctx, "slot1", []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, _ := Open(path)
	ms, _ = (&Boxes{DB: db2}).List(ctx, "inbox1")
	if len(ms) != box.MaxMessages || ms[0].Data != "card" {
		t.Fatalf("after reopen: %d messages, first %q", len(ms), ms[0].Data)
	}
	if n, _ := (&Boxes{DB: db2}).PurgeBoxes(ctx, time.Now()); n != 1 {
		t.Fatalf("purged %d old messages, want 1", n)
	}
	if got, _ := (&Slots{DB: db2}).Get(ctx, "slot1"); len(got) != 2 || got[1] != "b" {
		t.Fatalf("slot = %v", got)
	}
	if got, _ := (&Slots{DB: db2}).Get(ctx, "none"); got != nil {
		t.Fatalf("missing slot = %v", got)
	}
}
