package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/vault"
)

// The privacy page promises you can export and delete what the server
// holds about you: the export has the account and its businesses (no
// sign-in secrets); deleting takes you off your businesses, and closes
// one you managed alone, freeing its @name.
func TestExportAndDelete(t *testing.T) {
	ctx := context.Background()
	_, home, svc := newServerWithMeetups(t)
	home.Biz = business.New(nil)
	a := &Accounts{Home: home, Svc: svc, Vault: vault.NewMemoryStore()}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /account/start", a.Start)
	mux.HandleFunc("GET /account/data.json", a.DataExport)
	mux.HandleFunc("POST /account/delete", a.Delete)
	h := a.Middleware(mux)
	me := cookieFrom(do(h, "POST", "/account/start", url.Values{}, ""))

	var ex struct {
		Account struct {
			ID, TokenHash string
		}
	}
	w := do(h, "GET", "/account/data.json", nil, me)
	json.Unmarshal(w.Body.Bytes(), &ex)
	uid := ex.Account.ID
	if uid == "" || ex.Account.TokenHash != "" || strings.Contains(w.Body.String(), `"Sessions": [`) {
		t.Fatalf("export: %s", w.Body)
	}
	shared, _ := home.Biz.Create(ctx, "Shared Café", "cafe", "", uid, time.Now())
	shared.Managers = append(shared.Managers, "other")
	home.Biz.Save(ctx, shared)
	solo, _ := home.Biz.Create(ctx, "Solo Shop", "cafe", "", uid, time.Now())
	solo.Username = "solo-shop"
	home.Biz.Save(ctx, solo)
	svc.Store.ClaimUsername(ctx, "solo-shop", "biz:"+solo.ID)
	if w := do(h, "GET", "/account/data.json", nil, me); !strings.Contains(w.Body.String(), "Solo Shop") || !strings.Contains(w.Body.String(), "Shared Café") {
		t.Fatalf("export lacks the businesses: %s", w.Body)
	}

	do(h, "POST", "/account/delete", url.Values{"confirm": {"yes"}}, me)
	if b, err := home.Biz.Get(ctx, shared.ID); err != nil || len(b.Managers) != 1 || b.Managers[0] != "other" {
		t.Fatalf("shared business after delete: %+v %v", b, err)
	}
	if _, err := home.Biz.Get(ctx, solo.ID); err == nil {
		t.Fatal("a business with no managers left survived")
	}
	if owner, _ := svc.Store.LookupUsername(ctx, "solo-shop"); owner != "" {
		t.Fatalf("its @name is still taken by %q", owner)
	}
}
