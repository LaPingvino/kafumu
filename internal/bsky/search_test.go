package bsky

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const sample = `{"posts":[{"uri":"at://did:plc:abc/app.bsky.feed.post/3kxyz",
"author":{"handle":"ana.bsky.social","displayName":"Ana","labels":[{"val":"bot"}]},
"record":{"text":"Kafo? #geo9f469w #Esperanto","langs":["eo"],"createdAt":"2026-10-03T12:00:00Z",
"facets":[{"features":[{"$type":"app.bsky.richtext.facet#tag","tag":"geo9f469w"}]},
{"features":[{"$type":"app.bsky.richtext.facet#tag","tag":"Esperanto"}]}]}}]}`

func TestSearchTagCaches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.URL.Query().Get("tag"); got != "geo9f469w" {
			t.Errorf("tag = %q", got)
		}
		fmt.Fprint(w, sample)
	}))
	defer srv.Close()
	old := AppViews
	AppViews = []string{srv.URL}
	defer func() { AppViews = old }()

	c := NewClient()
	for i := 0; i < 3; i++ {
		posts := c.SearchTag(context.Background(), "#GEO9f469w", 10)
		if len(posts) != 1 {
			t.Fatalf("got %d posts", len(posts))
		}
		p := posts[0]
		if p.URL != "https://bsky.app/profile/ana.bsky.social/post/3kxyz" || !p.Bot || p.Via != "geo9f469w" {
			t.Errorf("post = %+v", p)
		}
		if len(p.Tags) != 2 || p.Tags[1] != "esperanto" {
			t.Errorf("tags = %v", p.Tags)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hit %d times, want 1", hits.Load())
	}
}
