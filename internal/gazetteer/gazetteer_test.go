package gazetteer

import (
	"testing"

	"github.com/LaPingvino/kafumu/internal/geo"
)

func TestLoadAndLookup(t *testing.T) {
	g := Load()
	if len(g.Places) < 50 {
		t.Fatalf("only %d places", len(g.Places))
	}
	seen := map[string]bool{}
	for _, p := range g.Places {
		for _, tag := range append([]string{p.Tag}, p.Aliases...) {
			if seen[tag] {
				t.Errorf("duplicate tag %q", tag)
			}
			seen[tag] = true
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

	// The middle of the North Sea has no place tags.
	if got := g.Tags([]string{geo.Cell(54.5, 3.0)}); len(got) != 0 {
		t.Errorf("North Sea tags = %+v", got)
	}

	// Ambiguous places weigh less than unambiguous ones at full coverage.
	for _, pt := range g.Tags([]string{geo.Cell(48.857, 2.352)}) {
		if pt.Tag == "paris" && (pt.Weight != 0.5 || !pt.Ambiguous) {
			t.Errorf("paris = %+v", pt)
		}
	}
}
