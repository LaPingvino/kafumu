package oln

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mine is eolnpoc's CreatePoWMessage, with a fixed time.
func mine(bitsWanted int, at time.Time, msg, keywords string) string {
	format := "%d;" + at.UTC().Format("20060102150405") + ";" + base64.URLEncoding.EncodeToString([]byte(msg)) + ";" + keywords
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
	if got := n.ExpiresAt.Sub(n.At); got != BaseTTL*time.Duration(1<<(n.Bits-BaseBits)) && got != MaxTTL {
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
				r := fmt.Sprintf("%d;%s;%s;#geo8ccgqw", i, now.Format("20060102150405"), base64.URLEncoding.EncodeToString([]byte("x")))
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
	if Required(0, 0) != 12 || Required(30, 0) != 13 || Required(90, 0) != 14 || Required(1e9, 0) != MaxBits {
		t.Errorf("Required: %d %d %d", Required(0, 0), Required(30, 0), Required(90, 0))
	}
	// A burst: 75 messages in ten minutes → 4 doublings.
	if Required(75, 75) != 16 {
		t.Errorf("burst Required = %d, want 16", Required(75, 75))
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
