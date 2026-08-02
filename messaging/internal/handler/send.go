package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/quillit/messaging-svc/internal/smtp"
)

// Send handles POST /send, delivering an email via the configured Sender
// once the caller has proven knowledge of the shared messaging secret.
type Send struct {
	sender smtp.Sender
	secret string
}

// NewSend constructs a Send handler around the given Sender and shared
// secret. secret is compared against the caller-supplied
// X-Messaging-Secret header on every request.
func NewSend(sender smtp.Sender, secret string) *Send {
	return &Send{sender: sender, secret: secret}
}

// SendRequest is the body for send.
type SendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html,omitempty"`
}

// OkResponse indicates the email was accepted for delivery.
type OkResponse struct {
	Ok bool `json:"ok"`
}

// ErrorResponse is a generic error body.
type ErrorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}

// Send godoc
// @Summary      Send an email
// @Description  Sends a transactional email via SMTP. Requires the shared X-Messaging-Secret header.
// @Tags         messaging
// @Accept       json
// @Produce      json
// @Param        X-Messaging-Secret  header    string       true  "Shared messaging secret"
// @Param        body                body      SendRequest  true  "Email to send"
// @Success      200  {object}  OkResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      502  {object}  ErrorResponse
// @Router       /send [post]
func (s *Send) Send(w http.ResponseWriter, r *http.Request) {
	// subtle.ConstantTimeCompare guards against timing attacks on the shared
	// secret; a plain == comparison would leak how many leading bytes match.
	got := r.Header.Get("X-Messaging-Secret")
	if got == "" || len(got) != len(s.secret) || subtle.ConstantTimeCompare([]byte(got), []byte(s.secret)) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid or missing X-Messaging-Secret header")
		return
	}

	var body SendRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.To == "" || body.Subject == "" || body.Text == "" {
		writeError(w, http.StatusBadRequest, "to, subject and text are required")
		return
	}

	if err := s.sender.Send(body.To, body.Subject, body.Text, body.HTML); err != nil {
		// Never echo the underlying Sender error to the caller: it may
		// contain SMTP server/internal network details that shouldn't leak
		// across the service boundary.
		writeError(w, http.StatusBadGateway, "failed to send email")
		return
	}

	writeJSON(w, http.StatusOK, OkResponse{Ok: true})
}
