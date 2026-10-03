package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	s := NewService(NewMemoryStore())
	u, cred, err := s.Create(ctx, "pt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(u.TokenHash, strings.SplitN(cred, ".", 2)[1]) {
		t.Fatal("token stored in clear")
	}
	got, err := s.Resolve(ctx, cred)
	if err != nil || got.ID != u.ID {
		t.Fatalf("Resolve = %v, %v", got, err)
	}
	for _, bad := range []string{"", "nodot", u.ID + ".wrong", "x." + strings.SplitN(cred, ".", 2)[1]} {
		if _, err := s.Resolve(ctx, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("Resolve(%q) err = %v", bad, err)
		}
	}

	if err := s.SetUsername(ctx, u, "Joop"); err != nil {
		t.Fatal(err)
	}
	v, _, _ := s.Create(ctx, "en")
	if err := s.SetUsername(ctx, v, "joop"); !errors.Is(err, ErrTaken) {
		t.Errorf("duplicate username err = %v", err)
	}
	if err := s.SetUsername(ctx, v, "no spaces"); err == nil {
		t.Error("invalid username accepted")
	}
	// Renaming frees the old name.
	if err := s.SetUsername(ctx, u, "joop2"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUsername(ctx, v, "joop"); err != nil {
		t.Errorf("old name not released: %v", err)
	}

	if err := s.Delete(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, cred); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted account still resolves: %v", err)
	}
}

func TestProfile(t *testing.T) {
	ctx := context.Background()
	s := NewService(NewMemoryStore())
	u, _, _ := s.Create(ctx, "en")
	// No username: can't be visible.
	s.SetProfile(ctx, u, "8ccgqw", "hi", "", nil, nil, time.Hour)
	if ps, _ := s.People(ctx, []string{"8ccgqw"}); len(ps) != 0 {
		t.Fatalf("nameless user visible: %+v", ps)
	}
	s.SetUsername(ctx, u, "ana")
	err := s.SetProfile(ctx, u, "8CCGQW", "Esperanto & robots", "blue hat",
		[]string{"epo/native", "por/learning", "bad", "EPO/native"}, []string{"AI", "ai", "opensource"}, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !u.VisibleUntil.Before(time.Now().Add(MaxVisible + time.Minute)) {
		t.Errorf("visibility not capped: %v", u.VisibleUntil)
	}
	ps, _ := s.People(ctx, []string{"8ccgqw", "8ccgqx"})
	if len(ps) != 1 || ps[0].Name != "ana" || len(ps[0].Langs) != 2 || len(ps[0].Tags) != 2 || ps[0].Where != "blue hat" {
		t.Fatalf("people = %+v", ps)
	}
	s.SetProfile(ctx, u, "8ccgqw", "", "", nil, nil, 0)
	if ps, _ := s.People(ctx, []string{"8ccgqw"}); len(ps) != 0 {
		t.Errorf("hidden user still visible")
	}
}

func TestNewSessionKeepsLink(t *testing.T) {
	ctx := context.Background()
	s := NewService(NewMemoryStore())
	u, link, _ := s.Create(ctx, "en")
	sess, err := s.NewSession(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{link, sess} {
		if _, err := s.Resolve(ctx, c); err != nil {
			t.Errorf("%q no longer works: %v", c, err)
		}
	}
	for i := 0; i < 6; i++ {
		s.NewSession(ctx, u)
	}
	if _, err := s.Resolve(ctx, sess); err == nil {
		t.Error("oldest session should have been dropped after five more")
	}
	if _, err := s.Resolve(ctx, link); err != nil {
		t.Errorf("link broke: %v", err)
	}
}
