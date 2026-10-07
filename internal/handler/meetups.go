package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/business"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/atp"
	"github.com/LaPingvino/kafumu/internal/feeds"
	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

// Meetups handles hosting and joining meetups. Hosting is free, always.
type Meetups struct {
	Home       *Home
	Svc        *meetup.Service
	Importer   *importer.Importer
	Businesses *business.Store
}

type meetupPage struct {
	page
	M     *meetup.Meetup
	Going bool
	Mine  bool
	Error string
	// HostAs: the live business accounts you may host as (meetups/new).
	HostAs []*business.Business
}

// New handles GET /meetups/new.
func (h *Meetups) New(w http.ResponseWriter, r *http.Request) {
	p := meetupPage{page: h.Home.newPage(r, "")}
	p.Title, p.Tab = locale.T(p.Lang, "meetup.new_title"), "around"
	if r.URL.Query().Get("err") != "" {
		p.Error = locale.T(p.Lang, "meetup.err_"+r.URL.Query().Get("err"))
	}
	if u := p.User; u != nil && h.Businesses != nil {
		bs, _ := h.Home.bizFor(r.Context(), u.ID)
		p.HostAs = bs // hosting under the business's name is free (LOOP-STATE 45b)
	}
	h.Home.render(w, "meetup_new.html", p)
}

// Create handles POST /meetups.
func (h *Meetups) Create(w http.ResponseWriter, r *http.Request) {
	u := UserFrom(r.Context())
	if u == nil {
		http.Redirect(w, r, "/meetups/new", http.StatusSeeOther)
		return
	}
	start, err := time.Parse(time.RFC3339, r.FormValue("start_iso"))
	if err != nil {
		// No JavaScript: read the local field as UTC rather than fail.
		start, _ = time.Parse("2006-01-02T15:04", r.FormValue("start"))
	}
	mins, _ := strconv.Atoi(r.FormValue("duration"))
	if mins <= 0 {
		mins = 120
	}
	m := &meetup.Meetup{
		Title: r.FormValue("title"), Text: r.FormValue("text"), Venue: r.FormValue("venue"), Link: r.FormValue("link"),
		Cell: r.FormValue("cell"), StartAt: start.UTC(), EndAt: start.UTC().Add(time.Duration(mins) * time.Minute),
		Tags: strings.FieldsFunc(r.FormValue("tags"), func(c rune) bool { return c == ',' || c == ' ' }),
	}
	// A meetup inside a running event (Web Summit) carries its tag.
	if h.Home.Gaz != nil && geo.Valid(strings.ToLower(m.Cell)) {
		for _, e := range h.Home.Gaz.EventsAt([]string{strings.ToLower(m.Cell)}, m.StartAt) {
			m.Tags = append([]string{e.Tag}, m.Tags...)
		}
	}
	name := u.Username
	var host *business.Business // hosting as a business: its Bluesky, never yours
	if as := r.FormValue("as"); as != "" && h.Businesses != nil {
		if b, err := h.Businesses.Get(r.Context(), as); err == nil && h.Home.managesBiz(r.Context(), b, u.ID) {
			m.Business, host = b.Name, b
		}
	}
	switch err := h.Svc.Create(r.Context(), m, u.ID, name); {
	case errors.Is(err, meetup.ErrTooMany):
		http.Redirect(w, r, "/meetups/new?err=too_many", http.StatusSeeOther)
	case errors.Is(err, meetup.ErrInvalid):
		http.Redirect(w, r, "/meetups/new?err=invalid", http.StatusSeeOther)
	case err != nil:
		log.Printf("meetup: create: %v", err)
		http.Error(w, "could not save the meetup", http.StatusInternalServerError)
	default:
		h.publishEvent(r, u, host, m)
		http.Redirect(w, r, "/meetups/"+m.ID, http.StatusSeeOther)
	}
}

// publishEvent also writes the meetup to the host's own ATproto repo when
// they connected one. Failures are logged, never shown as a failed meetup.
func (h *Meetups) publishEvent(r *http.Request, u *account.User, host *business.Business, m *meetup.Meetup) {
	did, session := bskyAccount(u, host)
	if h.Home.ATproto == nil || did == "" {
		return
	}
	lat, lon := geo.Center(m.Cell)
	link := h.Home.Origin(r) + "/meetups/" + m.ID
	rec := atp.EventRecord(m.Title, m.Text, m.StartAt, m.EndAt, m.Venue, lat, lon, link, time.Now())
	uri, cid, err := h.Home.ATproto.CreateRecord(r.Context(), did, session, "community.lexicon.calendar.event", rec)
	if err != nil {
		log.Printf("atproto: event: %v", err)
		return
	}
	if _, err := h.Svc.Store.Update(r.Context(), m.ID, func(x *meetup.Meetup) error { x.ATURI, x.ATCID = uri, cid; return nil }); err != nil {
		log.Printf("atproto: event uri: %v", err)
	}
}

