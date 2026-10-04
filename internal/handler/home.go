// Package handler holds Kafumu's HTTP handlers.
package handler

import (
	"encoding/json"
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
	Cfg  *config.Config
	Tmpl *template.Template
	Bsky *bsky.Client
	Gaz  *gazetteer.Gazetteer
	// Notes, if set, are the OLN local messages included in bundles.
	Notes *oln.Service
	// Reports, if set, hides what moderators hid (posts, people) from bundles.
	Reports *report.Service
	// ATproto, if set, lets people connect their own ATproto account.
	ATproto *atp.Service
	// Meetups and Accounts, if set, are included in bundles.
	Meetups  *meetup.Service
	Accounts *account.Service
}

// page is the data every full page gets.
type page struct {
	Brand string
	Title string
	Tab   string // active view-switcher tab
	Cell  string
	Lang  string
	V     string // asset version, so a deploy never mixes old and new JS
	// MovedTo is set on a legacy origin: the canonical origin to move to.
	MovedTo string
	// Passkeys is true when passkeys work on this host.
	Passkeys bool
	// ATproto is true when connecting an ATproto account works here.
	ATproto bool
	Langs   []locale.Lang
	User    *account.User
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
	return page{Brand: h.Cfg.Brand, Title: title, Lang: lang, V: h.Cfg.Version, Langs: locale.Langs(), MovedTo: moved,
		Passkeys: h.Cfg.Passkeys && moved == "", ATproto: h.ATproto != nil && moved == "",
		User: UserFrom(r.Context()), JS: locale.Prefix(lang, "js.")}
}

// ShowHome renders the shell; the cell is computed on the device and the list
// is filled from /bundle, so the server never sees coordinates.
func (h *Home) ShowHome(w http.ResponseWriter, r *http.Request) {
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
		if c := guessCell(r.Header.Get("X-Appengine-Citylatlong")); c != "" {
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
	if h.Gaz != nil {
		for i := range b.Posts {
			b.Posts[i].PlaceTags = h.Gaz.CountPlaces(b.Posts[i].Tags)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30") // people and meetups change; posts are cached server-side anyway
	json.NewEncoder(w).Encode(b)
}
