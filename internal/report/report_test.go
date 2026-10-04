package report

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LaPingvino/kafumu/internal/pow"
)

func post(s *Service, body string, work bool) int {
	req := httptest.NewRequest("POST", "/api/report", strings.NewReader(body))
	if work {
		req.Header.Set("X-Kafumu-Work", pow.Mine([]byte(body), "report", time.Now()))
	}
	w := httptest.NewRecorder()
	s.Handle(w, req)
	return w.Code
}

func TestReportQueueHide(t *testing.T) {
	s, ctx := New(nil), context.Background()
	if c := post(s, `{"kind":"post","item":"at://x/1","reason":"spam"}`, false); c != http.StatusPaymentRequired {
		t.Fatalf("no work = %d", c)
	}
	if c := post(s, `{"kind":"post","item":"at://x/1","reason":"nope"}`, true); c != http.StatusBadRequest {
		t.Fatalf("bad reason = %d", c)
	}
	post(s, `{"kind":"post","item":"at://x/1","reason":"spam","snippet":"buy"}`, true)
	post(s, `{"kind":"post","item":"at://x/1","reason":"scam"}`, true)
	post(s, `{"kind":"note","item":"abc","reason":"other"}`, true)
	q, _ := s.Queue(ctx, time.Now())
	if len(q) != 2 || q[0].Item != "at://x/1" || q[0].Count != 2 || q[0].Reasons["spam"] != 1 {
		t.Fatalf("queue = %+v", q)
	}
	if err := s.Hide(ctx, "post", "at://x/1"); err != nil {
		t.Fatal(err)
	}
	if !s.Hidden(ctx, "post", "at://x/1") || s.Hidden(ctx, "post", "at://x/2") {
		t.Fatal("hidden set wrong")
	}
	if q, _ = s.Queue(ctx, time.Now()); len(q) != 1 {
		t.Fatalf("after hide queue = %+v", q)
	}
}
