package session

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"time"
)

const cookieName = "session"
const ttl = 7 * 24 * time.Hour

type Store struct {
	db     *sql.DB
	secure bool
}

func NewStore(db *sql.DB, secure bool) *Store {
	return &Store{db: db, secure: secure}
}

func (s *Store) Create(w http.ResponseWriter, jwt string) error {
	id, err := randomID()
	if err != nil {
		return err
	}
	now := time.Now()
	exp := now.Add(ttl)
	_, err = s.db.Exec(
		"INSERT INTO sessions (id, jwt, expires_at, created_at) VALUES (?, ?, ?, ?)",
		id, jwt, exp.Unix(), now.Unix(),
	)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
	return nil
}

// Get returns the JWT for the session ID in the cookie, or an error if missing/expired.
func (s *Store) Get(r *http.Request) (string, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", errors.New("no session")
	}

	var jwt string
	var expiresAt int64
	err = s.db.QueryRow(
		"SELECT jwt, expires_at FROM sessions WHERE id = ?", cookie.Value,
	).Scan(&jwt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("session not found")
	}
	if err != nil {
		return "", err
	}
	if time.Now().Unix() > expiresAt {
		return "", errors.New("session expired")
	}
	return jwt, nil
}

func (s *Store) Delete(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return
	}
	_, _ = s.db.Exec("DELETE FROM sessions WHERE id = ?", cookie.Value)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		MaxAge:   -1,
	})
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
