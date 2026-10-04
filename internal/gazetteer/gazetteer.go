// Package gazetteer connects #geo cells to the place hashtags people already
// use (#amsterdam, #ams), so a cell has content from the existing hashtag
// ecosystem before any Kafumu user posts there. v0 is a small hand-made list;
// it will be replaced by data from the public geotags repo.
package gazetteer

import (
	_ "embed"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/LaPingvino/kafumu/internal/geo"
)

// places_geonames.json is vendored from github.com/LaPingvino/geotags
// (derived from GeoNames, CC BY 4.0). places.json holds hand-made entries
// (small towns, local nicknames) that override it by tag.
//
//go:embed places_geonames.json
var geonamesJSON []byte

//go:embed places.json
var placesJSON []byte

// towns.json (geotags): every place of 15k+ people, for search only.
//
//go:embed towns.json
var townsJSON []byte

type town struct {
	name, country string
	lat, lon      float64
	key           string // folded name
}

var towns = func() []town {
	var rows [][]any
	if err := json.Unmarshal(townsJSON, &rows); err != nil {
		panic("gazetteer: towns: " + err.Error())
	}
	out := make([]town, 0, len(rows))
	for _, r := range rows {
		if len(r) != 4 {
			continue
		}
		n, _ := r[0].(string)
		cc, _ := r[1].(string)
		la, _ := r[2].(float64)
		lo, _ := r[3].(float64)
		out = append(out, town{n, cc, la, lo, fold(n)})
	}
	return out
}()

//go:embed events.json
var eventsJSON []byte

// Event is a time-bound place tag: a conference or festival whose hashtag
// (#websummit) is local only while it runs.
type Event struct {
	Tag     string   `json:"tag"`
	Aliases []string `json:"aliases,omitempty"`
	Name    string   `json:"name"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Km      float64  `json:"km"`
	From    string   `json:"from"` // YYYY-MM-DD, local dates, inclusive
	To      string   `json:"to"`
	URL     string   `json:"url,omitempty"`
}

// EventTag is an event reachable from a cell around now.
type EventTag struct {
	Tag  string   `json:"tag"`
	Also []string `json:"also,omitempty"`
	Name string   `json:"name"`
	From string   `json:"from"`
	To   string   `json:"to"`
	URL  string   `json:"url,omitempty"`
	// Live is true during the event; before it the tag is shown as upcoming.
	Live bool `json:"live"`
}

// eventLead is how long before an event its tag starts showing up locally.
const eventLead = 45 * 24 * time.Hour

// Place is one city or area and the hashtags that name it.
type Place struct {
	Tag     string   `json:"tag"`
	Aliases []string `json:"aliases,omitempty"`
	Name    string   `json:"name"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Km      float64  `json:"km"`
	Country string   `json:"country,omitempty"`
	// Population breaks ties: the bigger city's tag comes first.
	Population int `json:"population,omitempty"`
	// Ambiguous marks tags that also name other places (#paris, #london ON).
	// Posts found through them rank lower on the device.
	Ambiguous bool `json:"ambiguous,omitempty"`
}

// PlaceTag is a hashtag reachable from a cell.
type PlaceTag struct {
	Tag       string  `json:"tag"`
	Place     string  `json:"place"`
	Weight    float64 `json:"weight"`
	Ambiguous bool    `json:"ambiguous,omitempty"`
	dist      float64 // km from the nearest looked-at cell to the place's centre
}

// Gazetteer indexes places by the cell of their centre and events by the
// cells they cover. Place lookups search outward from a cell, so thousands
// of cities cost a few thousand map entries, not a map per covered cell.
type Gazetteer struct {
	Places  []Place
	Events  []Event
	byHome  map[string][]int
	maxKm   float64
	evCells []map[string]bool

	mu   sync.Mutex
	near map[string][]int // memo for placesNear
}

// Load returns the embedded gazetteer. It panics on bad data: the file is
// part of the build and covered by tests.
func Load() *Gazetteer {
	var places, hand []Place
	if err := json.Unmarshal(geonamesJSON, &places); err != nil {
		panic("gazetteer: geonames: " + err.Error())
	}
	if err := json.Unmarshal(placesJSON, &hand); err != nil {
		panic("gazetteer: places: " + err.Error())
	}
	places = merge(places, hand)
	var events []Event
	if err := json.Unmarshal(eventsJSON, &events); err != nil {
		panic("gazetteer: events: " + err.Error())
	}
	g := New(places)
	g.Events = events
	for _, e := range events {
		set := map[string]bool{}
		for _, c := range cover(e.Lat, e.Lon, e.Km) {
			set[c] = true
		}
		g.evCells = append(g.evCells, set)
	}
	return g
}

