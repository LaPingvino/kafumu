package geo

import "testing"

func TestCell(t *testing.T) {
	cases := []struct {
		lat, lon float64
		want     string
	}{
		{52.3676, 4.9041, "9f469w"},    // Amsterdam
		{52.3731, 4.8926, "9f469v"},    // Dam square
		{-23.5505, -46.6333, "588mc9"}, // São Paulo
		{0, 0, "6fg222"},
		{89.99999, 179.99999, "cvxxxx"},
		{-90, -180, "222222"},
	}
	for _, c := range cases {
		if got := Cell(c.lat, c.lon); got != c.want {
			t.Errorf("Cell(%v,%v) = %s, want %s", c.lat, c.lon, got, c.want)
		}
		if !Valid(Cell(c.lat, c.lon)) {
			t.Errorf("Cell(%v,%v) not Valid", c.lat, c.lon)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, c := range []string{"9f469w", "588mc9", "6fg222", "cvxxxx"} {
		lat, lon := Center(c)
		if got := Cell(lat, lon); got != c {
			t.Errorf("Cell(Center(%s)) = %s", c, got)
		}
	}
}

func TestRings(t *testing.T) {
	if n := len(Ring("9f469w", 1)); n != 8 {
		t.Errorf("ring 1 has %d cells, want 8", n)
	}
	if n := len(Ring("9f469w", 2)); n != 16 {
		t.Errorf("ring 2 has %d cells, want 16", n)
	}
	all := Rings("9f469w", 2)
	if len(all) != 25 || all[0] != "9f469w" {
		t.Errorf("Rings = %v", all)
	}
	// Antimeridian: neighbours of a cell at lon≈179.97 wrap to lon≈-179.97.
	east := Cell(0, 179.97)
	found := false
	for _, c := range Ring(east, 1) {
		if c == Cell(0, -179.97) {
			found = true
		}
	}
	if !found {
		t.Errorf("ring around %s does not wrap the antimeridian: %v", east, Ring(east, 1))
	}
}