// Show handles GET /meetups/{id}.
func (h *Meetups) Show(w http.ResponseWriter, r *http.Request) {
	m, err := h.Svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p := meetupPage{page: h.Home.newPage(r, m.Title), M: m}
	p.Tab = "around"
	if u := p.User; u != nil {
		p.Going, p.Mine = m.HasRSVP(u.ID), m.AuthorID == u.ID
	}
	h.Home.render(w, "meetup.html", p)
}

// RSVP handles POST /meetups/{id}/rsvp: toggle going.
func (h *Meetups) RSVP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u := UserFrom(r.Context())
	if u == nil {
		http.Redirect(w, r, "/account?next=/meetups/"+id, http.StatusSeeOther)
		return
	}
	going, m, err := h.Svc.Toggle(r.Context(), id, u.ID)
	if err != nil && !errors.Is(err, meetup.ErrNotFound) {
		log.Printf("meetup: rsvp: %v", err)
	}
	// "Going" to an event that exists on ATproto: say so in your own repo too.
	if err == nil && going && m.ATURI != "" && m.ATCID != "" && u.DID != "" && h.Home.ATproto != nil {
		if _, _, err := h.Home.ATproto.CreateRecord(r.Context(), u.DID, u.ATSession, "community.lexicon.calendar.rsvp",
			atp.RSVPRecord(m.ATURI, m.ATCID, time.Now())); err != nil {
			log.Printf("atproto: rsvp: %v", err)
		}
	}
	http.Redirect(w, r, "/meetups/"+id, http.StatusSeeOther)
}

// Delete handles POST /meetups/{id}/delete by the host.
func (h *Meetups) Delete(w http.ResponseWriter, r *http.Request) {
	if u := UserFrom(r.Context()); u != nil {
		if err := h.Svc.Delete(r.Context(), r.PathValue("id"), u.ID); err != nil && !errors.Is(err, meetup.ErrNotFound) {
			log.Printf("meetup: delete: %v", err)
		}
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ICS handles GET /meetups/{id}/ics and GET /cal/{cell}: add to calendar.
func (h *Meetups) ICS(w http.ResponseWriter, r *http.Request) {
	var ms []*meetup.Meetup
	name := h.Home.Cfg.Brand
	if id := r.PathValue("id"); id != "" {
		m, err := h.Svc.Get(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ms, name = []*meetup.Meetup{m}, m.Title
	} else {
		cell := strings.TrimSuffix(strings.ToLower(r.PathValue("cell")), ".ics")
		if !geo.Valid(cell) || IsBot(r) && r.Header.Get("Accept") == "" {
			http.NotFound(w, r)
			return
		}
		var err error
		if ms, err = h.Svc.InCells(r.Context(), geo.Rings(cell, 1)); err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		name += " #geo" + cell
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Write([]byte(meetup.ICS(name, h.Home.Origin(r), ms)))
}

// Import handles POST /meetups/import {url}: read an event page (Luma,
// Meetup, anything with schema.org Event data) to prefill the form.
// Accounts only, so the importer can't be used as an open fetch proxy.
func (h *Meetups) Import(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if UserFrom(r.Context()) == nil || h.Importer == nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "account"})
		return
	}
	ev, err := h.Importer.Fetch(r.Context(), r.FormValue("url"))
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	out := map[string]any{"title": ev.Title, "start": ev.Start.UTC().Format(time.RFC3339), "venue": ev.Venue, "text": ev.Text, "link": ev.Link}
	if !ev.End.IsZero() {
		out["minutes"] = int(ev.End.Sub(ev.Start).Minutes())
	}
	if ev.HasGeo {
		out["cell"] = geo.Cell(ev.Lat, ev.Lon)
	}
	json.NewEncoder(w).Encode(out)
}

// SyncFeeds handles GET /cron/feeds, run by App Engine cron (which sets
// X-Appengine-Cron; App Engine strips that header from outside requests).
func (h *Meetups) SyncFeeds(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Appengine-Cron") != "true" {
		http.NotFound(w, r)
		return
	}
	w.Write([]byte(h.RunFeeds(r.Context()) + "\n"))
}

// RunFeeds syncs the public calendars now (cron, or by hand from /admin).
func (h *Meetups) RunFeeds(ctx context.Context) string {
	fs := feeds.Load()
	// Eventa Servo: the Esperanto calendars of the countries people looked
	// at lately (no key needed: one public .ics per country).
	for _, cc := range h.Home.SeenCountries(ctx, time.Now()) {
		fs = append(fs, feeds.EventaServo(cc))
	}
	res := feeds.Sync(ctx, fs, h.Importer, h.Svc.Store, time.Now())
	h.Svc.ForgetAll()
	log.Printf("feeds: %s", res)
	return res.String()
}

// bskyAccount: whose Bluesky a write goes to: the business's when it's done
// as one (its own account, or none at all: never the manager's), else yours.
func bskyAccount(u *account.User, as *business.Business) (did, session string) {
	if as != nil {
		return as.DID, as.ATSession
	}
	if u == nil {
		return "", ""
	}
	return u.DID, u.ATSession
}
