package handler

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
)

// Countries people look at (Eventa Servo, LOOP-STATE 60a): the feeds job
// pulls the Esperanto calendars of the countries looked at in the last few
// days. Kept as one small entry in the shared cache, written at most once
// an hour per country per instance; bots never count.
const seenKey = "seen-countries"

type countryLog struct {
	mu    sync.Mutex
	noted map[string]time.Time
}

func (h *Home) noteCountry(ctx context.Context, cell string) {
	if h.Gaz == nil || h.Cache == nil {
		return
	}
	cc := h.Gaz.CountryOf(cell)
	if cc == "" {
		return
	}
	now := time.Now()
	h.seen.mu.Lock()
	if h.seen.noted == nil {
		h.seen.noted = map[string]time.Time{}
	}
	if now.Sub(h.seen.noted[cc]) < time.Hour {
		h.seen.mu.Unlock()
		return
	}
	h.seen.noted[cc] = now
	h.seen.mu.Unlock()
	m := h.seenCountries(ctx)
	m[cc] = now.Unix()
	if b, err := json.Marshal(m); err == nil {
		h.Cache.Set(ctx, seenKey, b, 7*24*time.Hour)
		// Kept in Datastore too: shared memcache may drop it any time, and
		// then the feeds job forgot which calendars to pull (seen at
		// midnight: 2 feeds instead of 4). At most once an hour per country
		// per instance.
		if h.DB != nil {
			_, _ = h.DB.Put(ctx, seenDSKey(), &seenEntity{JSON: string(b)})
		}
	}
}

type seenEntity struct {
	JSON string `datastore:"json,noindex"`
}

func seenDSKey() *datastore.Key { return datastore.NameKey("Config", seenKey, nil) }

// seenCountries: the shared cache's list, or Datastore's when the cache
// lost it (then put back in the cache).
func (h *Home) seenCountries(ctx context.Context) map[string]int64 {
	m := map[string]int64{}
	if b, ok := h.Cache.Get(ctx, seenKey); ok {
		json.Unmarshal(b, &m)
		return m
	}
	if h.DB != nil {
		var e seenEntity
		if h.DB.Get(ctx, seenDSKey(), &e) == nil {
			json.Unmarshal([]byte(e.JSON), &m)
			h.Cache.Set(ctx, seenKey, []byte(e.JSON), 7*24*time.Hour)
		}
	}
	return m
}

// SeenCountries: the countries looked at in the last three days, most
// recent first, at most ten.
func (h *Home) SeenCountries(ctx context.Context, now time.Time) []string {
	if h.Cache == nil {
		return nil
	}
	m := h.seenCountries(ctx)
	var out []string
	for cc, at := range m {
		if now.Sub(time.Unix(at, 0)) < 3*24*time.Hour {
			out = append(out, cc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m[out[i]] > m[out[j]] })
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}
