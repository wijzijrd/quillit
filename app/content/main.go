package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/quillit/gen/internalauth"
	contentv1connect "github.com/quillit/gen/quillit/content/v1/contentv1connect"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db"
	"github.com/quillit/content-svc/internal/handler"
	"github.com/quillit/content-svc/internal/rpc"
	"github.com/quillit/content-svc/internal/storage"
)

func main() {
	port := env("PORT", "3004")
	dbPath := env("DB_PATH", "./quillit-content.db")
	jwtSecret := mustEnv("JWT_SECRET")
	svcURL := env("SVC_SERVICE_URL", "http://localhost:3000")
	// Shared secret gating every quillit-internal connect RPC (both the
	// ContentInternalService server mounted below and the SvcInternalService
	// client authz.NewSvcChecker builds) — see gen/internalauth. Read the
	// same way in app/svc, app/auth and app/messaging: an env var, wired
	// into infra/docker-compose.yml and documented in .env.example.
	internalRPCSecret := env("INTERNAL_RPC_SECRET", "")
	if internalRPCSecret == "" {
		log.Println("WARNING: INTERNAL_RPC_SECRET is unset — internal RPC calls will fail")
	}

	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer database.Close()

	// MinIO blob storage (optional — gracefully skipped if not configured,
	// matching svc/main.go's pattern; entries.Create/Update fail loud at
	// request time when it's needed but absent).
	var blobs handler.BlobStore
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

	// #44: content authorizes every request per-caller (see
	// internal/handler/helpers.go's "Cross-domain auth" section) by asking
	// svc whether the caller belongs to the project in question — svc owns
	// project_members, content doesn't duplicate it. checker is shared
	// across every handler so its membership cache (short-TTL, with a
	// bounded stale-fallback window for svc outages — see internal/authz)
	// does the most good.
	checker := authz.NewSvcChecker(svcURL, internalRPCSecret, &http.Client{Timeout: 3 * time.Second})

	health := handler.NewHealth(database)
	entries := handler.NewEntries(database, jwtSecret, blobs, checker)
	renderer := handler.NewRender(database, blobs, jwtSecret, checker)
	exporter := handler.NewExport(database, blobs, jwtSecret, checker)
	facets := handler.NewFacets(database, jwtSecret, checker)
	links := handler.NewLinks(database, blobs, jwtSecret, checker)
	search := handler.NewSearch(database, jwtSecret, checker)
	assign := handler.NewAssign(database, blobs, jwtSecret, checker)
	importer := handler.NewImport(database, jwtSecret, blobs, checker)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health probe — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	r.Route("/content", func(r chi.Router) {
		r.Get("/projects/{id}/entries", entries.List)
		r.Post("/projects/{id}/entries", entries.Create)
		r.Get("/entries/{id}", entries.Get)
		r.Patch("/entries/{id}", entries.Update)
		r.Delete("/entries/{id}", entries.Delete)
		r.Post("/entries/{id}/images", entries.UploadImage)
		r.Get("/entries/{id}/images/{filename}", entries.GetImage)
		r.Get("/entries/{id}/render", renderer.Render)
		r.Get("/entries/{id}/export", exporter.Export)
		r.Get("/entries/{id}/links", links.GetForEntry)
		r.Post("/entries/{id}/assign", assign.Assign)

		r.Get("/projects/{id}/search", search.Search)
		r.Post("/projects/{id}/import", importer.ImportProject)

		r.Get("/facets", facets.ListGlobal)
		r.Post("/facets", facets.CreateGlobal)
		r.Delete("/facets/{name}", facets.DeleteGlobal)
		r.Get("/projects/{id}/facets", facets.ListEffectiveForProject)
		r.Post("/projects/{id}/facets", facets.CreateForProject)
		r.Delete("/projects/{id}/facets/{name}", facets.DeleteForProject)
		r.Post("/projects/{id}/compile", links.CompileProject)
	})

	// ContentInternalService: content's server-to-server connect RPC
	// surface, reached only by svc (never a browser, never proxied through
	// requireCaller/requireProjectMember the way everything under /content
	// above is — see internal/rpc/content_internal.go's package doc).
	// Gated by the shared-secret interceptor rather than a per-request JWT.
	// Replaces the old POST /content/internal/projects/{id}/deleted route
	// (NotifyProjectDeleted) outright; GetEntry is a new, separate surface
	// alongside (not a replacement for) the still-live GET
	// /content/entries/{id} HTTP route, which UI-facing traffic still needs
	// (see docstring above).
	contentInternal := rpc.NewContentInternalServer(database, blobs)
	contentRPCPath, contentRPCHandler := contentv1connect.NewContentInternalServiceHandler(
		contentInternal,
		connect.WithInterceptors(internalauth.NewServerInterceptor(internalRPCSecret)),
	)
	r.Mount(contentRPCPath, contentRPCHandler)

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

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env var %s is required", key)
	}
	return v
}
