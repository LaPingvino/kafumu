package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/business"
)

// The pay-what-you-want section gives the reference to put in the note:
// your @username, or the business's @name while acting as it.
func TestPatronsReference(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	render := func(u *account.User, acting *business.Business) string {
		p := infoPage{page: home.newPage(httptest.NewRequest("GET", "/patrons", nil), ""), PayPal: "https://paypal.example/joop"}
		p.User, p.Acting = u, acting
		w := httptest.NewRecorder()
		home.render(w, "patrons.html", p)
		return w.Body.String()
	}
	if b := render(&account.User{ID: "0123456789abcdef", Username: "joop"}, nil); !strings.Contains(b, `value="Kafumu @joop"`) || !strings.Contains(b, `id="pay"`) {
		t.Fatal("no @username reference")
	}
	if b := render(&account.User{ID: "0123456789abcdef", Username: "joop"}, &business.Business{ID: "b1", Username: "cafe-x"}); !strings.Contains(b, `value="Kafumu @cafe-x"`) {
		t.Fatal("no business reference while acting")
	}
	if b := render(&account.User{ID: "0123456789abcdef"}, nil); !strings.Contains(b, `value="Kafumu 01234567"`) || !strings.Contains(b, `href="/account"`) {
		t.Fatal("no fallback reference for an account without a username")
	}
}

// Without any way to pay configured (a fork, a self-hosted node), the page
// doesn't ask people to pay "through the ways above".
func TestPatronsNoLinks(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	p := infoPage{page: home.newPage(httptest.NewRequest("GET", "/patrons", nil), "")}
	w := httptest.NewRecorder()
	home.render(w, "patrons.html", p)
	if strings.Contains(w.Body.String(), `id="pay"`) {
		t.Fatal("pay-what-you-want shown with no way to pay")
	}
}
