// Package handler holds Kafumu's HTTP handlers.
package handler

import (
	"cloud.google.com/go/datastore"
	"context"
	"encoding/json"
	"github.com/LaPingvino/kafumu/internal/brand"
	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/report"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/atp"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/langs"
	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/meetup"
	"github.com/LaPingvino/kafumu/internal/oln"
)

const (
	// maxBundleCells bounds a bundle request to rings 0–2 (5×5).
	maxBundleCells = 25
	// maxPlaceTags bounds upstream searches for gazetteer place tags per bundle.
	maxPlaceTags = 4
)

type Home struct {
	// Brands: the faces of Kafumu on other hosts (bahais.in…); nil = none.
	Brands *brand.Store
	// Biz: business accounts, for acting as one (acting.go).
	Biz  *business.Store
	bizc bizCache
	// Cache: the shared cache (memcache on App Engine), for small notes
	// like the countries looked at (countries.go).
	Cache cache.Cache
	seen  countryLog
	Cfg   *config.Config
	Tmpl  *template.Template
	Bsky  *bsky.Client
	Gaz   *gazetteer.Gazetteer
	// Notes, if set, are the OLN local messages included in bundles.
	Notes *oln.Service
	// FollowHandle, if set, serves kafumu.com/@name links.
	FollowHandle func(w http.ResponseWriter, r *http.Request, name string)
	// Reports, if set, hides what moderators hid (posts, people) from bundles.
	Reports *report.Service
	// ATproto, if set, lets people connect their own ATproto account.
	ATproto *atp.Service
	// Meetups and Accounts, if set, are included in bundles.
	Meetups  *meetup.Service
	Accounts *account.Service
	// DB holds site settings (footer); MakerLive says whether a username's
	// kafumu.com/@name link is on.
	DB        *datastore.Client
	MakerLive func(ctx context.Context, name string) bool
	foot      footerCache
}

// page is the data every full page gets.
type page struct {
	Brand string
	// BrandInfo: the brand of this host (its colour, tagline, main button,
	// default tags), nil on plain Kafumu.
	BrandInfo *brand.Brand
	// BrandAdmin: the signed-in user manages this host's brand (/brand).
	BrandAdmin bool
	Title      string
	Tab        string // active view-switcher tab
	Cell       string
	Lang       string
	V          string // asset version, so a deploy never mixes old and new JS
	// MovedTo is set on a legacy origin: the canonical origin to move to.
	MovedTo string
	// Passkeys is true when passkeys work on this host.
	Passkeys bool
	// ATproto is true when connecting an ATproto account works here.
	ATproto bool
	Langs   []locale.Lang
	User    *account.User
	// Acting: the business you're using Kafumu as (nil: yourself).
	Acting *business.Business
	// Now: the time of this request (for Live checks in templates).
	Now time.Time
	// Maker, Contact, ContactText: footer "Contact the maker" (see footer.go).
	Maker, Contact, ContactText string
	// JS holds the "js." strings for client-side code.
	JS map[string]string
}

// newPage fills the common fields. The language comes from the "lang" cookie
// (set by the switcher, client-side) or Accept-Language; nothing is stored.
func (h *Home) newPage(r *http.Request, title string) page {
	choice := ""
	if c, err := r.Cookie("lang"); err == nil {
		choice = c.Value
	}
	lang := locale.Pick(choice, r.Header.Get("Accept-Language"))
	moved := ""
	if canon := h.Cfg.Origin; canon != "" && len(h.Cfg.LegacyOrigins) > 0 && !strings.HasSuffix(canon, "://"+r.Host) {
		moved = canon
	}
	maker, contact, contactText := h.footer(r.Context())
	var acting *business.Business
	if u := UserFrom(r.Context()); u != nil {
		acting = h.acting(r.Context(), r, u.ID)
	}
	name, bi := h.Cfg.Brand, h.Brands.For(r.Context(), r.Host)
	if bi != nil {
		name = bi.Name
	}
	brandAdmin := h.isBrandAdmin(r.Context(), bi, UserFrom(r.Context()))
	return page{Acting: acting, Now: time.Now(), Brand: name, BrandInfo: bi, BrandAdmin: brandAdmin, Title: title, Lang: lang, V: h.Cfg.Version, Langs: locale.Langs(), MovedTo: moved,
		Passkeys: h.Cfg.Passkeys && moved == "", ATproto: h.ATproto != nil && moved == "",
		User: UserFrom(r.Context()), JS: locale.Prefix(lang, "js."), Maker: maker, Contact: contact, ContactText: contactText}
}

