package short

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/pow"
)

func TestShort(t *testing.T) {
	s := New(nil)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/short", s.Make)
	mux.HandleFunc("GET /j/{code}", s.Follow)
	payload := "v1." + strings.Repeat("A", 87)
	body := `{"payload":"` + payload + `"}`
	r := httptest.NewRequest("POST", "/api/short", strings.NewReader(body))
	r.Header.Set("X-Kafumu-Work", pow.Mine([]byte(body), "short", time.Now()))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("make: %d %s", w.Code, w.Body)
	}
	code := strings.Split(w.Body.String(), `"`)[3]
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/j/"+strings.ToLower(code), nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/c#"+payload {
		t.Errorf("follow: %d %q", w.Code, w.Header().Get("Location"))
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/short", strings.NewReader(body)))
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("no work: %d", w.Code)
	}
}
