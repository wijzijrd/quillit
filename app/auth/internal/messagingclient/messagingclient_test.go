package messagingclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/messaging/v1"
	"github.com/quillit/gen/quillit/messaging/v1/messagingv1connect"
)

// stubMessagingInternal is a minimal
// messagingv1connect.MessagingInternalServiceHandler backed by a plain func
// field, so each test can supply exactly the behavior it needs without a
// real messaging service.
type stubMessagingInternal struct {
	sendEmail func(context.Context, *connect.Request[v1.SendEmailRequest]) (*connect.Response[v1.SendEmailResponse], error)
}

func (s stubMessagingInternal) SendEmail(ctx context.Context, req *connect.Request[v1.SendEmailRequest]) (*connect.Response[v1.SendEmailResponse], error) {
	return s.sendEmail(ctx, req)
}

func newTestMessagingServer(t *testing.T, stub stubMessagingInternal) *httptest.Server {
	t.Helper()
	path, h := messagingv1connect.NewMessagingInternalServiceHandler(stub)
	mux := http.NewServeMux()
	mux.Handle(path, h)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestClient_SendEmail_Success(t *testing.T) {
	var got *v1.SendEmailRequest
	srv := newTestMessagingServer(t, stubMessagingInternal{
		sendEmail: func(_ context.Context, req *connect.Request[v1.SendEmailRequest]) (*connect.Response[v1.SendEmailResponse], error) {
			got = req.Msg
			return connect.NewResponse(&v1.SendEmailResponse{Ok: true}), nil
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	if err := c.SendEmail(context.Background(), "a@example.com", "hi", "hello", "<p>hello</p>"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if got == nil {
		t.Fatal("server never received a request")
	}
	if got.GetTo() != "a@example.com" || got.GetSubject() != "hi" || got.GetText() != "hello" || got.GetHtml() != "<p>hello</p>" {
		t.Errorf("unexpected request: %+v", got)
	}
}

func TestClient_SendEmail_ServerErrorIsError(t *testing.T) {
	srv := newTestMessagingServer(t, stubMessagingInternal{
		sendEmail: func(context.Context, *connect.Request[v1.SendEmailRequest]) (*connect.Response[v1.SendEmailResponse], error) {
			return nil, connect.NewError(connect.CodeUnavailable, errors.New("failed to send email"))
		},
	})

	c := NewClient(srv.URL, "test-secret", nil)
	if err := c.SendEmail(context.Background(), "a@example.com", "hi", "hello", ""); err == nil {
		t.Error("expected an error for a CodeUnavailable response, got nil")
	}
}

func TestClient_SendEmail_UnreachableServerIsError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "test-secret", nil) // nothing listens here
	if err := c.SendEmail(context.Background(), "a@example.com", "hi", "hello", ""); err == nil {
		t.Error("expected an error for an unreachable messaging service, got nil")
	}
}
