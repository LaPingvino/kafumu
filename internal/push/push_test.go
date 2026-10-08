package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeClient struct {
	reqs   []*http.Request
	status int
}

func (f *fakeClient) Do(r *http.Request) (*http.Response, error) {
	f.reqs = append(f.reqs, r)
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestSubscribeAndNotify(t *testing.T) {
	st := NewMemoryStore()
	fc := &fakeClient{status: 201}
	s := &Service{Store: st, Contact: "https://kafumu.test", Client: fc}
	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	rand.Read(auth)
	box := strings.Repeat("ab", 32)
	tag := "p" + strings.Repeat("ef", 16) // an OLN pair tag: chat lines and answers (77f)
	body := `{"subscription":{"endpoint":"https://push.example/x","keys":{"p256dh":"` +
		base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()) + `","auth":"` + base64.RawURLEncoding.EncodeToString(auth) +
		`"}},"boxes":["` + box + `","not-a-box","` + tag + `"]}`
	w := httptest.NewRecorder()
	s.Subscribe(w, httptest.NewRequest("POST", "/api/push/subscribe", strings.NewReader(body)))
	if w.Code != http.StatusNoContent {
		t.Fatalf("subscribe: %d", w.Code)
	}
	s.Notify(context.Background(), box)
	if len(fc.reqs) != 1 || fc.reqs[0].URL.String() != "https://push.example/x" || !strings.HasPrefix(fc.reqs[0].Header.Get("Authorization"), "vapid ") {
		t.Fatalf("push requests = %v", fc.reqs)
	}
	s.Notify(context.Background(), strings.Repeat("cd", 32))
	if len(fc.reqs) != 1 {
		t.Error("notified for a box nobody watches")
	}
	s.Notify(context.Background(), tag)
	if len(fc.reqs) != 2 {
		t.Errorf("a watched pair tag: %d requests", len(fc.reqs))
	}
	fc.status = http.StatusGone
	s.Notify(context.Background(), box)
	if subs, _ := st.ForBox(context.Background(), box); len(subs) != 0 {
		t.Error("gone subscription not removed")
	}
}
