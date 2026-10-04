package oln

import (
	"encoding/json"
	"errors"
	"github.com/LaPingvino/kafumu/internal/geo"
	"io"
	"log"
	"net/http"
	"strings"
)

// HandlePost handles POST /api/oln with a raw message as the body. No
// account, no cookie: the proof of work is the price of speaking.
func (s *Service) HandlePost(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, MaxRaw+1))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	n, err := s.Post(r.Context(), string(raw))
	w.Header().Set("Content-Type", "application/json")
	switch {
	case errors.Is(err, ErrWork):
		w.WriteHeader(http.StatusPaymentRequired) // pay in work: mine more bits
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
	case errors.Is(err, ErrFormat), errors.Is(err, ErrClock), errors.Is(err, ErrPlace):
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
	case err != nil:
		log.Printf("oln: post: %v", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	default:
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
