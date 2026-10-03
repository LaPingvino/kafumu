package purge

import (
	"context"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/datastore"
)

// Runs only against the Datastore emulator:
//
//	DATASTORE_EMULATOR_HOST=localhost:8432 go test ./internal/purge/
func TestRun(t *testing.T) {
	if os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		t.Skip("needs the Datastore emulator")
	}
	ctx := context.Background()
	db, err := datastore.NewClient(ctx, "kafumu-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	type exp struct {
		ExpiresAt time.Time `datastore:"expires_at"`
	}
	type user struct {
		Username   string    `datastore:"username"`
		LastSeenAt time.Time `datastore:"last_seen_at"`
		KeepDays   int       `datastore:"keep_days,noindex"`
	}
	put := func(k *datastore.Key, v any) {
		if _, err := db.Put(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	put(datastore.NameKey("Meetup", "old", nil), &exp{now.Add(-time.Hour)})
	put(datastore.NameKey("Meetup", "new", nil), &exp{now.Add(time.Hour)})
	put(datastore.NameKey("Box", "old", nil), &exp{now.Add(-time.Hour)})
	put(datastore.NameKey("Slot", "old", nil), &exp{now.Add(-time.Hour)})
	put(datastore.NameKey("User", "anon-idle", nil), &user{"", now.Add(-40 * 24 * time.Hour), 0})
	put(datastore.NameKey("User", "anon-active", nil), &user{"", now.Add(-time.Hour), 0})
	put(datastore.NameKey("User", "named-idle-60d", nil), &user{"ana", now.Add(-60 * 24 * time.Hour), 0})
	put(datastore.NameKey("User", "named-gone", nil), &user{"bea", now.Add(-400 * 24 * time.Hour), 0})
	put(datastore.NameKey("User", "keep-forever", nil), &user{"cai", now.Add(-400 * 24 * time.Hour), -1})
	put(datastore.NameKey("Username", "bea", nil), &struct {
		UserID string `datastore:"user_id,noindex"`
	}{"named-gone"})

	r, err := Run(ctx, db, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Meetups != 1 || r.Boxes != 1 || r.Slots != 1 || r.Users != 2 || r.Usernames != 1 {
		t.Errorf("result = %s", r)
	}
	for _, k := range []*datastore.Key{datastore.NameKey("Meetup", "new", nil), datastore.NameKey("User", "anon-active", nil),
		datastore.NameKey("User", "named-idle-60d", nil), datastore.NameKey("User", "keep-forever", nil)} {
		var v datastore.PropertyList
		if err := db.Get(ctx, k, &v); err != nil {
			t.Errorf("%s should survive: %v", k, err)
		}
	}
}
