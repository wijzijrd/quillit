package middleware

import (
	"context"
	"net/http"

	"github.com/quillit/svc/internal/session"
)

type contextKey string

const jwtKey contextKey = "jwt"

func RequireSession(store *session.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwt, err := store.Get(r)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), jwtKey, jwt)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
