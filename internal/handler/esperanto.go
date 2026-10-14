package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/feeds"
	"github.com/LaPingvino/kafumu/internal/importer"
)

// onlineCalendars: online events per tag (90b): calendars of events with no
// place, filed by what they're about. Eventa Servo's online calendar is just
// the one for #esperanto; more can be added here (and later by people, like
// an area's sources).
var onlineCalendars = map[string][]feeds.Feed{
	"esperanto": {feeds.EventaServo("ol")},
}

// OnlineTags: the tags that have online calendars (for the client to know).
func OnlineTags() []string {
	var out []string
	for t := range onlineCalendars {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// Online handles GET /api/online?tag=…: the online events for a tag, the next
// two months, at most 30, soonest first (running ones after). Each calendar
// is fetched at most every six hours (shared cache), never for bots.
// /api/esperanto/online is the same for #esperanto.
func (h *Meetups) Online(w http.ResponseWriter, r *http.Request) {
	if IsBot(r) {
		http.Error(w, "not for robots", http.StatusForbidden)
		return
	}
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "esperanto"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	cals, ok := onlineCalendars[tag]
	if !ok {
		w.Write([]byte("[]"))
		return
	}
	key := "online-v2:" + tag
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
		Host  string    `json:"host"`
	}
	out := []event{}
	now := time.Now()
	for _, f := range cals {
		if h.Importer == nil {
			break
		}
		body, err := h.Importer.Get(r.Context(), f.URL)
		if err != nil {
			continue
		}
		for _, ev := range importer.ParseICS(body) {
			end := ev.End
			if end.IsZero() {
				end = ev.Start
			}
			if end.Before(now) || ev.Start.After(now.Add(60*24*time.Hour)) {
				continue
			}
			out = append(out, event{ev.Title, ev.Start.UTC(), ev.End.UTC(), feeds.EventLink(ev, f.URL), hostOfURL(f.URL)})
		}
	}
	// What's coming first (soonest first), then what's already running
	// (contests and courses that last weeks), ending soonest first.
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

func hostOfURL(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.TrimPrefix(u.Hostname(), "www.")
	}
	return ""
}
