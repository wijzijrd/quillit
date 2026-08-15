package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/quillit/svc/internal/middleware"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		// The OS CSPRNG failing is not a recoverable condition — returning a
		// zero-derived ID would risk predictable/duplicate primary keys.
		panic(err)
	}
	return hex.EncodeToString(b)
}

func nowUnix() int64 {
	return time.Now().Unix()
}

// ── Caller identity ───────────────────────────────────────────────────────────
//
// Shared by every handler that needs to know who's calling: extracts the
// JWT "sub" claim, with a test-only bypass (WithTestCallerID) so handler
// tests don't need to mint real JWTs. Moved here from projects.go (the
// first handler that needed it) so entries/annotations reuse the same
// extraction logic instead of each growing their own copy — see #30.

type testCallerKey struct{}

// WithTestCallerID injects a caller ID into a request context for tests,
// bypassing JWT parsing entirely.
func WithTestCallerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, testCallerKey{}, id)
}

// callerIDFromRequest extracts the caller's user ID (JWT "sub" claim) from
// the request, or the test-injected id if WithTestCallerID was used.
func callerIDFromRequest(r *http.Request, jwtSecret []byte) (string, bool) {
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}
