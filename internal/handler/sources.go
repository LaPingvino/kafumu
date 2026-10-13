package handler

import (
	"context"
	"encoding/json"
	"html"
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
	hosts map[string]time.Time // when an area's shared sites were last counted
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

// ---- Suggested sources (81f) ----
// The sites an area's Bluesky posts link to most are counted when its bundle
// is built (≤ once an hour per cell per instance); the feeds job looks on the
// most shared ones for a news feed (RSS/Atom autodiscovery) and the area's
// page then suggests adding it: people here care about this site.

// notASource: big platforms and link shorteners, never a local source.
var notASource = regexp.MustCompile(`(^|\.)(bsky\.app|bsky\.social|youtube\.com|youtu\.be|instagram\.com|facebook\.com|fb\.me|x\.com|twitter\.com|t\.co|bit\.ly|tinyurl\.com|linktr\.ee|tiktok\.com|google\.[a-z.]+|goo\.gl|spotify\.com|apple\.com|amazon\.[a-z.]+|wikipedia\.org|github\.com|linkedin\.com|threads\.net|mastodon\.[a-z]+|reddit\.com|ow\.ly|buff\.ly|lnkd\.in|smokesignal\.events|kafumu\.com)$`)

func hostsKey(cell string) string   { return "hosts:" + cell }
func suggestKey(cell string) string { return "suggest:" + cell }
func feedOfKey(host string) string  { return "feedof:" + host }

// noteHosts counts the sites posts here link to (each post once per site).
func (h *Home) noteHosts(ctx context.Context, cell string, links [][]string) {
	if h.Cache == nil {
		return
	}
	now := time.Now()
	h.seenCells.mu.Lock()
	if h.seenCells.hosts == nil {
		h.seenCells.hosts = map[string]time.Time{}
	}
	if now.Sub(h.seenCells.hosts[cell]) < time.Hour {
		h.seenCells.mu.Unlock()
		return
	}
	h.seenCells.hosts[cell] = now
	h.seenCells.mu.Unlock()
	count := map[string]int{}
	for _, ls := range links {
		seen := map[string]bool{}
		for _, l := range ls {
			u, err := url.Parse(l)
			if err != nil {
				continue
			}
			host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
			if host == "" || notASource.MatchString(host) || seen[host] {
				continue
			}
			seen[host] = true
			count[host]++
		}
	}
	if b, err := json.Marshal(count); err == nil {
		h.Cache.Set(ctx, hostsKey(cell), b, 3*24*time.Hour)
	}
}

// Suggestion: a site people here share, with the feed found on it.
type Suggestion struct {
	Host  string `json:"host"`
	Feed  string `json:"feed"`
	Count int    `json:"count"`
}

// Suggestions: what the feeds job found for cell (bundle).
func (h *Home) Suggestions(ctx context.Context, cell string) []Suggestion {
	out := []Suggestion{}
	if h.Cache != nil {
		if b, ok := h.Cache.Get(ctx, suggestKey(cell)); ok {
			json.Unmarshal(b, &out)
		}
	}
	return out
}

var feedLink = regexp.MustCompile(`(?is)<link\b[^>]*>`)
var attr = regexp.MustCompile(`(?i)\b(rel|type|href)\s*=\s*["']([^"']*)["']`)

// discoverFeed finds a page's RSS/Atom feed (<link rel="alternate" ...>).
func discoverFeed(page, base string) string {
	for _, tag := range feedLink.FindAllString(page, 50) {
		a := map[string]string{}
		for _, m := range attr.FindAllStringSubmatch(tag, -1) {
			a[strings.ToLower(m[1])] = m[2]
		}
		t := strings.ToLower(a["type"])
		if strings.Contains(strings.ToLower(a["rel"]), "alternate") && (strings.Contains(t, "rss") || strings.Contains(t, "atom")) && a["href"] != "" {
			b, err := url.Parse(base)
			if err != nil {
				return ""
			}
			u, err := b.Parse(html.UnescapeString(a["href"]))
			if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
				return ""
			}
			return u.String()
		}
	}
	return ""
}

// Limits for suggestions per feeds run.
const (
	maxProbesPerRun  = 10
	minSharesToProbe = 3
	maxSuggestions   = 3
)

// suggestSources: for seen cells, the most shared sites that have a feed
// and aren't a source there yet. Probes are cached per site for a week
// (also "no feed"), ≤ maxProbesPerRun new ones per run.
func (h *Meetups) suggestSources(ctx context.Context, now time.Time) string {
	home := h.Home
	if home.Cache == nil || h.Importer == nil {
		return ""
	}
	probes, made := 0, 0
	for i, cell := range home.SeenCells(ctx, now) {
		if i >= 100 {
			break
		}
		count := map[string]int{}
		if b, ok := home.Cache.Get(ctx, hostsKey(cell)); ok {
			json.Unmarshal(b, &count)
		}
		hosts := make([]string, 0, len(count))
		for host, n := range count {
			if n >= minSharesToProbe {
				hosts = append(hosts, host)
			}
		}
		sort.Slice(hosts, func(a, b int) bool { return count[hosts[a]] > count[hosts[b]] })
		have := map[string]bool{} // sources already here
		if home.Notes != nil {
			if ns, err := home.Notes.InCells(ctx, []string{cell}); err == nil {
				for _, n := range ns {
					if oln.IsSource(n) {
						if u, err := url.Parse(firstURL.FindString(n.Text)); err == nil {
							have[strings.TrimPrefix(u.Hostname(), "www.")] = true
						}
					}
				}
			}
		}
		var out []Suggestion
		for _, host := range hosts {
			if len(out) >= maxSuggestions || have[host] {
				continue
			}
			feed := ""
			if b, ok := home.Cache.Get(ctx, feedOfKey(host)); ok {
				feed = string(b)
			} else if probes < maxProbesPerRun {
				probes++
				if page, err := h.Importer.Get(ctx, "https://"+host+"/"); err == nil {
					feed = discoverFeed(page, "https://"+host+"/")
				}
				if feed == "" {
					feed = "-"
				}
				home.Cache.Set(ctx, feedOfKey(host), []byte(feed), 7*24*time.Hour)
			} else {
				continue
			}
			if feed != "-" && feed != "" {
				out = append(out, Suggestion{Host: host, Feed: feed, Count: count[host]})
			}
		}
		if len(out) > 0 {
			if b, err := json.Marshal(out); err == nil {
				home.Cache.Set(ctx, suggestKey(cell), b, 24*time.Hour)
				made++
			}
		} else {
			home.Cache.Delete(ctx, suggestKey(cell))
		}
	}
	return "suggestions: " + itoa(probes) + " sites probed, " + itoa(made) + " areas"
}
