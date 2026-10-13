package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LaPingvino/kafumu/internal/feeds"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// Local sources (81d): the feeds job reads the sources (#feed lines) of the
// areas people looked at in the last three days, never others. A calendar
// becomes meetups (the usual feed path); a news feed's items are kept in the
// shared cache per area for half a day and go out with the area's bundle,
// carrying the value (bits) of the line that added the source.

const seenCellsKey = "seen-cells"

type cellLog struct {
	mu    sync.Mutex
	noted map[string]time.Time
}

// noteCell records that someone (never a bot: the bundle refuses those)
// looked at cell: at most once an hour per cell per instance.
func (h *Home) noteCell(ctx context.Context, cell string) {
	if h.Cache == nil {
		return
	}
	now := time.Now()
	h.seenCells.mu.Lock()
	if h.seenCells.noted == nil {
		h.seenCells.noted = map[string]time.Time{}
	}
	if now.Sub(h.seenCells.noted[cell]) < time.Hour {
		h.seenCells.mu.Unlock()
		return
	}
	h.seenCells.noted[cell] = now
	h.seenCells.mu.Unlock()
	m := map[string]int64{}
	if b, ok := h.Cache.Get(ctx, seenCellsKey); ok {
		json.Unmarshal(b, &m)
	}
	m[cell] = now.Unix()
	for c, at := range m { // three days, and a bounded list
		if now.Sub(time.Unix(at, 0)) > 3*24*time.Hour {
			delete(m, c)
		}
	}
	if b, err := json.Marshal(m); err == nil && len(m) <= 2000 {
		h.Cache.Set(ctx, seenCellsKey, b, 4*24*time.Hour)
	}
}

// SeenCells: the cells looked at in the last three days, newest first.
func (h *Home) SeenCells(ctx context.Context, now time.Time) []string {
	m := map[string]int64{}
	if h.Cache != nil {
		if b, ok := h.Cache.Get(ctx, seenCellsKey); ok {
			json.Unmarshal(b, &m)
		}
	}
	var out []string
	for c, at := range m {
		if now.Sub(time.Unix(at, 0)) < 3*24*time.Hour {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return m[out[i]] > m[out[j]] })
	return out
}

// NewsItem is one entry of a source's news feed, shown in its area.
type NewsItem struct {
	Title string    `json:"title"`
	Link  string    `json:"link"`
	Text  string    `json:"text,omitempty"`
	At    time.Time `json:"at"`
	Host  string    `json:"host"`
	By    string    `json:"by,omitempty"` // who added the source
	Bits  int       `json:"bits"`         // the source line's value
	Src   string    `json:"src"`          // the source line's id
	Cell  string    `json:"cell"`
}

func newsKey(cell string) string { return "news:" + cell }

// News: what the sources of these cells brought in (bundle; first 9 cells).
func (h *Home) News(ctx context.Context, cells []string) []NewsItem {
	out := []NewsItem{}
	if h.Cache == nil {
		return out
	}
	for i, c := range cells {
		if i >= 9 {
			break
		}
		if b, ok := h.Cache.Get(ctx, newsKey(c)); ok {
			var items []NewsItem
			if json.Unmarshal(b, &items) == nil {
				out = append(out, items...)
			}
		}
	}
	return out
}

var firstURL = regexp.MustCompile(`https?://[^\s<>"]+`)

// Limits per feeds run: the job runs every six hours.
const (
	maxSourcesPerRun  = 40
	maxCellsPerRun    = 200
	maxSourcesPerCell = 5
	maxNewsPerSource  = 10
	maxNewsPerCell    = 30
	newsMaxAge        = 7 * 24 * time.Hour
)

// pullSources reads the sources of recently seen cells; it returns the
// calendar feeds to hand to the usual feed sync, and a summary.
func (h *Meetups) pullSources(ctx context.Context, now time.Time) ([]feeds.Feed, string) {
	home := h.Home
	if home.Notes == nil || home.Cache == nil || h.Importer == nil {
		return nil, ""
	}
	var cals []feeds.Feed
	read, items := 0, 0
	// A seen cell and the ring around it: an area shows its neighbours too.
	var todo []string
	done := map[string]bool{}
	for _, seen := range home.SeenCells(ctx, now) {
		for _, c := range geo.Rings(seen, 1) {
			if !done[c] {
				done[c] = true
				todo = append(todo, c)
			}
		}
	}
	for i, cell := range todo {
		if read >= maxSourcesPerRun || i >= maxCellsPerRun { // one cached query per cell
			break
		}
		ns, err := home.Notes.InCells(ctx, []string{cell})
		if err != nil {
			continue
		}
		var news []NewsItem
		n := 0
		for _, s := range ns {
			if !oln.IsSource(s) || n >= maxSourcesPerCell || read >= maxSourcesPerRun {
				continue
			}
			link := firstURL.FindString(s.Text)
			if link == "" {
				continue
			}
			n++
			read++
			body, err := h.Importer.Get(ctx, link)
			if err != nil {
				log.Printf("sources: %s: %v", link, err)
				continue
			}
			host := ""
			if u, err := url.Parse(link); err == nil {
				host = strings.TrimPrefix(u.Hostname(), "www.")
			}
			if f, ok := importer.ParseFeed(body); ok {
				for i, it := range f.Items {
					if i >= maxNewsPerSource || (!it.At.IsZero() && now.Sub(it.At) > newsMaxAge) {
						continue
					}
					at := it.At
					if at.IsZero() || at.After(now) {
						at = now
					}
					by := s.Author
					if by == "" {
						by = s.Src
					}
					news = append(news, NewsItem{Title: it.Title, Link: it.Link, Text: it.Text, At: at, Host: host, By: by, Bits: s.Bits, Src: s.ID, Cell: cell})
				}
				continue
			}
			// Not news: a calendar, read the usual way into meetups.
			kind := "jsonld"
			if strings.Contains(body, "BEGIN:VCALENDAR") {
				kind = "ics"
			}
			if kind == "ics" || len(importer.ParseAll(body)) > 0 {
				cals = append(cals, feeds.Feed{URL: link, Kind: kind, Cell: cell, Note: "source #geo" + cell + " by @" + s.Src})
			}
		}
		sort.Slice(news, func(i, j int) bool { return news[i].At.After(news[j].At) })
		if len(news) > maxNewsPerCell {
			news = news[:maxNewsPerCell]
		}
		if b, err := json.Marshal(news); err == nil && n > 0 {
			home.Cache.Set(ctx, newsKey(cell), b, 12*time.Hour)
		}
		items += len(news)
	}
	return cals, "sources: " + itoa(read) + " read, " + itoa(items) + " news items, " + itoa(len(cals)) + " calendars"
}