// New indexes places by every cell whose centre lies within the place radius
// (plus the place's own cell, so tiny places still cover something).
func New(places []Place) *Gazetteer {
	g := &Gazetteer{Places: places, byHome: map[string][]int{}}
	for i, p := range places {
		h := geo.Cell(p.Lat, p.Lon)
		g.byHome[h] = append(g.byHome[h], i)
		g.maxKm = math.Max(g.maxKm, p.Km)
	}
	return g
}

// merge lets hand-made entries replace generated ones with the same tag (or
// an alias of it) and appends the rest.
func merge(gen, hand []Place) []Place {
	drop := map[string]bool{}
	for _, h := range hand {
		drop[h.Tag] = true
	}
	out := hand
	for _, p := range gen {
		if !drop[p.Tag] {
			out = append(out, p)
		}
	}
	return out
}

// placesNear returns the places whose radius covers cell's centre,
// memoised per cell (the set of cells people look at is small).
func (g *Gazetteer) placesNear(cell string) []int {
	g.mu.Lock()
	if v, ok := g.near[cell]; ok {
		g.mu.Unlock()
		return v
	}
	g.mu.Unlock()
	v := g.searchNear(cell)
	g.mu.Lock()
	if g.near == nil || len(g.near) > 50000 {
		g.near = map[string][]int{}
	}
	g.near[cell] = v
	g.mu.Unlock()
	return v
}

func (g *Gazetteer) searchNear(cell string) []int {
	lat, lon := geo.Center(cell)
	rings := int(math.Ceil(g.maxKm/(geo.CellDeg*111.32*math.Max(math.Cos(lat*math.Pi/180), 0.2)))) + 1
	var out []int
	for r := 0; r <= rings; r++ {
		for _, c := range geo.Ring(cell, r) {
			for _, i := range g.byHome[c] {
				p := g.Places[i]
				if distKm(p.Lat, p.Lon, lat, lon) <= math.Max(p.Km, 2.8) {
					out = append(out, i)
				}
			}
		}
	}
	return out
}

// cover returns the cell of (lat, lon) plus every cell whose centre lies
// within km of it, so tiny places still cover something.
func cover(lat, lon, km float64) []string {
	home := geo.Cell(lat, lon)
	out := []string{home}
	// Rings are 0.05° tall; widen in longitude by 1/cos(lat).
	latKm := geo.CellDeg * 111.32
	r := int(math.Ceil(km/(latKm*math.Max(math.Cos(lat*math.Pi/180), 0.2)))) + 1
	for k := 1; k <= r; k++ {
		for _, c := range geo.Ring(home, k) {
			la, lo := geo.Center(c)
			if distKm(lat, lon, la, lo) <= km {
				out = append(out, c)
			}
		}
	}
	return out
}

// EventsAt returns events touching any of cells that are upcoming (within
// eventLead) or running at now.
func (g *Gazetteer) EventsAt(cells []string, now time.Time) []EventTag {
	var out []EventTag
	for i, e := range g.Events {
		from, err1 := time.Parse("2006-01-02", e.From)
		to, err2 := time.Parse("2006-01-02", e.To)
		if err1 != nil || err2 != nil {
			continue
		}
		// Generous edges: dates are local and evenings run late.
		start, end := from.Add(-12*time.Hour), to.Add(36*time.Hour)
		if now.Before(start.Add(-eventLead)) || now.After(end) {
			continue
		}
		for _, c := range cells {
			if g.evCells[i][strings.ToLower(c)] {
				out = append(out, EventTag{Tag: e.Tag, Also: e.Aliases, Name: e.Name,
					From: e.From, To: e.To, URL: e.URL, Live: !now.Before(start)})
				break
			}
		}
	}
	return out
}

