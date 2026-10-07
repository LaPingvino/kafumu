package sqlstore

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/business"
	"path/filepath"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/handle"
	"github.com/LaPingvino/kafumu/internal/kv"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// The self-hosted purge keeps the promises of /privacy: expired messages
// and links go, idle accounts go by the same rules as on App Engine (with
// their vault and username), recent and kept-forever ones stay.
func TestPurge(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "k.db"))
	if err != nil {
		t.Fatal(err)
	}
	kv.Default = &KV{DB: db}
	defer func() { kv.Default = nil }()
	ctx, now := context.Background(), time.Now()
	notes := &Notes{DB: db}
	notes.Put(ctx, &oln.Note{ID: "old", Raw: "x", Text: "gone", Cell: "8ccgmw", ExpiresAt: now.Add(-time.Minute)})
	notes.Put(ctx, &oln.Note{ID: "new", Raw: "y", Text: "stays", Cell: "8ccgmw", ExpiresAt: now.Add(time.Hour)})
	h := handle.New(nil)
	h.Set(ctx, "oldlink", "u", "v1.x", now.Add(-handle.TTL-time.Hour))
	h.Set(ctx, "newlink", "u", "v1.y", now)

	acc := &Accounts{DB: db}
	put := func(id, name string, idle time.Duration, keep int) {
		u := &account.User{ID: id, Username: name, KeepDays: keep, LastSeenAt: now.Add(-idle)}
		acc.Put(ctx, u)
		if name != "" {
			acc.ClaimUsername(ctx, name, id)
		}
		(&Vaults{DB: db}).Put(ctx, id, 0, []byte("sealed"), now)
	}
	bs := business.New(nil)
	shared, _ := bs.Create(ctx, "Shared Café", "cafe", "", "named-old", now)
	shared.Managers = append(shared.Managers, "anon-new")
	bs.Save(ctx, shared)
	solo, _ := bs.Create(ctx, "Solo Shop", "cafe", "", "named-old", now)
	solo.Username = "solo-shop"
	bs.Save(ctx, solo)
	acc.ClaimUsername(ctx, "solo-shop", "biz:"+solo.ID)
	put("anon-old", "", 31*24*time.Hour, 0)
	put("anon-new", "", 2*24*time.Hour, 0)
	put("named-old", "olda", 366*24*time.Hour, 0)
	put("named-kept", "keeper", 400*24*time.Hour, -1)

	res, err := Purge(ctx, db, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(res)
	if got, _ := notes.Get(ctx, "old"); got != nil {
		t.Error("expired note kept")
	}
	if got, _ := notes.Get(ctx, "new"); got == nil {
		t.Error("live note deleted")
	}
	links, _ := (&KV{DB: db}).LoadKind("Handle")
	if _, ok := links["oldlink"]; ok || len(links) != 1 {
		t.Errorf("links after purge: %d (old one kept: %v)", len(links), ok)
	}
	for id, want := range map[string]bool{"anon-old": false, "anon-new": true, "named-old": false, "named-kept": true} {
		_, err := acc.Get(ctx, id)
		if (err == nil) != want {
			t.Errorf("%s kept = %v, want %v", id, err == nil, want)
		}
		if v, _ := (&Vaults{DB: db}).Get(ctx, id); (v != nil) != want {
			t.Errorf("%s vault kept = %v, want %v", id, v != nil, want)
		}
	}
	if owner, _ := acc.LookupUsername(ctx, "olda"); owner != "" {
		t.Error("deleted account's username still taken")
	}
	// The purged manager's businesses: the shared one keeps its other
	// manager, the solo one is closed and its @name freed.
	after := business.New(nil)
	if b, err := after.Get(ctx, shared.ID); err != nil || len(b.Managers) != 1 || b.Managers[0] != "anon-new" {
		t.Errorf("shared business after purge: %+v %v", b, err)
	}
	if _, err := after.Get(ctx, solo.ID); err == nil {
		t.Error("business without managers kept")
	}
	if owner, _ := acc.LookupUsername(ctx, "solo-shop"); owner != "" {
		t.Error("closed business's name still taken")
	}
	if owner, _ := acc.LookupUsername(ctx, "keeper"); owner != "named-kept" {
		t.Error("kept account lost its name")
	}
}
