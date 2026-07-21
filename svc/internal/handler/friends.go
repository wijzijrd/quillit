package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/quillit/svc/internal/middleware"
)

// FriendsHandler exposes friend-request and friendship endpoints.
type FriendsHandler struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewFriends(db *sql.DB, jwtSecret string) *FriendsHandler {
	return &FriendsHandler{db: db, jwtSecret: []byte(jwtSecret)}
}

// NewFriendsForTest creates a handler that resolves the caller ID from the test
// context helper (WithTestCallerID) instead of parsing a real JWT.
func NewFriendsForTest(db *sql.DB) *FriendsHandler {
	return &FriendsHandler{db: db, jwtSecret: nil}
}

// callerID extracts the user ID (sub) from the JWT stored in request context.
// In tests, a caller ID can be injected directly via WithTestCallerID.
func (h *FriendsHandler) callerID(r *http.Request) (string, bool) {
	if id, ok := r.Context().Value(testCallerKey{}).(string); ok && id != "" {
		return id, true
	}
	mc, err := middleware.ClaimsFromContext(r.Context(), h.jwtSecret)
	if err != nil {
		return "", false
	}
	sub, _ := mc["sub"].(string)
	return sub, sub != ""
}

// ── Response types ─────────────────────────────────────────────────────────────

// FriendRequest represents a single row of the friend_requests table, covering
// both the pending and accepted states.
type FriendRequest struct {
	ID                string `json:"id"`
	RequesterID       string `json:"requesterId"`
	RequesterUsername string `json:"requesterUsername"`
	AddresseeID       string `json:"addresseeId"`
	AddresseeUsername string `json:"addresseeUsername"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"createdAt"`
	AcceptedAt        *int64 `json:"acceptedAt"`
}

// Friend represents a resolved accepted friendship from the caller's point of
// view — userId/username refer to the OTHER party, not the caller.
type Friend struct {
	ID         string `json:"id"`
	UserID     string `json:"userId"`
	Username   string `json:"username"`
	FriendedAt int64  `json:"friendedAt"`
}

func scanFriendRequest(row *sql.Row) (FriendRequest, error) {
	var fr FriendRequest
	var acceptedAt sql.NullInt64
	err := row.Scan(&fr.ID, &fr.RequesterID, &fr.RequesterUsername, &fr.AddresseeID, &fr.AddresseeUsername, &fr.Status, &fr.CreatedAt, &acceptedAt)
	if acceptedAt.Valid {
		v := acceptedAt.Int64
		fr.AcceptedAt = &v
	}
	return fr, err
}

const friendRequestSelect = `SELECT id, requester_id, requester_username, addressee_id, addressee_username, status, created_at, accepted_at FROM friend_requests`

// SendRequest godoc
// @Summary      Send a friend request
// @Description  Creates a pending friend request between the caller and another user.
// @Tags         friends
// @Accept       json
// @Produce      json
// @Param        body  body  object  true  "{ userId, username, requesterUsername }"
// @Success      201  {object}  FriendRequest
// @Failure      400  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/friends/requests [post]
func (h *FriendsHandler) SendRequest(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		UserID            string `json:"userId"`
		Username          string `json:"username"`
		RequesterUsername string `json:"requesterUsername"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID == "" {
		writeError(w, http.StatusBadRequest, "userId required")
		return
	}
	if body.UserID == callerID {
		writeError(w, http.StatusBadRequest, "cannot friend yourself")
		return
	}

	existing, err := scanFriendRequest(h.db.QueryRowContext(r.Context(),
		friendRequestSelect+` WHERE (requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?)`,
		callerID, body.UserID, body.UserID, callerID,
	))
	if err == nil {
		switch existing.Status {
		case "accepted":
			writeError(w, http.StatusConflict, "already friends")
		default:
			writeError(w, http.StatusConflict, "request already pending")
		}
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	fr := FriendRequest{
		ID:                newID(),
		RequesterID:       callerID,
		RequesterUsername: body.RequesterUsername,
		AddresseeID:       body.UserID,
		AddresseeUsername: body.Username,
		Status:            "pending",
		CreatedAt:         nowUnix(),
	}
	if _, err := h.db.ExecContext(r.Context(),
		`INSERT INTO friend_requests (id, requester_id, requester_username, addressee_id, addressee_username, status, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', ?)`,
		fr.ID, fr.RequesterID, fr.RequesterUsername, fr.AddresseeID, fr.AddresseeUsername, fr.CreatedAt,
	); err != nil {
		if isUniqueConstraintErr(err) {
			writeError(w, http.StatusConflict, "request already pending")
			return
		}
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, fr)
}

