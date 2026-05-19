package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"rehab-app/internal/config"
	"rehab-app/internal/db"
	appmiddleware "rehab-app/internal/middleware"
	"rehab-app/internal/site"
	"rehab-app/internal/web"
)

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.RunMigrations {
		if err := db.RunMigrations(database, "migrations"); err != nil {
			log.Fatal(err)
		}
	}
	if err := db.EnsureNutritionCompatibility(database); err != nil {
		log.Fatal(err)
	}

	if cfg.SeedData {
		if err := db.Seed(database); err != nil {
			log.Fatal(err)
		}
	}

	router := chi.NewRouter()
	router.Use(middleware.RealIP)
	router.Use(middleware.StripSlashes)
	router.Use(appmiddleware.Logger)
	router.Use(appmiddleware.Recover)
	router.Use((&appmiddleware.CSRFManager{
		CookieName: cfg.CookieName + "_csrf",
		Secure:     cfg.CookieSecure,
	}).Protect)

	renderer, err := web.NewRenderer()
	if err != nil {
		log.Fatal(err)
	}

	sessions := &appmiddleware.SessionManager{
		DB:         database,
		CookieName: cfg.CookieName,
		SessionTTL: cfg.SessionTTL,
	}

	router.Handle("/assets/*", web.StaticHandler())
	router.Handle("/uploads/*", http.StripPrefix("/uploads/", http.FileServer(http.Dir("uploads"))))
	router.Mount("/", site.New(database, renderer, sessions, cfg).Router())

	log.Printf("Server running on %s", cfg.Addr)
	log.Fatal(http.ListenAndServe(cfg.Addr, router))
}
