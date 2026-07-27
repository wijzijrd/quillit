package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Auth struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewAuth(db *sql.DB, jwtSecret string) *Auth {
	return &Auth{db: db, jwtSecret: []byte(jwtSecret)}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// StatusResponse represents the registration status.
type StatusResponse struct {
	Registered bool `json:"registered"`
}

// RegisterRequest is the body for register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginRequest is the body for login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// TokenResponse carries a signed JWT.
type TokenResponse struct {
	Token string `json:"token"`
}

// VerifyRequest carries the token to verify.
type VerifyRequest struct {
	Token string `json:"token"`
}

// ClaimsResponse is the decoded token payload.
type ClaimsResponse struct {
	Sub    string  `json:"sub"`
	Email  string  `json:"email"`
	Role   string  `json:"role"`
	Active bool    `json:"active"`
	Exp    float64 `json:"exp"`
}

// UserResponse is a user record returned to admins.
type UserResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
	CreatedAt int64  `json:"createdAt"`
}

// ErrorResponse is a generic error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Status godoc
// @Summary      Registration status
// @Description  Returns whether any users are registered in the auth database.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  StatusResponse
// @Router       /auth/status [get]
func (a *Auth) Status(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	writeJSON(w, http.StatusOK, map[string]bool{"registered": count > 0})
}

// Register godoc
// @Summary      Register a new user
// @Description  Creates a user account. Returns 409 if email or username is taken.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      RegisterRequest  true  "Registration credentials"
// @Success      201   {object}  TokenResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      409   {object}  ErrorResponse
// @Router       /auth/register [post]
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Email == "" || body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email, username and password are required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id := newID()
	now := time.Now().Unix()
	_, err = a.db.Exec(
		"INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'user', ?, ?)",
		id, body.Email, body.Username, string(hash), now, now,
	)
	if err != nil {
		writeError(w, http.StatusConflict, "email or username already in use")
		return
	}

	token, err := a.issueToken(id, body.Email, "user", true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

// Login godoc
// @Summary      Login
// @Description  Authenticates with email and password and returns a signed JWT.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest  true  "Login credentials"
// @Success      200   {object}  TokenResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Router       /auth/login [post]
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	var id, hash, role string
	var active int
	err := a.db.QueryRow(
		"SELECT id, password_hash, role, active FROM users WHERE email = ?", body.Email,
	).Scan(&id, &hash, &role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if active == 0 {
		writeError(w, http.StatusUnauthorized, "account disabled")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := a.issueToken(id, body.Email, role, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// Verify godoc
// @Summary      Verify a JWT
// @Description  Validates a JWT and returns its decoded claims. Used internally by quillit-svc.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      VerifyRequest   true  "Token to verify"
// @Success      200   {object}  ClaimsResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Router       /auth/verify [post]
func (a *Auth) Verify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}

	claims, err := a.parseToken(body.Token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	mc, ok := claims.(jwt.MapClaims)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid token claims")
		return
	}

	sub, _ := mc["sub"].(string)

	// Re-check current active/role from the DB rather than trusting the
	// claims baked in at token-issue time — otherwise a user deactivated
	// after login keeps verifying as active until the token expires.
	var role string
	var active int
	err = a.db.QueryRow("SELECT role, active FROM users WHERE id = ?", sub).Scan(&role, &active)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sub":    sub,
		"email":  mc["email"],
		"role":   role,
		"active": active == 1,
		"exp":    mc["exp"],
	})
}

// RequireAnyAuth validates any valid Bearer JWT, regardless of role.
func (a *Auth) RequireAnyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "authorization required")
			return
		}
		if _, err := a.parseToken(strings.TrimPrefix(auth, "Bearer ")); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserSearchResult is the minimal user info returned by SearchUsers.
type UserSearchResult struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

