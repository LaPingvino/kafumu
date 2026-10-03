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
		"other body":  Check(s, []byte("other"), "boxabc", now),
		"other scope": Check(s, body, "boxdef", now),
		"too old":     Check(stamp(body, "boxabc", now.Add(-time.Hour)), body, "boxabc", now),
		"empty":       Check("", body, "boxabc", now),
	} {
		if bad == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
