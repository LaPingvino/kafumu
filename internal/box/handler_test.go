package box

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	s.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
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
