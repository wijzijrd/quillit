// @title           Quillit BFF API
// @version         1.0
// @description     Backend-for-frontend: session management, campaigns, entries, annotations.
// @host            localhost:3000
// @BasePath        /

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	_ "github.com/quillit/svc/docs"
	"github.com/quillit/svc/internal/db"
	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/session"
)

func main() {
	port := env("PORT", "3000")
	dbPath := env("DB_PATH", "./quillit.db")
	authURL := env("AUTH_SERVICE_URL", "http://localhost:3002")
	corsOrigin := env("CORS_ORIGIN", "http://localhost:5173")
	jwtSecret := mustEnv("JWT_SECRET")
	cookieSecure := os.Getenv("COOKIE_SECURE") == "true"

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	sessions := session.NewStore(database, cookieSecure)

	auth := handler.NewAuth(authURL, sessions, jwtSecret)
	campaigns := handler.NewCampaigns(database)
	entries := handler.NewEntries(database)
	annotations := handler.NewAnnotations(database)
	quickview := handler.NewQuickView(database)
	share := handler.NewShare(database)
	migrate := handler.NewMigrate(database)
	categories := handler.NewCategories(database)
	relations := handler.NewRelations(database)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware(corsOrigin))

	// Auth routes (no session required)
	r.Get("/api/auth/status", auth.Status)
	r.Post("/api/auth/register", auth.Register)
	r.Post("/api/auth/login", auth.Login)
	r.Post("/api/auth/logout", auth.Logout)
	r.Get("/api/auth/me", auth.Me)

	// Public share routes (no session required)
	r.Get("/api/share/{token}", share.GetEntries)
	r.Get("/api/share/{token}/notes", share.ListNotes)
	r.Post("/api/share/{token}/notes", share.CreateNote)
	r.Patch("/api/share/{token}/notes/{noteId}", share.UpdateNote)
	r.Delete("/api/share/{token}/notes/{noteId}", share.DeleteNote)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireSession(sessions))

		r.Get("/api/entries", entries.List)
		r.Get("/api/entries/{id}", entries.Get)
		r.Post("/api/entries", entries.Create)
		r.Patch("/api/entries/{id}", entries.Update)
		r.Delete("/api/entries/{id}", entries.Delete)

		r.Get("/api/campaigns", campaigns.List)
		r.Post("/api/campaigns", campaigns.Create)
		r.Patch("/api/campaigns/{id}", campaigns.Update)
		r.Delete("/api/campaigns/{id}", campaigns.Delete)
		r.Post("/api/campaigns/{campaignId}/players", campaigns.AddPlayer)
		r.Delete("/api/campaigns/{campaignId}/players/{playerId}", campaigns.RemovePlayer)

		r.Get("/api/annotations", annotations.List)
		r.Post("/api/annotations", annotations.Create)
		r.Patch("/api/annotations/{id}", annotations.Update)
		r.Delete("/api/annotations/{id}", annotations.Delete)

		r.Get("/api/quickview", quickview.List)
		r.Put("/api/quickview/{category}", quickview.Upsert)
		r.Delete("/api/quickview/{category}", quickview.Delete)

		r.Post("/api/migrate/import", migrate.Import)

		r.Get("/api/categories", categories.List)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin([]byte(jwtSecret)))
			r.Post("/api/categories", categories.Create)
			r.Patch("/api/categories/{id}", categories.Update)
			r.Delete("/api/categories/{id}", categories.Delete)
			r.Post("/api/categories/{id}/tags", categories.AddTag)
			r.Delete("/api/categories/{id}/tags/{tagId}", categories.RemoveTag)
		})

		r.Get("/api/relation-labels", relations.Labels)
		r.Get("/api/entries/{id}/relations", relations.ListForEntry)
		r.Post("/api/entries/{id}/relations", relations.Create)
		r.Delete("/api/entry-relations/{relationId}", relations.Delete)
	})

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("quillit-svc listening on %s (HTTP/2 cleartext)", addr)
	h2s := &http2.Server{}
	log.Fatal(http.ListenAndServe(addr, h2c.NewHandler(r, h2s)))
}

func corsMiddleware(origin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env var %s is required", key)
	}
	return v
}
