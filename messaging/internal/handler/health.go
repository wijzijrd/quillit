package handler

import "net/http"

// Health reports service liveness for deploy/uptime probes.
//
// Unlike auth's Health handler, messaging-svc has no database (or any other
// stateful dependency) to verify, so there's nothing to ping here — the
// struct exists purely to keep the constructor-based handler style used
// elsewhere in this codebase (e.g. handler.NewSend), even though it
// currently holds no fields.
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
