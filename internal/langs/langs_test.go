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

// The holywritings.net languages (from Joop's holywritings.db) are all
// pickable.
func TestHolywritingsLanguages(t *testing.T) {
	for _, code := range []string{"eng", "por", "fas", "deu", "jpn", "nld", "ron", "cat", "pol", "aze", "ell", "isl", "cmn",
		"spa", "fra", "kor", "hat", "tha", "hin", "dan", "bul", "tgl", "ita", "mal", "hye", "rus", "hun", "kir", "urd", "ara",
		"slk", "ind", "fin", "swe", "lav", "afr", "nor", "bos", "sqi", "vie", "kan", "mlg", "ben", "lug", "kal", "cym", "bis",
		"hrv", "est", "mlt", "her", "epo", "fij", "slv", "amh", "tpi", "srn", "gil", "cha", "nai", "ukr", "fry", "eus", "bel",
		"mah", "lit", "sot", "ces", "cnr", "cos", "mri", "pap", "ltz", "ipk", "tam", "nep", "haw", "khm", "fao", "gle"} {
		if Names[code] == "" {
			t.Errorf("%s missing", code)
		}
	}
}
