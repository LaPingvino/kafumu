package handler

import (
	"context"
	"strings"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/business"
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
		if b := h.bizByID(ctx, id); b != nil && h.managesBiz(ctx, b, u.ID) {
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

// managesBiz: userID manages b, directly or as a manager of a business
// that is one of b's managers ("biz:<id>"). One level only: no chains,
// no loops.
func (h *Home) managesBiz(ctx context.Context, b *business.Business, userID string) bool {
	if b == nil || userID == "" {
		return false
	}
	if b.Manages(userID) {
		return true
	}
	for _, m := range b.Managers {
		if id, ok := strings.CutPrefix(m, "biz:"); ok && id != b.ID {
			if mb := h.bizByID(ctx, id); mb != nil && mb.Manages(userID) {
				return true
			}
		}
	}
	return false
}

// bizFor: the businesses userID manages, directly or through a business
// of theirs that manages them (one indexed query per business of theirs).
func (h *Home) bizFor(ctx context.Context, userID string) ([]*business.Business, error) {
	if h.Biz == nil {
		return nil, nil
	}
	direct, err := h.Biz.ForUser(ctx, userID)
	out, seen := append([]*business.Business(nil), direct...), map[string]bool{}
	for _, b := range direct {
		seen[b.ID] = true
	}
	for _, b := range direct {
		more, _ := h.Biz.ForUser(ctx, "biz:"+b.ID)
		for _, m := range more {
			if !seen[m.ID] {
				seen[m.ID] = true
				out = append(out, m)
			}
		}
	}
	return out, err
}
