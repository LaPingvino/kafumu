package oln

import (
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/geo"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// HandlePost handles POST /api/oln with a raw message as the body. No
// account, no cookie: the proof of work is the price of speaking.
func (s *Service) HandlePost(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxRaw+1))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	author := ""
	if s.AuthorFor != nil {
		author = s.AuthorFor(r)
	}
	n, err := s.PostAs(withReq(r), string(raw), author)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case errors.Is(err, ErrWork):
		w.WriteHeader(http.StatusPaymentRequired) // pay in work: mine more bits
		out := map[string]any{"error": err.Error()}
		var ne *NeedError
		if errors.As(err, &ne) {
			out["need"] = ne.Need
		}
		json.NewEncoder(w).Encode(out)
	case errors.Is(err, ErrRepeat):
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
	case errors.Is(err, ErrFormat), errors.Is(err, ErrClock), errors.Is(err, ErrPlace):
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
	case err != nil:
		log.Printf("oln: post: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	default:
		if n.Pair != "" && s.OnPair != nil {
			s.OnPair(r.Context(), n.Pair)
		}
		json.NewEncoder(w).Encode(n)
	}
}

// HandleAsks is GET /api/asks?tags=opensource,esperanto: live questions
// about those subjects from anywhere; the device keeps the ones near enough.
func (s *Service) HandleAsks(w http.ResponseWriter, r *http.Request) {
	var tags []string
	for _, t := range strings.Split(strings.ToLower(r.URL.Query().Get("tags")), ",") {
		if t = strings.TrimPrefix(strings.TrimSpace(t), "#"); t != "" && len(t) <= 40 {
			tags = append(tags, t)
		}
	}
	if len(tags) == 0 {
		http.Error(w, "want ?tags=a,b", http.StatusBadRequest)
		return
	}
	ns, err := s.Asks(r.Context(), tags)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if ns == nil {
		ns = []*Note{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	json.NewEncoder(w).Encode(ns)
}

// HandleRequired is GET /api/oln/required?cell=…: the work (bits) a new
// message in that cell needs right now.
func (s *Service) HandleRequired(w http.ResponseWriter, r *http.Request) {
	cell := strings.ToLower(r.URL.Query().Get("cell"))
	if !geo.Valid(cell) {
		http.Error(w, "want ?cell=<6-char #geo cell>", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=30")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]int{"bits": s.RequiredFor(r.Context(), cell)})
}

// HandlePair is GET /api/oln/pair/{tag}: the private messages waiting under
// a pair tag (unguessable; their text is ciphertext only the pair can read).
func (s *Service) HandlePair(w http.ResponseWriter, r *http.Request) {
	ns, err := s.ForPair(r.Context(), r.PathValue("tag"))
	if errors.Is(err, ErrFormat) {
		http.Error(w, "bad tag", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	type msg struct {
		ID   string    `json:"id"`
		Text string    `json:"text"`
		At   time.Time `json:"at"`
	}
	out := make([]msg, 0, len(ns))
	for _, n := range ns {
		out = append(out, msg{n.ID, n.Text, n.At})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(out)
}

// HandleRe is GET /api/oln/re?ids=a,b: public replies and reactions to
// those posts (#re ids), wherever they were posted, so their author sees
// them from any area (77b). Anonymous; the ids are public tags.
func (s *Service) HandleRe(w http.ResponseWriter, r *http.Request) {
	var ids []string
	for _, id := range strings.Split(strings.ToLower(r.URL.Query().Get("ids")), ",") {
		if id = strings.TrimSpace(id); reTag.MatchString("re" + id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > MaxReIDs {
		http.Error(w, "want ?ids=a,b (1-20 ids of 10 hex)", http.StatusBadRequest)
		return
	}
	ns, err := s.Replies(r.Context(), ids)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if ns == nil {
		ns = []*Note{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	json.NewEncoder(w).Encode(ns)
}
