package mailclient

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
)

// IMAPConfig describes how to reach and authenticate against an IMAP server.
type IMAPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	Mailbox  string // defaults to "INBOX"

	// ImplicitTLS selects IMAPS (typically port 993) instead of STARTTLS
	// (typically port 143).
	ImplicitTLS bool

	// InsecureSkipVerify disables certificate verification. Only ever set
	// this for tests against an in-process fake server.
	InsecureSkipVerify bool
}

// Attachment is a single MIME attachment extracted from a fetched message.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// FetchedMessage is a single message retrieved from an IMAP mailbox.
type FetchedMessage struct {
	UID         uint32
	MessageID   string
	InReplyTo   string
	SenderEmail string
	SenderName  string
	Recipients  []string
	Subject     string
	TextBody    string
	HTMLBody    string
	Date        time.Time
	Attachments []Attachment
}

// FetchResult carries newly fetched messages plus the watermark to persist
// for the next incremental sync.
type FetchResult struct {
	Messages []FetchedMessage
	LastUID  uint32
}

func dialIMAP(cfg IMAPConfig) (*imapclient.Client, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	tlsConfig := &tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // test-only opt-in

	if cfg.ImplicitTLS {
		return imapclient.DialTLS(addr, &imapclient.Options{TLSConfig: tlsConfig})
	}
	return imapclient.DialStartTLS(addr, &imapclient.Options{TLSConfig: tlsConfig})
}

// TestLogin verifies that cfg's IMAP server is reachable and that the
// configured credentials are accepted, without fetching any messages.
func TestLogin(cfg IMAPConfig) error {
	if cfg.Host == "" {
		return errors.New("mailclient: imap host is required")
	}

	client, err := dialIMAP(cfg)
	if err != nil {
		return fmt.Errorf("mailclient: imap connect failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Login(cfg.Username, cfg.Password).Wait(); err != nil {
		return fmt.Errorf("mailclient: imap login failed: %w", err)
	}
	return client.Logout().Wait()
}

// FetchNewMessages performs an incremental UID-based fetch of every message
// with a UID greater than sinceUID, parsing sender, subject, body, and
// attachments. It returns an honest error on any connection, auth, or
// protocol failure — never a fabricated empty result.
func FetchNewMessages(cfg IMAPConfig, sinceUID uint32) (FetchResult, error) {
	if cfg.Host == "" {
		return FetchResult{}, errors.New("mailclient: imap host is required")
	}
	mailbox := cfg.Mailbox
	if mailbox == "" {
		mailbox = "INBOX"
	}

	client, err := dialIMAP(cfg)
	if err != nil {
		return FetchResult{}, fmt.Errorf("mailclient: imap connect failed: %w", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Login(cfg.Username, cfg.Password).Wait(); err != nil {
		return FetchResult{}, fmt.Errorf("mailclient: imap login failed: %w", err)
	}

	selectData, err := client.Select(mailbox, &imap.SelectOptions{}).Wait()
	if err != nil {
		return FetchResult{}, fmt.Errorf("mailclient: imap select %q failed: %w", mailbox, err)
	}

	result := FetchResult{LastUID: sinceUID}
	if selectData.NumMessages == 0 {
		return result, nil
	}

	var uidSet imap.UIDSet
	uidSet.AddRange(imap.UID(sinceUID+1), 0) // 0 == "*" (highest UID)

	fetchCmd := client.Fetch(uidSet, &imap.FetchOptions{
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{}},
	})
	buffers, err := fetchCmd.Collect()
	if err != nil {
		return FetchResult{}, fmt.Errorf("mailclient: imap fetch failed: %w", err)
	}

	for _, buf := range buffers {
		raw := buf.FindBodySection(&imap.FetchItemBodySection{})
		fm, err := parseMessage(raw)
		if err != nil {
			return FetchResult{}, fmt.Errorf("mailclient: failed to parse message uid=%d: %w", buf.UID, err)
		}
		fm.UID = uint32(buf.UID)
		result.Messages = append(result.Messages, fm)
		if uint32(buf.UID) > result.LastUID {
			result.LastUID = uint32(buf.UID)
		}
	}

	return result, nil
}

func parseMessage(raw []byte) (FetchedMessage, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return FetchedMessage{}, err
	}
	defer func() { _ = mr.Close() }()

	fm := FetchedMessage{}

	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		fm.SenderEmail = from[0].Address
		fm.SenderName = from[0].Name
	}
	if to, err := mr.Header.AddressList("To"); err == nil {
		for _, addr := range to {
			fm.Recipients = append(fm.Recipients, addr.Address)
		}
	}
	if subject, err := mr.Header.Subject(); err == nil {
		fm.Subject = subject
	}
	if date, err := mr.Header.Date(); err == nil {
		fm.Date = date
	}
	if msgID, err := mr.Header.MessageID(); err == nil {
		fm.MessageID = msgID
	}
	if refs, err := mr.Header.MsgIDList("In-Reply-To"); err == nil && len(refs) > 0 {
		fm.InReplyTo = refs[0]
	}

	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		body, readErr := io.ReadAll(part.Body)
		if readErr != nil {
			return FetchedMessage{}, readErr
		}

		switch h := part.Header.(type) {
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			fm.Attachments = append(fm.Attachments, Attachment{
				Filename:    filename,
				ContentType: h.Get("Content-Type"),
				Data:        body,
			})
		case *mail.InlineHeader:
			contentType := strings.ToLower(h.Get("Content-Type"))
			if strings.Contains(contentType, "text/html") {
				fm.HTMLBody = string(body)
			} else {
				fm.TextBody = string(body)
			}
		}
	}

	return fm, nil
}
