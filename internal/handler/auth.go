package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

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
	Sub   string  `json:"sub"`
	Email string  `json:"email"`
	Role  string  `json:"role"`
	Exp   float64 `json:"exp"`
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
// @Description  Creates a GM account. Returns 409 if email or username is taken.
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
		"INSERT INTO users (id, email, username, password_hash, role, created_at, updated_at) VALUES (?, ?, ?, ?, 'gm', ?, ?)",
		id, body.Email, body.Username, string(hash), now, now,
	)
	if err != nil {
		writeError(w, http.StatusConflict, "email or username already in use")
		return
	}

	token, err := a.issueToken(id, body.Email, "gm")
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
	err := a.db.QueryRow(
		"SELECT id, password_hash, role FROM users WHERE email = ?", body.Email,
	).Scan(&id, &hash, &role)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := a.issueToken(id, body.Email, role)
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

	writeJSON(w, http.StatusOK, map[string]any{
		"sub":   mc["sub"],
		"email": mc["email"],
		"role":  mc["role"],
		"exp":   mc["exp"],
	})
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
	Email string `json:"email"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

func (a *Auth) issueToken(userID, email, role string) (string, error) {
	now := time.Now()
	claims := quilltClaims{
		Email: email,
		Role:  role,
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
