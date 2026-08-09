package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
// no/invalid token is present — unlike svc's callerIDFromRequest, this does
// not gate access.
//
// TODO(#44): this is the "simplified/stubbed check" #44's issue text
// anticipates retrofitting — content doesn't yet verify project membership
// (svc owns that data) and doesn't yet reject unauthenticated requests.
// #44 picks the real cross-domain auth mechanism (svc-verified JWT claims
// vs. an internal membership lookup) and applies it across every endpoint
// added in #37-#43, including this one.
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
