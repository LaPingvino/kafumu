package meetup

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCreateListRSVP(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	s := NewService(NewMemoryStore())
	s.Now = func() time.Time { return now }

	m := &Meetup{Title: "  Esperanto coffee  ", Cell: "8CCGQW", StartAt: now.Add(6 * time.Hour), Tags: []string{"#WebSummit", "websummit", "lang:epo"}}
	if err := s.Create(ctx, m, "u1", "joop"); err != nil {
		t.Fatal(err)
	}
	if m.Title != "Esperanto coffee" || m.Cell != "8ccgqw" || len(m.Tags) != 2 || !m.EndAt.Equal(m.StartAt.Add(2*time.Hour)) {
		t.Errorf("cleaned = %+v", m)
	}
	for _, bad := range []*Meetup{
		{Title: "", Cell: "8ccgqw", StartAt: now},
		{Title: "x", Cell: "nope", StartAt: now},
		{Title: "x", Cell: "8ccgqw", StartAt: now.Add(-5 * time.Hour)},
		{Title: "x", Cell: "8ccgqw", StartAt: now, Link: "javascript:alert(1)"},
	} {
		if err := s.Create(ctx, bad, "u1", "joop"); !errors.Is(err, ErrInvalid) {
			t.Errorf("Create(%+v) err = %v", bad, err)
		}
	}

	list, err := s.InCells(ctx, []string{"8ccgqw", "8ccgqx"})
	if err != nil || len(list) != 1 || list[0].Going != 1 {
		t.Fatalf("InCells = %v, %v", list, err)
	}
	going, got, err := s.Toggle(ctx, m.ID, "u2")
	if err != nil || !going || len(got.RSVPs) != 2 {
		t.Fatalf("Toggle = %v %v %v", going, got, err)
	}
	if list, _ = s.InCells(ctx, []string{"8ccgqw"}); list[0].Going != 2 {
		t.Errorf("cache not refreshed after RSVP: %d", list[0].Going)
	}
	if going, _, _ = s.Toggle(ctx, m.ID, "u2"); going {
		t.Error("second toggle should un-RSVP")
	}

	if err := s.Delete(ctx, m.ID, "u2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("non-author delete: %v", err)
	}

	// Over: gone from lists once it ended, gone entirely after the grace day.
	now = m.EndAt.Add(time.Minute)
	s.forget("8ccgqw")
	if list, _ = s.InCells(ctx, []string{"8ccgqw"}); len(list) != 0 {
		t.Errorf("ended meetup still listed")
	}
	now = m.ExpiresAt.Add(time.Minute)
	if _, err := s.Get(ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired meetup still readable: %v", err)
	}
}

func TestActiveLimit(t *testing.T) {
	ctx := context.Background()
	s := NewService(NewMemoryStore())
	for i := 0; i < MaxActive; i++ {
		if err := s.Create(ctx, &Meetup{Title: "x", Cell: "8ccgqw", StartAt: time.Now().Add(time.Hour)}, "u", "u"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Create(ctx, &Meetup{Title: "x", Cell: "8ccgqw", StartAt: time.Now().Add(time.Hour)}, "u", "u"); !errors.Is(err, ErrTooMany) {
		t.Errorf("11th meetup: %v", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	s := NewService(NewMemoryStore())
	m := &Meetup{Title: "x", Cell: "8ccgqw", StartAt: time.Now().Add(time.Hour)}
	s.Create(ctx, m, "u", "u")
	if err := s.Delete(ctx, m.ID, "u"); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.InCells(ctx, []string{"8ccgqw"}); len(ms) != 0 {
		t.Errorf("deleted meetup listed")
	}
}
