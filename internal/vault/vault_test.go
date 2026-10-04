package vault

import (
	"context"
	"testing"
	"time"
)

func TestVersioning(t *testing.T) {
	s, ctx, now := NewMemoryStore(), context.Background(), time.Now()
	v, err := s.Put(ctx, "u", 0, []byte("a"), now)
	if err != nil || v != 1 {
		t.Fatalf("first put = %d, %v", v, err)
	}
	if _, err := s.Put(ctx, "u", 0, []byte("b"), now); err != ErrConflict {
		t.Fatalf("stale put = %v, want conflict", err)
	}
	if v, err = s.Put(ctx, "u", 1, []byte("c"), now); err != nil || v != 2 {
		t.Fatalf("second put = %d, %v", v, err)
	}
	if got, _ := s.Get(ctx, "u"); string(got.Data) != "c" {
		t.Fatalf("get = %q", got.Data)
	}
	if _, err := s.Put(ctx, "u", 2, make([]byte, MaxBytes+1), now); err != ErrTooBig {
		t.Fatalf("big put = %v", err)
	}
}
