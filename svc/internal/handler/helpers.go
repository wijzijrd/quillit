package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
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

// ── Entry project-membership scoping ─────────────────────────────────────────
//
// Entries are pre-migration: project membership is `campaign_ids`, a JSON
// array on the entry row, not a foreign key (see docs/web-refactor-spec.md
// §4.2 for the future project_id FK this becomes). These two helpers are
// the one place that array-intersection logic lives, shared by
// entries.go and annotations.go — see #30.

// callerProjectIDs returns the set of project ids callerID is a member of.
func callerProjectIDs(ctx context.Context, db *sql.DB, callerID string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT project_id FROM project_members WHERE user_id = ?", callerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

// entryAccessible reports whether an entry whose campaign_ids raw JSON
// array is campaignIDs belongs to at least one project in memberProjects.
// An entry with no campaign_ids (e.g. "[]", a personal/session-note entry
// per member.go's convention) is accessible to no one via this check —
// those are only ever meant to be reached through their own member-scoped
// endpoints, not the general entries API.
func entryAccessible(campaignIDs json.RawMessage, memberProjects map[string]bool) bool {
	var ids []string
	if err := json.Unmarshal(campaignIDs, &ids); err != nil {
		return false
	}
	for _, id := range ids {
		if memberProjects[id] {
			return true
		}
	}
	return false
}
