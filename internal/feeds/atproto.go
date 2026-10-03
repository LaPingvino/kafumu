package feeds

import (
	"context"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/importer"
)

// ATproto events (community.lexicon.calendar.event, as written by Smoke
// Signal) live in their authors' own PDSes. A "smokesignal" feed reads the
// event links from a Smoke Signal page, resolves each author's DID to their
// PDS and fetches the record itself — the user's own data, read in place.

var ssLinkRE = regexp.MustCompile(`/(did:plc:[a-z0-9]{24})/([a-z0-9]{13})`)

const maxATEvents = 60

type eventRecord struct {
	Value struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		StartsAt    string `json:"startsAt"`
		EndsAt      string `json:"endsAt"`
		Mode        string `json:"mode"`
		Locations   []struct {
			Type      string `json:"$type"`
			Latitude  any    `json:"latitude"`
			Longitude any    `json:"longitude"`
			Name      string `json:"name"`
			Street    string `json:"street"`
			Locality  string `json:"locality"`
		} `json:"locations"`
		URIs []struct {
			URI string `json:"uri"`
		} `json:"uris"`
	} `json:"value"`
}

func atprotoEvents(ctx context.Context, im *importer.Importer, page string) []*importer.Event {
	body, err := im.Get(ctx, page)
	if err != nil {
		return nil
	}
	body = strings.ReplaceAll(body, "&#x2f;", "/")
	seen := map[string]bool{}
	pds := map[string]string{}
	var out []*importer.Event
	for _, m := range ssLinkRE.FindAllStringSubmatch(body, -1) {
		did, rkey := m[1], m[2]
		if seen[did+rkey] || len(seen) >= maxATEvents {
			continue
		}
		seen[did+rkey] = true
		host, ok := pds[did]
		if !ok {
			host = resolvePDS(ctx, im, did)
			pds[did] = host
		}
		if host == "" {
			continue
		}
		raw, err := im.Get(ctx, host+"/xrpc/com.atproto.repo.getRecord?"+url.Values{
			"repo": {did}, "collection": {"community.lexicon.calendar.event"}, "rkey": {rkey}}.Encode())
		if err != nil {
			continue
		}
		if ev := toATEvent(raw, "https://smokesignal.events/"+did+"/"+rkey); ev != nil {
			out = append(out, ev)
		}
	}
	return out
}

func toATEvent(raw, link string) *importer.Event {
	var r eventRecord
	if json.Unmarshal([]byte(raw), &r) != nil {
		return nil
	}
	v := r.Value
	start, _ := time.Parse(time.RFC3339, v.StartsAt)
	if v.Name == "" || start.IsZero() || strings.HasSuffix(v.Mode, "#virtual") {
		return nil
	}
	end, _ := time.Parse(time.RFC3339, v.EndsAt)
	ev := &importer.Event{Title: v.Name, Start: start, End: end, Link: link}
	if len(v.Description) > 900 {
		v.Description = strings.ToValidUTF8(v.Description[:900], "") + "…"
	}
	ev.Text = v.Description
	for _, l := range v.Locations {
		switch {
		case strings.HasSuffix(l.Type, "location.geo"):
			lat, ok1 := num(l.Latitude)
			lon, ok2 := num(l.Longitude)
			if ok1 && ok2 && !ev.HasGeo {
				ev.Lat, ev.Lon, ev.HasGeo = lat, lon, true
			}
			if l.Name != "" && ev.Venue == "" {
				ev.Venue = l.Name
			}
		case strings.HasSuffix(l.Type, "location.address") && ev.Venue == "":
			ev.Venue = strings.Trim(strings.Join([]string{l.Name, l.Street, l.Locality}, ", "), ", ")
		}
	}
	if !ev.HasGeo {
		return nil // no coordinates, no cell; geocoding addresses is a later step
	}
	return ev
}

// resolvePDS finds a did:plc's PDS through plc.directory.
func resolvePDS(ctx context.Context, im *importer.Importer, did string) string {
	raw, err := im.Get(ctx, "https://plc.directory/"+did)
	if err != nil {
		return ""
	}
	var doc struct {
		Service []struct {
			ID, Type, ServiceEndpoint string
		} `json:"service"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return ""
	}
	for _, s := range doc.Service {
		if s.ID == "#atproto_pds" && strings.HasPrefix(s.ServiceEndpoint, "https://") {
			return strings.TrimSuffix(s.ServiceEndpoint, "/")
		}
	}
	return ""
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}
