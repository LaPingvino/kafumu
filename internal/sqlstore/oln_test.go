package sqlstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/oln"
)

func mine(bits int, at time.Time, msg, keywords string) string {
	f := "v2;%d;" + at.UTC().Format("20060102150405") + ";" + base64.URLEncoding.EncodeToString([]byte(msg)) + ";" + keywords
	for i := 0; ; i++ {
		if raw := fmt.Sprintf(f, i); oln.Bits(raw) >= bits {
			return raw
		}
	}
}

// The OLN service on SQLite: post, area lists, questions by subject,
// private chat, hiding, and survival across reopening the file.
func TestNotesOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "k.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, now := context.Background(), time.Now().UTC()
	s := oln.NewService(&Notes{DB: db})
	n, err := s.Post(ctx, mine(oln.BaseBits, now, "Kafo?", "#geo8ccgmw #coffee"))
	if err != nil {
		t.Fatal(err)
	}
	q, _ := s.Post(ctx, mine(oln.BaseBits, now, "Anyone into Go?", "#geo8ccgmw #ask #opensource"))
	p, _ := s.Post(ctx, mine(oln.BaseBits, now, "c2VjcmV0", "#p0123456789abcdef0123456789abcdef"))
	got, err := s.InCells(ctx, []string{"8ccgmw", "8ccgmx"})
	if err != nil || len(got) != 2 {
		t.Fatalf("InCells = %d, %v", len(got), err)
	}
	if as, _ := s.Asks(ctx, []string{"opensource"}); len(as) != 1 || as[0].ID != q.ID {
		t.Fatalf("Asks = %+v", as)
	}
	if ps, _ := s.ForPair(ctx, "p0123456789abcdef0123456789abcdef"); len(ps) != 1 || ps[0].ID != p.ID {
		t.Fatalf("ForPair = %+v", ps)
	}
	db.Close()
	db2, _ := Open(path)
	s2 := oln.NewService(&Notes{DB: db2})
	if err := s2.Hide(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.InCells(ctx, []string{"8ccgmw"}); len(got) != 1 || got[0].ID != q.ID {
		t.Fatalf("after reopen and hide = %+v", got)
	}
}
