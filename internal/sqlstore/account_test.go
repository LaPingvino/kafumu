package sqlstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
)

// Accounts on SQLite through the real service: create, username claims
// (one owner per name, businesses included), a public profile in its
// area, and survival across reopening.
func TestAccountsOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	s := account.NewService(&Accounts{DB: db})
	u, _, err := s.Create(ctx, "en")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUsername(ctx, u, "joop"); err != nil {
		t.Fatal(err)
	}
	v, _, _ := s.Create(ctx, "pt")
	if err := s.SetUsername(ctx, v, "joop"); !errors.Is(err, account.ErrTaken) {
		t.Fatalf("second claim of joop: %v", err)
	}
	if err := (&Accounts{DB: db}).ClaimUsername(ctx, "joop", "biz:cafe"); !errors.Is(err, account.ErrTaken) {
		t.Fatalf("a business claiming a person's name: %v", err)
	}
	u.Cell, u.VisibleUntil, u.Bio = "8ccgmw", time.Now().Add(time.Hour), "Esperanto and coffee"
	if err := s.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	db.Close()

	db2, _ := Open(path)
	st := &Accounts{DB: db2}
	got, err := st.Get(ctx, u.ID)
	if err != nil || got.Username != "joop" || got.Bio != "Esperanto and coffee" || got.TokenHash == "" {
		t.Fatalf("after reopen: %+v, %v", got, err)
	}
	if owner, _ := st.LookupUsername(ctx, "joop"); owner != u.ID {
		t.Fatalf("joop owned by %q", owner)
	}
	if vis, _ := st.VisibleIn(ctx, []string{"8ccgmw", "8ccgmx"}, time.Now()); len(vis) != 1 || vis[0].ID != u.ID {
		t.Fatalf("visible = %+v", vis)
	}
	if _, err := st.Get(ctx, "nobody"); !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
}
