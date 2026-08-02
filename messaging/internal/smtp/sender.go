// Package smtp provides a small abstraction over sending email so that
// callers depend on an interface (Sender) rather than a concrete transport.
package smtp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
)

// Sender sends an email message. Implementations are expected to deliver
// (or queue for delivery) a message with the given subject and body to a
// single recipient.
type Sender interface {
	// Send delivers an email to the given recipient with the given subject.
	// text is the plain-text body. If html is non-empty, the message is
	// built as a multipart/alternative message containing both the text
	// and html parts.
	Send(to, subject, text, html string) error
}

// SMTPSender is a Sender implementation that delivers mail via a real SMTP
// server using Go's standard library net/smtp package.
//
// Deliberately untested: unlike, say, a SQLite-backed repository which can
// be exercised against an in-memory (":memory:") database, there is no
// lightweight in-process SMTP server available here to send against, so a
// unit test for SMTPSender would either require a live mail server or
// devolve into asserting on internal implementation details (e.g. mocking
// net/smtp.SendMail) rather than real behavior. That trade-off isn't worth
// it, so this file intentionally ships without a _test.go companion. The
// Sender interface exists precisely so that HTTP-layer code can be tested
// against a fake Sender instead — see
// messaging/internal/handler/send_test.go (added in a later task), which
// covers the request/response behavior of the handler that calls Sender.Send
// without needing a real SMTP connection.
type SMTPSender struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

// Send builds an RFC 5322 message and delivers it via smtp.SendMail.
//
// When html is non-empty, the message is built as a multipart/alternative
// message with both a text/plain and a text/html part, in that order, so
// mail clients that can render HTML prefer it while clients that cannot
// still show the plain-text version. When html is empty, a simple
// text/plain message is sent.
//
// smtp.SendMail speaks plain SMTP and automatically upgrades to STARTTLS if
// the target server advertises support for it, so no additional TLS
// configuration is required here.
func (s *SMTPSender) Send(to, subject, text, html string) error {
	msg, err := buildMessage(s.From, to, subject, text, html)
	if err != nil {
		return fmt.Errorf("smtp: build message: %w", err)
	}

	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
	addr := s.Host + ":" + s.Port

	if err := smtp.SendMail(addr, auth, s.From, []string{to}, msg); err != nil {
		return fmt.Errorf("smtp: send mail: %w", err)
	}

	return nil
}

// buildMessage constructs an RFC 5322 email message as raw bytes, suitable
// for passing to smtp.SendMail. If html is non-empty the message is a
// multipart/alternative message with both text and html parts; otherwise it
// is a single text/plain message.
//
// from, to, and subject are caller-supplied (ultimately attacker-influenced,
// e.g. an unvalidated registration email address) and are sanitized against
// header injection before use; an error here means one of those fields
// contained a bare CR or LF, which would otherwise let an attacker splice in
// arbitrary extra headers (a forged From, an extra Bcc, etc).
func buildMessage(from, to, subject, text, html string) ([]byte, error) {
	from, err := sanitizeHeaderValue(from)
	if err != nil {
		return nil, fmt.Errorf("from contains invalid characters: %w", err)
	}
	to, err = sanitizeHeaderValue(to)
	if err != nil {
		return nil, fmt.Errorf("to contains invalid characters: %w", err)
	}
	subject, err = sanitizeHeaderValue(subject)
	if err != nil {
		return nil, fmt.Errorf("subject contains invalid characters: %w", err)
	}

	var buf bytes.Buffer

	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
	}

	if html == "" {
		headers["Content-Type"] = `text/plain; charset="UTF-8"`
		headers["Content-Transfer-Encoding"] = "8bit"

		writeHeaders(&buf, headers)
		buf.WriteString("\r\n")
		buf.WriteString(text)

		return buf.Bytes(), nil
	}

	boundary := generateBoundary()

	headers["Content-Type"] = fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary)
	writeHeaders(&buf, headers)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(text)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(html)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "--\r\n")

	return buf.Bytes(), nil
}

// sanitizeHeaderValue rejects header values containing a bare CR or LF
// anywhere in the string. Header values in RFC 5322 messages are terminated
// by CRLF, so an attacker-controlled value containing an embedded CR/LF can
// inject arbitrary extra headers (e.g. a spoofed From or an extra Bcc) once
// written by writeHeaders. Values are checked before trimming so that a
// CRLF hidden at the very edge of the string (which TrimSpace would
// otherwise silently remove) is still caught. Rejection, rather than silent
// stripping, is deliberate: stripping the bytes would hide the malformed
// input rather than surface it as an error the caller must handle.
func sanitizeHeaderValue(value string) (string, error) {
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("value contains a CR or LF character")
	}
	return strings.TrimSpace(value), nil
}

// generateBoundary returns a random MIME multipart boundary, unique per
// call. A static boundary would let attacker- or accident-controlled text
// or html body content containing a matching "--boundary" line corrupt the
// MIME structure; using crypto/rand instead of math/rand makes the
// resulting boundary unpredictable, matching the CSPRNG posture already
// used elsewhere in this codebase (see auth/internal/handler/auth.go's
// newID()). As with newID(), a failure to read from the OS CSPRNG is not a
// recoverable condition — falling back to a predictable boundary would
// reintroduce the exact issue this function exists to prevent — so this
// panics rather than returning a degraded value.
func generateBoundary() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "quillit-boundary-" + hex.EncodeToString(b)
}

// writeHeaders writes RFC 5322 headers in a stable order (From, To,
// Subject, MIME-Version, Content-Type, Content-Transfer-Encoding) followed
// by a CRLF-terminated line per header. Values are expected to already be
// sanitized (see sanitizeHeaderValue); TrimSpace here is a harmless no-op
// for already-trimmed values and a defensive backstop for the
// caller-independent, hardcoded values in this map (MIME-Version,
// Content-Type, Content-Transfer-Encoding).
func writeHeaders(buf *bytes.Buffer, headers map[string]string) {
	order := []string{"From", "To", "Subject", "MIME-Version", "Content-Type", "Content-Transfer-Encoding"}
	for _, key := range order {
		value, ok := headers[key]
		if !ok {
			continue
		}
		buf.WriteString(key)
		buf.WriteString(": ")
		buf.WriteString(strings.TrimSpace(value))
		buf.WriteString("\r\n")
	}
}
