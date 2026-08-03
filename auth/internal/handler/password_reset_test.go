package handler_test

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/quillit/auth-svc/internal/db"
	"github.com/quillit/auth-svc/internal/handler"
)

// sentEmail mirrors messaging-svc's SendRequest body shape
// (auth/../messaging/internal/handler/send.go), which is what the fake
// server below decodes incoming requests into.
type sentEmail struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// newTestDB opens a fresh in-memory database for a single test, mirroring
// the pattern in auth_test.go.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// newMessagingFake stands in for messaging-svc: a real httptest.NewServer
// that records every request it receives into *received, decoded as an
// email send. This is the established pattern for faking cross-service HTTP
// calls in this codebase, as opposed to the Sender-interface fake used
// inside messaging/ itself (that exception exists because SMTP has no
// httptest-server equivalent; an HTTP call to another Go service does).
func newMessagingFake(t *testing.T) (*httptest.Server, *[]sentEmail) {
	t.Helper()
	received := &[]sentEmail{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body sentEmail
		_ = json.NewDecoder(r.Body).Decode(&body)
		*received = append(*received, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	t.Cleanup(srv.Close)
	return srv, received
}

var seedUserCounter int

// seedUser inserts a user with a unique id/username derived from a counter,
// so multiple users can be seeded into the same test database without
// colliding on the username UNIQUE constraint.
func seedUser(t *testing.T, database *sql.DB, email string) string {
	t.Helper()
	seedUserCounter++
	id := fmt.Sprintf("u%d", seedUserCounter)
	username := fmt.Sprintf("user%d", seedUserCounter)
	hash, err := bcrypt.GenerateFromPassword([]byte("originalPassw0rd!"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	now := time.Now().Unix()
	if _, err := database.Exec(
		`INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)`,
		id, email, username, string(hash), now, now,
	); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// seedResetToken inserts a password_reset_tokens row directly via SQL for a
// known raw token, so ResetPassword tests can exercise expired/used/garbage
// cases without going through ForgotPassword.
//
// The hashing scheme (SHA-256 hex digest of the raw token) is an assumption
// on our part: the brief only pins that the stored token_hash is 64 hex
// characters, which is consistent with a SHA-256 hex digest and is the
// natural choice for "hash the token so a DB leak doesn't expose usable
// tokens." Task 8 must hash lookups the same way for these ResetPassword
// tests to pass.
func seedResetToken(t *testing.T, database *sql.DB, userID, rawToken string, expiresAt int64, used int) {
	t.Helper()
	sum := sha256.Sum256([]byte(rawToken))
	hashHex := hex.EncodeToString(sum[:])
	if _, err := database.Exec(
		`INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at, used, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"prt-"+rawToken, userID, hashHex, expiresAt, used, time.Now().Unix(),
	); err != nil {
		t.Fatalf("seed reset token: %v", err)
	}
}

func doForgotPassword(auth *handler.Auth, email string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"email": email})
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	auth.ForgotPassword(rec, req)
	return rec
}

func doResetPassword(auth *handler.Auth, token, newPassword string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"token": token, "newPassword": newPassword})
	req := httptest.NewRequest(http.MethodPost, "/auth/reset-password", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	auth.ResetPassword(rec, req)
	return rec
}

func TestForgotPassword(t *testing.T) {
	// Captured from the "existing user" subtest below and reused by the
	// nonexistent-email and messaging-unreachable subtests: the whole point
	// of this endpoint is that all three produce a byte-identical response,
	// so an attacker can't use response shape to enumerate registered
	// emails or infer messaging-svc health.
	var successBody []byte

	// Case 1.
	t.Run("existing user creates a token and sends the reset email", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, received := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "reset-me@example.com")

		before := time.Now().Unix()
		rec := doForgotPassword(auth, "reset-me@example.com")
		after := time.Now().Unix()

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		successBody = rec.Body.Bytes()

		var resp map[string]any
		if err := json.Unmarshal(successBody, &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if ok, _ := resp["ok"].(bool); !ok {
			t.Errorf(`response = %v, want {"ok":true}`, resp)
		}
		if _, hasToken := resp["token"]; hasToken {
			t.Errorf("response body must not expose a token field: %v", resp)
		}

		rows, err := database.Query(`SELECT token_hash, expires_at FROM password_reset_tokens WHERE user_id = ?`, userID)
		if err != nil {
			t.Fatalf("query tokens: %v", err)
		}
		defer rows.Close()

		var count int
		var tokenHash string
		var expiresAt int64
		for rows.Next() {
			count++
			if err := rows.Scan(&tokenHash, &expiresAt); err != nil {
				t.Fatalf("scan: %v", err)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		if count != 1 {
			t.Fatalf("password_reset_tokens rows for user = %d, want exactly 1", count)
		}
		if len(tokenHash) != 64 {
			t.Errorf("token_hash length = %d, want 64 (hex-encoded sha256)", len(tokenHash))
		}
		if _, err := hex.DecodeString(tokenHash); err != nil {
			t.Errorf("token_hash %q is not valid hex: %v", tokenHash, err)
		}
		const tolerance = 10 // seconds
		if expiresAt < before+3600-tolerance || expiresAt > after+3600+tolerance {
			t.Errorf("expires_at = %d, want ~%d (now+3600, +/-%ds)", expiresAt, before+3600, tolerance)
		}

		if len(*received) != 1 {
			t.Fatalf("messaging fake received %d calls, want exactly 1", len(*received))
		}
		email := (*received)[0]
		if email.To != "reset-me@example.com" {
			t.Errorf("email.To = %q, want %q", email.To, "reset-me@example.com")
		}
		const wantLink = "http://localhost:5173/reset-password?token="
		if !strings.Contains(email.Text, wantLink) {
			t.Errorf("email.Text = %q, want it to contain %q", email.Text, wantLink)
		}
		if !strings.Contains(email.HTML, wantLink) {
			t.Errorf("email.HTML = %q, want it to contain %q", email.HTML, wantLink)
		}
	})

	// Case 2.
	t.Run("second call for the same user replaces the prior unused token", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "twice@example.com")

		if rec := doForgotPassword(auth, "twice@example.com"); rec.Code != http.StatusOK {
			t.Fatalf("first call status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		var firstID string
		if err := database.QueryRow(`SELECT id FROM password_reset_tokens WHERE user_id = ?`, userID).Scan(&firstID); err != nil {
			t.Fatalf("query first token: %v", err)
		}

		if rec := doForgotPassword(auth, "twice@example.com"); rec.Code != http.StatusOK {
			t.Fatalf("second call status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}

		rows, err := database.Query(`SELECT id FROM password_reset_tokens WHERE user_id = ?`, userID)
		if err != nil {
			t.Fatalf("query tokens: %v", err)
		}
		defer rows.Close()
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("rows: %v", err)
		}
		if len(ids) != 1 {
			t.Fatalf("live token rows for user = %d, want exactly 1 (prior unused token must be deleted, not accumulated)", len(ids))
		}
		if ids[0] == firstID {
			t.Errorf("token row id unchanged across calls (%s); expected the first row to be replaced by a new one", ids[0])
		}
	})

	// Case 3 — the critical anti-enumeration assertion.
	t.Run("nonexistent email returns a response byte-identical to the success case", func(t *testing.T) {
		if successBody == nil {
			t.Fatal("successBody was not captured by the 'existing user' subtest; subtests must run in order")
		}
		database := newTestDB(t)
		messagingSrv, received := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		rec := doForgotPassword(auth, "nobody-here@example.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !bytes.Equal(rec.Body.Bytes(), successBody) {
			t.Errorf("response body = %q, want byte-identical to the existing-user success body %q (anti-enumeration)", rec.Body.String(), successBody)
		}

		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM password_reset_tokens`).Scan(&count); err != nil {
			t.Fatalf("count tokens: %v", err)
		}
		if count != 0 {
			t.Errorf("password_reset_tokens rows = %d, want 0", count)
		}
		if len(*received) != 0 {
			t.Errorf("messaging fake received %d calls, want 0", len(*received))
		}
	})

	// Case 4.
	t.Run("malformed body returns 400", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		t.Run("missing email field", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader(`{}`))
			rec := httptest.NewRecorder()
			auth.ForgotPassword(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})

		t.Run("invalid JSON", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", strings.NewReader(`{not-json`))
			rec := httptest.NewRecorder()
			auth.ForgotPassword(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		})
	})

	// Case 5.
	t.Run("messaging-svc unreachable still returns ok, byte-identical to the success case", func(t *testing.T) {
		if successBody == nil {
			t.Fatal("successBody was not captured by the 'existing user' subtest; subtests must run in order")
		}
		database := newTestDB(t)

		// A server that's already Close()'d: its URL is well-formed but
		// nothing is listening, so any request against it fails at the
		// transport level, simulating messaging-svc being unreachable.
		deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		deadURL := deadSrv.URL
		deadSrv.Close()

		auth := handler.NewAuth(database, "test-secret", deadURL, "test-messaging-secret", "http://localhost:5173")
		seedUser(t, database, "unreachable@example.com")

		rec := doForgotPassword(auth, "unreachable@example.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		if !bytes.Equal(rec.Body.Bytes(), successBody) {
			t.Errorf("response body = %q, want byte-identical to the existing-user success body %q", rec.Body.String(), successBody)
		}
	})

	// Case 6.
	t.Run("empty messaging config degrades gracefully", func(t *testing.T) {
		database := newTestDB(t)
		auth := handler.NewAuth(database, "test-secret", "", "", "")
		seedUser(t, database, "no-config@example.com")

		rec := doForgotPassword(auth, "no-config@example.com")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}
		// No messaging fake exists in this subtest at all, and messagingServiceURL
		// is "". Reaching this point without panicking (Recoverer middleware isn't
		// in play here, since we call the handler method directly) is itself part
		// of the assertion: an empty URL must be treated as "messaging not
		// configured, skip the call" rather than attempted as a real request.
	})
}

