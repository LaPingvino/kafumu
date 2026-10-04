package box

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/pow"
)

const id = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func serve(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/box/{id}", h.Get)
	mux.HandleFunc("POST /api/box/{id}", h.Post)
	mux.HandleFunc("POST /api/box/{id}/ack", h.Ack)
	return mux
}

func call(t *testing.T, s http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == "POST" && !strings.HasSuffix(path, "/ack") && len(body) <= MaxMessage {
		r.Header.Set("X-Kafumu-Work", pow.Mine([]byte(body), "box"+strings.TrimPrefix(path, "/api/box/"), time.Now()))
	}
	s.ServeHTTP(w, r)
	return w
}

func list(t *testing.T, s http.Handler) []Message {
	t.Helper()
	var out struct{ Messages []Message }
	if err := json.Unmarshal(call(t, s, "GET", "/api/box/"+id, "").Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Messages
}

func TestBox(t *testing.T) {
	s := serve(NewHandler(NewMemoryStore()))

	if w := call(t, s, "GET", "/api/box/nothex", ""); w.Code != http.StatusBadRequest {
		t.Errorf("bad id: %d", w.Code)
	}
	if w := call(t, s, "GET", "/api/box/"+id, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"messages":[]`) {
		t.Errorf("unknown box: %d %s", w.Code, w.Body)
	}
	if w := call(t, s, "POST", "/api/box/"+id, strings.Repeat("a", MaxMessage+1)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize: %d", w.Code)
	}
	for _, m := range []string{"one", "two"} {
		if w := call(t, s, "POST", "/api/box/"+id, m); w.Code != http.StatusNoContent {
			t.Fatalf("post: %d", w.Code)
		}
	}
	ms := list(t, s)
	if len(ms) != 2 || ms[0].Data != "one" {
		t.Fatalf("list = %+v", ms)
	}
	// Reading does not delete; only an ack does.
	if len(list(t, s)) != 2 {
		t.Error("GET was destructive")
	}
	ack, _ := json.Marshal([]string{ms[0].ID, "unknown"})
	if w := call(t, s, "POST", "/api/box/"+id+"/ack", string(ack)); w.Code != http.StatusNoContent {
		t.Fatalf("ack: %d", w.Code)
	}
	if ms = list(t, s); len(ms) != 1 || ms[0].Data != "two" {
		t.Errorf("after ack = %+v", ms)
	}
	for i := 0; i < MaxMessages; i++ {
		call(t, s, "POST", "/api/box/"+id, "x")
	}
	if w := call(t, s, "POST", "/api/box/"+id, "x"); w.Code != http.StatusTooManyRequests {
		t.Errorf("full box: %d", w.Code)
	}
}

func TestRateLimit(t *testing.T) {
	h := NewHandler(NewMemoryStore())
	h.limiter = newLimiter(3, 1<<62)
	s := serve(h)
	for i := 0; i < 3; i++ {
		call(t, s, "GET", "/api/box/"+id, "")
	}
	if w := call(t, s, "GET", "/api/box/"+id, ""); w.Code != http.StatusTooManyRequests {
		t.Errorf("4th request: %d", w.Code)
	}
}

// countingStore counts reads that reach the underlying store.
type countingStore struct {
	Store
	reads int
}

func (c *countingStore) List(ctx context.Context, id string) ([]Message, error) {
	c.reads++
	return c.Store.List(ctx, id)
}

func TestCachedStore(t *testing.T) {
	inner := &countingStore{Store: NewMemoryStore()}
	s := &CachedStore{Store: inner, Cache: newTestCache()}
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		s.List(ctx, id)
	}
	if inner.reads != 1 {
		t.Errorf("5 polls of an empty box cost %d reads, want 1", inner.reads)
	}
	m, _ := NewMessage("x")
	s.Append(ctx, id, m)
	if ms, _ := s.List(ctx, id); len(ms) != 1 {
		t.Errorf("write not visible after append: %v", ms)
	}
	s.Ack(ctx, id, []string{m.ID})
	if ms, _ := s.List(ctx, id); len(ms) != 0 {
		t.Errorf("ack not visible: %v", ms)
	}
}

type testCache struct{ m map[string][]byte }

func newTestCache() *testCache { return &testCache{m: map[string][]byte{}} }
func (c *testCache) Get(_ context.Context, k string) ([]byte, bool) {
	v, ok := c.m[k]
	return v, ok
}
func (c *testCache) Set(_ context.Context, k string, v []byte, _ time.Duration) { c.m[k] = v }
func (c *testCache) Delete(_ context.Context, k string)                         { delete(c.m, k) }

func TestPostNeedsWork(t *testing.T) {
	s := serve(NewHandler(NewMemoryStore()))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("POST", "/api/box/"+id, strings.NewReader("x")))
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("post without work: %d", w.Code)
	}
}

func TestInboxPrice(t *testing.T) {
	h := NewHandler(NewMemoryStore())
	prices := NewPrices(nil)
	prices.Set(context.Background(), id, 6)
	h.Price = prices.Price
	s := serve(h)
	cheap := httptest.NewRequest("POST", "/api/box/"+id, strings.NewReader("hi"))
	cheap.Header.Set("X-Kafumu-Work", pow.MineBits([]byte("hi"), "box"+id, time.Now(), pow.MinBits))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, cheap)
	// A cheap stamp may happen to reach 6 bits; only check when it didn't.
	if w.Code == http.StatusNoContent && pow.CheckBits(cheap.Header.Get("X-Kafumu-Work"), []byte("hi"), "box"+id, time.Now(), 6) != nil {
		t.Errorf("under-priced message accepted")
	}
	paid := httptest.NewRequest("POST", "/api/box/"+id, strings.NewReader("hi there"))
	paid.Header.Set("X-Kafumu-Work", pow.MineBits([]byte("hi there"), "box"+id, time.Now(), 6))
	w = httptest.NewRecorder()
	s.ServeHTTP(w, paid)
	if w.Code != http.StatusNoContent {
		t.Errorf("paid message: %d", w.Code)
	}
}
