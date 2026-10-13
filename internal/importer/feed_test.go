package importer

import (
	"net"
	"strings"
	"testing"
)

func TestParseFeed(t *testing.T) {
	rss := `<?xml version="1.0"?><rss version="2.0"><channel><title>Stadskrant</title><link>https://krant.example/</link>
<item><title>Kermis op het &amp; plein</title><link>https://krant.example/kermis</link>
<description>&lt;p&gt;Vanaf &lt;b&gt;vrijdag&lt;/b&gt;&amp;nbsp;draait de kermis.&lt;/p&gt;</description>
<pubDate>Fri, 9 Oct 2026 10:00:00 +0200</pubDate><guid>krant-1</guid></item>
<item><title></title><link>https://krant.example/empty</link></item>
<item><title>No link</title><link>javascript:alert(1)</link></item>
</channel></rss>`
	f, ok := ParseFeed(rss)
	if !ok || f.Title != "Stadskrant" || len(f.Items) != 1 {
		t.Fatalf("rss = %v %+v", ok, f)
	}
	it := f.Items[0]
	if it.Title != "Kermis op het & plein" || it.Text != "Vanaf vrijdag draait de kermis." || it.GUID != "krant-1" || it.At.IsZero() {
		t.Errorf("rss item = %+v", it)
	}

	atom := `<feed xmlns="http://www.w3.org/2005/Atom"><title>Concerten</title><link href="https://zaal.example/"/>
<entry><title>Jazz avond</title><link rel="alternate" href="https://zaal.example/jazz"/><id>tag:zaal,1</id>
<updated>2026-10-10T20:00:00Z</updated><summary>Live in de zaal</summary></entry></feed>`
	if f, ok = ParseFeed(atom); !ok || f.Link != "https://zaal.example/" || len(f.Items) != 1 || f.Items[0].Link != "https://zaal.example/jazz" || f.Items[0].GUID != "tag:zaal,1" || f.Items[0].At.IsZero() {
		t.Errorf("atom = %v %+v", ok, f)
	}

	if _, ok = ParseFeed("<html><body>not a feed</body></html>"); ok {
		t.Error("an HTML page parsed as a feed")
	}
}

func TestFetchFeedRefusesLocal(t *testing.T) {
	if _, err := New().FetchFeed(t.Context(), "http://127.0.0.1:9/feed"); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Errorf("local feed: %v", err)
	}
}

func TestPublicRefusesSharedAddressSpace(t *testing.T) {
	for _, a := range []string{"100.64.0.1", "169.254.169.254", "10.0.0.1", "::1", "fd00::1", "::ffff:127.0.0.1"} {
		if public(net.ParseIP(a)) {
			t.Errorf("%s counted as public", a)
		}
	}
	if !public(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 refused")
	}
}
