package handler

import (
	"encoding/json"
	"net/http"
)

// Health reports service liveness for deploy/uptime probes.
//
// Unlike auth's Health handler, messaging-svc has no database (or any other
// stateful dependency) to verify, so there's nothing to ping here — the
// struct exists purely to keep the constructor-based handler style used
// elsewhere in this codebase, even though it currently holds no fields.
type Health struct{}

func NewHealth() *Health {
	return &Health{}
}

// Check reports that the process is up. There is no dependency to verify
// (no DB, unlike auth's health handler), so this always returns 200.
// @Summary  Health check
// @Tags     health
// @Produce  json
// @Success  200 {object} map[string]string
// @Router   /healthz [get]
func (h *Health) Check(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON is the only remaining HTTP JSON helper in this package: the old
// POST /send route (and its writeError/OkResponse/ErrorResponse siblings)
// was removed in favor of the MessagingInternalService connect RPC (see
// internal/rpc) — /healthz is now the sole plain-HTTP route messaging-svc
// serves.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
