package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/meetup"
)

// Meetups handles hosting and joining meetups. Hosting is free, always.
type Meetups struct {
	Home *Home
	Svc  *meetup.Service
}

type meetupPage struct {
	page
	M     *meetup.Meetup
	Going bool
	Mine  bool
	Error string
}

// New handles GET /meetups/new.
func (h *Meetups) New(w http.ResponseWriter, r *http.Request) {
	p := meetupPage{page: h.Home.newPage(r, "")}
	p.Title, p.Tab = locale.T(p.Lang, "meetup.new_title"), "around"
	if r.URL.Query().Get("err") != "" {
		p.Error = locale.T(p.Lang, "meetup.err_"+r.URL.Query().Get("err"))
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
	switch err := h.Svc.Create(r.Context(), m, u.ID, name); {
	case errors.Is(err, meetup.ErrTooMany):
		http.Redirect(w, r, "/meetups/new?err=too_many", http.StatusSeeOther)
	case errors.Is(err, meetup.ErrInvalid):
		http.Redirect(w, r, "/meetups/new?err=invalid", http.StatusSeeOther)
	case err != nil:
		log.Printf("meetup: create: %v", err)
		http.Error(w, "could not save the meetup", http.StatusInternalServerError)
	default:
		http.Redirect(w, r, "/meetups/"+m.ID, http.StatusSeeOther)
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
	if _, _, err := h.Svc.Toggle(r.Context(), id, u.ID); err != nil && !errors.Is(err, meetup.ErrNotFound) {
		log.Printf("meetup: rsvp: %v", err)
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