// ShowHome renders the shell; the cell is computed on the device and the list
// is filled from /bundle, so the server never sees coordinates.
func (h *Home) ShowHome(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/@") && h.FollowHandle != nil {
		h.FollowHandle(w, r, strings.TrimPrefix(r.URL.Path, "/@"))
		return
	}
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	cell := strings.ToLower(r.URL.Query().Get("cell"))
	if !geo.Valid(cell) {
		cell = ""
	}
	p := homePage{page: h.newPage(r, ""), LangNames: langs.Names, LangTags: langs.Tags, From1: langs.From1}
	p.Tab, p.Cell = "around", cell
	if u := p.User; u != nil {
		p.MyLangs, p.MyTags = u.Langs, u.Tags
	}
	// First visit without a cell (desktop, mostly): App Engine's own city
	// guess from the IP, turned into a cell for this page only (never
	// stored or logged), so the device can ask "Are you in …?".
	if cell == "" && !IsBot(r) && h.Gaz != nil {
		c := guessCell(r.Header.Get("X-Appengine-Citylatlong"))
		if c == "" {
			// No coordinates for this connection: try the city name.
			if city := strings.TrimSpace(r.Header.Get("X-Appengine-City")); city != "" && city != "?" {
				cc := strings.ToUpper(r.Header.Get("X-Appengine-Country"))
				for _, m := range h.Gaz.Search(city, 5) {
					if cc == "" || m.Country == "" || strings.EqualFold(m.Country, cc) {
						c = m.Cell
						break
					}
				}
			}
		}
		if c != "" {
			p.GuessCell = c
			if n := h.Gaz.Nearest(c); n != nil {
				p.GuessName = n.Name
			}
			w.Header().Set("Cache-Control", "private")
		}
	}
	h.render(w, "home.html", p)
}

// homePage adds what the device needs to rank people: your own public
// languages and interests (you already see them) and language names.
type homePage struct {
	page
	MyLangs, MyTags []string
	LangNames       map[string]string
	LangTags        map[string]langs.Tag
	From1           map[string]string
	// GuessCell/GuessName: the area App Engine guesses from the IP.
	GuessCell, GuessName string
}

// guessCell turns "52.040000,5.665000" into a cell; "" if absent or 0,0.
func guessCell(latlon string) string {
	parts := strings.Split(latlon, ",")
	if len(parts) != 2 {
		return ""
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || (lat == 0 && lon == 0) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return ""
	}
	return geo.Cell(lat, lon)
}

// ShowAbout renders the static explanation page.
func (h *Home) ShowAbout(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "nav.about"), "about"
	h.render(w, "about.html", p)
}

func (h *Home) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Cookie, Accept-Language")
	if err := h.Tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// bundle is everything public and current for the requested cells. Ranking
// happens on the device against tags and pairs the server never sees.
type bundle struct {
	Cells []string `json:"cells"`
	// Places are the human hashtags that name this area (#amsterdam), with
	// weights the client uses to rank posts found through them.
	Places []gazetteer.PlaceTag `json:"places"`
	// Near names the place closest to the first cell, for the heading.
	Near *gazetteer.Near `json:"near,omitempty"`
	// Events are conferences and festivals here, upcoming or running.
	Events []gazetteer.EventTag `json:"events"`
	// Notes are local OLN messages (proof of work, no account), and
	// RequiredBits is what a new message in the first cell must carry.
	Notes        []*oln.Note `json:"notes"`
	RequiredBits int         `json:"requiredBits"`
	// People who chose to be discoverable here, unranked (the device ranks).
	People []account.Person `json:"people"`
	// Meetups hosted on Kafumu in these cells, soonest first.
	Meetups []*meetup.Meetup `json:"meetups"`
	Posts   []bsky.Post      `json:"posts"`
}

