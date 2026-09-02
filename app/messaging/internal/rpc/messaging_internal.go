// Package rpc implements messaging's server-to-server connect RPC surface:
// MessagingInternalService (proto/quillit/messaging/v1, generated into
// github.com/quillit/gen/quillit/messaging/v1). It is the connectrpc
// replacement for the old POST /send HTTP route
// (app/messaging/internal/handler/send.go's Send), called today by auth's
// password-reset flow (app/auth/internal/handler/password_reset.go's
// sendResetEmail). That HTTP route, and its handler-local
// X-Messaging-Secret check, are removed now that this RPC is its only
// caller — authentication is enforced once, at the transport level, by
// gen/internalauth's shared-secret interceptor (mounted in
// app/messaging/main.go) rather than inside the handler.
//
// SendEmail is backed by the exact same smtp.Sender the old HTTP handler
// used — this package only swaps the transport and validation shape (a
// connect error instead of a JSON error body).
package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/messaging/v1"

	"github.com/quillit/messaging-svc/internal/smtp"
)

// MessagingInternalServer implements
// messagingv1connect.MessagingInternalServiceHandler.
type MessagingInternalServer struct {
	sender smtp.Sender
}

// NewMessagingInternalServer builds a MessagingInternalServer around sender.
func NewMessagingInternalServer(sender smtp.Sender) *MessagingInternalServer {
	return &MessagingInternalServer{sender: sender}
}

// SendEmail validates the request and delivers it via sender.Send, mirroring
// the old Send HTTP handler's behavior: to/subject/text are required
// (CodeInvalidArgument, the RPC analogue of the old 400), and any Sender
// failure is reported as a generic error (CodeUnavailable, the RPC analogue
// of the old 502) — the underlying error is never echoed back to the
// caller, since it may contain SMTP server/internal network details that
// shouldn't leak across the service boundary.
func (s *MessagingInternalServer) SendEmail(ctx context.Context, req *connect.Request[v1.SendEmailRequest]) (*connect.Response[v1.SendEmailResponse], error) {
	to := req.Msg.GetTo()
	subject := req.Msg.GetSubject()
	text := req.Msg.GetText()
	html := req.Msg.GetHtml()

	if to == "" || subject == "" || text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("to, subject and text are required"))
	}

	if err := s.sender.Send(to, subject, text, html); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to send email"))
	}

	return connect.NewResponse(&v1.SendEmailResponse{Ok: true}), nil
}
