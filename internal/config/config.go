// Package config reads deployment settings from the environment, so the
// brand and domain (kafumu.com is not final) are one app.yaml edit away.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port      string
	ProjectID string
	Brand     string
	// Origin is the public origin, e.g. https://lokumo.ew.r.appspot.com.
	// Passkeys bind to its host; after a domain move users sign in once with
	// their magic link and add a new passkey.
	Origin string
	// Version tags static asset URLs (?v=…). App Engine sets GAE_VERSION per
	// deploy; locally the start time does.
	Version string
	// LegacyOrigins are where Kafumu used to live (the appspot address);
	// pages there offer to move your device data to Origin.
	LegacyOrigins []string
	// Passkeys are bound to Origin's domain; switched on with KAFUMU_PASSKEYS=1.
	Passkeys bool
	// ATproto OAuth (connect your Bluesky account); KAFUMU_ATPROTO=1.
	ATproto bool
	// Money and contact links; pages show "coming soon" while empty.
	PayPal, Liberapay, Stripe, Contact string
}

func Load() *Config {
	return &Config{
		Port:          env("PORT", "8080"),
		ProjectID:     env("GOOGLE_CLOUD_PROJECT", "lokumo"),
		Brand:         env("KAFUMU_BRAND", "Kafumu"),
		Origin:        env("KAFUMU_ORIGIN", "http://localhost:8080"),
		Version:       env("GAE_VERSION", strconv.FormatInt(time.Now().Unix(), 36)),
		PayPal:        os.Getenv("KAFUMU_PAYPAL"),
		LegacyOrigins: strings.Fields(os.Getenv("KAFUMU_LEGACY_ORIGINS")),
		Passkeys:      os.Getenv("KAFUMU_PASSKEYS") == "1",
		ATproto:       os.Getenv("KAFUMU_ATPROTO") == "1",
		Liberapay:     os.Getenv("KAFUMU_LIBERAPAY"),
		Stripe:        os.Getenv("KAFUMU_STRIPE"),
		Contact:       env("KAFUMU_CONTACT", "https://github.com/LaPingvino/kafumu/issues"),
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
