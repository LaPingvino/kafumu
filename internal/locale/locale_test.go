package locale

import (
	"reflect"
	"strings"
	"testing"
)

func TestAllLanguagesHaveAllKeys(t *testing.T) {
	want := Keys(Fallback)
	for _, l := range Langs() {
		if got := Keys(l.Code); !reflect.DeepEqual(got, want) {
			t.Errorf("%s keys differ from %s: got %d, want %d", l.Code, Fallback, len(got), len(want))
		}
		for _, k := range want {
			// Placeholders must survive translation.
			for _, ph := range []string{"{tag}", "{tags}", "{n}", "{name}", "{from}", "{to}"} {
				if strings.Contains(T(Fallback, k), ph) != strings.Contains(T(l.Code, k), ph) {
					t.Errorf("%s %s: placeholder %s mismatch", l.Code, k, ph)
				}
			}
		}
	}
	for _, c := range []string{"en", "pt", "eo", "nl"} {
		if !Supported(c) {
			t.Errorf("%s missing", c)
		}
	}
}

func TestPick(t *testing.T) {
	cases := []struct{ choice, accept, want string }{
		{"", "pt-PT,pt;q=0.9,en;q=0.8", "pt"},
		{"eo", "pt-PT", "eo"},
		{"xx", "nl-BE,fr", "nl"},
		{"", "qq,xx", "en"},
		{"", "zh-TW,zh;q=0.9", "zh-hant"},
		{"", "zh-Hant-HK", "zh-hant"},
		{"", "zh-CN,zh", "zh"},
		{"", "", "en"},
	}
	for _, c := range cases {
		if got := Pick(c.choice, c.accept); got != c.want {
			t.Errorf("Pick(%q,%q) = %s, want %s", c.choice, c.accept, got, c.want)
		}
	}
}
