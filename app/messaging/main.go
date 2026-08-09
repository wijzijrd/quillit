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

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	_ "github.com/quillit/messaging-svc/docs"
	"github.com/quillit/messaging-svc/internal/handler"
	"github.com/quillit/messaging-svc/internal/smtp"
)

func main() {
	port := env("PORT", "3003")
	smtpHost := env("SMTP_HOST", "")
	smtpPort := env("SMTP_PORT", "")
	smtpUsername := env("SMTP_USERNAME", "")
	smtpPassword := env("SMTP_PASSWORD", "")
	smtpFrom := env("SMTP_FROM", "")
	messagingSecret := env("MESSAGING_SECRET", "")

	sender := &smtp.SMTPSender{
		Host:     smtpHost,
		Port:     smtpPort,
		Username: smtpUsername,
		Password: smtpPassword,
		From:     smtpFrom,
	}

	send := handler.NewSend(sender, messagingSecret)
	health := handler.NewHealth()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health probe — used by the deploy pipeline & uptime monitors
	r.Get("/healthz", health.Check)

	r.Post("/send", send.Send)

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
