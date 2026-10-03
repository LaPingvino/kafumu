package slot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlot(t *testing.T) {
	h := NewHandler(NewMemoryStore())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/slot/{id}", h.Get)
	mux.HandleFunc("PUT /api/slot/{id}", h.Put)
	id := strings.Repeat("ab", 32)
	call := func(method, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, "/api/slot/"+id, strings.NewReader(body)))
		return w
	}
	if w := call("GET", ""); !strings.Contains(w.Body.String(), `"tokens":[]`) {
		t.Errorf("empty slot: %s", w.Body)
	}
	tok := strings.Repeat("0f", 16)
	if w := call("PUT", `["`+tok+`"]`); w.Code != http.StatusNoContent {
		t.Fatalf("put: %d", w.Code)
	}
	if w := call("GET", ""); !strings.Contains(w.Body.String(), tok) {
		t.Errorf("get: %s", w.Body)
	}
	if w := call("PUT", `["not a token"]`); w.Code != http.StatusBadRequest {
		t.Errorf("bad token: %d", w.Code)
	}
}
