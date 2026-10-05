package handler

import (
	"context"
	"strings"

	"github.com/LaPingvino/kafumu/internal/account"
)

// One kind of "who", for persons and businesses alike (Joop: business
// accounts should be equivalent to personal ones; LOOP-STATE 76). An owner
// is a user id, or "biz:<id>" for a business: the same form the username
// registry uses. Wherever Kafumu lets someone (a brand admin, later a
// business's manager…) be named, it takes either, and checks it here.

// actsFor: u may act for owner: it's u, or a business u manages.
func (h *Home) actsFor(ctx context.Context, u *account.User, owner string) bool {
	if u == nil || owner == "" {
		return false
	}
	if owner == u.ID {
		return true
	}
	if id, ok := strings.CutPrefix(owner, "biz:"); ok {
		if b := h.bizByID(ctx, id); b != nil && b.Manages(u.ID) {
			return true
		}
	}
	return false
}

// ownerName: "@name" of an owner (person or business), "" if unnamed.
func (h *Home) ownerName(ctx context.Context, svc *account.Service, owner string) string {
	if id, ok := strings.CutPrefix(owner, "biz:"); ok {
		if b := h.bizByID(ctx, id); b != nil && b.Username != "" {
			return "@" + b.Username
		}
		return ""
	}
	if u, err := svc.ByID(ctx, owner); err == nil && u != nil && u.Username != "" {
		return "@" + u.Username
	}
	return ""
}
