package handler

import (
	"net/http"
	"regexp"
	"time"

	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/oln"
)

var noteID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ShowNote handles GET /n/{id}: a line's own address (86c). A live line
// opens in its area, at the line. One that has expired but is still stored
// (Datastore deletes expired entities late) is shown as expired; one that is
// gone gets a page saying lines fade, and how OLN works. Private lines (pair
// tags) and hidden ones are never shown here.
func (h *Home) ShowNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !noteID.MatchString(id) || IsBot(r) {
		http.NotFound(w, r)
		return
	}
	var n *oln.Note
	if h.Notes != nil {
		if got, err := h.Notes.Store.Get(r.Context(), id); err == nil && got.Pair == "" && !h.Notes.IsHidden(r.Context(), id) {
			n = got
		}
	}
	now := time.Now()
	if n != nil && n.ExpiresAt.After(now) && n.Cell != oln.Everywhere {
		http.Redirect(w, r, "/?cell="+n.Cell+"#re-"+id[:10], http.StatusFound)
		return
	}
	p := struct {
		page
		N    *oln.Note
		Live bool
		Area string
	}{page: h.newPage(r, "")}
	p.Title, p.Tab = locale.T(p.Lang, "note.title"), "around"
	if n != nil {
		p.N, p.Live = n, n.ExpiresAt.After(now)
		if n.Cell != oln.Everywhere {
			p.Area = n.Cell
		}
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
	}
	h.render(w, "note.html", p)
}
