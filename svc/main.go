// @title           Quillit BFF API
// @version         1.0
// @description     Backend-for-frontend: session management, campaigns, entries, annotations.
// @host            localhost:3000
// @BasePath        /

package main

import (
	"context"
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
	"github.com/quillit/svc/internal/storage"
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

	// MinIO blob storage (optional — gracefully skipped if not configured).
	var blobs *storage.MinioStore
	if os.Getenv("MINIO_ENDPOINT") != "" {
		store, err := storage.NewMinio()
		if err != nil {
			log.Printf("minio: init failed (%v) — blob storage disabled", err)
		} else if err := store.EnsureBucket(context.Background()); err != nil {
			log.Printf("minio: bucket init failed (%v) — blob storage disabled", err)
		} else {
			blobs = store
			log.Printf("minio: blob storage enabled")
		}
	}

	auth := handler.NewAuth(authURL, sessions, jwtSecret)
	admin := handler.NewAdmin(database, jwtSecret, authURL)
	campaigns := handler.NewCampaigns(database)
	projects := handler.NewProjects(database, jwtSecret)
	entries := handler.NewEntriesWithBlobs(database, jwtSecret, blobs)
	entryShares := handler.NewEntryShares(database, jwtSecret, authURL)
	annotations := handler.NewAnnotations(database, jwtSecret)
	member := handler.NewMember(database, jwtSecret)
	settings := handler.NewSettings(database, jwtSecret)
	quickview := handler.NewQuickView(database)
	share := handler.NewShare(database)
	migrate := handler.NewMigrate(database)
	categories := handler.NewCategories(database)
	relations := handler.NewRelations(database)
	health := handler.NewHealth(database)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware(corsOrigin))

	// Health probe (no session required) — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

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
		r.Use(middleware.RequireSession(sessions, []byte(jwtSecret)))

		r.Get("/api/entries", entries.List)
		r.Get("/api/entries/{id}", entries.Get)
		r.Post("/api/entries", entries.Create)
		r.Patch("/api/entries/{id}", entries.Update)
		r.Delete("/api/entries/{id}", entries.Delete)
		r.Post("/api/entries/{id}/images", entries.UploadImage)

		r.Get("/api/entries/{id}/shares", entryShares.ListShares)
		r.Post("/api/entries/{id}/shares", entryShares.AddShares)
		r.Delete("/api/entries/{id}/shares/{userId}", entryShares.RemoveShare)

		r.Get("/api/users/search", entryShares.SearchUsers)

		r.Get("/api/me/settings", settings.Get)
		r.Patch("/api/me/settings", settings.Update)

		// Projects API (typed replacement for campaigns)
		r.Get("/api/projects/types", projects.Types)
		r.Get("/api/projects", projects.List)
		r.Post("/api/projects", projects.Create)
		r.Post("/api/projects/join", projects.Join)
		r.Patch("/api/projects/{id}", projects.Update)
		r.Delete("/api/projects/{id}", projects.Delete)
		r.Get("/api/projects/{id}/members", projects.ListMembers)
		r.Post("/api/projects/{id}/members", projects.AddMember)
		r.Delete("/api/projects/{id}/members/{userId}", projects.RemoveMember)
		r.Post("/api/projects/{id}/invite", projects.CreateInvite)
		r.Delete("/api/projects/{id}/invite/{token}", projects.RevokeInvite)

		// Project categories
		r.Get("/api/projects/{projectId}/categories", projects.ListProjectCategories)
		r.Post("/api/projects/{projectId}/categories", projects.CreateProjectCategory)
		r.Post("/api/projects/{projectId}/categories/global/{catId}", projects.OptInGlobalCategory)
		r.Delete("/api/projects/{projectId}/categories/{catId}", projects.RemoveProjectCategory)

		// Legacy campaign routes (kept for backwards compat during transition)
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

			// Categories (admin only)
			r.Post("/api/categories", categories.Create)
			r.Patch("/api/categories/{id}", categories.Update)
			r.Delete("/api/categories/{id}", categories.Delete)
			r.Post("/api/categories/{id}/tags", categories.AddTag)
			r.Delete("/api/categories/{id}/tags/{tagId}", categories.RemoveTag)

			// Admin: user management (proxied to auth-svc)
			r.Get("/api/admin/users", admin.ListUsers)
			r.Patch("/api/admin/users/{id}", admin.UpdateUser)
			r.Delete("/api/admin/users/{id}", admin.DeleteUser)

			// Admin: project management
			r.Get("/api/admin/projects", admin.ListProjects)
			r.Get("/api/admin/projects/{id}/members", admin.ListProjectMembers)
		})

		r.Get("/api/relation-labels", relations.Labels)
		r.Get("/api/entries/{id}/relations", relations.ListForEntry)
		r.Post("/api/entries/{id}/relations", relations.Create)
		r.Delete("/api/entry-relations/{relationId}", relations.Delete)

		// Member routes
		r.Get("/api/member/shared", member.SharedNotes)
		r.Get("/api/member/folders", member.ListFolders)
		r.Post("/api/member/folders", member.CreateFolder)
		r.Patch("/api/member/folders/{id}", member.UpdateFolder)
		r.Delete("/api/member/folders/{id}", member.DeleteFolder)
		r.Post("/api/member/folders/{id}/entries", member.AddFolderEntry)
		r.Delete("/api/member/folders/{id}/entries/{entryId}", member.RemoveFolderEntry)
		r.Put("/api/member/entries/{id}/meta", member.UpsertEntryMeta)
		r.Get("/api/member/session-notes", member.ListSessionNotes)
		r.Post("/api/member/session-notes", member.CreateSessionNote)
		r.Patch("/api/member/session-notes/{id}", member.UpdateSessionNote)
		r.Delete("/api/member/session-notes/{id}", member.DeleteSessionNote)
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
