package gazetteer

import (
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/geo"
)

func TestLoadAndLookup(t *testing.T) {
	g := Load()
	if len(g.Places) < 5000 {
		t.Fatalf("only %d places", len(g.Places))
	}
	// A tag naming several places (Lincoln, Santa Cruz) must be flagged.
	byTag := map[string][]Place{}
	for _, p := range g.Places {
		byTag[p.Tag] = append(byTag[p.Tag], p)
	}
	for tag, ps := range byTag {
		if len(ps) > 1 && !ps[0].Ambiguous {
			t.Errorf("duplicate tag %q not marked ambiguous", tag)
		}
	}

	dam := geo.Cell(52.3731, 4.8926)
	tags := g.Tags(geo.Rings(dam, 1))
	if len(tags) == 0 || tags[0].Tag != "amsterdam" {
		t.Fatalf("Dam square tags = %+v", tags)
	}
	if tags[0].Weight != 1 {
		t.Errorf("amsterdam weight = %v, want 1 (all 9 cells covered)", tags[0].Weight)
	}
	if !hasTag(tags, "mokum") {
		t.Errorf("curated alias missing: %+v", tags[:3])
	}

	// The middle of the North Sea has no place tags.
	if got := g.Tags([]string{geo.Cell(54.5, 3.0)}); len(got) != 0 {
		t.Errorf("North Sea tags = %+v", got)
	}

	// Ambiguous places weigh less than unambiguous ones at full coverage.
	for _, pt := range g.Tags([]string{geo.Cell(48.857, 2.352)}) { // #paris: also Paris Hilton
		if pt.Tag == "paris" && (pt.Weight != 0.5 || !pt.Ambiguous) {
			t.Errorf("paris = %+v", pt)
		}
	}
}

func TestEvents(t *testing.T) {
	g := Load()
	arena := []string{geo.Cell(38.768, -9.094)}
	barreiro := []string{geo.Cell(38.663, -9.072)}
	day := func(s string) time.Time { tm, _ := time.Parse("2006-01-02 15:04", s); return tm }

	if ev := g.EventsAt(arena, day("2026-11-10 10:00")); len(ev) != 1 || ev[0].Tag != "websummit" || !ev[0].Live {
		t.Errorf("during: %+v", ev)
	}
	if ev := g.EventsAt(arena, day("2026-10-03 10:00")); len(ev) != 1 || ev[0].Live {
		t.Errorf("five weeks before: %+v", ev)
	}
	if ev := g.EventsAt(arena, day("2026-11-20 10:00")); len(ev) != 0 {
		t.Errorf("after: %+v", ev)
	}
	if ev := g.EventsAt(barreiro, day("2026-11-10 10:00")); len(ev) != 0 {
		t.Errorf("Barreiro is not at the venue: %+v", ev)
	}
	if tags := g.Tags(barreiro); len(tags) == 0 || tags[0].Tag != "barreiro" {
		t.Errorf("Barreiro tags = %+v", tags)
	}
}

func hasTag(ts []PlaceTag, tag string) bool {
	for _, t := range ts {
		if t.Tag == tag {
			return true
		}
	}
	return false
}

func BenchmarkTags(b *testing.B) {
	g := Load()
	cells := geo.Rings(geo.Cell(38.72, -9.14), 2)
	for i := 0; i < b.N; i++ {
		g.Tags(cells)
	}
}

func TestSearch(t *testing.T) {
	g := Load()
	if m := g.Search("Lisbo", 5); len(m) == 0 || m[0].Tag != "lisbon" {
		t.Errorf("Lisbo → %+v", m)
	}
	if m := g.Search("sao pau", 3); len(m) == 0 || m[0].Tag != "saopaulo" {
		t.Errorf("sao pau → %+v", m)
	}
	if m := g.Search("barreiro", 3); len(m) == 0 || m[0].Cell != "8ccgmh" && m[0].Tag != "barreiro" {
		t.Errorf("barreiro → %+v", m)
	}
	if m := g.Search("Ede", 8); !hasMatch(m, "Ede", "NL") {
		t.Errorf("Ede → %+v", m)
	}
	if m := g.Search("x", 3); m != nil {
		t.Errorf("one letter searched: %+v", m)
	}
}

func hasMatch(ms []Match, name, cc string) bool {
	for _, m := range ms {
		if m.Name == name && m.Country == cc {
			return true
		}
	}
	return false
}

