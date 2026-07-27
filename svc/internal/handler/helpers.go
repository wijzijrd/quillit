package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
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
