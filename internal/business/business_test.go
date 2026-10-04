package business

import (
	"context"
	"testing"
	"time"
)

func TestBusiness(t *testing.T) {
	s, ctx, now := New(nil), context.Background(), time.Now()
	if _, err := s.Create(ctx, "", "cafe", "", "u1", now); err != ErrInvalid {
		t.Fatalf("no name = %v", err)
	}
	b, err := s.Create(ctx, "Café Lisboa", "cafe", "hi@example.org", "u1", now)
	if err != nil || !b.Live(now) || b.TrialOver(now) || !b.Manages("u1") {
		t.Fatalf("new business %+v %v", b, err)
	}
	if !b.TrialOver(now.Add(Trial+time.Hour)) || b.Live(now.Add(Trial+time.Hour)) {
		t.Fatal("trial should be over after a month")
	}
	mine, _ := s.ForUser(ctx, "u1")
	if len(mine) != 1 || mine[0].Name != "Café Lisboa" {
		t.Fatalf("ForUser = %+v", mine)
	}
}
