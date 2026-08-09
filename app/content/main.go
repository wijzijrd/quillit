package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/quillit/content-svc/internal/db"
	"github.com/quillit/content-svc/internal/handler"
)

func main() {
	port := env("PORT", "3004")
	dbPath := env("DB_PATH", "./quillit-content.db")

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	health := handler.NewHealth(database)
	smoke := handler.NewSmoke()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health probe — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	// Proves pkg/contentengine imports and runs correctly from inside
	// this service. Remove once #37+ add real content-engine-backed
	// endpoints.
	r.Get("/smoke", smoke.Parse)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("quillit-content-svc listening on %s (HTTP/2 cleartext)", addr)
	h2s := &http2.Server{}
	log.Fatal(http.ListenAndServe(addr, h2c.NewHandler(r, h2s)))
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
