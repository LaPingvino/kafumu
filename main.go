// Kafumu: what is near me right now that I would want to know about?
// See VISION.md for the design; CLAUDE.md for working rules.
package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/datastore"

	"github.com/LaPingvino/kafumu/internal/account"
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
	accounts := &handler.Accounts{Home: home, Svc: account.NewService(userStore(cfg))}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", home.ShowHome)
	mux.HandleFunc("GET /about", home.ShowAbout)
	mux.HandleFunc("GET /bundle", home.Bundle)
	mux.HandleFunc("GET /card", home.ShowCard)
	mux.HandleFunc("GET /account", accounts.Show)
	mux.HandleFunc("POST /account/start", accounts.Start)
	mux.HandleFunc("POST /account/name", accounts.SetName)
	mux.HandleFunc("POST /account/signout", accounts.SignOut)
	mux.HandleFunc("POST /account/delete", accounts.Delete)
	mux.HandleFunc("GET /auth/link", accounts.Link)
	// Locally serve what app.yaml serves statically on GAE.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/robots.txt") })

	log.Printf("%s listening on :%s", cfg.Brand, cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, accounts.Middleware(mux)))
}

// userStore uses Datastore on App Engine (or with the emulator) and memory
// for plain local runs, so `go run .` needs no credentials.
func userStore(cfg *config.Config) account.Store {
	if os.Getenv("GAE_ENV") == "" && os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		log.Printf("accounts: in memory (set DATASTORE_EMULATOR_HOST to use the emulator)")
		return account.NewMemoryStore()
	}
	db, err := datastore.NewClient(context.Background(), cfg.ProjectID)
	if err != nil {
		log.Fatalf("datastore: %v", err)
	}
	return &account.DatastoreStore{DB: db}
}
