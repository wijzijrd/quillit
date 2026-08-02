package handler

import (
	"bytes"
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

	var userID string
	if err := a.db.QueryRow("SELECT id FROM users WHERE email = ?", body.Email).Scan(&userID); err != nil {
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
	if err := a.storeResetToken(userID, tokenHash, now); err != nil {
		log.Printf("forgot-password: token store error: %v", err)
		ok()
		return
	}

	if a.messagingServiceURL == "" || a.appBaseURL == "" {
		log.Printf("forgot-password: messaging not configured, skipping send for user %s", userID)
		ok()
		return
	}

	link := fmt.Sprintf("%s/reset-password?token=%s", a.appBaseURL, tokenHex)
	if err := a.sendResetEmail(body.Email, link); err != nil {
		log.Printf("forgot-password: send error: %v", err)
	}
	ok()
}

// storeResetToken invalidates any prior unused reset tokens for userID and
// stores a new one, atomically. Every step's error is checked — a silently
// discarded Exec error here could let Commit succeed while the delete or
// insert never actually happened.
func (a *Auth) storeResetToken(userID, tokenHash string, now int64) error {
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM password_reset_tokens WHERE user_id = ? AND used = 0", userID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		"INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at) VALUES (?, ?, ?, ?, 0, ?)",
		newID(), userID, tokenHash, now+3600, now,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *Auth) sendResetEmail(to, link string) error {
	payload, _ := json.Marshal(map[string]string{
		"to":      to,
		"subject": "Reset your Quillit password",
		"text":    fmt.Sprintf("Reset your password: %s\n\nThis link expires in 1 hour.", link),
		"html":    fmt.Sprintf(`<p>Reset your password: <a href="%s">%s</a></p><p>This link expires in 1 hour.</p>`, link, link),
	})
	req, err := http.NewRequest(http.MethodPost, a.messagingServiceURL+"/send", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Messaging-Secret", a.messagingSecret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("messaging-svc returned %d", resp.StatusCode)
	}
	return nil
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

	var id, userID string
	now := time.Now().Unix()
	err := a.db.QueryRow(
		"SELECT id, user_id FROM password_reset_tokens WHERE token_hash = ? AND used = 0 AND expires_at > ?",
		tokenHash, now,
	).Scan(&id, &userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired reset link")
		return
	}

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

	if _, err := tx.Exec("UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", string(hash), now, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := tx.Exec("UPDATE password_reset_tokens SET used = 1 WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, OkResponse{Ok: true})
}
