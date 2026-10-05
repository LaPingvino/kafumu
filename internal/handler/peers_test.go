package handler

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/oln"
)

func mineLine(at time.Time, msg, keywords string) string {
	f := "v2;%d;" + at.UTC().Format("20060102150405") + ";" + base64.URLEncoding.EncodeToString([]byte(msg)) + ";" + keywords
	for i := 0; ; i++ {
		if raw := fmt.Sprintf(f, i); oln.Bits(raw) >= oln.BaseBits {
			return raw
		}
	}
}

// Linking a node in admin: a peer with a message, linked with an area;
// "Pull now" brings the message here (marked via the peer), and the admin
// page shows the peer with its last pull.
func TestLinkedNodes(t *testing.T) {
	ctx := context.Background()
	peer := oln.NewService(oln.NewMemoryStore())
	if _, err := peer.Post(ctx, mineLine(time.Now(), "Kafo ĉe la placo?", "#geo8ccgmw")); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(peer.Export("http://peer.example", "Peer"))
	defer srv.Close()

	_, home, _ := newServerWithMeetups(t)
	a := &Admin{Home: home, Notes: oln.NewService(oln.NewMemoryStore())}
	if peerOrigin("ftp://x") != "" || peerOrigin("http://evil.example") != "" || peerOrigin("https://node.example/path") != "https://node.example" {
		t.Fatal("peerOrigin")
	}
	c := a.loadPeers(ctx)
	c.Peers = append(c.Peers, peerEntry{Origin: srv.URL, Cells: cleanCells("#geo8ccgmw, 8ccgmw nope")})
	if err := a.savePeers(ctx, c); err != nil || len(a.loadPeers(ctx).Peers[0].Cells) != 1 {
		t.Fatalf("save: %v %+v", err, a.loadPeers(ctx))
	}
	if res := a.PullPeers(ctx); !strings.Contains(res, "1 new") {
		t.Fatalf("pull: %s", res)
	}
	got, _ := a.Notes.InCells(ctx, []string{"8ccgmw"})
	if len(got) != 1 || got[0].Via != srv.URL {
		t.Fatalf("pulled: %+v", got)
	}
	p := adminPage{page: home.newPage(httptest.NewRequest("GET", "/admin", nil), "Admin"), Full: true, Peers: a.loadPeers(ctx).Peers}
	w := httptest.NewRecorder()
	home.render(w, "admin.html", p)
	if body := w.Body.String(); !strings.Contains(body, srv.URL) || !strings.Contains(body, "1 seen, 1 new") || !strings.Contains(body, "#geo8ccgmw") {
		t.Fatal("admin page lacks the linked node and its last pull")
	}
}
