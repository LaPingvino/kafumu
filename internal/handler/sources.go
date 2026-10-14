package handler

import (
	"context"
	"encoding/json"
	"html"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
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
	m := h.seenCellsMap(ctx)
	m[cell] = now.Unix()
	for c, at := range m { // three days, and a bounded list
		if now.Sub(time.Unix(at, 0)) > 3*24*time.Hour {
			delete(m, c)
		}
	}
	if b, err := json.Marshal(m); err == nil && len(m) <= 2000 {
		h.Cache.Set(ctx, seenCellsKey, b, 4*24*time.Hour)
		// Kept in Datastore too, like the seen countries: memcache may drop it,
		// and then the feeds job reads no sources at all (seen at 06:00: "0
		// read" after 3 at midnight). At most once an hour per cell per instance.
		if h.DB != nil {
			_, _ = h.DB.Put(ctx, datastore.NameKey("Config", seenCellsKey, nil), &seenEntity{JSON: string(b)})
		}
	}
}

// seenCellsMap: the shared cache's list, or Datastore's when the cache lost
// it (then put back in the cache).
func (h *Home) seenCellsMap(ctx context.Context) map[string]int64 {
	m := map[string]int64{}
	if h.Cache == nil {
		return m
	}
	if b, ok := h.Cache.Get(ctx, seenCellsKey); ok {
		json.Unmarshal(b, &m)
		return m
	}
	if h.DB != nil {
		var e seenEntity
		if h.DB.Get(ctx, datastore.NameKey("Config", seenCellsKey, nil), &e) == nil {
			json.Unmarshal([]byte(e.JSON), &m)
			h.Cache.Set(ctx, seenCellsKey, []byte(e.JSON), 4*24*time.Hour)
		}
	}
	return m
}

// SeenCells: the cells looked at in the last three days, newest first.
func (h *Home) SeenCells(ctx context.Context, now time.Time) []string {
	m := h.seenCellsMap(ctx)
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
	Lang  string    `json:"lang,omitempty"` // the feed's language ("pt"), when it says (90c)
}

func newsKey(cell string) string { return "news:" + cell }

