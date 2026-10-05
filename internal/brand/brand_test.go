package brand

import (
	"context"
	"testing"
)

func TestBrands(t *testing.T) {
	ctx := context.Background()
	s := New(nil)
	b := &Brand{Host: "WWW.Bahais.in:443", Name: "Bahá'í Local", Accent: "#3a5f8a", Button: "🙏 Who wants to pray with me?", Tags: []string{"#Bahai", "prayer", "bad tag"}}
	if err := s.Save(ctx, b); err != nil {
		t.Fatal(err)
	}
	got := s.For(ctx, "bahais.in")
	if got == nil || got.Name != "Bahá'í Local" || got.Accent != "#3a5f8a" || len(got.Tags) != 2 || got.Tags[0] != "bahai" {
		t.Fatalf("brand = %+v", got)
	}
	if s.For(ctx, "kafumu.com") != nil {
		t.Fatal("plain host has a brand")
	}
	if err := s.Save(ctx, &Brand{Host: "evil.example", Name: "X", Accent: "red;}body{display:none"}); err != nil || s.For(ctx, "evil.example").Accent != "" {
		t.Fatal("a non-hex accent got through")
	}
	if err := s.Save(ctx, &Brand{Host: "not a host", Name: "X"}); err == nil {
		t.Fatal("bad host accepted")
	}
	s.Delete(ctx, "bahais.in")
	if s.For(ctx, "bahais.in") != nil {
		t.Fatal("deleted brand still there")
	}
}