// ListIncoming godoc
// @Summary      List incoming friend requests
// @Tags         friends
// @Produce      json
// @Success      200  {array}  FriendRequest
// @Router       /api/friends/requests/incoming [get]
func (h *FriendsHandler) ListIncoming(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.listRequests(w, r, `WHERE addressee_id = ? AND status = 'pending' ORDER BY created_at`, callerID)
}

// ListOutgoing godoc
// @Summary      List outgoing friend requests
// @Tags         friends
// @Produce      json
// @Success      200  {array}  FriendRequest
// @Router       /api/friends/requests/outgoing [get]
func (h *FriendsHandler) ListOutgoing(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.listRequests(w, r, `WHERE requester_id = ? AND status = 'pending' ORDER BY created_at`, callerID)
}

func (h *FriendsHandler) listRequests(w http.ResponseWriter, r *http.Request, whereClause string, arg any) {
	rows, err := h.db.QueryContext(r.Context(), friendRequestSelect+" "+whereClause, arg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	out := []FriendRequest{}
	for rows.Next() {
		var fr FriendRequest
		var acceptedAt sql.NullInt64
		if err := rows.Scan(&fr.ID, &fr.RequesterID, &fr.RequesterUsername, &fr.AddresseeID, &fr.AddresseeUsername, &fr.Status, &fr.CreatedAt, &acceptedAt); err != nil {
			continue
		}
		if acceptedAt.Valid {
			v := acceptedAt.Int64
			fr.AcceptedAt = &v
		}
		out = append(out, fr)
	}
	writeJSON(w, http.StatusOK, out)
}

// AcceptRequest godoc
// @Summary      Accept a friend request
// @Tags         friends
// @Produce      json
// @Param        id  path  string  true  "Friend request ID"
// @Success      200  {object}  FriendRequest
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      409  {object}  ErrorResponse
// @Router       /api/friends/requests/{id}/accept [post]
func (h *FriendsHandler) AcceptRequest(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")

	existing, err := scanFriendRequest(h.db.QueryRowContext(r.Context(), friendRequestSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if callerID != existing.AddresseeID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	if existing.Status != "pending" {
		writeError(w, http.StatusConflict, "request is not pending")
		return
	}

	now := nowUnix()
	if _, err := h.db.ExecContext(r.Context(),
		`UPDATE friend_requests SET status = 'accepted', accepted_at = ? WHERE id = ?`,
		now, id,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}

	existing.Status = "accepted"
	existing.AcceptedAt = &now
	writeJSON(w, http.StatusOK, existing)
}

// DeleteRequest godoc
// @Summary      Decline, cancel, or unfriend
// @Description  Deletes a friend_requests row. Covers declining an incoming pending
// @Description  request, cancelling an outgoing pending request, and unfriending an
// @Description  accepted relationship — all are a hard delete by either party.
// @Tags         friends
// @Param        id  path  string  true  "Friend request ID"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /api/friends/requests/{id} [delete]
func (h *FriendsHandler) DeleteRequest(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id := chi.URLParam(r, "id")

	var requesterID, addresseeID string
	err := h.db.QueryRowContext(r.Context(),
		`SELECT requester_id, addressee_id FROM friend_requests WHERE id = ?`, id,
	).Scan(&requesterID, &addresseeID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if callerID != requesterID && callerID != addresseeID {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}

	if _, err := h.db.ExecContext(r.Context(), `DELETE FROM friend_requests WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListFriends godoc
// @Summary      List friends
// @Tags         friends
// @Produce      json
// @Success      200  {array}  Friend
// @Router       /api/friends [get]
func (h *FriendsHandler) ListFriends(w http.ResponseWriter, r *http.Request) {
	callerID, ok := h.callerID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id,
		       CASE WHEN requester_id = ? THEN addressee_id ELSE requester_id END AS userId,
		       CASE WHEN requester_id = ? THEN addressee_username ELSE requester_username END AS username,
		       accepted_at AS friendedAt
		FROM friend_requests
		WHERE status = 'accepted' AND (requester_id = ? OR addressee_id = ?)
	`, callerID, callerID, callerID, callerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	out := []Friend{}
	for rows.Next() {
		var f Friend
		var friendedAt sql.NullInt64
		if err := rows.Scan(&f.ID, &f.UserID, &f.Username, &friendedAt); err != nil {
			continue
		}
		f.FriendedAt = friendedAt.Int64
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, out)
}

// isFriend checks whether an accepted friend_requests row exists for the pair
// (a, b) in either direction. Not yet wired into entry_shares.go — that's a
// later task.
func isFriend(ctx context.Context, db *sql.DB, a, b string) bool {
	var count int
	_ = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM friend_requests
		WHERE status = 'accepted' AND ((requester_id = ? AND addressee_id = ?) OR (requester_id = ? AND addressee_id = ?))
	`, a, b, b, a).Scan(&count)
	return count > 0
}
