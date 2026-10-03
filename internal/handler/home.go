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

	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/geo"
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
}

// page is the data every full page gets.
type page struct {
	Brand string
	Title string
	Cell  string
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
	h.render(w, "home.html", page{Brand: h.Cfg.Brand, Cell: cell})
}

// ShowAbout renders the static explanation page.
func (h *Home) ShowAbout(w http.ResponseWriter, r *http.Request) {
	h.render(w, "about.html", page{Brand: h.Cfg.Brand, Title: "About"})
}

func (h *Home) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
	Posts  []bsky.Post          `json:"posts"`
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

	b := bundle{Cells: cells, Places: []gazetteer.PlaceTag{}, Events: []gazetteer.EventTag{}, Posts: []bsky.Post{}}
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
