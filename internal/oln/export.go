package oln

import (
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/LaPingvino/kafumu/internal/geo"
)

// The OLN JSON format from github.com/LaPingvino/eolnpoc (olnjson.Format),
// so other OLN nodes can index what Kafumu carries — and push to us.
type Format struct {
	Server   ServerInfo          `json:"server"`
	Messages map[string]Message  `json:"messages"`
	Index    map[string][]string `json:"index"`
	Feeds    []string            `json:"feeds"`
	Push     []string            `json:"push"`
}

type ServerInfo struct {
	Link       string `json:"link"`
	Name       string `json:"name"`
	PubKey     string `json:"pubkey"`
	AcceptPush bool   `json:"acceptpush"`
}

type Message struct {
	Raw       string    `json:"raw"`
	Origin    Origin    `json:"origin"`
	Sig       string    `json:"sig"`
	Timestamp time.Time `json:"timestamp"`
	TTL       int       `json:"ttl"` // days, as in eolnpoc
	Hops      int       `json:"hops"`
	Tags      []string  `json:"tags"`
}

type Origin struct {
	Display    string `json:"display"`
	PubKey     string `json:"pubkey"`
	ServerName string `json:"servername"`
}

// Export handles GET /oln.json?cell=… (the cell and its neighbours): the
// live messages there in OLN JSON. Messages carry no author — that's the
// point of proof of work instead of accounts.
func (s *Service) Export(origin, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cell := strings.ToLower(r.URL.Query().Get("cell"))
		if !geo.Valid(cell) {
			http.Error(w, "want ?cell=<6-char #geo cell>", http.StatusBadRequest)
			return
		}
		ns, err := s.InCells(r.Context(), geo.Rings(cell, 1))
		if err != nil {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		now := s.Now()
		f := Format{
			Server:   ServerInfo{Link: origin, Name: name, AcceptPush: true},
			Messages: map[string]Message{}, Index: map[string][]string{},
			Feeds: []string{}, Push: []string{origin + "/api/oln"},
		}
		for _, n := range ns {
			days := int(math.Ceil(n.ExpiresAt.Sub(now).Hours() / 24))
			tags := make([]string, len(n.Tags))
			for i, t := range n.Tags {
				tags[i] = "#" + t
				f.Index[tags[i]] = append(f.Index[tags[i]], n.ID)
			}
			o := Origin{ServerName: name}
			if n.Author != "" {
				o.Display = "@" + n.Author
			}
			f.Messages[n.ID] = Message{Raw: n.Raw, Origin: o, Timestamp: n.At, TTL: max(days, 1), Tags: tags}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(f)
	}
}
