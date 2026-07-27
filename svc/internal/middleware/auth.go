package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

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

// revalidateTTL bounds how long RequireSession trusts a previously-confirmed
// active status before re-checking with auth-svc. It exists because the JWT's
// own "active" claim is baked in at issue time and never changes for the life
// of the token — without this, deactivating a user has no effect until their
// session (7-day TTL) naturally expires.
const revalidateTTL = 60 * time.Second

type activityCache struct {
	mu      sync.Mutex
	entries map[string]activityCacheEntry
}

type activityCacheEntry struct {
	active    bool
	checkedAt time.Time
}

func newActivityCache() *activityCache {
	return &activityCache{entries: make(map[string]activityCacheEntry)}
}

func (c *activityCache) get(token string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[token]
	if !ok || time.Since(e.checkedAt) > revalidateTTL {
		return false, false
	}
	return e.active, true
}

func (c *activityCache) set(token string, active bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Bound memory: a cache that only ever grows would leak one entry per
	// distinct session JWT seen. Since entries are only useful within
	// revalidateTTL anyway, sweep expired ones out on every write.
	for k, e := range c.entries {
		if time.Since(e.checkedAt) > revalidateTTL {
			delete(c.entries, k)
		}
	}
	c.entries[token] = activityCacheEntry{active: active, checkedAt: time.Now()}
}

// verifyActive asks auth-svc whether the account behind raw is still active,
// via the same DB-backed check used by Verify. Errors fail closed.
func verifyActive(authURL, raw string) (bool, error) {
	body, err := json.Marshal(map[string]string{"token": raw})
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Post(authURL+"/auth/verify", "application/json", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var out struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Active, nil
}

// RequireSession validates the session cookie and, if found, checks that the
// account is still active before passing the raw JWT string into context.
// JWT expiry is intentionally not enforced here — session TTL (7 days) governs
// access; the JWT expiry only applies to admin routes via RequireAdmin.
//
// Active status is revalidated against auth-svc at most once per
// revalidateTTL per session, rather than trusted from the JWT's baked claim
// for the whole 7-day session — see revalidateTTL.
func RequireSession(store *session.Store, jwtSecret []byte, authURL string) func(http.Handler) http.Handler {
	cache := newActivityCache()
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

			if _, ok := token.Claims.(jwt.MapClaims); !ok {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			active, fresh := cache.get(raw)
			if !fresh {
				active, err = verifyActive(authURL, raw)
				if err != nil {
					http.Error(w, `{"error":"auth service unavailable"}`, http.StatusServiceUnavailable)
					return
				}
				cache.set(raw, active)
			}
			if !active {
				http.Error(w, `{"error":"account disabled"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), jwtKey, raw)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
