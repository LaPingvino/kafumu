package sqlstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/vault"
)

// The sync vault on SQLite: versions go up one at a time, a write on top
// of a stale version is refused (two devices syncing at once), and it
// survives reopening.
func TestVaultsOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, now := context.Background(), time.Now()
	s := &Vaults{DB: db}
	if v, err := s.Get(ctx, "u1"); v != nil || err != nil {
		t.Fatalf("empty = %v, %v", v, err)
	}
	if n, err := s.Put(ctx, "u1", 0, []byte("first"), now); n != 1 || err != nil {
		t.Fatalf("first put = %d, %v", n, err)
	}
	if n, err := s.Put(ctx, "u1", 1, []byte("phone"), now); n != 2 || err != nil {
		t.Fatalf("second put = %d, %v", n, err)
	}
	if _, err := s.Put(ctx, "u1", 1, []byte("laptop, stale"), now); !errors.Is(err, vault.ErrConflict) {
		t.Fatalf("stale put: %v", err)
	}
	if _, err := s.Put(ctx, "u1", 2, make([]byte, vault.MaxBytes+1), now); !errors.Is(err, vault.ErrTooBig) {
		t.Fatalf("too big: %v", err)
	}
	db.Close()
	db2, _ := Open(path)
	v, err := (&Vaults{DB: db2}).Get(ctx, "u1")
	if err != nil || v.Version != 2 || string(v.Data) != "phone" {
		t.Fatalf("after reopen: %+v, %v", v, err)
	}
	if err := (&Vaults{DB: db2}).Delete(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := (&Vaults{DB: db2}).Get(ctx, "u1"); v != nil {
		t.Fatal("deleted vault still there")
	}
}
