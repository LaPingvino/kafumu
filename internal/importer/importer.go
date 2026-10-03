// Package importer turns an event page (Luma, Meetup, Eventbrite, anything
// with schema.org Event JSON-LD) into the fields of a Kafumu meetup.
package importer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Event is what a page told us.
type Event struct {
	Title  string    `json:"title"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end,omitempty"`
	Venue  string    `json:"venue,omitempty"`
	Text   string    `json:"text,omitempty"`
	Link   string    `json:"link"`
	Lat    float64   `json:"lat,omitempty"`
	Lon    float64   `json:"lon,omitempty"`
	HasGeo bool      `json:"hasGeo"`
	// ATURI/ATCID identify an ATproto event record, when it came from one.
	ATURI string `json:"-"`
	ATCID string `json:"-"`
}

var ErrNoEvent = errors.New("importer: no event found on that page")

// Importer fetches pages safely and caches results.
type Importer struct {
	Client *http.Client
	mu     sync.Mutex
	cache  map[string]cached
}

type cached struct {
	ev  *Event
	err error
	at  time.Time
}

// New returns an Importer whose client refuses private, loopback and
// link-local addresses (no SSRF into App Engine's metadata server).
func New() *Importer {
	dialer := &net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || !public(ip) {
			return fmt.Errorf("importer: refusing to connect to %s", host)
		}
		return nil
	}}
	tr := &http.Transport{DialContext: dialer.DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second}
	return &Importer{
		Client: &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 4 || (req.URL.Scheme != "https" && req.URL.Scheme != "http") {
				return errors.New("importer: too many or odd redirects")
			}
			return nil
		}},
		cache: map[string]cached{},
	}
}

func public(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast())
}

// Fetch imports the event at raw.
func (im *Importer) Fetch(ctx context.Context, raw string) (*Event, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(raw) > 500 {
		return nil, errors.New("importer: not a web link")
	}
	key := u.String()
	im.mu.Lock()
	if c, ok := im.cache[key]; ok && time.Since(c.at) < 10*time.Minute {
		im.mu.Unlock()
		return c.ev, c.err
	}
	im.mu.Unlock()

	ev, err := im.fetch(ctx, key)
	im.mu.Lock()
	if len(im.cache) > 2000 {
		im.cache = map[string]cached{}
	}
	im.cache[key] = cached{ev, err, time.Now()}
	im.mu.Unlock()
	return ev, err
}

func (im *Importer) fetch(ctx context.Context, u string) (*Event, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kafumu event import; +https://github.com/LaPingvino/kafumu)")
	req.Header.Set("Accept", "text/html")
	resp, err := im.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("importer: page answered %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return nil, err
	}
	ev, err := Parse(string(body))
	if err != nil {
		return nil, err
	}
	if ev.Link == "" {
		ev.Link = resp.Request.URL.String()
	}
	return ev, nil
}

var ldRE = regexp.MustCompile(`(?is)<script[^>]+type=["']?application/ld\+json["']?[^>]*>(.*?)</script>`)

// Parse finds the first schema.org Event in a page's JSON-LD.
func Parse(page string) (*Event, error) {
	all := ParseAll(page)
	if len(all) == 0 {
		return nil, ErrNoEvent
	}
	return all[0], nil
}

// ParseAll returns every schema.org Event in a page's JSON-LD, including
// those inside ItemLists (Luma city and calendar pages) and @graphs.
func ParseAll(page string) []*Event {
	var out []*Event
	for _, m := range ldRE.FindAllStringSubmatch(page, -1) {
		var v any
		if json.Unmarshal([]byte(strings.TrimSpace(m[1])), &v) != nil {
			continue
		}
		walkEvents(v, func(o map[string]any) {
			if ev := toEvent(o); ev != nil {
				out = append(out, ev)
			}
		})
	}
	return out
}

func walkEvents(v any, fn func(map[string]any)) {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			walkEvents(e, fn)
		}
	case map[string]any:
		if isEvent(x["@type"]) {
			fn(x)
			return
		}
		for _, k := range []string{"@graph", "itemListElement", "item", "subEvent"} {
			if c, ok := x[k]; ok {
				walkEvents(c, fn)
			}
		}
	}
}

// Get fetches a page's body with the same safety rules as Fetch.
func (im *Importer) Get(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", errors.New("importer: not a web link")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Kafumu event import; +https://github.com/LaPingvino/kafumu)")
	resp, err := im.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("importer: %s answered %d", u.Host, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	return string(b), err
}

func isEvent(t any) bool {
	switch x := t.(type) {
	case string:
		return strings.HasSuffix(x, "Event")
	case []any:
		for _, e := range x {
			if isEvent(e) {
				return true
			}
		}
	}
	return false
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(html.UnescapeString(x))
	case []any:
		if len(x) > 0 {
			return str(x[0])
		}
	case map[string]any:
		return str(x["name"])
	}
	return ""
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		var f float64
		_, err := fmt.Sscan(x, &f)
		return f, err == nil
	}
	return 0, false
}

func parseTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04Z07:00", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func toEvent(o map[string]any) *Event {
	ev := &Event{Title: str(o["name"]), Start: parseTime(str(o["startDate"])), End: parseTime(str(o["endDate"])), Link: str(o["url"])}
	if ev.Title == "" || ev.Start.IsZero() {
		return nil
	}
	if d := str(o["description"]); d != "" {
		if len(d) > 900 {
			d = strings.ToValidUTF8(d[:900], "") + "…"
		}
		ev.Text = d
	}
	locs, _ := o["location"].([]any)
	if l, ok := o["location"].(map[string]any); ok {
		locs = []any{l}
	}
	for _, li := range locs {
		l, ok := li.(map[string]any)
		if !ok {
			continue
		}
		var parts []string
		if n := str(l["name"]); n != "" {
			parts = append(parts, n)
		}
		if a, ok := l["address"].(map[string]any); ok {
			for _, k := range []string{"streetAddress", "addressLocality"} {
				if s := str(a[k]); s != "" && !contains(parts, s) {
					parts = append(parts, s)
				}
			}
		} else if s := str(l["address"]); s != "" && !contains(parts, s) {
			parts = append(parts, s)
		}
		if ev.Venue == "" {
			ev.Venue = strings.Join(parts, ", ")
		}
		geo, _ := l["geo"].(map[string]any)
		if geo == nil {
			geo = l
		}
		lat, ok1 := num(geo["latitude"])
		lon, ok2 := num(geo["longitude"])
		if ok1 && ok2 && (lat != 0 || lon != 0) && !ev.HasGeo {
			ev.Lat, ev.Lon, ev.HasGeo = lat, lon, true
		}
	}
	if len(ev.Venue) > 200 {
		ev.Venue = strings.ToValidUTF8(ev.Venue[:200], "")
	}
	return ev
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