func TestResetPassword(t *testing.T) {
	// Captured across the expired/used/garbage subtests below for the
	// byte-identical comparison in case 12.
	var expiredBody, usedBody, garbageBody []byte

	// Case 7.
	t.Run("valid unexpired unused token resets the password", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "resetme@example.com")
		const rawToken = "valid-token-abc123"
		seedResetToken(t, database, userID, rawToken, time.Now().Add(time.Hour).Unix(), 0)

		const newPassword = "brandNewPassw0rd!"
		rec := doResetPassword(auth, rawToken, newPassword)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
		}

		var hash string
		if err := database.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
			t.Fatalf("query password_hash: %v", err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(newPassword)); err != nil {
			t.Errorf("stored password_hash does not match the new password: %v", err)
		}

		var used int
		if err := database.QueryRow(`SELECT used FROM password_reset_tokens WHERE user_id = ?`, userID).Scan(&used); err != nil {
			t.Fatalf("query token used flag: %v", err)
		}
		if used != 1 {
			t.Errorf("token used = %d, want 1", used)
		}
	})

	// Case 8.
	t.Run("expired token returns 400 invalid-or-expired", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "expired@example.com")
		const rawToken = "expired-token-abc123"
		seedResetToken(t, database, userID, rawToken, time.Now().Add(-time.Hour).Unix(), 0)

		rec := doResetPassword(auth, rawToken, "somePassw0rd!")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["error"] != "invalid or expired reset link" {
			t.Errorf(`error = %q, want "invalid or expired reset link"`, resp["error"])
		}
		expiredBody = rec.Body.Bytes()
	})

	// Case 9.
	t.Run("already-used token returns 400 invalid-or-expired", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "used@example.com")
		const rawToken = "used-token-abc123"
		seedResetToken(t, database, userID, rawToken, time.Now().Add(time.Hour).Unix(), 1)

		rec := doResetPassword(auth, rawToken, "somePassw0rd!")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["error"] != "invalid or expired reset link" {
			t.Errorf(`error = %q, want "invalid or expired reset link"`, resp["error"])
		}
		usedBody = rec.Body.Bytes()
	})

	// Case 10.
	t.Run("garbage/nonexistent token returns 400 invalid-or-expired", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		rec := doResetPassword(auth, "this-token-was-never-issued", "somePassw0rd!")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp["error"] != "invalid or expired reset link" {
			t.Errorf(`error = %q, want "invalid or expired reset link"`, resp["error"])
		}
		garbageBody = rec.Body.Bytes()
	})

	// Case 11.
	t.Run("valid token with too-short password returns 400 password-length, after token validation", func(t *testing.T) {
		database := newTestDB(t)
		messagingSrv, _ := newMessagingFake(t)
		auth := handler.NewAuth(database, "test-secret", messagingSrv.URL, "test-messaging-secret", "http://localhost:5173")

		userID := seedUser(t, database, "shortpw@example.com")
		const rawToken = "valid-token-shortpw"
		seedResetToken(t, database, userID, rawToken, time.Now().Add(time.Hour).Unix(), 0)

		rec := doResetPassword(auth, rawToken, "short1")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Literal match against Register's existing message (auth.go): the
		// two endpoints must fail identically on a too-short password, and
		// distinctly from the token-invalid message above, so an attacker
		// probing token validity with a short password can't tell "bad
		// token" from "bad password" apart from a genuinely bad token.
		if resp["error"] != "password must be at least 8 characters" {
			t.Errorf(`error = %q, want "password must be at least 8 characters"`, resp["error"])
		}

		var used int
		if err := database.QueryRow(`SELECT used FROM password_reset_tokens WHERE user_id = ?`, userID).Scan(&used); err != nil {
			t.Fatalf("query token used flag: %v", err)
		}
		if used != 0 {
			t.Errorf("token used = %d, want 0 (a rejected request must not consume the token)", used)
		}
	})

	// Case 12 — pins "no distinguishing signal between invalid/expired/used".
	t.Run("expired, used, and garbage tokens all return byte-identical responses", func(t *testing.T) {
		if expiredBody == nil || usedBody == nil || garbageBody == nil {
			t.Fatal("expiredBody/usedBody/garbageBody were not captured by earlier subtests; subtests must run in order")
		}
		if !bytes.Equal(expiredBody, usedBody) {
			t.Errorf("expired-token body %q != used-token body %q", expiredBody, usedBody)
		}
		if !bytes.Equal(usedBody, garbageBody) {
			t.Errorf("used-token body %q != garbage-token body %q", usedBody, garbageBody)
		}
		if !bytes.Equal(expiredBody, garbageBody) {
			t.Errorf("expired-token body %q != garbage-token body %q", expiredBody, garbageBody)
		}
	})
}