// ShowCard renders the "my card" editor; the card itself lives on the device.
func (h *Home) ShowCard(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "card.title"), "card"
	h.render(w, "card.html", p)
}

// ShowConnect renders "show my code"; the code is made on the device.
func (h *Home) ShowConnect(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "connect.title"), "connect"
	h.render(w, "connect.html", p)
}

// ShowBadge renders a printable long-lived connect code.
func (h *Home) ShowBadge(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "badge.title"), "connect"
	h.render(w, "badge.html", p)
}

// ShowAccept renders the page a scanned code opens. The code itself is in
// the URL fragment, which browsers never send to the server.
func (h *Home) ShowAccept(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "accept.title"), "connect"
	w.Header().Set("Referrer-Policy", "no-referrer")
	h.render(w, "accept.html", p)
}

// ShowContacts renders the contacts list; contacts live on the device.
func (h *Home) ShowContacts(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "contacts.title"), "contacts"
	h.render(w, "contacts.html", p)
}

// ShowMove renders the page a scanned move code opens on the old device.
func (h *Home) ShowMove(w http.ResponseWriter, r *http.Request) {
	p := h.newPage(r, "")
	p.Title, p.Tab = locale.T(p.Lang, "move.title"), "contacts"
	w.Header().Set("Referrer-Policy", "no-referrer")
	h.render(w, "move.html", p)
}

// infoPage is a static-ish page with the money/contact links.
type infoPage struct {
	page
	PayPal, Bunq, BCH, Liberapay, Stripe, Contact string
}

// Info renders /patrons, /for-cafes and /privacy.
func (h *Home) Info(name, titleKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := infoPage{page: h.newPage(r, ""), PayPal: h.Cfg.PayPal, Bunq: h.Cfg.Bunq, BCH: h.Cfg.BCH, Liberapay: h.Cfg.Liberapay, Stripe: h.Cfg.Stripe, Contact: h.Cfg.Contact}
		p.Title, p.Tab = locale.T(p.Lang, titleKey), "about"
		h.render(w, name, p)
	}
}

// ShowImport renders /import: the receiving end of a move from a legacy
// origin. It accepts device data only from the configured legacy origins.
func (h *Home) ShowImport(w http.ResponseWriter, r *http.Request) {
	p := struct {
		page
		From []string
	}{page: h.newPage(r, ""), From: h.Cfg.LegacyOrigins}
	p.Title = locale.T(p.Lang, "move.title")
	h.render(w, "import.html", p)
}

// CanonicalHost redirects www.<domain> to the canonical origin.
func (h *Home) CanonicalHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if canon := h.Cfg.Origin; strings.HasPrefix(r.Host, "www.") && strings.HasSuffix(canon, "://"+strings.TrimPrefix(r.Host, "www.")) {
			code := http.StatusMovedPermanently
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				code = http.StatusPermanentRedirect
			}
			http.Redirect(w, r, canon+r.URL.RequestURI(), code)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Funcs are the template functions. Translations come from our own files,
// so they may contain markup.
var Funcs = template.FuncMap{
	"t":     func(lang, key string) template.HTML { return template.HTML(locale.T(lang, key)) },
	"ts":    locale.T,
	"venue": meetup.CleanVenue,
	// bskyURL: at://did/app.bsky.feed.post/rkey → its bsky.app page.
	"bskyURL": func(uri string) string {
		p := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
		if len(p) == 3 {
			return "https://bsky.app/profile/" + p[0] + "/post/" + p[2]
		}
		return "https://bsky.app"
	},
	// host shows a link by its site: "luma.com".
	"host": func(link string) string {
		if u, err := url.Parse(link); err == nil && u.Host != "" {
			return strings.TrimPrefix(u.Host, "www.")
		}
		return link
	},
	"list":     func(xs ...string) []string { return xs },
	"langs":    func() []langs.Lang { return langs.All },
	"langName": func(code string) string { return langs.Names[code] },
	"has": func(xs []string, x string) bool {
		for _, v := range xs {
			if v == x {
				return true
			}
		}
		return false
	},
}