// Around's heading: the nearest named place, not just the big city.
func TestNearest(t *testing.T) {
	g := Load()
	for _, c := range []struct {
		lat, lon   float64
		want, city string
	}{
		{38.663, -9.072, "Barreiro", ""}, // Joop's town, across the river from Lisbon
		{52.040, 5.665, "Ede", ""},
		{52.085, 5.622, "Lunteren", ""}, // Joop: showed as Bennekom while towns < 15k were missing
		{38.735, -9.135, "Lisbon", ""},  // the city leads; Areeiro, Graça… go in the cloud
	} {
		if n := g.Nearest(geo.Cell(c.lat, c.lon)); n == nil || n.Name != c.want || n.City != c.city {
			t.Errorf("Nearest(%v,%v) = %+v, want %s", c.lat, c.lon, n, c.want)
		}
	}
	if n := g.Nearest(geo.Cell(0.01, 0.01)); n != nil {
		t.Errorf("open sea named %+v", n)
	}
}

func TestCountPlaces(t *testing.T) {
	g := Load()
	if n := g.CountPlaces([]string{"london", "Paris", "berlin", "coffee", "websummit"}); n != 3 {
		t.Errorf("CountPlaces = %d, want 3", n)
	}
}

// Eventa Servo places events by "…, City, CC": Görlitz in Germany, and
// back from its cell to its country.
func TestLocateAndCountry(t *testing.T) {
	g := Load()
	lat, lon, ok := g.Locate("Görlitz", "de")
	if !ok || lat < 51 || lat > 51.3 || lon < 14.8 || lon > 15.1 {
		t.Fatalf("Görlitz, DE = %v %v %v", lat, lon, ok)
	}
	// The names Eventa Servo uses: Esperanto, local, English.
	for _, c := range [][2]string{{"Parizo", "FR"}, {"Varsovio", "PL"}, {"Munkeno", "DE"}, {"München", "DE"}, {"Munich", "DE"},
		{"Romo", "IT"}, {"Bjalistoko", "PL"}, {"Madrido", "ES"}, {"Kraków", "PL"}, {"Maceió", "BR"}} {
		if _, _, ok := g.Locate(c[0], c[1]); !ok {
			t.Errorf("%s, %s not found", c[0], c[1])
		}
	}
	if lat, _, _ := g.Locate("Parizo", "FR"); lat < 48.7 || lat > 49 {
		t.Errorf("Parizo is not Paris: %v", lat)
	}
	if _, _, ok := g.Locate("Görlitz", "BR"); ok {
		t.Fatal("Görlitz found in Brazil")
	}
	if cc := g.CountryOf(geo.Cell(lat, lon)); cc != "DE" {
		t.Fatalf("country of Görlitz's cell = %q", cc)
	}
	if cc := g.CountryOf(geo.Cell(38.72, -9.14)); cc != "PT" {
		t.Fatalf("country of Lisbon's cell = %q", cc)
	}
}

// Events known only as "in Paris" reach all of Paris (Joop: bigger blocks
// for bigger cities): Paris has a city-sized area, and from La Défense its
// centre cell is among the cities covering you; a small town has none.
func TestCityArea(t *testing.T) {
	g := Load()
	lat, lon, km, ok := g.LocateArea("Parizo", "FR")
	if !ok || km < 5 {
		t.Fatalf("Parizo: %v %v km=%v ok=%v", lat, lon, km, ok)
	}
	centre := geo.Cell(lat, lon)
	defense := geo.Cell(48.892, 2.236)
	found := false
	for _, c := range g.CityCentres([]string{defense}) {
		found = found || c == centre
	}
	if !found {
		t.Fatalf("La Défense (%s) doesn't reach Paris's centre %s: %v", defense, centre, g.CityCentres([]string{defense}))
	}
	if _, _, km, ok := g.LocateArea("Görlitz", "DE"); !ok || km != 0 {
		t.Fatalf("Görlitz should be placed without an area, km=%v", km)
	}
}

// Town tags come accented and plain (Bluesky treats #évora and #evora as
// different tags).
func TestTownTagsAccents(t *testing.T) {
	g := Load()
	c := geo.Cell(38.571, -7.909) // Évora
	tags := g.TownTags(geo.Rings(c, 1), 10)
	has := map[string]bool{}
	for _, x := range tags {
		has[x] = true
	}
	if !has["évora"] || !has["evora"] {
		t.Fatalf("tags around Évora: %v", tags)
	}
}
