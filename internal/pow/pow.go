// Package pow checks the small proof of work that comes with every write to
// a mailbox or slot (Joop: "no proof of work should be minimal proof of
// work"). It reuses the OLN v2 shape so one miner serves both:
//
//	"v2;<nonce>;<YYYYMMDDhhmmss>;<base64url(sha256(body))>;#<scope>"
//
// must have MinBits leading zero bits of Argon2id work (see oln.Bits), with
// the time (UTC) within ±10 minutes. The client sends "v2;<nonce>;<date>"
// in X-Kafumu-Work.
package pow

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/oln"
)

// MinBits: 4 Argon2id attempts, about a third of a second on a phone; a real
// cost for anything that wants to write millions of times.
const MinBits = 2

var ErrWork = errors.New("pow: missing or insufficient X-Kafumu-Work")

// Check verifies stamp ("<nonce>;<date>") for body written to scope.
func Check(stamp string, body []byte, scope string, now time.Time) error {
	return CheckBits(stamp, body, scope, now, MinBits)
}

// CheckBits is Check with a required number of bits (a person's price).
func CheckBits(stamp string, body []byte, scope string, now time.Time, need int) error {
	v, rest, ok := strings.Cut(stamp, ";")
	nonce, date, ok2 := strings.Cut(rest, ";")
	if !ok || !ok2 || v != "v2" || nonce == "" || len(stamp) > 64 {
		return ErrWork
	}
	at, err := time.Parse("20060102150405", date)
	if err != nil {
		return ErrWork
	}
	if d := now.UTC().Sub(at); d > oln.Window || d < -oln.Window {
		return ErrWork
	}
	h := sha256.Sum256(body)
	raw := "v2;" + nonce + ";" + date + ";" + base64.URLEncoding.EncodeToString(h[:]) + ";#" + scope
	if oln.Bits(raw) < need {
		return ErrWork
	}
	return nil
}

// Mine makes a stamp for body and scope (tests and Go clients).
func Mine(body []byte, scope string, now time.Time) string {
	return MineBits(body, scope, now, MinBits)
}

// MineBits makes a stamp with at least bits.
func MineBits(body []byte, scope string, now time.Time, bits int) string {
	h := sha256.Sum256(body)
	date := now.UTC().Format("20060102150405")
	tail := ";" + date + ";" + base64.URLEncoding.EncodeToString(h[:]) + ";#" + scope
	for i := 0; ; i++ {
		if oln.Bits(fmt.Sprintf("v2;%d", i)+tail) >= bits {
			return fmt.Sprintf("v2;%d;%s", i, date)
		}
	}
}
