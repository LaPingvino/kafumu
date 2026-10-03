// Package handler holds Kafumu's HTTP handlers.
package handler

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/meetup"
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
	// Meetups, if set, are included in bundles.
	Meetups *meetup.Service
}

// page is the data every full page gets.
type page struct {
	Brand string
	Title string
	Tab   string // active view-switcher tab
	Cell  string
	Lang  string
	V     string // asset version, so a deploy never mixes old and new JS
	Langs []locale.Lang
	User  *account.User
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
	return page{Brand: h.Cfg.Brand, Title: title, Lang: lang, V: h.Cfg.Version, Langs: locale.Langs(),
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
	p := h.newPage(r, "")
	p.Tab, p.Cell = "around", cell
	h.render(w, "home.html", p)
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
	// Events are conferences and festivals here, upcoming or running.
	Events []gazetteer.EventTag `json:"events"`
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

// Funcs are the template functions. Translations come from our own files,
// so they may contain markup.
var Funcs = template.FuncMap{
	"t":  func(lang, key string) template.HTML { return template.HTML(locale.T(lang, key)) },
	"ts": locale.T,
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

	b := bundle{Cells: cells, Places: []gazetteer.PlaceTag{}, Events: []gazetteer.EventTag{}, Meetups: []*meetup.Meetup{}, Posts: []bsky.Post{}}
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
				if !uris[p.URI] {
					uris[p.URI] = true
					b.Posts = append(b.Posts, p)
				}
			}
		}(tag)
	}
	wg.Wait()
	sort.Slice(b.Posts, func(i, j int) bool { return b.Posts[i].CreatedAt.After(b.Posts[j].CreatedAt) })

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	json.NewEncoder(w).Encode(b)
}
