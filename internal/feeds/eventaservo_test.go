package feeds

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

const esICS = `BEGIN:VCALENDAR
BEGIN:VEVENT
UID:x@eventaservo.org
DTSTART;VALUE=DATE:20991205
DTEND;VALUE=DATE:20991207
DESCRIPTION:Kristnaska renkontiĝo\\n\\nhttps://eventaservo.org/e/8d59b1
LOCATION:Peregrinus CVJM Herberge\\, Langenstr. 37\\, 02826 Görlitz\\, Görli
 tz\\, DE
SUMMARY:Kristnaska Kunveno
END:VEVENT
END:VCALENDAR
`

// An Eventa Servo event: placed by its town, linked to its own page,
// tagged Esperanto.
func TestEventaServoEvent(t *testing.T) {
	g := gazetteer.Load()
	Locate = g.LocateArea
	defer func() { Locate = nil }()
	evs := importer.ParseICS(esICS)
	if len(evs) != 1 {
		t.Fatalf("parsed %d events", len(evs))
	}
	f := EventaServo("de")
	lookups := 0
	if lat, lon, _, ok := placeTown(context.Background(), nil, evs[0].Venue, &lookups); ok {
		evs[0].Lat, evs[0].Lon, evs[0].HasGeo = lat, lon, true
	}
	m := toMeetup(evs[0], f, hostOf(f.URL))
	if m == nil {
		t.Fatal("not placed")
	}
	if m.Link != "https://eventaservo.org/e/8d59b1" || m.Via != "eventaservo.org" || !strings.Contains(strings.Join(m.Tags, " "), "esperanto") {
		t.Fatalf("meetup = %+v", m)
	}
	lat, lon, _ := g.Locate("Görlitz", "DE")
	if want := geo.Cell(lat, lon); m.Cell != want {
		t.Fatalf("cell %s, want Görlitz's %s", m.Cell, want)
	}
}

// Venues as Eventa Servo writes them: the town is not always the part
// before the country.
func TestPlaceTown(t *testing.T) {
	Locate = gazetteer.Load().LocateArea
	defer func() { Locate = nil }()
	for _, v := range []string{"Alt Schmidd, Kardinal-Wendel-Str. 2, 66440 Blieskastel, Blieskastel, Sarlando, DE",
		"hotelo Ausspann en Heidenau, Großlugauer Str. 1, Dresden - Heidenau, DE", "Köln, DE", "02826 Görlitz, DE"} {
		n := 0
		if _, _, _, ok := placeTown(context.Background(), nil, v, &n); !ok {
			t.Errorf("not placed: %q", v)
		}
	}
}

// Live (KAFUMU_LIVE=1): Germany's real calendar imports upcoming events.
func TestEventaServoLive(t *testing.T) {
	if os.Getenv("KAFUMU_LIVE") == "" {
		t.Skip("set KAFUMU_LIVE=1 to fetch eventaservo.org")
	}
	Locate = gazetteer.Load().LocateArea
	defer func() { Locate = nil }()
	store := meetup.NewMemoryStore()
	r := Sync(context.Background(), []Feed{EventaServo("de")}, importer.New(), store, time.Now())
	t.Logf("eventaservo de: %s", r)
	if r.Saved == 0 {
		t.Fatalf("nothing saved: %s %v", r, r.Errors)
	}
}
