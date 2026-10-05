package account

import (
	"testing"
	"time"
)

// Patrons wear the wings until their date, then simply don't.
func TestPatron(t *testing.T) {
	now := time.Now()
	u := &User{Username: "joop"}
	if u.Patron(now) || u.Public().Patron {
		t.Fatal("not a patron by default")
	}
	u.PatronUntil = now.Add(24 * time.Hour)
	if !u.Patron(now) || !u.Public().Patron {
		t.Fatal("patron until tomorrow")
	}
	if u.Patron(now.Add(48 * time.Hour)) {
		t.Fatal("still a patron after the date")
	}
}