// Tags returns the place hashtags for a set of cells, strongest first. A tag
// covering more of the requested cells, and unambiguous tags, weigh more.
func (g *Gazetteer) Tags(cells []string) []PlaceTag {
	hits := map[int]int{}
	dist := map[int]float64{}
	for _, c := range cells {
		la, lo := geo.Center(strings.ToLower(c))
		for _, i := range g.placesNear(strings.ToLower(c)) {
			hits[i]++
			d := distKm(la, lo, g.Places[i].Lat, g.Places[i].Lon)
			if old, ok := dist[i]; !ok || d < old {
				dist[i] = d
			}
		}
	}
	var out []PlaceTag
	for i, n := range hits {
		p := g.Places[i]
		w := float64(n) / float64(len(cells))
		if p.Ambiguous {
			w *= 0.5
		}
		for j, t := range append([]string{p.Tag}, p.Aliases...) {
			tw := w
			if j > 0 {
				tw *= 0.8 // aliases are noisier (#la, #sf, #rio)
			}
			out = append(out, PlaceTag{Tag: t, Place: p.Name, Weight: round2(tw), Ambiguous: p.Ambiguous || j > 0 && len(t) <= 3, dist: dist[i]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		// Equal coverage: the place you're actually in beats the big city
		// whose radius reaches you (Barreiro before Lisbon).
		if out[i].dist != out[j].dist {
			return out[i].dist < out[j].dist
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

func distKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	rad := math.Pi / 180
	dLat, dLon := (lat2-lat1)*rad, (lon2-lon1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * R * math.Asin(math.Sqrt(a))
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

// Match is a search result for picking an area by name.
type Match struct {
	Tag     string  `json:"tag"`
	Name    string  `json:"name"`
	Country string  `json:"country,omitempty"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Km      float64 `json:"km"`
	Cell    string  `json:"cell"`
}

// Search finds places whose tag, alias or name starts with q (accents and
// spacing ignored), biggest first.
func (g *Gazetteer) Search(q string, limit int) []Match {
	q = fold(q)
	if len(q) < 2 {
		return nil
	}
	type hit struct {
		i     int
		exact bool
	}
	var hits []hit
	for i, p := range g.Places {
		keys := append([]string{p.Tag, fold(p.Name)}, p.Aliases...)
		for _, k := range keys {
			k = fold(k)
			if strings.HasPrefix(k, q) {
				hits = append(hits, hit{i, k == q})
				break
			}
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].exact != hits[b].exact {
			return hits[a].exact
		}
		return g.Places[hits[a].i].Population > g.Places[hits[b].i].Population
	})
	var out []Match
	seen := map[string]bool{}
	for _, h := range hits {
		if len(out) == limit {
			break
		}
		p := g.Places[h.i]
		seen[fold(p.Name)+p.Country] = true
		if p.Country == "" {
			seen[fold(p.Name)+"*"] = true // hand entries lack a country: a town of that name is the same place
		}
		out = append(out, Match{Tag: p.Tag, Name: p.Name, Country: p.Country, Lat: p.Lat, Lon: p.Lon, Km: p.Km, Cell: geo.Cell(p.Lat, p.Lon)})
	}
	// Then smaller towns (15k+), exact names first.
	for pass := 0; pass < 2 && len(out) < limit; pass++ {
		for _, t := range towns {
			if len(out) == limit {
				break
			}
			if (pass == 0 && t.key != q) || (pass == 1 && (t.key == q || !strings.HasPrefix(t.key, q))) || seen[t.key+t.country] || seen[t.key+"*"] && pass == 0 {
				continue
			}
			seen[t.key+t.country] = true
			out = append(out, Match{Name: t.name, Country: t.country, Lat: t.lat, Lon: t.lon, Km: 3, Cell: geo.Cell(t.lat, t.lon)})
		}
	}
	return out
}

// fold lowercases and strips accents and non-letters: "São Paulo" → "saopaulo".
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Near names the place closest to a cell, for a human heading ("Barreiro")
// instead of the cell code.
type Near struct {
	Name    string  `json:"name"`
	Country string  `json:"country,omitempty"`
	Km      float64 `json:"km"`
	// City is the big city this place lies in (a district of it), if any.
	City string `json:"city,omitempty"`
}

// cityKm: a nearest place this close to a big city's centre is part of it.
const cityKm = 8

// nearMaxKm: past this, a place name would mislead more than help.
const nearMaxKm = 20

// Nearest returns the named place (town of 15k+ or city) closest to the
// cell's centre, or nil if none is within nearMaxKm.
func (g *Gazetteer) Nearest(cell string) *Near {
	if !geo.Valid(cell) {
		return nil
	}
	lat, lon := geo.Center(cell)
	km := func(la, lo float64) float64 {
		x := (lo - lon) * math.Cos((la+lat)/2*math.Pi/180)
		return math.Hypot(la-lat, x) * 111.2
	}
	var best *Near
	consider := func(name, cc string, la, lo float64) {
		if d := km(la, lo); d <= nearMaxKm && (best == nil || d < best.Km) {
			best = &Near{Name: name, Country: cc, Km: math.Round(d*10) / 10}
		}
	}
	for _, t := range towns {
		consider(t.name, t.country, t.lat, t.lon)
	}
	city, cityD := "", math.Inf(1)
	for _, p := range g.Places {
		consider(p.Name, p.Country, p.Lat, p.Lon)
		if d := km(p.Lat, p.Lon); d < cityD && p.Population >= 100000 {
			city, cityD = p.Name, d
		}
	}
	if best != nil && cityD <= cityKm && city != best.Name {
		best.City = city
	}
	return best
}
