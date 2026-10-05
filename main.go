// Kafumu: what is near me right now that I would want to know about?
// See VISION.md for the design; CLAUDE.md for working rules.
package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"github.com/LaPingvino/kafumu/internal/feeds"
	"github.com/LaPingvino/kafumu/internal/kv"
	"github.com/LaPingvino/kafumu/internal/sqlstore"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/bluesky-social/indigo/atproto/auth/oauth"
	"google.golang.org/appengine/v2"

	"github.com/LaPingvino/kafumu/internal/account"
	"github.com/LaPingvino/kafumu/internal/atp"
	"github.com/LaPingvino/kafumu/internal/box"
	"github.com/LaPingvino/kafumu/internal/bsky"
	"github.com/LaPingvino/kafumu/internal/business"
	"github.com/LaPingvino/kafumu/internal/cache"
	"github.com/LaPingvino/kafumu/internal/config"
	"github.com/LaPingvino/kafumu/internal/gazetteer"
	"github.com/LaPingvino/kafumu/internal/handle"
	"github.com/LaPingvino/kafumu/internal/handler"
	"github.com/LaPingvino/kafumu/internal/importer"
	"github.com/LaPingvino/kafumu/internal/locale"
	"github.com/LaPingvino/kafumu/internal/meetup"
	"github.com/LaPingvino/kafumu/internal/oln"
	"github.com/LaPingvino/kafumu/internal/purge"
	"github.com/LaPingvino/kafumu/internal/push"
	"github.com/LaPingvino/kafumu/internal/report"
	"github.com/LaPingvino/kafumu/internal/short"
	"github.com/LaPingvino/kafumu/internal/slot"
	"github.com/LaPingvino/kafumu/internal/vault"
)

//go:embed templates/*.html
var templateFS embed.FS

