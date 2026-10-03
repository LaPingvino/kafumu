package langs

import "testing"

func TestTags(t *testing.T) {
	if Tags["esperanto"].Codes[0] != "epo" || Tags["tokipona"].Weight != 1 {
		t.Errorf("tags = %+v %+v", Tags["esperanto"], Tags["tokipona"])
	}
	for tag, lt := range Tags {
		for _, c := range lt.Codes {
			if Names[c] == "" {
				t.Errorf("#%s → %s, which is not in the language list", tag, c)
			}
		}
	}
	for two, three := range From1 {
		if Names[three] == "" {
			t.Errorf("%s → %s missing from the language list", two, three)
		}
	}
}
