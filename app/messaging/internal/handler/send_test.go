package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quillit/messaging-svc/internal/handler"
)

type call struct{ to, subject, text, html string }

type fakeSender struct {
	calls []call
	err   error
}

func (f *fakeSender) Send(to, subject, text, html string) error {
	f.calls = append(f.calls, call{to, subject, text, html})
	return f.err
}

const testSecret = "test-secret"

func newSendRequest(body string, secret string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(body))
	if secret != "" {
		req.Header.Set("X-Messaging-Secret", secret)
	}
	return req
}

func TestSend(t *testing.T) {
	t.Run("missing secret header", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi","text":"hello"}`
		req := newSendRequest(body, "")
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(sender.calls) != 0 {
			t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
		}
	})

	t.Run("wrong secret header", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi","text":"hello"}`
		req := newSendRequest(body, "wrong-secret")
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(sender.calls) != 0 {
			t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
		}
	})

	t.Run("missing to", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"subject":"hi","text":"hello"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if len(sender.calls) != 0 {
			t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
		}
	})

	t.Run("missing subject", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","text":"hello"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if len(sender.calls) != 0 {
			t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
		}
	})

	t.Run("missing text", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
		if len(sender.calls) != 0 {
			t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
		}
	})

	t.Run("valid body with html", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi","text":"hello","html":"<p>hello</p>"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if len(sender.calls) != 1 {
			t.Fatalf("sender.calls = %d, want 1", len(sender.calls))
		}
		got := sender.calls[0]
		want := call{to: "a@example.com", subject: "hi", text: "hello", html: "<p>hello</p>"}
		if got != want {
			t.Fatalf("sender call = %+v, want %+v", got, want)
		}
	})

	t.Run("valid body without html", func(t *testing.T) {
		sender := &fakeSender{}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi","text":"hello"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if len(sender.calls) != 1 {
			t.Fatalf("sender.calls = %d, want 1", len(sender.calls))
		}
		got := sender.calls[0]
		want := call{to: "a@example.com", subject: "hi", text: "hello", html: ""}
		if got != want {
			t.Fatalf("sender call = %+v, want %+v", got, want)
		}
	})

	t.Run("sender error returns generic 502", func(t *testing.T) {
		rawErr := "smtp: send mail: dial tcp: connection refused by mail.internal.example.com"
		sender := &fakeSender{err: errors.New(rawErr)}
		send := handler.NewSend(sender, testSecret)

		body := `{"to":"a@example.com","subject":"hi","text":"hello"}`
		req := newSendRequest(body, testSecret)
		rec := httptest.NewRecorder()
		send.Send(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", rec.Code)
		}
		if strings.Contains(rec.Body.String(), rawErr) {
			t.Fatalf("response body leaked raw sender error: %s", rec.Body.String())
		}

		var resp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(bytes.NewReader(rec.Body.Bytes())).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Error == "" {
			t.Fatalf("expected a non-empty generic error message")
		}
		if strings.Contains(resp.Error, rawErr) {
			t.Fatalf("error field leaked raw sender error: %s", resp.Error)
		}
	})
}
