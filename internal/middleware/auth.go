package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/quillit/svc/internal/session"
)

type contextKey string

const jwtKey contextKey = "jwt"

// RawJWTFromContext returns the raw JWT string stored in context by RequireSession.
func RawJWTFromContext(ctx context.Context) (string, bool) {
	raw, ok := ctx.Value(jwtKey).(string)
	return raw, ok && raw != ""
}

// ClaimsFromContext extracts and parses the JWT stored in context by RequireSession.
// Expiry is not validated — use this only for identity extraction, not auth decisions.
func ClaimsFromContext(ctx context.Context, jwtSecret []byte) (jwt.MapClaims, error) {
	raw, ok := ctx.Value(jwtKey).(string)
	if !ok || raw == "" {
		return nil, errors.New("no jwt in context")
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithoutClaimsValidation(),
	)
	token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return mc, nil
}

// RequireSession validates the session cookie and, if found, checks that the
// account is still active before passing the raw JWT string into context.
// JWT expiry is intentionally not enforced here — session TTL (7 days) governs
// access; the JWT expiry only applies to admin routes via RequireAdmin.
func RequireSession(store *session.Store, jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, err := store.Get(r)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Parse without expiry validation so long-lived sessions remain valid.
			parser := jwt.NewParser(
				jwt.WithValidMethods([]string{"HS256"}),
				jwt.WithoutClaimsValidation(),
			)
			token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
				return jwtSecret, nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			mc, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			// Reject sessions belonging to disabled accounts.
			if active, _ := mc["active"].(bool); !active {
				http.Error(w, `{"error":"account disabled"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), jwtKey, raw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