// SearchUsers godoc
// @Summary      Search users
// @Description  Returns users matching the query (email or username). Any valid JWT required.
// @Tags         auth
// @Produce      json
// @Param        q  query  string  false  "Search term"
// @Success      200  {array}   UserSearchResult
// @Failure      401  {object}  ErrorResponse
// @Router       /auth/users/search [get]
func (a *Auth) SearchUsers(w http.ResponseWriter, r *http.Request) {
	q := "%" + r.URL.Query().Get("q") + "%"
	rows, err := a.db.Query(
		`SELECT id, email, username FROM users WHERE email LIKE ? OR username LIKE ? ORDER BY username LIMIT 20`,
		q, q,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	results := []UserSearchResult{}
	for rows.Next() {
		var u UserSearchResult
		if err := rows.Scan(&u.ID, &u.Email, &u.Username); err == nil {
			results = append(results, u)
		}
	}
	writeJSON(w, http.StatusOK, results)
}

// RequireAdmin is middleware that validates a Bearer JWT with role=admin.
func (a *Auth) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "authorization required")
			return
		}
		claims, err := a.parseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		mc, ok := claims.(jwt.MapClaims)
		if !ok || mc["role"] != "admin" {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ListUsers godoc
// @Summary      List users
// @Description  Returns all users, optionally filtered. Requires admin JWT.
// @Tags         admin
// @Produce      json
// @Param        q       query  string  false  "Search by email or username"
// @Param        active  query  string  false  "Filter by active status: true or false"
// @Success      200  {array}   UserResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /auth/users [get]
func (a *Auth) ListUsers(w http.ResponseWriter, r *http.Request) {
	q := "%" + r.URL.Query().Get("q") + "%"
	activeFilter := r.URL.Query().Get("active")

	query := `SELECT id, email, username, role, active, created_at FROM users WHERE (email LIKE ? OR username LIKE ?)`
	args := []any{q, q}

	switch activeFilter {
	case "true":
		query += " AND active = 1"
	case "false":
		query += " AND active = 0"
	}
	query += " ORDER BY created_at DESC"

	rows, err := a.db.Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	users := []UserResponse{}
	for rows.Next() {
		var u UserResponse
		var active int
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Role, &active, &u.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		u.Active = active == 1
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, users)
}

// UpdateUser godoc
// @Summary      Update a user
// @Description  Updates user fields (currently: active). Requires admin JWT.
// @Tags         admin
// @Accept       json
// @Produce      json
// @Param        id    path  string  true  "User ID"
// @Param        body  body  object  true  "Fields to update"
// @Success      200   {object}  UserResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      403   {object}  ErrorResponse
// @Failure      404   {object}  ErrorResponse
// @Router       /auth/users/{id} [patch]
func (a *Auth) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		Active *bool `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Active == nil {
		writeError(w, http.StatusBadRequest, "no updatable fields provided")
		return
	}

	activeVal := 0
	if *body.Active {
		activeVal = 1
	}
	res, err := a.db.Exec(
		"UPDATE users SET active = ?, updated_at = ? WHERE id = ?",
		activeVal, time.Now().Unix(), id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var u UserResponse
	var active int
	if err := a.db.QueryRow(
		"SELECT id, email, username, role, active, created_at FROM users WHERE id = ?", id,
	).Scan(&u.ID, &u.Email, &u.Username, &u.Role, &active, &u.CreatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	u.Active = active == 1
	writeJSON(w, http.StatusOK, u)
}

// DeleteUser godoc
// @Summary      Delete a user
// @Description  Permanently removes a user account. Requires admin JWT.
// @Tags         admin
// @Produce      json
// @Param        id  path  string  true  "User ID"
// @Success      204
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /auth/users/{id} [delete]
func (a *Auth) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := a.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SeedAdmin creates an admin user if one with the given email doesn't exist.
func (a *Auth) SeedAdmin(email, password string) error {
	var count int
	_ = a.db.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?", email).Scan(&count)
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}
	id := newID()
	now := time.Now().Unix()
	_, err = a.db.Exec(
		"INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'admin', ?, ?)",
		id, email, "admin", string(hash), now, now,
	)
	return err
}

type quilltClaims struct {
	Email  string `json:"email"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
	jwt.RegisteredClaims
}

func (a *Auth) issueToken(userID, email, role string, active bool) (string, error) {
	now := time.Now()
	claims := quilltClaims{
		Email:  email,
		Role:   role,
		Active: active,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(a.jwtSecret)
}

func (a *Auth) parseToken(raw string) (jwt.Claims, error) {
	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return a.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return token.Claims, nil
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
