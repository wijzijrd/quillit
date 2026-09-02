package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/quillit/content-svc/internal/authz"
	"github.com/quillit/content-svc/internal/db/sqlc"
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

// callerIDFromRequest extracts the caller's user id (JWT "sub" claim) from
// a bearer token on the Authorization header, if present and valid against
// jwtSecret. It returns ("", false) rather than rejecting the request when
// no/invalid token is present — identity extraction only, not an access
// decision. requireCaller (below) is what actually gates a request on the
// result; callers that need mere identity (e.g. Create's owner_user_id)
// should now go through requireProjectMember/requireCaller too, since #44
// made authentication mandatory everywhere rather than optional.
func callerIDFromRequest(r *http.Request, jwtSecret []byte) (string, bool) {
	auth := r.Header.Get("Authorization")
	raw, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok || raw == "" || len(jwtSecret) == 0 {
		return "", false
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithoutClaimsValidation(),
	)
	token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", false
	}
	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

// ── Cross-domain auth (#44) ─────────────────────────────────────────────────
//
// content is internal-only (no exposed port — infra/docker-compose.yml),
// reached only server-to-server (today: svc; see also the health probe and
// docs/web-refactor-spec.md §10.11). That network boundary proves a
// request came from a trusted caller, but it says nothing about *which end
// user* is asking or what they're allowed to see — svc forwards real user
// requests, it doesn't originate its own — so content still authenticates
// and authorizes every request per-user, same as if it were public.
//
// requireCaller answers "who is asking" (a valid bearer JWT); requireProjectMember
// additionally answers "are they allowed to touch this project" via a
// Checker (see internal/authz) — svc-backed in production, since svc owns
// project_members and content doesn't duplicate that data locally.

type testCallerKey struct{}

// WithTestCallerID injects a caller ID into a request context for tests,
// bypassing JWT parsing entirely — mirrors svc's identical seam
// (app/svc/internal/handler/helpers.go).
func WithTestCallerID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, testCallerKey{}, id)
}

// requireCaller resolves the authenticated caller's user id, writing 401
// and returning ok=false if the request carries no valid identity (a
// test-injected id, or otherwise a valid bearer JWT).
func requireCaller(w http.ResponseWriter, r *http.Request, jwtSecret []byte) (string, bool) {
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	id, ok := callerIDFromRequest(r, jwtSecret)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}

// requireProjectMember resolves the authenticated caller and checks that
// they belong to projectID via checker, writing the appropriate error
// response and returning ok=false on any failure:
//   - 401 if the caller isn't authenticated at all (via requireCaller)
//   - 503 if checker couldn't produce an answer (e.g. svc unreachable and
//     no cached answer survives — see authz.SvcChecker's outage handling)
//   - 403 if the caller is authenticated but not a member
func requireProjectMember(w http.ResponseWriter, r *http.Request, jwtSecret []byte, checker authz.Checker, projectID string) (string, bool) {
	callerID, ok := requireCaller(w, r, jwtSecret)
	if !ok {
		return "", false
	}
	member, err := checker.IsMember(r.Context(), callerID, projectID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "membership check unavailable")
		return "", false
	}
	if !member {
		writeError(w, http.StatusForbidden, "not a project member")
		return "", false
	}
	return callerID, true
}

// projectIDForEntry looks up entryID's owning project — the piece every
// entry-scoped endpoint (Get/Update/Delete/UploadImage/Render/GetForEntry)
// needs before it can ask requireProjectMember the right question. Returns
// sql.ErrNoRows when the entry doesn't exist, for callers to map to 404.
func projectIDForEntry(ctx context.Context, q *sqlc.Queries, entryID string) (string, error) {
	return q.GetProjectIDForEntry(ctx, entryID)
}
