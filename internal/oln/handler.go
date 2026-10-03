package oln

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
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