func main() {
	cfg := config.Load()
	tmpl := template.Must(template.New("").Funcs(handler.Funcs).ParseFS(templateFS, "templates/*.html"))

	home := &handler.Home{Cfg: cfg, Tmpl: tmpl, Bsky: bsky.NewClient(), Gaz: gazetteer.Load()}
	if sq := sqliteDB(); sq != nil {
		kv.Default = &sqlstore.KV{DB: sq} // before the stores that load from it
	}
	users, boxes, meetupStore, slots, db := stores(cfg)
	kv := cache.New()
	home.Cache = kv
	feeds.Locate = home.Gaz.Locate
	home.Meetups = meetup.NewService(meetupStore)
	notes := oln.NewService(olnStore(db))
	notes.AuthorFor = func(r *http.Request) string {
		if u := handler.UserFrom(r.Context()); u != nil && u.Username != "" && r.Header.Get("X-Kafumu-As") == "1" {
			return u.Username
		}
		return ""
	}
	notes.BizFor = func(r *http.Request) (string, bool) {
		if b := home.ActingAs(r); b != nil {
			return b.Name, b.Live(time.Now())
		}
		return "", false
	}
	home.Notes = notes
	businesses := business.New(db)
	home.Biz = businesses
	meetups := &handler.Meetups{Home: home, Svc: home.Meetups, Importer: importer.New(), Businesses: businesses}
	accounts := &handler.Accounts{Home: home, Svc: account.NewService(users)}
	home.Accounts = accounts.Svc
	mailbox := box.NewHandler(&box.CachedStore{Store: boxes, Cache: kv})
	slotAPI := slot.NewHandler(&slot.CachedStore{Store: slots, Cache: kv})
	var pushStore push.Store = push.NewMemoryStore()
	if db != nil {
		pushStore = &push.DatastoreStore{DB: db}
	}
	pusher := &push.Service{Store: pushStore, Contact: cfg.Origin, Text: func(l string) string { return locale.T(l, "push.signal") }}
	mailbox.OnAppend = pusher.Notify
	prices := box.NewPrices(db)
	mailbox.Price = prices.Price
	accounts.Prices = prices
	accounts.Cache = kv
	accounts.Handles = handle.New(db)
	home.DB = db
	home.MakerLive = func(ctx context.Context, name string) bool {
		_, ok := accounts.Handles.Get(ctx, name, time.Now())
		return ok
	}
	home.FollowHandle = accounts.FollowHandle
	accounts.Vault = vault.NewMemoryStore()
	if db != nil {
		accounts.Vault = &vault.DatastoreStore{DB: db}
	} else if sq := sqliteDB(); sq != nil {
		accounts.Vault = &sqlstore.Vaults{DB: sq}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", home.ShowHome)
	mux.HandleFunc("GET /about", home.ShowAbout)
	mux.HandleFunc("GET /patrons", home.Info("patrons.html", "patrons.title"))
	// The venues page is gone until there's a model worth describing (Joop).
	mux.HandleFunc("GET /for-cafes", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/about", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /privacy", home.Info("privacy.html", "privacy.title"))
	mux.HandleFunc("GET /oln", home.Info("oln.html", "olnpage.title"))
	mux.HandleFunc("GET /built", home.Info("built.html", "built.title"))
	mux.HandleFunc("GET /bundle", home.Bundle)
	mux.HandleFunc("GET /places", home.Places)
	mux.HandleFunc("GET /tagposts", home.TagPosts)
	mux.HandleFunc("GET /card", home.ShowCard)
	mux.HandleFunc("GET /connect", home.ShowConnect)
	mux.HandleFunc("GET /badge", home.ShowBadge)
	mux.HandleFunc("GET /c", home.ShowAccept)
	mux.HandleFunc("GET /contacts", home.ShowContacts)
	mux.HandleFunc("GET /m", home.ShowMove)
	mux.HandleFunc("GET /import", home.ShowImport)
	mux.HandleFunc("GET /account/link.json", accounts.LinkJSON)
	if cfg.ATproto {
		var st oauth.ClientAuthStore = atp.NewMemoryStore()
		if db != nil {
			st = &atp.DatastoreStore{DB: db}
		}
		at := &handler.ATproto{Accounts: accounts, Svc: atp.New(cfg.Origin, cfg.Brand, st)}
		home.ATproto = at.Svc
		mux.HandleFunc("GET /oauth/client-metadata.json", at.Svc.Metadata)
		mux.HandleFunc("POST /oauth/login", at.Login)
		mux.HandleFunc("GET /oauth/callback", at.Callback)
		mux.HandleFunc("POST /oauth/disconnect", at.Disconnect)
		mux.HandleFunc("POST /post", at.Post)
	}
	if cfg.Passkeys {
		if pk := handler.NewPasskeys(accounts, cfg.Origin, kv); pk != nil {
			mux.HandleFunc("POST /auth/passkey/register/begin", pk.RegisterBegin)
			mux.HandleFunc("POST /auth/passkey/register/finish", pk.RegisterFinish)
			mux.HandleFunc("POST /auth/passkey/login/begin", pk.LoginBegin)
			mux.HandleFunc("POST /auth/passkey/login/finish", pk.LoginFinish)
		}
	}
	mux.HandleFunc("GET /account", accounts.Show)
	mux.HandleFunc("GET /findable", accounts.ShowFindable)
	mux.HandleFunc("POST /account/start", accounts.Start)
	mux.HandleFunc("POST /account/name", accounts.SetName)
	mux.HandleFunc("POST /account/profile", accounts.SetProfile)
	mux.HandleFunc("POST /account/inbox", accounts.SetInbox)
	mux.HandleFunc("GET /account/move", accounts.MoveRequest)
	mux.HandleFunc("POST /account/move", accounts.MoveRequest)
	mux.HandleFunc("DELETE /account/move", accounts.MoveRequest)
	biz := &handler.Businesses{Home: home, Accounts: accounts, Store: businesses}
	mux.HandleFunc("POST /account/as", biz.Use)
	mux.HandleFunc("POST /business/{id}/name", biz.Name)
	mux.HandleFunc("POST /business/{id}/sync", biz.Sync)
	mux.HandleFunc("GET /api/business/{id}/vault", biz.VaultAPI)
	mux.HandleFunc("PUT /api/business/{id}/vault", biz.VaultAPI)
	mux.HandleFunc("GET /api/business/{id}/key", biz.KeyAPI)
	mux.HandleFunc("GET /api/business/{id}/keyreq", biz.KeyReqAPI)
	mux.HandleFunc("POST /api/business/{id}/keyreq", biz.KeyReqAPI)
	mux.HandleFunc("POST /api/business/{id}/keygrant", biz.KeyReqAPI)
	accounts.BizProfile = biz.Profile
	mux.HandleFunc("GET /business", biz.Show)
	mux.HandleFunc("POST /business", biz.Create)
	mux.HandleFunc("POST /business/{id}/managers", biz.Managers)
	mux.HandleFunc("PUT /api/handle", accounts.SetHandle)
	mux.HandleFunc("GET /api/handle", accounts.HandleViews)
	mux.HandleFunc("DELETE /api/handle", accounts.DeleteHandle)
	mux.HandleFunc("GET /api/vault", accounts.VaultAPI)
	mux.HandleFunc("PUT /api/vault", accounts.VaultAPI)
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
	mux.HandleFunc("GET /api/esperanto/online", meetups.EsperantoOnline)
	runPurge := func(ctx context.Context) string {
		if sq := sqliteDB(); sq != nil && db == nil {
			res, err := sqlstore.Purge(ctx, sq, time.Now())
			log.Printf("purge (sqlite): %s err=%v", res, err)
			return fmt.Sprintf("%s err=%v", res, err)
		}
		if db == nil {
			return "no Datastore"
		}
		res, err := purge.Run(ctx, db, time.Now())
		log.Printf("purge: %s err=%v", res, err)
		return fmt.Sprintf("%s err=%v", res, err)
	}
	if sqliteDB() != nil && db == nil {
		// Self-hosted: no cron and no TTL policy, so purge here, at start
		// and every six hours.
		go func() {
			for {
				runPurge(context.Background())
				time.Sleep(6 * time.Hour)
			}
		}()
	}
	mux.HandleFunc("GET /cron/purge", func(w http.ResponseWriter, r *http.Request) {
		// App Engine cron sets this header and strips it from outside requests.
		if r.Header.Get("X-Appengine-Cron") != "true" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, runPurge(r.Context()))
	})
	adminH := &handler.Admin{Home: home, Accounts: accounts, Meetups: meetups, Notes: notes, DB: db, SQL: sqliteDB(),
		Jobs: map[string]func(context.Context) string{"purge": runPurge, "feeds": meetups.RunFeeds}}
	adminH.Jobs["oln-pull"] = adminH.PullPeers
	// Linked nodes: pulled from cron where scheduled (on App Engine each
	// cron wake costs instance hours, so it isn't in cron.yaml by default),
	// and every ten minutes self-hosted.
	mux.HandleFunc("GET /cron/oln-pull", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Appengine-Cron") != "true" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintln(w, adminH.PullPeers(r.Context()))
	})
	if sqliteDB() != nil && db == nil {
		go func() {
			for {
				time.Sleep(10 * time.Minute)
				if res := adminH.PullPeers(context.Background()); res != "no linked nodes" {
					log.Printf("oln-pull: %s", res)
				}
			}
		}()
	}
	mux.HandleFunc("GET /admin/initial", adminH.Initial)
	mux.HandleFunc("GET /admin", adminH.Show)
	mux.HandleFunc("POST /admin/action", adminH.Action)
	mux.HandleFunc("GET /api/slot/{id}", slotAPI.Get)
	mux.HandleFunc("PUT /api/slot/{id}", slotAPI.Put)
	mux.HandleFunc("POST /api/oln", notes.HandlePost)
	mux.HandleFunc("GET /api/oln/required", notes.HandleRequired)
	mux.HandleFunc("GET /api/oln/pair/{tag}", notes.HandlePair)
	mux.HandleFunc("GET /api/asks", func(w http.ResponseWriter, r *http.Request) {
		if handler.IsBot(r) {
			http.Error(w, "not for robots", http.StatusForbidden)
			return
		}
		notes.HandleAsks(w, r)
	})
	mux.HandleFunc("GET /oln.json", notes.Export(cfg.Origin, cfg.Brand))
	reports := report.New(db)
	home.Reports = reports
	adminH.Reports = reports
	adminH.Businesses = businesses
	mux.HandleFunc("POST /api/report", reports.Handle)
	shorts := short.New(db)
	mux.HandleFunc("POST /api/short", shorts.Make)
	mux.HandleFunc("GET /j/{code}", shorts.Follow)
	mux.HandleFunc("GET /api/push/key", pusher.Key)
	mux.HandleFunc("POST /api/push/subscribe", pusher.Subscribe)
	mux.HandleFunc("POST /api/push/unsubscribe", pusher.Unsubscribe)
	mux.HandleFunc("GET /api/box/{id}", mailbox.Get)
	mux.HandleFunc("POST /api/box/{id}", mailbox.Post)
	mux.HandleFunc("POST /api/box/{id}/ack", mailbox.Ack)
	// Locally serve what app.yaml serves statically on GAE.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/sw.js") })
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "static/robots.txt") })

	root := cache.Middleware(home.CanonicalHost(accounts.Middleware(mux)))
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
	if sq := sqliteDB(); sq != nil {
		// Self-hosting: stores move to SQLite one by one (LOOP-STATE 64b).
		return &sqlstore.Accounts{DB: sq}, &sqlstore.Boxes{DB: sq}, &sqlstore.Meetups{DB: sq}, &sqlstore.Slots{DB: sq}, nil
	}
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

func olnStore(db *datastore.Client) oln.Store {
	if sq := sqliteDB(); sq != nil {
		return &sqlstore.Notes{DB: sq}
	}
	if db == nil {
		return oln.NewMemoryStore()
	}
	return &oln.DatastoreStore{DB: db}
}

// sqliteDB opens KAFUMU_SQLITE (self-hosting) once; nil when unset. Stores
// move to it one by one (LOOP-STATE 64b); the rest stay in memory there.
var sqliteDB = sync.OnceValue(func() *sql.DB {
	path := os.Getenv("KAFUMU_SQLITE")
	if path == "" {
		return nil
	}
	db, err := sqlstore.Open(path)
	if err != nil {
		log.Fatalf("sqlite: %v", err)
	}
	log.Printf("stores: SQLite at %s (local messages, meetups, accounts, mailboxes, slots, sync; more to come)", path)
	return db
})
