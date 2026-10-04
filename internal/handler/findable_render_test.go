package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LaPingvino/kafumu/internal/account"
)

// The Findable page renders saved languages with CEFR levels.
func TestFindableRendersLevels(t *testing.T) {
	_, home, _ := newServerWithMeetups(t)
	p := accountPage{page: home.newPage(httptest.NewRequest("GET", "/findable", nil), ""), Findable: true,
		MyLangs: []myLang{{"epo", "Esperanto", "B2"}, {"x:Ladino", "Ladino", "A2"}}}
	p.User = &account.User{ID: "u1", Username: "joop"}
	w := httptest.NewRecorder()
	home.render(w, "account.html", p)
	body := w.Body.String()
	for _, want := range []string{`name="level_epo"`, `value="B2" selected`, "Ladino", `id="own-lang"`} {
		if !strings.Contains(body, want) {
			t.Errorf("findable page lacks %q", want)
		}
	}
}
