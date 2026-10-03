// Kafumu: what is near me right now that I would want to know about?
// See VISION.md for the design; CLAUDE.md for working rules.
package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/appengine/v2"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/box"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/handler"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/meetup"
	"github.com/LaPingvino/kafumu/internal/purge"
	"github.com/LaPingvino/kafumu/internal/slot"
)

//go:embed templates/*.html
var templateFS embed.FS

func main() {
	cfg := config.Load()
	tmpl := template.Must(template.New("").Funcs(handler.Funcs).ParseFS(templateFS, "templates/*.html"))

	home := &handler.Home{Cfg: cfg, Tmpl: tmpl, Bsky: bsky.NewClient(), Gaz: gazetteer.Load()}
	users, boxes, meetupStore, slots, db := stores(cfg)
	home.Meetups = meetup.NewService(meetupStore)
	meetups := &handler.Meetups{Home: home, Svc: home.Meetups, Importer: importer.New()}
	accounts := &handler.Accounts{Home: home, Svc: account.NewService(users)}
	home.Accounts = accounts.Svc
	kv := cache.New()
	mailbox := box.NewHandler(&box.CachedStore{Store: boxes, Cache: kv})
	slotAPI := slot.NewHandler(&slot.CachedStore{Store: slots, Cache: kv})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", home.ShowHome)
	mux.HandleFunc("GET /about", home.ShowAbout)
	mux.HandleFunc("GET /patrons", home.Info("patrons.html", "patrons.title"))
	mux.HandleFunc("GET /for-cafes", home.Info("cafes.html", "cafes.title"))
	mux.HandleFunc("GET /privacy", home.Info("privacy.html", "privacy.title"))
	mux.HandleFunc("GET /bundle", home.Bundle)
	mux.HandleFunc("GET /places", home.Places)
	mux.HandleFunc("GET /card", home.ShowCard)
	mux.HandleFunc("GET /connect", home.ShowConnect)
	mux.HandleFunc("GET /badge", home.ShowBadge)
	mux.HandleFunc("GET /c", home.ShowAccept)
	mux.HandleFunc("GET /contacts", home.ShowContacts)
	mux.HandleFunc("GET /m", home.ShowMove)
	mux.HandleFunc("GET /account", accounts.Show)
	mux.HandleFunc("POST /account/start", accounts.Start)
	mux.HandleFunc("POST /account/name", accounts.SetName)
	mux.HandleFunc("POST /account/profile", accounts.SetProfile)
	mux.HandleFunc("POST /account/signout", accounts.SignOut)
	mux.HandleFunc("POST /account/delete", accounts.Delete)
	mux.HandleFunc("GET /auth/link", accounts.Link)
	mux.HandleFunc("GET /meetups/new", meetups.New)
	mux.HandleFunc("POST /meetups", meetups.Create)
	mux.HandleFunc("POST /meetups/import", meetups.Import)
	mux.HandleFunc("GET /meetups/{id}", meetups.Show)
	mux.HandleFunc("POST /meetups/{id}/rsvp", meetups.RSVP)
	mux.HandleFunc("POST /meetups/{id}/delete", meetups.Delete)
	mux.HandleFunc("GET /meetups/{id}/ics", meetups.ICS)
	mux.HandleFunc("GET /cal/{cell}", meetups.ICS)
	mux.HandleFunc("GET /cron/feeds", meetups.SyncFeeds)
	mux.HandleFunc("GET /cron/purge", func(w http.ResponseWriter, r *http.Request) {
		// App Engine cron sets this header and strips it from outside requests.
		if r.Header.Get("X-Appengine-Cron") != "true" || db == nil {
			http.NotFound(w, r)
			return
		}
		res, err := purge.Run(r.Context(), db, time.Now())
		log.Printf("purge: %s err=%v", res, err)
		fmt.Fprintf(w, "%s err=%v\n", res, err)
	})
	mux.HandleFunc("GET /api/slot/{id}", slotAPI.Get)
	mux.HandleFunc("PUT /api/slot/{id}", slotAPI.Put)
	mux.HandleFunc("GET /api/box/{id}", mailbox.Get)
	mux.HandleFunc("POST /api/box/{id}", mailbox.Post)
	mux.HandleFunc("POST /api/box/{id}/ack", mailbox.Ack)
	// Locally serve what app.yaml serves statically on GAE.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/sw.js") })
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/robots.txt") })

	root := cache.Middleware(accounts.Middleware(mux))
	if cache.OnAppEngine() {
		// Bundled services (memcache) need appengine.Main to serve.
		http.Handle("/", root)
		appengine.Main()
		return
	}
	log.Printf("%s listening on :%s", cfg.Brand, cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, root))
}

// stores uses Datastore on App Engine (or with the emulator) and memory for
// plain local runs, so `go run .` needs no credentials.
func stores(cfg *config.Config) (account.Store, box.Store, meetup.Store, slot.Store, *datastore.Client) {
	if os.Getenv("GAE_ENV") == "" && os.Getenv("DATASTORE_EMULATOR_HOST") == "" {
		log.Printf("stores: in memory (set DATASTORE_EMULATOR_HOST to use the emulator)")
		return account.NewMemoryStore(), box.NewMemoryStore(), meetup.NewMemoryStore(), slot.NewMemoryStore(), nil
	}
	db, err := datastore.NewClient(context.Background(), cfg.ProjectID)
	if err != nil {
		log.Fatalf("datastore: %v", err)
	}
	return &account.DatastoreStore{DB: db}, &box.DatastoreStore{DB: db}, &meetup.DatastoreStore{DB: db}, &slot.DatastoreStore{DB: db}, db
}
