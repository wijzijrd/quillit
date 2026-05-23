package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/quillit/svc/internal/session"
)

type AuthHandler struct {
	authServiceURL string
	sessions       *session.Store
	jwtSecret      []byte
}

func NewAuth(authServiceURL string, sessions *session.Store, jwtSecret string) *AuthHandler {
	return &AuthHandler{authServiceURL: authServiceURL, sessions: sessions, jwtSecret: []byte(jwtSecret)}
}

// AuthStatusResponse mirrors the auth-svc status response.
type AuthStatusResponse struct {
	Registered bool `json:"registered"`
}

// AuthRegisterRequest is the registration body forwarded to auth-svc.
type AuthRegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthLoginRequest is the login body forwarded to auth-svc.
type AuthLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// OkResponse is a simple success acknowledgement.
type OkResponse struct {
	Ok bool `json:"ok"`
}

// MeResponse carries the current user's identity from the session JWT.
type MeResponse struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// ErrorResponse is a generic error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Status godoc
// @Summary      Auth status
// @Description  Proxies to auth-svc to check whether any users are registered.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  AuthStatusResponse
// @Router       /api/auth/status [get]
func (a *AuthHandler) Status(w http.ResponseWriter, r *http.Request) {
	a.proxy(w, r, http.MethodGet, "/auth/status", nil)
}

// Register godoc
// @Summary      Register
// @Description  Forwards credentials to auth-svc, creates an HTTP-only session cookie on success.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthRegisterRequest  true  "Registration credentials"
// @Success      200   {object}  OkResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      409   {object}  ErrorResponse
// @Router       /api/auth/register [post]
func (a *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	a.loginOrRegister(w, r, "/auth/register", http.StatusCreated)
}

// Login godoc
// @Summary      Login
// @Description  Forwards credentials to auth-svc, creates an HTTP-only session cookie on success.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      AuthLoginRequest  true  "Login credentials"
// @Success      200   {object}  OkResponse
// @Failure      400   {object}  ErrorResponse
// @Failure      401   {object}  ErrorResponse
// @Router       /api/auth/login [post]
func (a *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	a.loginOrRegister(w, r, "/auth/login", http.StatusOK)
}

// Logout godoc
// @Summary      Logout
// @Description  Deletes the session and clears the session cookie.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  OkResponse
// @Router       /api/auth/logout [post]
func (a *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	a.sessions.Delete(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Me godoc
// @Summary      Current user
// @Description  Returns identity decoded from the session JWT cookie.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  MeResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /api/auth/me [get]
func (a *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	raw, err := a.sessions.Get(r)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	token, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return a.jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"sub":   mc["sub"],
		"email": mc["email"],
		"role":  mc["role"],
	})
}

// loginOrRegister forwards the request body to auth-svc and, on success, creates a session.
func (a *AuthHandler) loginOrRegister(w http.ResponseWriter, r *http.Request, path string, successCode int) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	resp, err := http.Post(a.authServiceURL+path, "application/json", bytes.NewReader(body))
	if err != nil {
		writeError(w, http.StatusBadGateway, "auth service unavailable")
		return
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != successCode {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(respBody)
		return
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil || result.Token == "" {
		writeError(w, http.StatusInternalServerError, "invalid auth response")
		return
	}

	if err := a.sessions.Create(w, result.Token); err != nil {
		writeError(w, http.StatusInternalServerError, "session error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// proxy forwards a request to the auth service verbatim.
func (a *AuthHandler) proxy(w http.ResponseWriter, r *http.Request, method, path string, body io.Reader) {
	req, err := http.NewRequestWithContext(r.Context(), method, a.authServiceURL+path, body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("auth service unavailable: %v", err))
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}
