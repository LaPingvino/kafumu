package pow

import (
	"testing"
	"time"
)

func stamp(body []byte, scope string, at time.Time) string { return Mine(body, scope, at) }

func TestCheck(t *testing.T) {
	now := time.Now()
	body := []byte("ciphertext")
	s := stamp(body, "boxabc", now)
	if err := Check(s, body, "boxabc", now); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string]error{
		// At MinBits (2) a stamp passes for a random other line 1 time in 4, so
		// "bound to body and scope" is checked at 16 bits (1 in 65536).
		"other body":  CheckBits(s, []byte("other"), "boxabc", now, 16),
		"other scope": CheckBits(s, body, "boxdef", now, 16),
		"too old":     Check(stamp(body, "boxabc", now.Add(-time.Hour)), body, "boxabc", now),
		"empty":       Check("", body, "boxabc", now),
	} {
		if bad == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
