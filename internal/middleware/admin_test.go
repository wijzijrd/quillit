package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func makeTestJWT(secret []byte, role string) string {
	claims := jwt.MapClaims{"sub": "u1", "role": role}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString(secret)
	return signed
}

func ctxWithJWT(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, jwtKey, raw)
}

func TestRequireAdmin_AllowsAdmin(t *testing.T) {
	secret := []byte("testsecret")
	called := false
	h := RequireAdmin(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil).WithContext(
		ctxWithJWT(context.Background(), makeTestJWT(secret, "admin")),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called {
		t.Fatal("expected handler to be called for admin role")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireAdmin_BlocksNonAdmin(t *testing.T) {
	secret := []byte("testsecret")
	h := RequireAdmin(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil).WithContext(
		ctxWithJWT(context.Background(), makeTestJWT(secret, "user")),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequireAdmin_BlocksMissingJWT(t *testing.T) {
	secret := []byte("testsecret")
	h := RequireAdmin(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
