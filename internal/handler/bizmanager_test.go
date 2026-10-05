package handler

import (
	"context"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/business"
)

// A business can manage another business (76a): an agency's managers
// manage the café it manages, list it among their businesses and can act
// as it; one level only, and a business can't manage itself.
func TestBusinessAsManager(t *testing.T) {
	ctx := context.Background()
	_, home, svc := newServerWithMeetups(t)
	home.Biz = business.New(nil)

	agency, _ := home.Biz.Create(ctx, "Agency", "company", "", "ana", time.Now())
	agency.Username = "agency"
	home.Biz.Save(ctx, agency)
	svc.Store.ClaimUsername(ctx, "agency", "biz:"+agency.ID)
	cafe, _ := home.Biz.Create(ctx, "Café", "cafe", "", "cafeowner", time.Now())
	cafe.Managers = append(cafe.Managers, "biz:"+agency.ID)
	home.Biz.Save(ctx, cafe)

	if !home.managesBiz(ctx, cafe, "ana") || home.managesBiz(ctx, cafe, "stranger") || !home.managesBiz(ctx, cafe, "cafeowner") {
		t.Fatal("managesBiz")
	}
	mine, _ := home.bizFor(ctx, "ana")
	if len(mine) != 2 {
		t.Fatalf("ana's businesses: %d, want the agency and the café", len(mine))
	}
	// One level only: a business that the café manages is not ana's.
	shop, _ := home.Biz.Create(ctx, "Shop", "cafe", "", "x", time.Now())
	shop.Managers = append(shop.Managers, "biz:"+cafe.ID)
	home.Biz.Save(ctx, shop)
	home.forgetBiz(shop.ID)
	if home.managesBiz(ctx, shop, "ana") {
		t.Fatal("chains of businesses followed")
	}
	if n := home.ownerName(ctx, svc, "biz:"+agency.ID); n != "@agency" {
		t.Fatalf("owner name %q", n)
	}
}
