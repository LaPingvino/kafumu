package sqlstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/meetup"
)

// Meetups on SQLite through the real service: create, list, RSVP (in a
// transaction), the per-author limit, and survival across reopening.
func TestMeetupsOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, now := context.Background(), time.Now().UTC()
	s := meetup.NewService(&Meetups{DB: db})
	m := &meetup.Meetup{Title: "Esperanto coffee", Cell: "8ccgqw", StartAt: now.Add(6 * time.Hour), Tags: []string{"websummit"}}
	if err := s.Create(ctx, m, "u1", "joop"); err != nil {
		t.Fatal(err)
	}
	if going, got, err := s.Toggle(ctx, m.ID, "u2"); err != nil || !going || len(got.RSVPs) != 2 {
		t.Fatalf("Toggle = %v %+v %v", going, got, err)
	}
	if n, _ := (&Meetups{DB: db}).ActiveBy(ctx, "u1", now); n != 1 {
		t.Fatalf("ActiveBy = %d", n)
	}
	db.Close()

	db2, _ := Open(path)
	s2 := meetup.NewService(&Meetups{DB: db2})
	list, err := s2.InCells(ctx, []string{"8ccgqw", "8ccgqx"})
	if err != nil || len(list) != 1 || list[0].Going != 2 || list[0].AuthorID != "u1" || list[0].Tags[0] != "websummit" {
		t.Fatalf("after reopen: %+v, %v", list, err)
	}
	if err := s2.Delete(ctx, m.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Get(ctx, m.ID); !errors.Is(err, meetup.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
