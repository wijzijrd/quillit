package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/quillit/auth-svc/internal/db/sqlc"
)

// OkResponse is a simple success acknowledgement.
type OkResponse struct {
	Ok bool `json:"ok"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ForgotPassword godoc
// @Summary      Request a password reset
// @Description  Sends a reset link if the email is registered. Always returns 200 to avoid leaking account existence.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ForgotPasswordRequest  true  "Email to send the reset link to"
// @Success      200   {object}  OkResponse
// @Failure      400   {object}  ErrorResponse
// @Router       /auth/forgot-password [post]
func (a *Auth) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var body ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	ok := func() { writeJSON(w, http.StatusOK, OkResponse{Ok: true}) }

	userID, err := a.q.GetUserIDByEmail(r.Context(), body.Email)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("forgot-password: lookup error: %v", err)
		}
		ok()
		return
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		panic(err) // same posture as newID()'s existing CSPRNG-failure handling
	}
	tokenHex := hex.EncodeToString(rawToken)
	// Hash the same string representation that ResetPassword receives and
	// hashes (the hex-encoded token, not the raw bytes) so the two handlers'
	// hashes agree without a decode round-trip. See ResetPassword below.
	sum := sha256.Sum256([]byte(tokenHex))
	tokenHash := hex.EncodeToString(sum[:])

	now := time.Now().Unix()
	if err := a.storeResetToken(r.Context(), userID, tokenHash, now); err != nil {
		log.Printf("forgot-password: token store error: %v", err)
		ok()
		return
	}

	if a.messaging == nil || a.appBaseURL == "" {
		log.Printf("forgot-password: messaging not configured, skipping send for user %s", userID)
		ok()
		return
	}

	link := fmt.Sprintf("%s/reset-password?token=%s", a.appBaseURL, tokenHex)
	// Use WithoutCancel so a disconnected browser can't cancel an in-flight
	// reset email — the request's own 5s client-side timeout (see
	// messagingclient.NewClient's http.Client) still bounds how long this
	// can run.
	if err := a.sendResetEmail(context.WithoutCancel(r.Context()), body.Email, link); err != nil {
		log.Printf("forgot-password: send error: %v", err)
	}
	ok()
}

// storeResetToken invalidates any prior unused reset tokens for userID and
// stores a new one, atomically. Every step's error is checked — a silently
// discarded Exec error here could let Commit succeed while the delete or
// insert never actually happened.
func (a *Auth) storeResetToken(ctx context.Context, userID, tokenHash string, now int64) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := a.q.WithTx(tx)

	if err := qtx.DeleteUnusedResetTokens(ctx, userID); err != nil {
		return err
	}
	if err := qtx.InsertResetToken(ctx, sqlc.InsertResetTokenParams{
		ID:        newID(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: now + 3600,
		CreatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Auth) sendResetEmail(ctx context.Context, to, link string) error {
	subject := "Reset your Quillit password"
	text := fmt.Sprintf("Reset your password: %s\n\nThis link expires in 1 hour.", link)
	html := fmt.Sprintf(`<p>Reset your password: <a href="%s">%s</a></p><p>This link expires in 1 hour.</p>`, link, link)
	return a.messaging.SendEmail(ctx, to, subject, text, html)
}

type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

// ResetPassword godoc
// @Summary      Reset password
// @Description  Consumes a reset token and sets a new password. Same generic error for any invalid/expired/used token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      ResetPasswordRequest  true  "Reset token and new password"
// @Success      200   {object}  OkResponse
// @Failure      400   {object}  ErrorResponse
// @Router       /auth/reset-password [post]
func (a *Auth) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" || body.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "invalid or expired reset link")
		return
	}

	sum := sha256.Sum256([]byte(body.Token))
	tokenHash := hex.EncodeToString(sum[:])

	now := time.Now().Unix()
	tok, err := a.q.GetValidResetToken(r.Context(), sqlc.GetValidResetTokenParams{
		TokenHash: tokenHash,
		ExpiresAt: now,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired reset link")
		return
	}
	id, userID := tok.ID, tok.UserID

	if len(body.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tx, err := a.db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback()
	qtx := a.q.WithTx(tx)

	if err := qtx.UpdateUserPassword(r.Context(), sqlc.UpdateUserPasswordParams{
		PasswordHash: string(hash),
		UpdatedAt:    now,
		ID:           userID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := qtx.MarkResetTokenUsed(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, OkResponse{Ok: true})
}
