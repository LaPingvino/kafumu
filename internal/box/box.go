// Package box is the signalling mailbox: unidirectional queues addressed by
// random ids that only the two paired devices can compute, carrying
// ciphertext the server cannot read, with a short TTL. It is SimpleX-shaped:
// no accounts, no sender identity (VISION.md, tier 3).
package box

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"time"
)

const (
	MaxMessage  = 8 << 10 // bytes of ciphertext per message
	MaxMessages = 32      // pending messages per box
	TTL         = 7 * 24 * time.Hour
)

var (
	ErrFull   = errors.New("box: full")
	ErrTooBig = errors.New("box: message too big")
	idPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ValidID reports whether id looks like a box id (hex HMAC-SHA256).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Message is one opaque blob in a box.
type Message struct {
	ID   string    `json:"id"`
	Data string    `json:"data"` // base64 ciphertext, opaque to the server
	At   time.Time `json:"at"`
}

// Store keeps boxes. Datastore in production, memory locally and in tests.
type Store interface {
	Append(ctx context.Context, box string, m Message) error
	List(ctx context.Context, box string) ([]Message, error)
	// Ack deletes the given message ids; unknown ids are ignored.
	Ack(ctx context.Context, box string, ids []string) error
}

// NewMessage wraps data with a random id and the current time.
func NewMessage(data string) (Message, error) {
	if len(data) > MaxMessage {
		return Message{}, ErrTooBig
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return Message{}, err
	}
	return Message{ID: hex.EncodeToString(b), Data: data, At: time.Now().UTC()}, nil
}

// live drops expired messages.
func live(ms []Message, now time.Time) []Message {
	out := ms[:0:0]
	for _, m := range ms {
		if now.Sub(m.At) < TTL {
			out = append(out, m)
		}
	}
	return out
}

func remove(ms []Message, ids []string) []Message {
	drop := map[string]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	out := ms[:0:0]
	for _, m := range ms {
		if !drop[m.ID] {
			out = append(out, m)
		}
	}
	return out
}
