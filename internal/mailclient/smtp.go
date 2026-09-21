// Package mailclient provides real SMTP sending and IMAP fetching primitives
// used by internal/core/email. No fakes: every call either talks to a real
// (or test) mail server, or returns an honest error.
package mailclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message/mail"
)

// SMTPConfig describes how to reach and authenticate against an SMTP server.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string

	// ImplicitTLS selects SMTPS (TLS from the first byte, typically port 465)
	// instead of plaintext-then-STARTTLS (typically port 587/25).
	ImplicitTLS bool

	// InsecureSkipVerify disables certificate verification. Only ever set
	// this for tests against an in-process fake server.
	InsecureSkipVerify bool

	// AllowPlaintextAuth permits sending AUTH credentials over a connection
	// that never negotiated TLS. Only ever set this for tests.
	AllowPlaintextAuth bool
}

// OutgoingMessage is a message to be sent via SendMail.
type OutgoingMessage struct {
	From      string
	To        []string
	Cc        []string
	Subject   string
	TextBody  string
	HTMLBody  string
	MessageID string
	InReplyTo string
}

func (m OutgoingMessage) recipients() []string {
	out := make([]string, 0, len(m.To)+len(m.Cc))
	out = append(out, m.To...)
	out = append(out, m.Cc...)
	return out
}

var (
	ErrNoRecipients = errors.New("mailclient: at least one recipient is required")
	ErrNoSender     = errors.New("mailclient: sender address is required")
)

// connectAndAuth dials cfg.Host:cfg.Port, negotiates TLS (implicit or
// STARTTLS), and authenticates if credentials are set. The caller owns the
// returned client and must Close or Quit it.
func connectAndAuth(cfg SMTPConfig) (*smtp.Client, error) {
	if cfg.Host == "" {
		return nil, errors.New("mailclient: smtp host is required")
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	var client *smtp.Client
	var err error
	if cfg.ImplicitTLS {
		var conn net.Conn
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.InsecureSkipVerify}) //nolint:gosec // InsecureSkipVerify is test-only, gated by cfg.
		if err != nil {
			return nil, fmt.Errorf("mailclient: smtps dial failed: %w", err)
		}
		client, err = smtp.NewClient(conn, cfg.Host)
	} else {
		client, err = smtp.Dial(addr)
	}
	if err != nil {
		return nil, fmt.Errorf("mailclient: smtp connect failed: %w", err)
	}

	if !cfg.ImplicitTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.InsecureSkipVerify}); err != nil { //nolint:gosec
				_ = client.Close()
				return nil, fmt.Errorf("mailclient: starttls failed: %w", err)
			}
		} else if cfg.Username != "" && !cfg.AllowPlaintextAuth {
			_ = client.Close()
			return nil, errors.New("mailclient: server does not support STARTTLS; refusing to send credentials in plaintext")
		}
	}

	if cfg.Username != "" {
		auth := smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
		if err := client.Auth(auth); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("mailclient: smtp auth failed: %w", err)
		}
	}

	return client, nil
}

// TestAuth verifies that cfg's SMTP server is reachable and, if credentials
// are set, that they are accepted — without sending any mail.
func TestAuth(cfg SMTPConfig) error {
	client, err := connectAndAuth(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	return client.Quit()
}

// SendMail delivers msg over SMTP using cfg. It fails honestly: connection,
// auth, and protocol errors are returned as errors, never swallowed into a
// fake success.
func SendMail(cfg SMTPConfig, msg OutgoingMessage) error {
	if strings.TrimSpace(msg.From) == "" {
		return ErrNoSender
	}
	recipients := msg.recipients()
	if len(recipients) == 0 {
		return ErrNoRecipients
	}

	client, err := connectAndAuth(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := client.Mail(msg.From); err != nil {
		return fmt.Errorf("mailclient: MAIL FROM failed: %w", err)
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("mailclient: RCPT TO %s failed: %w", rcpt, err)
		}
	}

	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("mailclient: DATA failed: %w", err)
	}
	if err := writeMIME(wc, msg); err != nil {
		_ = wc.Close()
		return fmt.Errorf("mailclient: failed to build message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("mailclient: failed to finalize message: %w", err)
	}

	return client.Quit()
}

func writeMIME(w io.Writer, msg OutgoingMessage) error {
	var h mail.Header

	from, err := mail.ParseAddress(msg.From)
	if err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	h.SetAddressList("From", []*mail.Address{from})

	to, err := mail.ParseAddressList(strings.Join(msg.To, ","))
	if err != nil {
		return fmt.Errorf("invalid to address: %w", err)
	}
	h.SetAddressList("To", to)

	if len(msg.Cc) > 0 {
		cc, err := mail.ParseAddressList(strings.Join(msg.Cc, ","))
		if err != nil {
			return fmt.Errorf("invalid cc address: %w", err)
		}
		h.SetAddressList("Cc", cc)
	}

	h.SetSubject(msg.Subject)
	h.SetDate(time.Now())

	msgID := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(msg.MessageID), "<"), ">")
	if msgID != "" {
		h.SetMessageID(msgID)
	} else if err := h.GenerateMessageID(); err != nil {
		return fmt.Errorf("failed to generate message id: %w", err)
	}

	if msg.InReplyTo != "" {
		inReplyTo := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(msg.InReplyTo), "<"), ">")
		h.SetMsgIDList("In-Reply-To", []string{inReplyTo})
	}

	mw, err := mail.CreateWriter(w, h)
	if err != nil {
		return err
	}
	defer func() { _ = mw.Close() }()

	tw, err := mw.CreateInline()
	if err != nil {
		return err
	}
	defer func() { _ = tw.Close() }()

	if msg.TextBody != "" {
		var th mail.InlineHeader
		th.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := tw.CreatePart(th)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(pw, msg.TextBody); err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}

	if msg.HTMLBody != "" {
		var hh mail.InlineHeader
		hh.Set("Content-Type", "text/html; charset=utf-8")
		pw, err := tw.CreatePart(hh)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(pw, msg.HTMLBody); err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}

	if msg.TextBody == "" && msg.HTMLBody == "" {
		var th mail.InlineHeader
		th.Set("Content-Type", "text/plain; charset=utf-8")
		pw, err := tw.CreatePart(th)
		if err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}

	return nil
}