// News: what the sources of these cells brought in.
func (h *Home) News(ctx context.Context, cells []string) []NewsItem {
	out := []NewsItem{}
	if h.Cache == nil {
		return out
	}
	for i, c := range cells {
		if i >= 30 { // one cache read each
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
	newsQuietAge      = 30 * 24 * time.Hour // how far back a quiet source still shows
	minNewsPerSource  = 3                   // its newest few
)

// pullSources reads the sources of recently seen cells; it returns the
// calendar feeds to hand to the usual feed sync, and a summary.
func (h *Meetups) pullSources(ctx context.Context, now time.Time) ([]feeds.Feed, string) {
	home := h.Home
	if home.Notes == nil || home.Cache == nil || h.Importer == nil {
		return nil, ""
	}
	var cals []feeds.Feed
	read, items, renewed := 0, 0, 0
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
		renewed += home.Notes.RenewVoted(ctx, cell, now) // sources people react to stay (81h)
		var news []NewsItem
		n := 0
		for _, s := range ns {
			if !oln.IsSource(s) || n >= maxSourcesPerCell || read >= maxSourcesPerRun {
				continue
			}
			if firstURL.FindString(s.Text) == "" {
				continue
			}
			n++
			read++
			items, cal := h.readSource(ctx, s, cell, now)
			news = append(news, items...)
			if cal != nil {
				cals = append(cals, *cal)
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
	return cals, "sources: " + itoa(read) + " read, " + itoa(items) + " news items, " + itoa(len(cals)) + " calendars, " + itoa(renewed) + " kept by votes"
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

// readSource reads one source: a news feed's recent items, or (not news) the
// calendar it is, to hand to the feed sync.
func (h *Meetups) readSource(ctx context.Context, s *oln.Note, cell string, now time.Time) ([]NewsItem, *feeds.Feed) {
	link := firstURL.FindString(s.Text)
	body, err := h.Importer.Get(ctx, link)
	if err != nil {
		log.Printf("sources: %s: %v", link, err)
		return nil, nil
	}
	host := ""
	if u, err := url.Parse(link); err == nil {
		host = strings.TrimPrefix(u.Hostname(), "www.")
	}
	if f, ok := importer.ParseFeed(body); ok {
		var news []NewsItem
		by := s.Author
		if by == "" {
			by = s.Src
		}
		for i, it := range f.Items {
			// A week back, but a quiet local feed (a paper's page per town)
			// still shows its newest few from the last month (Joop: O
			// Setubalense's Barreiro page showed nothing).
			age := now.Sub(it.At)
			if i >= maxNewsPerSource || (!it.At.IsZero() && age > newsMaxAge && (len(news) >= minNewsPerSource || age > newsQuietAge)) {
				continue
			}
			at := it.At
			if at.IsZero() || at.After(now) {
				at = now
			}
			news = append(news, NewsItem{Title: it.Title, Link: it.Link, Text: it.Text, At: at, Host: host, By: by, Bits: s.Bits, Src: s.ID, Cell: cell, Lang: f.Lang})
		}
		return news, nil
	}
	kind := "jsonld"
	if strings.Contains(body, "BEGIN:VCALENDAR") {
		kind = "ics"
	}
	if kind == "ics" || len(importer.ParseAll(body)) > 0 {
		return nil, &feeds.Feed{URL: link, Kind: kind, Cell: cell, Note: "source #geo" + cell + " by @" + s.Src}
	}
	return nil, nil
}

// PullSource handles POST /api/oln/source/pull?id=…: read a source right
// after it was added (81i), so its news shows at once rather than after the
// next feeds run, or when its area has none of its news; once per 10
// minutes per source.
func (h *Meetups) PullSource(w http.ResponseWriter, r *http.Request) {
	notes := h.Home.Notes
	// Anyone may ask (not bots): it only reads a source the area already has,
	// at most once per 10 minutes, so a source added before this existed,
	// or whose news the cache dropped, shows up when someone looks.
	if notes == nil || IsBot(r) || h.Importer == nil || h.Home.Cache == nil {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	n, err := notes.Store.Get(r.Context(), r.URL.Query().Get("id"))
	if err != nil || !oln.IsSource(n) || n.Cell == oln.Everywhere {
		http.NotFound(w, r)
		return
	}
	ctx, now := r.Context(), time.Now()
	key := "pulled:" + n.ID
	if _, ok := h.Home.Cache.Get(ctx, key); ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.Home.Cache.Set(ctx, key, []byte("1"), 10*time.Minute)
	items, cal := h.readSource(ctx, n, n.Cell, now)
	if cal != nil {
		feeds.Sync(ctx, []feeds.Feed{*cal}, h.Importer, h.Svc.Store, now)
		h.Svc.ForgetAll()
	}
	if len(items) > 0 { // merged with what the area's other sources brought
		var all []NewsItem
		if b, ok := h.Home.Cache.Get(ctx, newsKey(n.Cell)); ok {
			json.Unmarshal(b, &all)
		}
		kept := all[:0]
		for _, it := range all {
			if it.Src != n.ID {
				kept = append(kept, it)
			}
		}
		all = append(kept, items...)
		sort.Slice(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
		if len(all) > maxNewsPerCell {
			all = all[:maxNewsPerCell]
		}
		if b, err := json.Marshal(all); err == nil {
			h.Home.Cache.Set(ctx, newsKey(n.Cell), b, 12*time.Hour)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"news": len(items), "calendar": map[bool]int{true: 1}[cal != nil]})
}

// sourcesIn: the source lines among ns.
func sourcesIn(ns []*oln.Note) []*oln.Note {
	var out []*oln.Note
	for _, n := range ns {
		if oln.IsSource(n) {
			out = append(out, n)
		}
	}
	return out
}

// keepSources adds back the sources a cut-off list lost.
func keepSources(ns, srcs []*oln.Note) []*oln.Note {
	have := map[string]bool{}
	for _, n := range ns {
		have[n.ID] = true
	}
	for _, s := range srcs {
		if !have[s.ID] {
			ns = append(ns, s)
		}
	}
	return ns
}

// NewsFor: the news of these sources, read from the cells they're in: an
// area shows a source from any of its rings, so it shows that source's news
// too, not only for the nearest cells (Joop: Azeitão showed Rostos without
// its stories).
func (h *Home) NewsFor(ctx context.Context, srcs []*oln.Note) []NewsItem {
	var cells []string
	seen := map[string]bool{}
	want := map[string]bool{}
	for _, s := range srcs {
		want[s.ID] = true
		if !seen[s.Cell] {
			seen[s.Cell] = true
			cells = append(cells, s.Cell)
		}
	}
	all := h.News(ctx, cells)
	out := all[:0]
	for _, it := range all {
		if want[it.Src] { // a source that's gone (taken back, expired) takes its news along
			out = append(out, it)
		}
	}
	return out
}
