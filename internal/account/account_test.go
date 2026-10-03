package account

import (
	"context"
	"errors"
	"strings"
	"testing"
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
