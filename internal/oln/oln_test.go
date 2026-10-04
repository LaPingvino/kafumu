package oln

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
