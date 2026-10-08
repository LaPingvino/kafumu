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

// Languages: codes or hand-typed names, CEFR levels (Joop); the old two
// levels still read.
func TestLanguageLevels(t *testing.T) {
	for l, ok := range map[string]bool{
		"por/B1": true, "pt-br/C1": true, "x:Ladino/A2": true, "epo/native": true, "eng/fluent": true,
		"por/Z9": false, "x:a<b/A1": false, "POR/B1": false, "x:/A1": false,
	} {
		if langRE.MatchString(l) != ok {
			t.Errorf("%q valid = %v, want %v", l, !ok, ok)
		}
	}
}

func TestCleanLangs(t *testing.T) {
	got := cleanLangs([]string{"EPO/b2", "x:Ladino/A2", "pt-BR/c1", "por/Z9", "epo/A1"})
	want := []string{"epo/B2", "x:Ladino/A2", "pt-br/C1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cleanLangs = %v, want %v", got, want)
	}
}

// Two instances share the store. B signs a new device in (a new session)
// while A still has an older cached copy; A's hourly "last seen" touch must
// not put that copy back and sign the new device out (it could, with Save).
func TestTouchKeepsOtherInstancesSession(t *testing.T) {
	ctx, st := context.Background(), NewMemoryStore()
	a, b := NewService(st), NewService(st)
	u, cred, err := a.Create(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Resolve(ctx, cred); err != nil { // A caches u
		t.Fatal(err)
	}
	ub, _ := b.ByID(ctx, u.ID)
	newDevice, err := b.NewSession(ctx, ub)
	if err != nil {
		t.Fatal(err)
	}
	// An hour on: A's cached copy is old and gets touched.
	a.mu.Lock()
	c := a.cache[u.ID]
	c.u.LastSeenAt = time.Now().Add(-2 * time.Hour)
	a.cache[u.ID] = c
	a.mu.Unlock()
	if _, err := a.Resolve(ctx, cred); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(st).Resolve(ctx, newDevice); err != nil {
		t.Fatalf("the new device was signed out by A's touch: %v", err)
	}
}
