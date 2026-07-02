// @title           Quillit Auth Service API
// @version         1.0
// @description     Authentication service: register, login, token verification.
// @host            localhost:3002
// @BasePath        /

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/quillit/auth-svc/internal/db"
	"github.com/quillit/auth-svc/internal/handler"
	_ "github.com/quillit/auth-svc/docs"
)

func main() {
	port := env("PORT", "3002")
	dbPath := env("DB_PATH", "./quillit-auth.db")
	jwtSecret := mustEnv("JWT_SECRET")

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	auth := handler.NewAuth(database, jwtSecret)
	health := handler.NewHealth(database)

	// Seed admin account if credentials are configured
	adminEmail := os.Getenv("SEED_ADMIN_EMAIL")
	adminPassword := os.Getenv("SEED_ADMIN_PASSWORD")
	if adminEmail != "" && adminPassword != "" {
		if err := auth.SeedAdmin(adminEmail, adminPassword); err != nil {
			log.Printf("seed admin: %v", err)
		} else {
			log.Printf("seed admin ready: %s", adminEmail)
		}
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health probe — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	r.Get("/auth/status", auth.Status)
	r.Post("/auth/register", auth.Register)
	r.Post("/auth/login", auth.Login)
	r.Post("/auth/verify", auth.Verify)

	// Any-auth user search (requires valid Bearer JWT, not admin-only)
	r.With(auth.RequireAnyAuth).Get("/auth/users/search", auth.SearchUsers)

	// Admin-only user management (requires Bearer JWT with role=admin)
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin)
		r.Get("/auth/users", auth.ListUsers)
		r.Patch("/auth/users/{id}", auth.UpdateUser)
		r.Delete("/auth/users/{id}", auth.DeleteUser)
	})

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("quillit-auth-svc listening on %s (HTTP/2 cleartext)", addr)
	h2s := &http2.Server{}
	log.Fatal(http.ListenAndServe(addr, h2c.NewHandler(r, h2s)))
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
