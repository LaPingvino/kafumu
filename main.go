// Kafumu: what is near me right now that I would want to know about?
// See VISION.md for the design; CLAUDE.md for working rules.
package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"

	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/handler"
)

//go:embed templates/*.html
var templateFS embed.FS

func main() {
	cfg := config.Load()
	tmpl := template.Must(template.New("").Funcs(handler.Funcs).ParseFS(templateFS, "templates/*.html"))

	home := &handler.Home{Cfg: cfg, Tmpl: tmpl, Bsky: bsky.NewClient(), Gaz: gazetteer.Load()}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", home.ShowHome)
	mux.HandleFunc("GET /about", home.ShowAbout)
	mux.HandleFunc("GET /bundle", home.Bundle)
	// Locally serve what app.yaml serves statically on GAE.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/robots.txt") })

	log.Printf("%s listening on :%s", cfg.Brand, cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
