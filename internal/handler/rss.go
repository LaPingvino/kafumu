package handler

import (
	"encoding/xml"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/geo"
)

// RSS handles GET /rss/{cell}: an area as a feed (81a), for feed readers
// and for local websites that want to show what goes on around them: the
// area's public lines (no private messages, no reactions) and its upcoming
// meetups nearby, from the same per-cell caches as the home page.
func (h *Meetups) RSS(w http.ResponseWriter, r *http.Request) {
	cell := strings.TrimSuffix(strings.ToLower(r.PathValue("cell")), ".xml")
	if !geo.Valid(cell) || IsBot(r) && r.Header.Get("Accept") == "" {
		http.NotFound(w, r)
		return
	}
	origin := h.Home.Origin(r)
	area := origin + "/?cell=" + cell
	type guid struct {
		Perma bool   `xml:"isPermaLink,attr"`
		ID    string `xml:",chardata"`
	}
	type item struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		Desc    string `xml:"description,omitempty"`
		Author  string `xml:"dc:creator,omitempty"`
		PubDate string `xml:"pubDate"`
		GUID    guid   `xml:"guid"`
		at      time.Time
	}
	var items []item
	if h.Home.Notes != nil {
		ns, err := h.Home.Notes.InCells(r.Context(), []string{cell})
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		for _, n := range ns {
			if n.Pair != "" || n.Re != "" || strings.TrimSpace(n.Text) == "" {
				continue
			}
			title, rest, _ := strings.Cut(strings.TrimSpace(n.Text), "\n")
			it := item{Title: clip(title, 120), Link: area, Desc: strings.TrimSpace(rest), at: n.At,
				PubDate: n.At.UTC().Format(http.TimeFormat), GUID: guid{ID: "oln:" + n.ID}}
			if n.Author != "" {
				it.Author = "@" + n.Author
			}
			items = append(items, it)
		}
	}
	ms, err := h.Svc.InCells(r.Context(), geo.Rings(cell, 1))
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	for _, m := range ms {
		desc := m.StartAt.UTC().Format("Mon 2 Jan 15:04 MST")
		if m.Venue != "" {
			desc += " · " + m.Venue
		}
		if m.Text != "" {
			desc += "\n\n" + m.Text
		}
		at := m.CreatedAt
		if at.IsZero() {
			at = m.StartAt
		}
		it := item{Title: "📅 " + m.Title, Link: origin + "/meetups/" + m.ID, Desc: desc, at: at,
			PubDate: at.UTC().Format(http.TimeFormat), GUID: guid{Perma: true, ID: origin + "/meetups/" + m.ID}}
		if m.AuthorName != "" {
			it.Author = "@" + m.AuthorName
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].at.After(items[j].at) })
	if len(items) > 50 {
		items = items[:50]
	}
	feed := struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		DC      string   `xml:"xmlns:dc,attr"`
		Channel struct {
			Title string `xml:"title"`
			Link  string `xml:"link"`
			Desc  string `xml:"description"`
			Items []item `xml:"item"`
		} `xml:"channel"`
	}{Version: "2.0", DC: "http://purl.org/dc/elements/1.1/"}
	feed.Channel.Title = h.Home.Cfg.Brand + " #geo" + cell
	feed.Channel.Link = area
	feed.Channel.Desc = "#geo" + cell
	feed.Channel.Items = items
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(feed)
}

// clip shortens s to n runes, with an ellipsis.
func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
