package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/geo"
	"github.com/LaPingvino/kafumu/internal/kv"
	"github.com/LaPingvino/kafumu/internal/oln"
)

// Linked OLN nodes (LOOP-STATE 64c): the peers this node pulls local
// messages from, each with the areas to pull besides the ones people here
// looked at recently, and how the last pull went.
type peerEntry struct {
	Origin string
	Cells  []string
	Last   oln.PullResult
}

type peerConfig struct{ Peers []peerEntry }

// stored in Datastore as one Config entity (JSON), or kv when self-hosted.
type peerEntity struct {
	JSON string `datastore:"json,noindex"`
}

func peerKey() *datastore.Key { return datastore.NameKey("Config", "peers", nil) }

func (a *Admin) loadPeers(ctx context.Context) peerConfig {
	var c peerConfig
	if a.DB != nil {
		var e peerEntity
		if a.DB.Get(ctx, peerKey(), &e) == nil {
			json.Unmarshal([]byte(e.JSON), &c)
		}
		return c
	}
	a.peersMu.Lock()
	defer a.peersMu.Unlock()
	if a.peersMem == nil {
		m := map[string]peerConfig{}
		kv.Load("Config", m)
		c = m["peers"]
		a.peersMem = &c
	}
	return *a.peersMem
}

func (a *Admin) savePeers(ctx context.Context, c peerConfig) error {
	if a.DB != nil {
		b, _ := json.Marshal(c)
		_, err := a.DB.Put(ctx, peerKey(), &peerEntity{JSON: string(b)})
		return err
	}
	a.peersMu.Lock()
	a.peersMem = &c
	a.peersMu.Unlock()
	kv.Save("Config", "peers", c)
	return nil
}

// peerOrigin cleans an address to scheme://host (http only for local tests).
func peerOrigin(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func cleanCells(s string) []string {
	var out []string
	for _, f := range strings.Fields(strings.ToLower(strings.ReplaceAll(s, ",", " "))) {
		f = strings.TrimPrefix(strings.TrimPrefix(f, "#"), "geo")
		if geo.Valid(f) && !slices.Contains(out, f) && len(out) < 20 {
			out = append(out, f)
		}
	}
	return out
}

// PullPeers pulls from every linked node (the admin job, the self-hosted
// loop, and /cron/oln-pull if scheduled). Returns a one-line summary.
func (a *Admin) PullPeers(ctx context.Context) string {
	if a.Notes == nil {
		return "no local messages here"
	}
	c := a.loadPeers(ctx)
	if len(c.Peers) == 0 {
		return "no linked nodes"
	}
	client := &http.Client{Timeout: 10 * time.Second}
	recent := a.Notes.RecentCells(time.Now())
	total, news := 0, 0
	for i, p := range c.Peers {
		cells := append(slices.Clone(p.Cells), recent...)
		slices.Sort(cells)
		cells = slices.Compact(cells)
		if len(cells) == 0 {
			c.Peers[i].Last = oln.PullResult{Peer: p.Origin, At: time.Now(), Err: "no areas yet: add some, or wait until people here look around"}
			continue
		}
		res := a.Notes.Pull(ctx, client, p.Origin, cells)
		c.Peers[i].Last = res
		total += res.Seen
		news += res.New
	}
	if err := a.savePeers(ctx, c); err != nil {
		return "pulled, but saving failed: " + err.Error()
	}
	return "pulled " + itoa(len(c.Peers)) + " node(s): " + itoa(total) + " lines seen, " + itoa(news) + " new"
}

func itoa(n int) string { return strconv.Itoa(n) }
