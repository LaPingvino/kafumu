package oln

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// mine is eolnpoc's CreatePoWMessage, with a fixed time.
func mine(bitsWanted int, at time.Time, msg, keywords string) string {
	format := "v2;%d;" + at.UTC().Format("20060102150405") + ";" + base64.URLEncoding.EncodeToString([]byte(msg)) + ";" + keywords
	for i := 0; ; i++ {
		raw := fmt.Sprintf(format, i)
		if Bits(raw) >= bitsWanted {
			return raw
		}
	}
}

// mineExact is mine with exactly bitsWanted: luck can give mine a few
// extra bits, and each doubles the lifetime, which a test about expiry
// can't have.
func mineExact(bitsWanted int, at time.Time, msg, keywords string) string {
	format := "v2;%d;" + at.UTC().Format("20060102150405") + ";" + base64.URLEncoding.EncodeToString([]byte(msg)) + ";" + keywords
	for i := 0; ; i++ {
		raw := fmt.Sprintf(format, i)
		if Bits(raw) == bitsWanted {
			return raw
		}
	}
}

func TestPostAndRank(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)
	s := NewService(NewMemoryStore())
	s.Now = func() time.Time { return now }

	raw := mine(BaseBits, now, "Kafo ĉe Pavilono 2?", "#geo8ccgqw #langepo #WebSummit")
	n, err := s.Post(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	if n.Cell != "8ccgqw" || n.Text != "Kafo ĉe Pavilono 2?" || len(n.Tags) != 3 || n.Tags[2] != "websummit" {
		t.Errorf("note = %+v", n)
	}
	// Anonymous: half the life its work would buy (Joop).
	if got := n.ExpiresAt.Sub(n.At); got != BaseTTL*time.Duration(1<<(n.Bits-BaseBits))/2 && got != MaxTTL/2 {
		t.Errorf("ttl = %v for %d bits", got, n.Bits)
	}
	if again, err := s.Post(ctx, raw); err != nil || again.ID != n.ID {
		t.Errorf("repost: %v", err)
	}
	for _, bad := range []struct {
		raw  string
		want error
	}{
		{"nonsense", ErrFormat},
		{mine(BaseBits, now.Add(-20*time.Minute), "old", "#geo8ccgqw"), ErrClock},
		{mine(BaseBits, now, "where?", "#hello"), ErrPlace},
		{func() string { // too little work: find a hash with exactly 0 leading zero bits
			for i := 0; ; i++ {
				r := fmt.Sprintf("v2;%d;%s;%s;#geo8ccgqw", i, now.Format("20060102150405"), base64.URLEncoding.EncodeToString([]byte("x")))
				if Bits(r) == 0 {
					return r
				}
			}
		}(), ErrWork},
	} {
		if _, err := s.Post(ctx, bad.raw); !errors.Is(err, bad.want) {
			t.Errorf("Post(%.30q) err = %v, want %v", bad.raw, err, bad.want)
		}
	}
	strong, _ := s.Post(ctx, mine(BaseBits+4, now, "Lasting notice", "#geo8ccgqw"))
	list, _ := s.InCells(ctx, []string{"8ccgqw"})
	if len(list) != 2 || list[0].ID != strong.ID {
		t.Errorf("more work should rank first: %+v", list)
	}
	s.Hide(ctx, strong.ID)
	if list, _ = s.InCells(ctx, []string{"8ccgqw"}); len(list) != 1 {
		t.Errorf("hidden note still listed")
	}
	now = now.Add(2 * time.Hour)
	s.cells = map[string]cellEntry{}
	if list, _ = s.InCells(ctx, []string{"8ccgqw"}); len(list) != 0 && list[0].Bits == BaseBits {
		t.Errorf("1-hour note still listed after 2 hours: %+v", list)
	}
}

