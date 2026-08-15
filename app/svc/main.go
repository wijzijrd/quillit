// @title           Quillit BFF API
// @version         1.0
// @description     Backend-for-frontend: session management, entries, annotations.
// @host            localhost:3000
// @BasePath        /

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	_ "github.com/quillit/svc/docs"
	"github.com/quillit/svc/internal/contentclient"
	"github.com/quillit/svc/internal/db"
	"github.com/quillit/svc/internal/handler"
	"github.com/quillit/svc/internal/middleware"
	"github.com/quillit/svc/internal/session"
	"github.com/quillit/svc/internal/ws"
)

func main() {
	port := env("PORT", "3000")
	dbPath := env("DB_PATH", "./quillit.db")
	authURL := env("AUTH_SERVICE_URL", "http://localhost:3002")
	contentURL := env("CONTENT_SERVICE_URL", "http://localhost:3004")
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
	admin := handler.NewAdmin(database, jwtSecret, authURL)
	projects := handler.NewProjects(database, jwtSecret)
	settings := handler.NewSettings(database, jwtSecret)
	content := &contentclient.Client{BaseURL: contentURL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	// Shared in-process WebSocket hub for Game Mode chat (single-instance only).
	hub := ws.NewHub()
	gameSessions := handler.NewGameSessions(database, jwtSecret, hub)
	chatWS := handler.NewChatWS(database, jwtSecret, hub, content, corsOrigin)
	health := handler.NewHealth(database)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(corsMiddleware(corsOrigin))

	// Health probe (no session required) — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	// Auth routes (no session required)
	r.Get("/api/auth/status", auth.Status)
	r.Get("/api/auth/users/available", auth.UsernameAvailable)
	r.Post("/api/auth/register", auth.Register)
	r.Post("/api/auth/login", auth.Login)
	r.Post("/api/auth/logout", auth.Logout)
	r.Get("/api/auth/me", auth.Me)
	r.Post("/api/auth/forgot-password", auth.ForgotPassword)
	r.Post("/api/auth/reset-password", auth.ResetPassword)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireSession(sessions, []byte(jwtSecret), authURL))

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

		// Game Mode: live session control-plane + chat history
		r.Post("/api/projects/{projectId}/session/start", gameSessions.Start)
		r.Post("/api/projects/{projectId}/session/stop", gameSessions.Stop)
		r.Get("/api/projects/{projectId}/session/status", gameSessions.Status)
		r.Get("/api/projects/{projectId}/session/{sessionId}/messages", gameSessions.ListMessages)
		r.Get("/api/projects/{projectId}/sessions", gameSessions.ListSessions)
		r.Get("/api/projects/{projectId}/session/socket", chatWS.Serve)

		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireAdmin([]byte(jwtSecret)))

			// Admin: user management (proxied to auth-svc)
			r.Get("/api/admin/users", admin.ListUsers)
			r.Patch("/api/admin/users/{id}", admin.UpdateUser)
			r.Delete("/api/admin/users/{id}", admin.DeleteUser)

			// Admin: project management
			r.Get("/api/admin/projects", admin.ListProjects)
			r.Get("/api/admin/projects/{id}/members", admin.ListProjectMembers)
		})
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
