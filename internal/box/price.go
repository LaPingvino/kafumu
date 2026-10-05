package box

import (
	"context"
	"github.com/LaPingvino/kafumu/internal/kv"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

const priceKind = "InboxPrice"

// Prices keeps the work price of public inboxes, keyed by box id, with a
// short per-instance cache (a price change can take a minute to apply).
type Prices struct {
	DB *datastore.Client

	mu    sync.Mutex
	mem   map[string]int // used when DB is nil (local runs, tests)
	cache map[string]priceEntry
}

type priceEntry struct {
	bits int
	at   time.Time
}

type priceEntity struct {
	Bits int `datastore:"bits,noindex"`
}

func NewPrices(db *datastore.Client) *Prices {
	p := &Prices{DB: db, mem: map[string]int{}, cache: map[string]priceEntry{}}
	if db == nil {
		kv.Load(priceKind, p.mem) // self-hosted: kept across restarts
	}
	return p
}

func (p *Prices) Set(ctx context.Context, id string, bits int) error {
	p.mu.Lock()
	p.cache[id] = priceEntry{bits, time.Now()}
	p.mem[id] = bits
	p.mu.Unlock()
	if p.DB == nil {
		kv.Save(priceKind, id, bits)
		return nil
	}
	_, err := p.DB.Put(ctx, datastore.NameKey(priceKind, id, nil), &priceEntity{bits})
	return err
}

func (p *Prices) Delete(ctx context.Context, id string) error {
	p.mu.Lock()
	delete(p.mem, id)
	p.cache[id] = priceEntry{0, time.Now()}
	p.mu.Unlock()
	if p.DB == nil {
		kv.Delete(priceKind, id)
		return nil
	}
	return p.DB.Delete(ctx, datastore.NameKey(priceKind, id, nil))
}

// Price returns the bits a box requires (0 if it isn't a public inbox).
func (p *Prices) Price(ctx context.Context, id string) int {
	p.mu.Lock()
	if e, ok := p.cache[id]; ok && time.Since(e.at) < time.Minute {
		p.mu.Unlock()
		return e.bits
	}
	if p.DB == nil {
		b := p.mem[id]
		p.mu.Unlock()
		return b
	}
	p.mu.Unlock()
	var e priceEntity
	bits := 0
	if err := p.DB.Get(ctx, datastore.NameKey(priceKind, id, nil), &e); err == nil {
		bits = e.Bits
		if bits > 12 { // a price set in v1 (SHA-1) bits: about 10 bits dearer per attempt now
			bits = max(2, bits-10)
		}
	}
	p.mu.Lock()
	if len(p.cache) > 50000 {
		p.cache = map[string]priceEntry{}
	}
	p.cache[id] = priceEntry{bits, time.Now()}
	p.mu.Unlock()
	return bits
}
