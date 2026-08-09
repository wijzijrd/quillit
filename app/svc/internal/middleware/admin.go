package middleware

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// RequireAdmin enforces role=admin on the JWT already stored in context by RequireSession.
func RequireAdmin(jwtSecret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := r.Context().Value(jwtKey).(string)
			if raw == "" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return jwtSecret, nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err != nil || !token.Valid {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			mc, ok := token.Claims.(jwt.MapClaims)
			if !ok || mc["role"] != "admin" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
