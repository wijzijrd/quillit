// @title           Quillit Messaging Service API
// @version         1.0
// @description     Sends transactional email via SMTP.
// @host            localhost:3003
// @BasePath        /

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/quillit/gen/internalauth"
	messagingv1connect "github.com/quillit/gen/quillit/messaging/v1/messagingv1connect"

	_ "github.com/quillit/messaging-svc/docs"
	"github.com/quillit/messaging-svc/internal/handler"
	"github.com/quillit/messaging-svc/internal/rpc"
	"github.com/quillit/messaging-svc/internal/smtp"
)

func main() {
	port := env("PORT", "3003")
	smtpHost := env("SMTP_HOST", "")
	smtpPort := env("SMTP_PORT", "")
	smtpUsername := env("SMTP_USERNAME", "")
	smtpPassword := env("SMTP_PASSWORD", "")
	smtpFrom := env("SMTP_FROM", "")
	// Shared secret gating the MessagingInternalService connect RPC mounted
	// below — see gen/internalauth. Fully replaces MESSAGING_SECRET/
	// X-Messaging-Secret, which used to gate the now-removed POST /send HTTP
	// route (app/messaging/internal/handler/send.go). Mirrors how
	// INTERNAL_RPC_SECRET is read in app/content and app/svc (Task 9): an
	// env var, not yet wired into infra/docker-compose.yml/.env.example
	// (that's a later task).
	internalRPCSecret := env("INTERNAL_RPC_SECRET", "")
	if internalRPCSecret == "" {
		log.Println("WARNING: INTERNAL_RPC_SECRET is unset — internal RPC calls will fail")
	}

	sender := &smtp.SMTPSender{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: smtpUsername,
		Password: smtpPassword,
		From:     smtpFrom,
	}

	health := handler.NewHealth()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health probe — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	// MessagingInternalService: messaging's server-to-server connect RPC
	// surface, reached only by auth (password-reset emails — see
	// app/auth/internal/handler/password_reset.go) and gated by the
	// shared-secret interceptor rather than a per-request header check.
	// Replaces the old POST /send HTTP route outright
	// (internal/rpc/messaging_internal.go).
	messagingInternal := rpc.NewMessagingInternalServer(sender)
	messagingRPCPath, messagingRPCHandler := messagingv1connect.NewMessagingInternalServiceHandler(
		messagingInternal,
		connect.WithInterceptors(internalauth.NewServerInterceptor(internalRPCSecret)),
	)
	r.Mount(messagingRPCPath, messagingRPCHandler)

	r.Get("/swagger/*", httpSwagger.WrapHandler)

	addr := fmt.Sprintf(":%s", port)
	log.Printf("quillit-messaging-svc listening on %s (HTTP/2 cleartext)", addr)
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
