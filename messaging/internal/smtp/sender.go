// Package smtp provides a small abstraction over sending email so that
// callers depend on an interface (Sender) rather than a concrete transport.
package smtp

import (
	"bytes"
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
func buildMessage(from, to, subject, text, html string) ([]byte, error) {
	var buf bytes.Buffer

	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
	}

	if html == "" {
		headers["Content-Type"] = `text/plain; charset="UTF-8"`

		writeHeaders(&buf, headers)
		buf.WriteString("\r\n")
		buf.WriteString(text)

		return buf.Bytes(), nil
	}

	const boundary = "quillit-boundary-42"

	headers["Content-Type"] = fmt.Sprintf(`multipart/alternative; boundary="%s"`, boundary)
	writeHeaders(&buf, headers)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(text)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	buf.WriteString("\r\n")
	buf.WriteString(html)
	buf.WriteString("\r\n")

	buf.WriteString("--" + boundary + "--\r\n")

	return buf.Bytes(), nil
}

// writeHeaders writes RFC 5322 headers in a stable order (From, To,
// Subject, MIME-Version, Content-Type) followed by a CRLF-terminated line
// per header.
func writeHeaders(buf *bytes.Buffer, headers map[string]string) {
	order := []string{"From", "To", "Subject", "MIME-Version", "Content-Type"}
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
