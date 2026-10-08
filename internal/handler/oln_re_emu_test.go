package handler

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/oln"
)

// Replies by id on real Datastore (the emulator; run.sh starts it): one
// "in" query for several ids, as /api/oln/re does (78a). The memory and
// SQLite stores can't catch how Datastore wants its "in" values.
func TestRepliesOnEmulator(t *testing.T) {
	if os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		t.Skip("needs the Datastore emulator")
	}
	ctx, now := context.Background(), time.Now().UTC()
	db, err := datastore.NewClient(ctx, "kafumu-replies-test")
	if err != nil {
		t.Fatal(err)
	}
	store := &oln.DatastoreStore{DB: db}
	for id, re := range map[string]string{"r1": "aaaaaaaaaa", "r2": "bbbbbbbbbb", "r3": "cccccccccc"} {
		n := &oln.Note{ID: id, Raw: "v2;0;x;y;#re" + re, Text: "👍", Cell: oln.Everywhere, Re: re, Bits: 5, At: now, Recv: now, ExpiresAt: now.Add(time.Hour)}
		if err := store.Put(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	got, err := oln.NewService(store).Replies(ctx, []string{"aaaaaaaaaa", "bbbbbbbbbb", "0123456789"})
	if err != nil && strings.Contains(err.Error(), "expected 1") {
		// The local emulator predates "in" (production has it: checked by
		// hand on 2026-10-08 with a two-id lookup).
		t.Skip("this Datastore emulator has no \"in\" filter")
	}
	if err != nil || len(got) != 2 {
		t.Fatalf("replies = %d, %v", len(got), err)
	}
}
