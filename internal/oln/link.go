package oln

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Linking nodes (LOOP-STATE 64c): a node pulls the OLN lines of the areas
// it cares about from its peers' /oln.json and relays them in. Pulls run
// from a job, never per request.

// PullResult is one pull from one peer.
type PullResult struct {
	Peer      string
	Seen, New int
	Err       string
	At        time.Time
}

// Pull fetches peer's lines for cells (each export covers a cell and its
// neighbours) and relays the new ones in. At most 20 cells per pull.
func (s *Service) Pull(ctx context.Context, client *http.Client, peer string, cells []string) PullResult {
	res := PullResult{Peer: peer, At: time.Now()}
	peer = strings.TrimRight(peer, "/")
	if u, err := url.Parse(peer); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		res.Err = "not a node address"
		return res
	}
	if len(cells) > 20 {
		cells = cells[:20]
	}
	seen := map[string]bool{}
	for _, c := range cells {
		req, _ := http.NewRequestWithContext(ctx, "GET", peer+"/oln.json?cell="+url.QueryEscape(c), nil)
		resp, err := client.Do(req)
		if err != nil {
			res.Err = err.Error()
			continue
		}
		var f Format
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&f)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil {
			res.Err = fmt.Sprintf("%s: status %d", c, resp.StatusCode)
			continue
		}
		for _, m := range f.Messages {
			if seen[m.Raw] {
				continue
			}
			seen[m.Raw] = true
			res.Seen++
			if _, fresh, err := s.Relay(ctx, m.Raw, peer); err == nil && fresh {
				res.New++
			}
		}
	}
	return res
}
