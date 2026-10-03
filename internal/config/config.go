// Package config reads deployment settings from the environment, so the
// brand and domain (kafumu.com is not final) are one app.yaml edit away.
package config

import "os"

type Config struct {
	Port      string
	ProjectID string
	Brand     string
	// Origin is the public origin, e.g. https://lokumo.ew.r.appspot.com.
	// Passkeys bind to its host; after a domain move users sign in once with
	// their magic link and add a new passkey.
	Origin string
}

func Load() *Config {
	return &Config{
		Port:      env("PORT", "8080"),
		ProjectID: env("GOOGLE_CLOUD_PROJECT", "lokumo"),
		Brand:     env("KAFUMU_BRAND", "Kafumu"),
		Origin:    env("KAFUMU_ORIGIN", "http://localhost:8080"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
