package handler

import (
	"net/http"
	"slices"
	"strings"

	"github.com/LaPingvino/kafumu/internal/brand"
)

// BrandPage handles GET/POST /brand on a brand's own host: its admins
// (set by the Kafumu admin) edit its name, tagline, main button, colour
// and the tags Around starts from. The host itself stays the admin's.
func (h *Home) BrandPage(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	bi := h.Brands.For(r.Context(), r.Host)
	if u == nil || bi == nil || !slices.Contains(bi.Admins, u.ID) {
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