// Places handles GET /places?q=lis: pick an area by name.
func (h *Home) Places(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	ms := []gazetteer.Match{}
	if h.Gaz != nil {
		if found := h.Gaz.Search(r.URL.Query().Get("q"), 8); found != nil {
			ms = found
		}
	}
	json.NewEncoder(w).Encode(ms)
}

// Bundle handles GET /bundle?cells=a,b,c. Bundles are per cell, not per user.
func (h *Home) Bundle(w http.ResponseWriter, r *http.Request) {
	if IsBot(r) {
		http.Error(w, "not for robots", http.StatusForbidden)
		return
	}
	var cells []string
	seen := map[string]bool{}
	for _, c := range strings.Split(strings.ToLower(r.URL.Query().Get("cells")), ",") {
		if geo.Valid(c) && !seen[c] && len(cells) < maxBundleCells {
			seen[c] = true
			cells = append(cells, c)
		}
	}
	if len(cells) == 0 {
		http.Error(w, "cells: want comma-separated 6-char #geo cells", http.StatusBadRequest)
		return
	}
	h.noteCountry(r.Context(), cells[0])

	b := bundle{Cells: cells, Places: []gazetteer.PlaceTag{}, Events: []gazetteer.EventTag{}, Meetups: []*meetup.Meetup{}, People: []account.Person{}, Notes: []*oln.Note{}, Posts: []bsky.Post{}}
	if h.Notes != nil {
		if ns, err := h.Notes.InCells(r.Context(), cells); err != nil {
			log.Printf("bundle: notes: %v", err)
		} else if ns != nil {
			if len(ns) > oln.PerBundle {
				ns = ns[:oln.PerBundle]
			}
			b.Notes = ns
		}
		b.RequiredBits = h.Notes.RequiredFor(r.Context(), cells[0])
	}
	if h.Accounts != nil {
		if ps, err := h.Accounts.People(r.Context(), cells); err != nil {
			log.Printf("bundle: people: %v", err)
		} else if ps != nil {
			for _, p := range ps {
				if h.Reports == nil || !h.Reports.Hidden(r.Context(), "person", p.Name) {
					b.People = append(b.People, p)
				}
			}
		}
	}
	if h.Biz != nil { // findable businesses (76c): one list, kept a minute
		in := map[string]bool{}
		for _, c := range cells {
			in[c] = true
		}
		now := time.Now()
		for _, bz := range h.Biz.Findable(r.Context(), now) {
			if in[bz.Cell] && bz.Username != "" && (h.Reports == nil || !h.Reports.Hidden(r.Context(), "person", bz.Username)) {
				b.People = append(b.People, account.Person{Name: bz.Username, Bio: bz.Bio, Where: bz.Where, Langs: bz.Langs, Tags: bz.Tags,
					Cell: bz.Cell, Patron: bz.Live(now), Biz: true})
			}
		}
	}
	if h.Meetups != nil {
		if ms, err := h.Meetups.InCells(r.Context(), cells); err != nil {
			log.Printf("bundle: meetups: %v", err)
		} else if ms != nil {
			b.Meetups = ms
		}
	}
	tags := make([]string, 0, len(cells)+maxPlaceTags)
	for _, c := range cells {
		tags = append(tags, geo.Tag(c))
	}
	if h.Gaz != nil {
		if ev := h.Gaz.EventsAt(cells, time.Now()); ev != nil {
			b.Events = ev
		}
		for _, e := range b.Events {
			tags = append(tags, e.Tag)
		}
		b.Near = h.Gaz.Nearest(cells[0])
		if pt := h.Gaz.Tags(cells); pt != nil {
			b.Places = pt
		}
		for i, pt := range b.Places {
			if i == maxPlaceTags {
				break
			}
			tags = append(tags, pt.Tag)
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	uris := map[string]bool{}
	for _, tag := range tags {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			posts := h.Bsky.SearchTag(r.Context(), tag, 25)
			mu.Lock()
			defer mu.Unlock()
			for _, p := range posts {
				if !uris[p.URI] && (h.Reports == nil || !h.Reports.Hidden(r.Context(), "post", p.URI)) {
					uris[p.URI] = true
					b.Posts = append(b.Posts, p)
				}
			}
		}(tag)
	}
	wg.Wait()
	sort.Slice(b.Posts, func(i, j int) bool { return b.Posts[i].CreatedAt.After(b.Posts[j].CreatedAt) })
	b.Events = append(b.Events, h.foundEvents(&b, time.Now())...)
	if h.Gaz != nil {
		for i := range b.Posts {
			b.Posts[i].PlaceTags = h.Gaz.CountPlaces(b.Posts[i].Tags)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30") // people and meetups change; posts are cached server-side anyway
	json.NewEncoder(w).Encode(b)
}

// TagPosts handles GET /tagposts?tag=websummit: Bluesky posts with that tag
// from anywhere, for "Elsewhere" when a filter finds little nearby. Cached
// per instance like every tag search; bots get nothing.
func (h *Home) TagPosts(w http.ResponseWriter, r *http.Request) {
	tag := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.URL.Query().Get("tag")), "#"))
	if IsBot(r) || tag == "" || len(tag) > 40 || strings.ContainsAny(tag, " /?&#") {
		http.Error(w, "want ?tag=", http.StatusBadRequest)
		return
	}
	posts := h.Bsky.SearchTag(r.Context(), tag, 25)
	if posts == nil {
		posts = []bsky.Post{}
	}
	if h.Gaz != nil {
		for i := range posts {
			posts[i].PlaceTags = h.Gaz.CountPlaces(posts[i].Tags)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=120")
	json.NewEncoder(w).Encode(posts)
}

// foundEvents spots events from the bundle itself: a tag shared by at least
// three upcoming meetups (next two weeks) or five local messages here.
// Place names, languages and Kafumu's own tags don't count.
func (h *Home) foundEvents(b *bundle, now time.Time) []gazetteer.EventTag {
	type agg struct {
		meetups, notes int
		from, to       time.Time
		live           bool
	}
	seen := map[string]*agg{}
	known := map[string]bool{}
	for _, e := range b.Events {
		known[e.Tag] = true
	}
	skip := func(t string) bool {
		if len(t) < 3 || known[t] || strings.HasPrefix(t, "geo") || strings.HasPrefix(t, "lang") || strings.HasPrefix(t, "re") && len(t) == 12 {
			return true
		}
		switch t {
		case "ask", "coffee", "learn", "meetup", "meetups", "event", "events":
			return true
		}
		return h.Gaz != nil && h.Gaz.CountPlaces([]string{t}) > 0
	}
	get := func(t string) *agg {
		if seen[t] == nil {
			seen[t] = &agg{}
		}
		return seen[t]
	}
	for _, m := range b.Meetups {
		if m.EndAt.Before(now) || m.StartAt.After(now.Add(14*24*time.Hour)) {
			continue
		}
		for _, t := range m.Tags {
			t = strings.ToLower(t)
			if skip(t) {
				continue
			}
			a := get(t)
			a.meetups++
			if a.from.IsZero() || m.StartAt.Before(a.from) {
				a.from = m.StartAt
			}
			if m.EndAt.After(a.to) {
				a.to = m.EndAt
			}
			if !m.StartAt.After(now) {
				a.live = true
			}
		}
	}
	for _, n := range b.Notes {
		for _, t := range n.Tags {
			if !skip(t) {
				a := get(t)
				a.notes++
				a.live = true // people are talking about it now
			}
		}
	}
	var out []gazetteer.EventTag
	for t, a := range seen {
		if a.meetups < 3 && a.notes < 5 {
			continue
		}
		e := gazetteer.EventTag{Tag: t, Name: "#" + t, Live: a.live, Found: true, N: a.meetups + a.notes}
		if !a.from.IsZero() {
			e.From, e.To = a.from.Format("2006-01-02"), a.to.Format("2006-01-02")
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N > out[j].N })
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
