package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/brand"
)

// A brand's host shows its own name, colour, main button, tagline and
// starting tag; the plain host shows Kafumu.
func TestBrandPage(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	home.Brands = brand.New(nil)
	home.Brands.Save(context.Background(), &brand.Brand{Host: "bahais.test", Name: "Bahá'í Local", Tagline: "Pray and serve together, near you",
		Accent: "#3a5f8a", Button: "🙏 Who wants to pray with me?", Tags: []string{"bahai"}})
	get := func(host string) string {
		r := httptest.NewRequest("GET", "/", nil)
		r.Host = host
		r.Header.Set("User-Agent", "Mozilla/5.0 Firefox/130")
		r.Header.Set("Accept-Language", "en")
		w := httptest.NewRecorder()
		home.ShowHome(w, r)
		return w.Body.String()
	}
	b := get("bahais.test")
	for _, want := range []string{"<title>Bahá&#39;í Local</title>", "--accent: #3a5f8a", "🙏 Who wants to pray with me?", "Pray and serve together, near you", `data-brand-tag="bahai"`} {
		if !strings.Contains(b, want) {
			t.Errorf("brand page lacks %q", want)
		}
	}
	if p := get("kafumu.test"); strings.Contains(p, "--accent: #3a5f8a") || strings.Contains(p, "pray with me") || !strings.Contains(p, "Kafumu") {
		t.Error("plain host shows the brand")
	}
}
