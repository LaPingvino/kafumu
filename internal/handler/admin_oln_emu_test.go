package handler

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/oln"
)

// The admin's OLN tools against the Datastore emulator (run.sh starts it):
// the stats list live public messages and count what's stored but not
// live; the purge removes expired and hidden messages and their markers.
func TestAdminOLNOnEmulator(t *testing.T) {
	if os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		t.Skip("needs the Datastore emulator")
	}
	ctx, now := context.Background(), time.Now().UTC()
	db, err := datastore.NewClient(ctx, "kafumu-admin-oln-test")
	if err != nil {
		t.Fatal(err)
	}
	store := &oln.DatastoreStore{DB: db}
	put := func(id, text string, expires time.Time) {
		if err := store.Put(ctx, &oln.Note{ID: id, Raw: "v2;0;x;y;#geo8ccgmw", Text: text, Cell: "8ccgmw", Bits: 5, At: now, ExpiresAt: expires}); err != nil {
			t.Fatal(err)
		}
	}
	put("live", "Fresh croissants", now.Add(time.Hour))
	put("old", "Yesterday's news", now.Add(-time.Hour))
	put("spam", "Cheap watches", now.Add(time.Hour))
	if err := store.Hide(ctx, "spam"); err != nil {
		t.Fatal(err)
	}
	a := &Admin{DB: db, Notes: oln.NewService(store)}

	st := a.olnStats(ctx, now)
	if st == nil || st.Total != 3 || st.Expired != 1 || st.Hidden != 1 || len(st.Messages) != 2 {
		t.Fatalf("stats = %+v", st)
	}
	e, h, err := a.olnPurge(ctx, now)
	if err != nil || e != 1 || h != 1 {
		t.Fatalf("purge = %d expired, %d hidden, %v", e, h, err)
	}
	st = a.olnStats(ctx, now)
	if st.Total != 1 || st.Hidden != 0 || len(st.Messages) != 1 || st.Messages[0].Text != "Fresh croissants" {
		t.Fatalf("after purge = %+v", st)
	}
	if left, _ := store.Hidden(ctx); len(left) != 0 {
		t.Fatalf("hidden markers left: %v", left)
	}
}

// Countries looked at survive the shared cache losing them (Datastore
// behind it), so the feeds job keeps pulling their calendars.
func TestSeenCountriesOnEmulator(t *testing.T) {
	if os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		t.Skip("needs the Datastore emulator")
	}
	ctx := context.Background()
	db, err := datastore.NewClient(ctx, "kafumu-seen-test")
	if err != nil {
		t.Fatal(err)
	}
	h := &Home{Gaz: gazetteer.Load(), Cache: cache.NewMemory(), DB: db}
	h.noteCountry(ctx, "8fw4v8") // Paris
	h.Cache = cache.NewMemory()  // the shared cache dropped everything
	got := h.SeenCountries(ctx, time.Now())
	if len(got) != 1 || got[0] != "FR" {
		t.Fatalf("after losing the cache: %v", got)
	}
}
