package gazetteer

import (
	"testing"
	"time"

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
