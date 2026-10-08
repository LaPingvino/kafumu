package business

import (
	"context"
	"fmt"
	"sync"
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

// Paid until: live through that date; after it, live only when set to stay.
func TestPaidUntil(t *testing.T) {
	now := time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC)
	b := &Business{Status: StatusActive}
	if !b.Live(now) || b.PaidOver(now) {
		t.Fatal("active without a date should be live")
	}
	b.PaidUntil = now.Add(24 * time.Hour)
	if !b.Live(now) || b.PaidOver(now) {
		t.Fatal("before paid-until: live")
	}
	later := now.Add(48 * time.Hour)
	b.AfterPaid = AfterStop
	if b.Live(later) || !b.PaidOver(later) {
		t.Fatal("after paid-until, stop: not live, follow up")
	}
	b.AfterPaid = AfterStay
	if !b.Live(later) || !b.PaidOver(later) {
		t.Fatal("after paid-until, stay: live, still follow up")
	}
}

// Update is atomic: many changes at once all land, none undoes another
// (a Get … Save pair lost a sync mode this way, in the browser test).
func TestUpdateConcurrent(t *testing.T) {
	s := New(nil)
	ctx := context.Background()
	b, err := s.Create(ctx, "Café", "cafe", "", "u0", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 1; i <= 20; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := s.Update(ctx, b.ID, func(x *Business) error { x.Managers = append(x.Managers, id); return nil }); err != nil {
				t.Error(err)
			}
		}(fmt.Sprintf("u%d", i))
	}
	wg.Wait()
	got, _ := s.Get(ctx, b.ID)
	if len(got.Managers) != 21 {
		t.Fatalf("managers = %d, want 21 (u0 + 20 added at once)", len(got.Managers))
	}
	if _, err := s.Update(ctx, "nope", func(*Business) error { return nil }); err != ErrNotFound {
		t.Fatalf("missing: %v", err)
	}
}
