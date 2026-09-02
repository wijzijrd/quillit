package rpc_test

import (
	"errors"
	"strings"
	"testing"

	"connectrpc.com/connect"

	v1 "github.com/quillit/gen/quillit/messaging/v1"

	"github.com/quillit/messaging-svc/internal/rpc"
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

func TestSendEmail_MissingTo(t *testing.T) {
	sender := &fakeSender{}
	srv := rpc.NewMessagingInternalServer(sender)

	_, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{Subject: "hi", Text: "hello"}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want CodeInvalidArgument", connect.CodeOf(err))
	}
	if len(sender.calls) != 0 {
		t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
	}
}

func TestSendEmail_MissingSubject(t *testing.T) {
	sender := &fakeSender{}
	srv := rpc.NewMessagingInternalServer(sender)

	_, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{To: "a@example.com", Text: "hello"}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want CodeInvalidArgument", connect.CodeOf(err))
	}
	if len(sender.calls) != 0 {
		t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
	}
}

func TestSendEmail_MissingText(t *testing.T) {
	sender := &fakeSender{}
	srv := rpc.NewMessagingInternalServer(sender)

	_, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{To: "a@example.com", Subject: "hi"}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want CodeInvalidArgument", connect.CodeOf(err))
	}
	if len(sender.calls) != 0 {
		t.Fatalf("sender.calls = %d, want 0", len(sender.calls))
	}
}

func TestSendEmail_ValidWithHTML(t *testing.T) {
	sender := &fakeSender{}
	srv := rpc.NewMessagingInternalServer(sender)

	resp, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{
		To: "a@example.com", Subject: "hi", Text: "hello", Html: "<p>hello</p>",
	}))
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if !resp.Msg.GetOk() {
		t.Error("Ok = false, want true")
	}
	if len(sender.calls) != 1 {
		t.Fatalf("sender.calls = %d, want 1", len(sender.calls))
	}
	got := sender.calls[0]
	want := call{to: "a@example.com", subject: "hi", text: "hello", html: "<p>hello</p>"}
	if got != want {
		t.Fatalf("sender call = %+v, want %+v", got, want)
	}
}

func TestSendEmail_ValidWithoutHTML(t *testing.T) {
	sender := &fakeSender{}
	srv := rpc.NewMessagingInternalServer(sender)

	resp, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{
		To: "a@example.com", Subject: "hi", Text: "hello",
	}))
	if err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	if !resp.Msg.GetOk() {
		t.Error("Ok = false, want true")
	}
	if len(sender.calls) != 1 {
		t.Fatalf("sender.calls = %d, want 1", len(sender.calls))
	}
	got := sender.calls[0]
	want := call{to: "a@example.com", subject: "hi", text: "hello", html: ""}
	if got != want {
		t.Fatalf("sender call = %+v, want %+v", got, want)
	}
}

func TestSendEmail_SenderErrorReturnsGenericCodeUnavailable(t *testing.T) {
	rawErr := "smtp: send mail: dial tcp: connection refused by mail.internal.example.com"
	sender := &fakeSender{err: errors.New(rawErr)}
	srv := rpc.NewMessagingInternalServer(sender)

	_, err := srv.SendEmail(t.Context(), connect.NewRequest(&v1.SendEmailRequest{
		To: "a@example.com", Subject: "hi", Text: "hello",
	}))
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Errorf("code = %v, want CodeUnavailable", connect.CodeOf(err))
	}
	if strings.Contains(err.Error(), rawErr) {
		t.Fatalf("error leaked raw sender error: %v", err)
	}
}
