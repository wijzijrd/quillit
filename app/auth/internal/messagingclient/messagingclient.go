// Package messagingclient is auth's connectrpc client for the quillit
// messaging service's MessagingInternalService — auth's one live
// cross-service dependency, used by the password-reset flow
// (internal/handler/password_reset.go's sendResetEmail) to deliver the
// reset-link email.
//
// This used to be a plain HTTP POST to messaging's /send route,
// authenticated with a hand-built X-Messaging-Secret header; it's now a
// connectrpc call against messaging's MessagingInternalService (see
// gen/quillit/messaging/v1 and app/messaging/internal/rpc for the server
// side), authenticated with the shared INTERNAL_RPC_SECRET via
// gen/internalauth's client interceptor instead.
package messagingclient

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	"github.com/quillit/gen/internalauth"
	v1 "github.com/quillit/gen/quillit/messaging/v1"
	"github.com/quillit/gen/quillit/messaging/v1/messagingv1connect"
)

// Client is a connectrpc client for messaging's MessagingInternalService.
type Client struct {
	rpc messagingv1connect.MessagingInternalServiceClient
}

// NewClient builds a Client pointed at messaging's baseURL, authenticating
// every call with secret (INTERNAL_RPC_SECRET — see gen/internalauth).
// httpClient may be nil, in which case http.DefaultClient is used.
func NewClient(baseURL, secret string, httpClient *http.Client) *Client {
	var hc connect.HTTPClient = http.DefaultClient
	if httpClient != nil {
		hc = httpClient
	}
	return &Client{
		rpc: messagingv1connect.NewMessagingInternalServiceClient(
			hc,
			baseURL,
			connect.WithInterceptors(internalauth.NewClientInterceptor(secret)),
		),
	}
}

// SendEmail delivers an email via messaging. to, subject and text are
// required by the receiving service; html may be empty. Returns a wrapped
// error for any failure (validation, a Sender failure on messaging's side,
// a network error, ...) — callers don't need to distinguish them, mirroring
// the old HTTP client's "non-200 is an error" behavior.
func (c *Client) SendEmail(ctx context.Context, to, subject, text, html string) error {
	_, err := c.rpc.SendEmail(ctx, connect.NewRequest(&v1.SendEmailRequest{
		To:      to,
		Subject: subject,
		Text:    text,
		Html:    html,
	}))
	if err != nil {
		return fmt.Errorf("SendEmail(%s): %w", to, err)
	}
	return nil
}