func TestRequiredAndTTL(t *testing.T) {
	if Required(0, 0) != BaseBits || Required(30, 0) != BaseBits+1 || Required(90, 0) != BaseBits+2 || Required(1e9, 0) != MaxBits {
		t.Errorf("Required: %d %d %d", Required(0, 0), Required(30, 0), Required(90, 0))
	}
	// A burst: 75 messages in ten minutes → 4 doublings.
	if Required(75, 75) != BaseBits+4 {
		t.Errorf("burst Required = %d, want %d", Required(75, 75), BaseBits+4)
	}
	if TTL(14, 14) != time.Hour || TTL(18, 14) != 16*time.Hour || TTL(30, 14) != MaxTTL || TTL(13, 14) != 0 {
		t.Error("TTL")
	}
}

func TestHandlePost(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	for _, c := range []struct {
		raw  string
		code int
	}{
		{mine(BaseBits, now, "Olá Barreiro", "#geo8ccgmw #langpor"), 200},
		{"garbage", 400},
	} {
		w := httptest.NewRecorder()
		s.HandlePost(w, httptest.NewRequest("POST", "/api/oln", strings.NewReader(c.raw)))
		if w.Code != c.code {
			t.Errorf("%.20q → %d, want %d: %s", c.raw, w.Code, c.code, w.Body)
		}
	}
}

func TestExport(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	n, err := s.Post(context.Background(), mine(BaseBits, now, "Olá", "#geo8ccgmw #coffee"))
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.Export("https://kafumu.test", "Kafumu")(w, httptest.NewRequest("GET", "/oln.json?cell=8ccgmw", nil))
	var f Format
	if err := json.Unmarshal(w.Body.Bytes(), &f); err != nil {
		t.Fatal(err)
	}
	m, ok := f.Messages[n.ID]
	if !ok || m.Raw != n.Raw || Bits(m.Raw) < BaseBits || len(f.Index["#coffee"]) != 1 || f.Push[0] != "https://kafumu.test/api/oln" {
		t.Errorf("export = %+v", f)
	}
}

