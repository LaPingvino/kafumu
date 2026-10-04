package handle

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHandle(t *testing.T) {
	s, ctx, now := New(nil), context.Background(), time.Now()
	p := "v1." + strings.Repeat("A", 87)
	if !PayloadRE.MatchString(p) {
		t.Fatal("payload pattern")
	}
	if err := s.Set(ctx, "lapingvino", "u1", p, now); err != nil {
		t.Fatal(err)
	}
	if got, ok := s.Get(ctx, "lapingvino", now); !ok || got != p {
		t.Fatalf("get = %q %v", got, ok)
	}
	if _, ok := s.Get(ctx, "lapingvino", now.Add(TTL+time.Hour)); ok {
		t.Fatal("expired handle still served")
	}
}
