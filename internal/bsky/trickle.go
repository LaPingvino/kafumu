package bsky

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Slow Bluesky for the wider rings (Joop: "slowly retrieve from Bluesky for
// the wider areas if the server is idle enough"). A quiet area's outer
// cells are never searched during a request: Cached answers from what's
// there and Later queues the rest, which a single worker fetches one at a
// time, only while no foreground search has run for a moment. The queue is
// small, skips duplicates and fresh tags, and the worker stops when it's
// empty.

const (
	trickleEvery = 3 * time.Second // at most one background search per 3 s
	trickleIdle  = 2 * time.Second // …and only after 2 s without a foreground one
	trickleMax   = 300             // queued tags at most
)

// Cached returns the posts for #tag if they're cached and fresh; it never
// fetches.
func (c *Client) Cached(tag string, limit int) ([]Post, bool) {
	key := fmt.Sprintf("%s/%d", strings.ToLower(strings.TrimPrefix(tag, "#")), limit)
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Since(e.at) >= c.TTL {
		return nil, false
	}
	return e.posts, true
}

// Later queues #tag for a background search when the server is idle.
func (c *Client) Later(tag string, limit int) {
	tag = strings.ToLower(strings.TrimPrefix(tag, "#"))
	if _, ok := c.Cached(tag, limit); ok {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queued == nil {
		c.queued = map[string]bool{}
	}
	if c.queued[tag] || len(c.queue) >= trickleMax {
		return
	}
	c.queued[tag] = true
	c.queue = append(c.queue, trickleItem{tag, limit})
	if !c.trickling {
		c.trickling = true
		go c.trickle()
	}
}

type trickleItem struct {
	tag   string
	limit int
}

// QueueLen is how many tags wait (for tests and the admin).
func (c *Client) QueueLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queue)
}

func (c *Client) trickle() {
	for {
		time.Sleep(c.every())
		c.mu.Lock()
		if time.Since(c.lastForeground) < trickleIdle {
			c.mu.Unlock()
			continue // busy: let people's own requests go first
		}
		if len(c.queue) == 0 {
			c.trickling = false
			c.mu.Unlock()
			return
		}
		it := c.queue[0]
		c.queue = c.queue[1:]
		delete(c.queued, it.tag)
		c.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		c.search(ctx, it.tag, it.limit, false)
		cancel()
	}
}

func (c *Client) every() time.Duration {
	if c.TrickleEvery > 0 {
		return c.TrickleEvery
	}
	return trickleEvery
}
