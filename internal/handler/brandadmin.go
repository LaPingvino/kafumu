package handler

import (
	"context"

	"github.com/LaPingvino/kafumu/internal/account"
	"net/http"
	"strings"

	"github.com/LaPingvino/kafumu/internal/brand"
)

// BrandPage handles GET/POST /brand on a brand's own host: its admins
// (set by the Kafumu admin) edit its name, tagline, main button, colour
// and the tags Around starts from. The host itself stays the admin's.
func (h *Home) BrandPage(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	bi := h.Brands.For(r.Context(), r.Host)
	if !h.isBrandAdmin(r.Context(), bi, u) {
		http.NotFound(w, r)
		return
	}
	saved := false
	if r.Method == http.MethodPost {
		b := *bi
		b.Name, b.Tagline, b.Button, b.Accent = r.FormValue("name"), r.FormValue("tagline"), r.FormValue("button"), r.FormValue("accent")
		b.Tags = strings.FieldsFunc(r.FormValue("tags"), func(c rune) bool { return c == ',' || c == ' ' })
		if err := h.Brands.Save(r.Context(), &b); err == nil {
			http.Redirect(w, r, "/brand?saved=1", http.StatusSeeOther)
			return
		}
	}
	saved = r.URL.Query().Get("saved") == "1"
	p := struct {
		page
		B     *brand.Brand
		Saved bool
	}{page: h.newPage(r, ""), B: bi, Saved: saved}
	p.Title, p.Tab = bi.Name, "account"
	h.render(w, "brand.html", p)
}

// isBrandAdmin: u may manage brand bi, as one of its admins in person, or
// as a manager of a business that is one ("biz:<id>"), acting as it or not:
// an organisation can run its own brand, whoever its managers are.
func (h *Home) isBrandAdmin(ctx context.Context, bi *brand.Brand, u *account.User) bool {
	if bi == nil || u == nil {
		return false
	}
	for _, a := range bi.Admins {
		if a == u.ID {
			return true
		}
		if id, ok := strings.CutPrefix(a, "biz:"); ok {
			if b := h.bizByID(ctx, id); b != nil && b.Manages(u.ID) {
				return true
			}
		}
	}
	return false
}
