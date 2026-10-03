// Package langs is the curated language list for profiles: ISO 639-3
// codes (which cover constructed, minority and sign languages) with their
// English and native names.
package langs

import (
	_ "embed"
	"encoding/json"
	"sort"
)

// languages.json is vendored from github.com/LaPingvino/geotags: hashtags
// people use for languages → ISO 639-3 codes (empty = any language).
//
//go:embed languages.json
var tagsJSON []byte

// Tag is a language hashtag.
type Tag struct {
	Tag    string   `json:"tag"`
	Codes  []string `json:"codes"`
	Weight float64  `json:"weight"`
}

// Tags maps hashtag → codes and weight, for ranking on the device.
var Tags = func() map[string]Tag {
	var ts []Tag
	if err := json.Unmarshal(tagsJSON, &ts); err != nil {
		panic("langs: " + err.Error())
	}
	m := map[string]Tag{}
	for _, t := range ts {
		m[t.Tag] = t
	}
	return m
}()

// From1 maps ISO 639-1 (what ATproto posts and browsers use) to 639-3.
var From1 = map[string]string{
	"ar": "ara", "bg": "bul", "bn": "ben", "ca": "cat", "cs": "ces", "cy": "cym", "da": "dan", "de": "deu",
	"el": "ell", "en": "eng", "eo": "epo", "es": "spa", "et": "est", "eu": "eus", "fa": "fas", "fi": "fin",
	"fr": "fra", "fy": "fry", "ga": "gle", "gl": "glg", "he": "heb", "hi": "hin", "hr": "hrv", "hu": "hun",
	"ia": "ina", "id": "ind", "io": "ido", "is": "isl", "it": "ita", "ja": "jpn", "ka": "kat", "ko": "kor",
	"lt": "lit", "lv": "lav", "ms": "msa", "nl": "nld", "no": "nor", "nb": "nor", "nn": "nor", "pl": "pol",
	"pt": "por", "ro": "ron", "ru": "rus", "sk": "slk", "sl": "slv", "sq": "sqi", "sr": "srp", "sv": "swe",
	"sw": "swa", "th": "tha", "tr": "tur", "uk": "ukr", "ur": "urd", "vi": "vie", "yi": "yid", "zh": "cmn", "zu": "zul",
}

// Lang is one language.
type Lang struct {
	Code, English, Native string
}

// All is sorted by English name.
var All = func() []Lang {
	l := []Lang{
		{"ara", "Arabic", "العربية"}, {"ase", "American Sign Language", "ASL"}, {"bfi", "British Sign Language", "BSL"},
		{"ben", "Bengali", "বাংলা"}, {"bul", "Bulgarian", "български"}, {"cat", "Catalan", "català"},
		{"ces", "Czech", "čeština"}, {"cmn", "Mandarin Chinese", "普通话"}, {"cym", "Welsh", "Cymraeg"},
		{"dan", "Danish", "dansk"}, {"deu", "German", "Deutsch"}, {"ell", "Greek", "Ελληνικά"},
		{"eng", "English", "English"}, {"epo", "Esperanto", "Esperanto"}, {"est", "Estonian", "eesti"},
		{"eus", "Basque", "euskara"}, {"fas", "Persian", "فارسی"}, {"fin", "Finnish", "suomi"},
		{"fra", "French", "français"}, {"fry", "Frisian", "Frysk"}, {"gle", "Irish", "Gaeilge"},
		{"glg", "Galician", "galego"}, {"heb", "Hebrew", "עברית"}, {"hin", "Hindi", "हिन्दी"},
		{"hrv", "Croatian", "hrvatski"}, {"hun", "Hungarian", "magyar"}, {"ido", "Ido", "Ido"},
		{"ina", "Interlingua", "Interlingua"}, {"ind", "Indonesian", "Bahasa Indonesia"}, {"isl", "Icelandic", "íslenska"},
		{"ita", "Italian", "italiano"}, {"jpn", "Japanese", "日本語"}, {"kat", "Georgian", "ქართული"},
		{"kor", "Korean", "한국어"}, {"lfn", "Lingua Franca Nova", "elefen"}, {"lit", "Lithuanian", "lietuvių"},
		{"lav", "Latvian", "latviešu"}, {"msa", "Malay", "Bahasa Melayu"}, {"ngt", "Dutch Sign Language", "NGT"},
		{"nld", "Dutch", "Nederlands"}, {"nor", "Norwegian", "norsk"}, {"pol", "Polish", "polski"},
		{"por", "Portuguese", "português"}, {"psr", "Portuguese Sign Language", "LGP"}, {"ron", "Romanian", "română"},
		{"rus", "Russian", "русский"}, {"slk", "Slovak", "slovenčina"}, {"slv", "Slovenian", "slovenščina"},
		{"spa", "Spanish", "español"}, {"sqi", "Albanian", "shqip"}, {"srp", "Serbian", "српски"},
		{"swa", "Swahili", "Kiswahili"}, {"swe", "Swedish", "svenska"}, {"tha", "Thai", "ไทย"},
		{"tok", "Toki Pona", "toki pona"}, {"tur", "Turkish", "Türkçe"}, {"ukr", "Ukrainian", "українська"},
		{"urd", "Urdu", "اردو"}, {"vie", "Vietnamese", "Tiếng Việt"}, {"yid", "Yiddish", "ייִדיש"},
		{"yue", "Cantonese", "粵語"}, {"zul", "Zulu", "isiZulu"},
	}
	sort.Slice(l, func(i, j int) bool { return l[i].English < l[j].English })
	return l
}()

// Names maps code → "Native (English)" for display.
var Names = func() map[string]string {
	m := map[string]string{}
	for _, l := range All {
		if l.Native == l.English {
			m[l.Code] = l.Native
		} else {
			m[l.Code] = l.Native + " (" + l.English + ")"
		}
	}
	return m
}()
