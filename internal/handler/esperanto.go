package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/LaPingvino/kafumu/internal/feeds"
	"github.com/LaPingvino/kafumu/internal/importer"
)

// EsperantoOnline handles GET /api/esperanto/online: Eventa Servo's online
// Esperanto events (ol.ics), the next two months, at most 30. Fetched from
// eventaservo.org at most every six hours (shared cache), never for bots.
// Around shows them when the app speaks Esperanto, or Esperanto (the
// language chip or #esperanto) is what you're looking at.
func (h *Meetups) EsperantoOnline(w http.ResponseWriter, r *http.Request) {
	if IsBot(r) {
		http.Error(w, "not for robots", http.StatusForbidden)
		return
	}
	const key = "eo-online-v1"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	if h.Home.Cache != nil {
		if b, ok := h.Home.Cache.Get(r.Context(), key); ok {
			w.Write(b)
			return
		}
	}
	type event struct {
		Title string    `json:"title"`
		Start time.Time `json:"start"`
		End   time.Time `json:"end,omitempty"`
		Link  string    `json:"link"`
	}
	out := []event{}
	f := feeds.EventaServo("ol")
	if h.Importer != nil {
		if body, err := h.Importer.Get(r.Context(), f.URL); err == nil {
			now := time.Now()
			for _, ev := range importer.ParseICS(body) {
				end := ev.End
				if end.IsZero() {
					end = ev.Start
				}
				if end.Before(now) || ev.Start.After(now.Add(60*24*time.Hour)) {
					continue
				}
				out = append(out, event{ev.Title, ev.Start.UTC(), ev.End.UTC(), feeds.EventLink(ev, f.URL)})
			}
		}
	}
	// What's coming first (soonest first), then what's already running
	// (contests and courses that last weeks), ending soonest first.
	now := time.Now()
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ua, ub := a.Start.After(now), b.Start.After(now); ua != ub {
			return ua
		} else if ua {
			return a.Start.Before(b.Start)
		}
		return a.End.Before(b.End)
	})
	if len(out) > 30 {
		out = out[:30]
	}
	b, _ := json.Marshal(out)
	if h.Home.Cache != nil && len(out) > 0 {
		h.Home.Cache.Set(r.Context(), key, b, 6*time.Hour)
	}
	w.Write(b)
}