// Questions are findable by subject from anywhere; ordinary notes are not.
func TestAsks(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	q, err := s.Post(context.Background(), mine(BaseBits, now, "Anyone into Go here?", "#geo8ccgmw #ask #opensource #langeng"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Post(context.Background(), mine(BaseBits, now, "Just a note", "#geo8ccgmw #opensource")); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Asks(context.Background(), []string{"opensource"})
	if len(got) != 1 || got[0].ID != q.ID || !slices.Equal(q.Asks, []string{"opensource"}) {
		t.Fatalf("asks = %+v (q.Asks %v)", got, q.Asks)
	}
}

// Replies and reactions are found by what they answer, from any area
// (77b): a reply posted far from the post still comes back to its author.
func TestReplies(t *testing.T) {
	s := NewService(NewMemoryStore())
	ctx, now := context.Background(), time.Now().UTC()
	post, err := s.Post(ctx, mine(BaseBits, now, "Coffee at the square?", "#geo8ccgmw"))
	if err != nil {
		t.Fatal(err)
	}
	re := post.ID[:10]
	near, _ := s.Post(ctx, mine(BaseBits, now, "👍", "#geo8ccgmw #re"+re))
	far, _ := s.Post(ctx, mine(BaseBits, now.Add(time.Second), "Count me in", "#geo9c2v2v #re"+re))
	if _, err := s.Post(ctx, mine(BaseBits, now, "unrelated", "#geo8ccgmw")); err != nil {
		t.Fatal(err)
	}
	if near.Re != re || far.Re != re || post.Re != "" {
		t.Fatalf("re = %q %q %q", near.Re, far.Re, post.Re)
	}
	got, err := s.Replies(ctx, []string{re, "0123456789"})
	if err != nil || len(got) != 2 || got[0].ID != near.ID || got[1].ID != far.ID {
		t.Fatalf("replies = %+v, %v", got, err)
	}
}

// Private messages: no place, one pair tag; a week's life at base work;
// fetched by tag, never in an area's list.
func TestPairMessages(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	tag := "p0123456789abcdef0123456789abcdef"
	n, err := s.Post(context.Background(), mine(BaseBits, now, "c2VjcmV0", "#"+tag))
	if err != nil {
		t.Fatal(err)
	}
	if n.Cell != "" || n.ExpiresAt.Sub(n.At) != PairTTL {
		t.Fatalf("pair note = %+v", n)
	}
	got, _ := s.ForPair(context.Background(), tag)
	if len(got) != 1 || got[0].ID != n.ID {
		t.Fatalf("for pair = %+v", got)
	}
	if _, err := s.Post(context.Background(), mine(BaseBits, now, "no place", "#coffee")); err != ErrPlace {
		t.Fatalf("placeless public note = %v", err)
	}
}

// Anonymous messages live half as long and rank below named ones (Joop).
func TestAuthor(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	anon, _ := s.Post(context.Background(), mine(BaseBits, now, "anon", "#geo8ccgmw"))
	named, _ := s.PostAs(context.Background(), mine(BaseBits, now, "named", "#geo8ccgmw"), "lapingvino")
	if anon.Author != "" || named.Author != "lapingvino" {
		t.Fatalf("authors %q %q", anon.Author, named.Author)
	}
	// Each against the life its own work buys (mining can overshoot the bits).
	if got, want := anon.ExpiresAt.Sub(anon.At), TTL(anon.Bits, BaseBits)/2; got != want {
		t.Fatalf("anon life %v, want half of %v", got, 2*want)
	}
	if got, want := named.ExpiresAt.Sub(named.At), TTL(named.Bits, BaseBits); got != want {
		t.Fatalf("named life %v, want %v", got, want)
	}
	// Same work, the name ranks higher.
	a2, n2 := *anon, *named
	a2.Bits, n2.Bits, a2.ExpiresAt, n2.ExpiresAt = BaseBits, BaseBits, now.Add(time.Hour), now.Add(time.Hour)
	if Priority(&n2, now) <= Priority(&a2, now) {
		t.Fatal("named should rank above anonymous")
	}
}

// Node policy: a repeat in the same cell is dropped; elsewhere it costs more.
func TestRepeats(t *testing.T) {
	s := NewService(NewMemoryStore())
	now := time.Now().UTC()
	ctx := context.Background()
	text := "Cheap sunglasses at the main square!"
	if _, err := s.Post(ctx, mine(BaseBits, now, text, "#geo8ccgmw")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Post(ctx, mine(BaseBits, now, text+" ", "#geo8ccgmw")); !errors.Is(err, ErrRepeat) {
		t.Fatalf("same cell repeat = %v", err)
	}
	_, err := s.Post(ctx, mine(BaseBits, now, text, "#geo9f469w"))
	var ne *NeedError
	if !errors.As(err, &ne) || ne.Need != BaseBits+RepeatBits {
		t.Fatalf("other cell repeat = %v", err)
	}
	if _, err := s.Post(ctx, mine(BaseBits, now, "👍", "#geo8ccgmw #re0123456789")); err != nil {
		t.Fatalf("reaction: %v", err)
	}
	if _, err := s.Post(ctx, mine(BaseBits, now, "👍", "#geo8ccgmw #re0123456789")); err != nil {
		t.Fatalf("second identical reaction: %v", err)
	}
}

// A mass release (Joop): a batch mined ahead of time for one moment, all
// posted at once in one area. Mining early doesn't make it cheaper: the
// node prices each message on arrival against what's already there, so
// as the area fills the cheap ones are refused (402 with a higher
// need) and the rest of the batch was work for nothing.
func TestMassRelease(t *testing.T) {
	s := NewService(NewMemoryStore())
	at := time.Now().UTC().Add(24 * time.Hour) // the target moment, a day ahead
	batch := make([]string, 30)
	for i := range batch {
		batch[i] = mine(BaseBits, at, fmt.Sprintf("Big sale, stall %d!", i), "#geo8ccgmw")
	}
	s.Now = func() time.Time { return at.Add(time.Minute) } // released inside the window
	ok := 0
	var need *NeedError
	for _, raw := range batch {
		_, err := s.Post(context.Background(), raw)
		switch {
		case err == nil:
			ok++
		case errors.As(err, &need):
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	// Mining "at base" overshoots (half the lines have a bit more), so about
	// half the batch gets in before the price outruns it; the rest is lost.
	if ok >= 22 || need == nil || need.Need <= BaseBits {
		t.Fatalf("mass release: %d of 30 pre-mined messages accepted (last need %+v)", ok, need)
	}
	t.Logf("mass release: %d of 30 accepted, then the node asked for %d bits", ok, need.Need)
}

// The same attack, offset (Joop: "you can offset a little and still
// defend"): every line claims a time 9:50 in the past (still inside the
// window) and they arrive one every 20 seconds. Counted by claimed time,
// each would leave the ten-minute count within seconds and the price would
// never rise; counted by arrival, it rises as with a plain burst.
func TestOffsetTrickle(t *testing.T) {
	s := NewService(NewMemoryStore())
	start := time.Now().UTC()
	now := start
	s.Now = func() time.Time { return now }
	ok := 0
	for i := 0; i < 30; i++ {
		now = start.Add(time.Duration(i) * 20 * time.Second)
		raw := mine(BaseBits, now.Add(-9*time.Minute-50*time.Second), fmt.Sprintf("Cheap watches %d", i), "#geo8ccgmw")
		if _, err := s.Post(context.Background(), raw); err == nil {
			ok++
		} else if _, isNeed := err.(*NeedError); !isNeed {
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok >= 22 {
		t.Fatalf("offset trickle: %d of 30 accepted; the price should rise with arrivals", ok)
	}
	t.Logf("offset trickle: %d of 30 accepted", ok)
}

// A full 500-character chat line is ciphertext in base64, far longer than
// 500: private messages take it; public notes keep the readable limit.
func TestSizes(t *testing.T) {
	now := time.Now().UTC()
	if _, err := Parse(mine(0, now, strings.Repeat("A", 2800), "#p0123456789abcdef0123456789abcdef"), now); err != nil {
		t.Fatalf("long private message: %v", err)
	}
	if _, err := Parse(mine(0, now, strings.Repeat("A", 501), "#geo8ccgmw"), now); !errors.Is(err, ErrFormat) {
		t.Fatalf("501-character public note: %v", err)
	}
	if _, err := Parse(mine(0, now, strings.Repeat("ĉ", 500), "#geo8ccgmw"), now); err != nil {
		t.Fatalf("500 characters (1000 bytes) public: %v", err)
	}
}

// Posting under your name while acting as a business: the note carries the
// business and whether it was live (gold wings) or not (grey). Anonymous
// posts never do.
func TestBizByline(t *testing.T) {
	s := NewService(NewMemoryStore())
	s.AuthorFor = func(r *http.Request) string {
		if r.Header.Get("X-Kafumu-As") == "1" {
			return "joop"
		}
		return ""
	}
	live := true
	s.BizFor = func(*http.Request) (string, bool) { return "Café Teste", live }
	post := func(text string, named bool) *Note {
		r := httptest.NewRequest("POST", "/api/oln", strings.NewReader(mine(BaseBits, time.Now().UTC(), text, "#geo8ccgmw")))
		if named {
			r.Header.Set("X-Kafumu-As", "1")
		}
		w := httptest.NewRecorder()
		s.HandlePost(w, r)
		var n Note
		json.Unmarshal(w.Body.Bytes(), &n)
		return &n
	}
	if n := post("Fresh croissants", true); n.Biz != "Café Teste" || !n.BizLive || n.Author != "" {
		t.Fatalf("named as business: %+v", n)
	}
	live = false
	if n := post("Still here", true); n.Biz != "Café Teste" || n.BizLive {
		t.Fatalf("after the trial: %+v", n)
	}
	if n := post("Anonymous", false); n.Biz != "" || n.Author != "" {
		t.Fatalf("anonymous: %+v", n)
	}
	// The manager is on record (moderation) but not in public, and the post
	// ranks and lives as a named one.
	ns, _ := s.InCells(context.Background(), []string{"8ccgmw"})
	for _, n := range ns {
		if n.Biz != "" && (n.By != "joop" || n.ExpiresAt.Sub(n.At) != TTL(n.Bits, BaseBits)) {
			t.Fatalf("business post: by %q, life %v", n.By, n.ExpiresAt.Sub(n.At))
		}
	}
}

// A patron's named post wears the wings; an anonymous one never does.
func TestPatronPost(t *testing.T) {
	s := NewService(NewMemoryStore())
	s.AuthorFor = func(r *http.Request) string {
		if r.Header.Get("X-Kafumu-As") == "1" {
			return "joop"
		}
		return ""
	}
	s.PatronFor = func(*http.Request) bool { return true }
	for _, named := range []bool{true, false} {
		r := httptest.NewRequest("POST", "/api/oln", strings.NewReader(mine(BaseBits, time.Now().UTC(), fmt.Sprint("patron ", named), "#geo8ccgmw")))
		if named {
			r.Header.Set("X-Kafumu-As", "1")
		}
		w := httptest.NewRecorder()
		s.HandlePost(w, r)
		var n Note
		json.Unmarshal(w.Body.Bytes(), &n)
		if n.Patron != named {
			t.Fatalf("named=%v: patron=%v", named, n.Patron)
		}
	}
}

// A private message (chat line, answer) wakes whoever watches its tag (77f).
func TestOnPair(t *testing.T) {
	s := NewService(NewMemoryStore())
	var woke []string
	s.OnPair = func(_ context.Context, tag string) { woke = append(woke, tag) }
	tag := "p0123456789abcdef0123456789abcdef"
	for _, raw := range []string{mine(BaseBits, time.Now().UTC(), "c2VjcmV0", "#"+tag), mine(BaseBits, time.Now().UTC(), "public", "#geo8ccgmw")} {
		w := httptest.NewRecorder()
		s.HandlePost(w, httptest.NewRequest("POST", "/api/oln", strings.NewReader(raw)))
		if w.Code != 200 {
			t.Fatalf("post: %d %s", w.Code, w.Body)
		}
	}
	if len(woke) != 1 || woke[0] != tag {
		t.Fatalf("woke = %v", woke)
	}
}

// A reaction needs no place (78a): "#re<id>" alone is filed under
// Everywhere, found by what it answers, and priced by that thing's replies.
func TestReactionWithoutPlace(t *testing.T) {
	s := NewService(NewMemoryStore())
	ctx, now := context.Background(), time.Now().UTC()
	post, err := s.Post(ctx, mine(BaseBits, now, "Coffee at the square?", "#geo8ccgmw"))
	if err != nil {
		t.Fatal(err)
	}
	re := post.ID[:10]
	r, err := s.Post(ctx, mine(BaseBits, now, "👍", "#re"+re))
	if err != nil || r.Cell != Everywhere || r.Re != re {
		t.Fatalf("reaction = %+v, %v", r, err)
	}
	if got, _ := s.Replies(ctx, []string{re}); len(got) != 1 || got[0].ID != r.ID {
		t.Fatalf("replies = %+v", got)
	}
	if got, _ := s.InCells(ctx, []string{"8ccgmw"}); len(got) != 1 {
		t.Fatalf("the area's list has %d, want just the post", len(got))
	}
	if _, err := s.Post(ctx, mine(BaseBits, now, "no place, no re", "#esperanto")); err != ErrPlace {
		t.Fatalf("subject-only: %v (78a-3 adds those)", err)
	}
	w := httptest.NewRecorder()
	s.HandleRequired(w, httptest.NewRequest("GET", "/api/oln/required?re="+re, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"bits"`) {
		t.Fatalf("required?re: %d %s", w.Code, w.Body)
	}
}
