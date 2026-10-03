// Package bsky reads public posts from the Bluesky AppView. Kafumu treats
// ATproto as data infrastructure: local posts live in their authors' PDSes,
// and we only search them by tag and cache the result.
package bsky

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AppViews are tried in order; public.api.bsky.app is the documented host but
// has refused some clients, api.bsky.app answers unauthenticated too.
var AppViews = []string{"https://public.api.bsky.app", "https://api.bsky.app"}

// Post is the subset of a Bluesky post the home list needs.
type Post struct {
	URI       string    `json:"uri"`
	URL       string    `json:"url"`
	Handle    string    `json:"handle"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar,omitempty"`
	Text      string    `json:"text"`
	Langs     []string  `json:"langs,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Bot       bool      `json:"bot,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// Via is the tag this post was found under (e.g. "geo9f469w", "amsterdam").
	Via string `json:"via"`
}

// Client searches the AppView with a per-instance TTL cache, so a busy cell
// costs one upstream request per TTL however many people open the app.
type Client struct {
	HTTP *http.Client
	TTL  time.Duration

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	posts []Post
	at    time.Time
}

func NewClient() *Client {
	return &Client{
		HTTP:  &http.Client{Timeout: 6 * time.Second},
		TTL:   10 * time.Minute,
		cache: map[string]cached{},
	}
}

// SearchTag returns recent posts carrying #tag. Errors are swallowed into an
// empty, briefly cached result: a missing feed must never break the home page.
func (c *Client) SearchTag(ctx context.Context, tag string, limit int) []Post {
	tag = strings.ToLower(strings.TrimPrefix(tag, "#"))
	key := fmt.Sprintf("%s/%d", tag, limit)
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < c.TTL {
		c.mu.Unlock()
		return e.posts
	}
	c.mu.Unlock()

	posts, err := c.fetch(ctx, tag, limit)
	at := time.Now()
	if err != nil {
		// Retry sooner than a full TTL, but not on every request.
		at = at.Add(-c.TTL + time.Minute)
	}
	c.mu.Lock()
	if len(c.cache) > 5000 {
		c.cache = map[string]cached{} // crude bound; per-instance and cheap to refill
	}
	c.cache[key] = cached{posts: posts, at: at}
	c.mu.Unlock()
	return posts
}

// Prime stores posts for tag in the cache, as if fetched now. For tests.
func (c *Client) Prime(tag string, limit int, posts []Post) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[fmt.Sprintf("%s/%d", tag, limit)] = cached{posts: posts, at: time.Now()}
}

func (c *Client) fetch(ctx context.Context, tag string, limit int) ([]Post, error) {
	q := url.Values{"q": {"#" + tag}, "tag": {tag}, "limit": {fmt.Sprint(limit)}, "sort": {"latest"}}
	var lastErr error
	for _, host := range AppViews {
		req, err := http.NewRequestWithContext(ctx, "GET", host+"/xrpc/app.bsky.feed.searchPosts?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "kafumu (+https://github.com/LaPingvino/kafumu)")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var body searchResponse
		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil {
			lastErr = fmt.Errorf("%s: status %d: %v", host, resp.StatusCode, err)
			continue
		}
		return body.posts(tag), nil
	}
	return nil, lastErr
}

type searchResponse struct {
	Posts []struct {
		URI    string `json:"uri"`
		Author struct {
			Handle      string `json:"handle"`
			DisplayName string `json:"displayName"`
			Avatar      string `json:"avatar"`
			Labels      []struct {
				Val string `json:"val"`
			} `json:"labels"`
		} `json:"author"`
		Record struct {
			Text      string    `json:"text"`
			Langs     []string  `json:"langs"`
			Tags      []string  `json:"tags"`
			CreatedAt time.Time `json:"createdAt"`
			Facets    []struct {
				Features []struct {
					Type string `json:"$type"`
					Tag  string `json:"tag"`
				} `json:"features"`
			} `json:"facets"`
		} `json:"record"`
	} `json:"posts"`
}

func (r *searchResponse) posts(via string) []Post {
	out := make([]Post, 0, len(r.Posts))
	for _, p := range r.Posts {
		post := Post{
			URI:       p.URI,
			URL:       webURL(p.URI, p.Author.Handle),
			Handle:    p.Author.Handle,
			Name:      p.Author.DisplayName,
			Avatar:    p.Author.Avatar,
			Text:      p.Record.Text,
			Langs:     p.Record.Langs,
			CreatedAt: p.Record.CreatedAt,
			Via:       via,
		}
		seen := map[string]bool{}
		add := func(t string) {
			t = strings.ToLower(t)
			if t != "" && !seen[t] {
				seen[t] = true
				post.Tags = append(post.Tags, t)
			}
		}
		for _, t := range p.Record.Tags {
			add(t)
		}
		for _, f := range p.Record.Facets {
			for _, ft := range f.Features {
				if ft.Type == "app.bsky.richtext.facet#tag" {
					add(ft.Tag)
				}
			}
		}
		for _, l := range p.Author.Labels {
			if l.Val == "bot" {
				post.Bot = true
			}
		}
		out = append(out, post)
	}
	return out
}

// webURL turns at://did/app.bsky.feed.post/rkey into a bsky.app link.
func webURL(uri, handle string) string {
	parts := strings.Split(strings.TrimPrefix(uri, "at://"), "/")
	if len(parts) != 3 {
		return ""
	}
	who := handle
	if who == "" {
		who = parts[0]
	}
	return "https://bsky.app/profile/" + who + "/post/" + parts[2]
}
