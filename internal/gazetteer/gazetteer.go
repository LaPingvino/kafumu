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

	"github.com/LaPingvino/kafumu/internal/geo"
)

//go:embed places.json
var placesJSON []byte

// Place is one city or area and the hashtags that name it.
type Place struct {
	Tag     string   `json:"tag"`
	Aliases []string `json:"aliases,omitempty"`
	Name    string   `json:"name"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Km      float64  `json:"km"`
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
}

// Gazetteer indexes places by the cells they cover.
type Gazetteer struct {
	Places []Place
	byCell map[string][]int
}

// Load returns the embedded gazetteer. It panics on bad data: the file is
// part of the build and covered by tests.
func Load() *Gazetteer {
	var places []Place
	if err := json.Unmarshal(placesJSON, &places); err != nil {
		panic("gazetteer: " + err.Error())
	}
	return New(places)
}

// New indexes places by every cell whose centre lies within the place radius
// (plus the place's own cell, so tiny places still cover something).
func New(places []Place) *Gazetteer {
	g := &Gazetteer{Places: places, byCell: map[string][]int{}}
	for i, p := range places {
		home := geo.Cell(p.Lat, p.Lon)
		g.byCell[home] = append(g.byCell[home], i)
		// Rings are 0.05° tall; widen in longitude by 1/cos(lat).
		latKm := geo.CellDeg * 111.32
		r := int(math.Ceil(p.Km/(latKm*math.Max(math.Cos(p.Lat*math.Pi/180), 0.2)))) + 1
		for k := 1; k <= r; k++ {
			for _, c := range geo.Ring(home, k) {
				la, lo := geo.Center(c)
				if distKm(p.Lat, p.Lon, la, lo) <= p.Km {
					g.byCell[c] = append(g.byCell[c], i)
				}
			}
		}
	}
	return g
}

// Tags returns the place hashtags for a set of cells, strongest first. A tag
// covering more of the requested cells, and unambiguous tags, weigh more.
func (g *Gazetteer) Tags(cells []string) []PlaceTag {
	hits := map[int]int{}
	for _, c := range cells {
		for _, i := range g.byCell[strings.ToLower(c)] {
			hits[i]++
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
			out = append(out, PlaceTag{Tag: t, Place: p.Name, Weight: round2(tw), Ambiguous: p.Ambiguous || j > 0 && len(t) <= 3})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
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
