package handler

import (
	"database/sql"
	"net/http"
)

// Health reports service liveness for deploy/uptime probes.
type Health struct {
	db *sql.DB
}

func NewHealth(db *sql.DB) *Health {
	return &Health{db: db}
}

// Check verifies the process is up and the database is reachable.
func (h *Health) Check(w http.ResponseWriter, r *http.Request) {
	if err := h.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "error", "db": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
