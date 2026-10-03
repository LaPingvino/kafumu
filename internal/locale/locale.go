// Package locale provides UI strings and language detection. Adapted from
// esperanto-kurso; Kafumu falls back to English and ships Esperanto from day
// one on purpose (language events are a launch target).
package locale

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed [a-z]*.json
var files embed.FS

// Fallback is used for missing keys and unsupported languages.
const Fallback = "en"

var translations = map[string]map[string]string{}

func init() {
	entries, err := files.ReadDir(".")
	if err != nil {
		panic("locale: " + err.Error())
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := files.ReadFile(e.Name())
		if err != nil {
			panic("locale: " + err.Error())
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			panic("locale: " + e.Name() + ": " + err.Error())
		}
		translations[strings.TrimSuffix(e.Name(), ".json")] = m
	}
}

// Supported reports whether lang has a translation file.
func Supported(lang string) bool { _, ok := translations[lang]; return ok }

// Langs lists the supported languages, each with its own name ("lang.self").
func Langs() []Lang {
	var out []Lang
	for code := range translations {
		out = append(out, Lang{Code: code, Name: T(code, "lang.self")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Lang is a UI language choice.
type Lang struct{ Code, Name string }

// T looks up key in lang, then in the fallback, then returns the key.
func T(lang, key string) string {
	if v, ok := translations[lang][key]; ok {
		return v
	}
	if v, ok := translations[Fallback][key]; ok {
		return v
	}
	return key
}

// Prefix returns all keys starting with prefix in lang, with fallbacks filled
// in, for handing to client-side code.
func Prefix(lang, prefix string) map[string]string {
	out := map[string]string{}
	for k := range translations[Fallback] {
		if strings.HasPrefix(k, prefix) {
			out[strings.TrimPrefix(k, prefix)] = T(lang, k)
		}
	}
	return out
}

// Pick chooses the UI language: an explicit choice (cookie) wins, then the
// first supported entry of Accept-Language, then the fallback.
func Pick(choice, acceptLang string) string {
	if Supported(choice) {
		return choice
	}
	for _, part := range strings.Split(acceptLang, ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		if Supported(tag) {
			return tag
		}
		if p := strings.SplitN(tag, "-", 2)[0]; Supported(p) {
			return p
		}
	}
	return Fallback
}

// Keys returns the sorted keys of lang's own file (for tests).
func Keys(lang string) []string {
	var out []string
	for k := range translations[lang] {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
