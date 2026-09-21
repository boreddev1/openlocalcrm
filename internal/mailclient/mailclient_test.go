package mailclient

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
)

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("build tls certificate: %v", err)
	}
	return cert
}

// --- Fake SMTP server -------------------------------------------------

type fakeSMTPServer struct {
	ln                net.Listener
	tlsConfig         *tls.Config
	advertiseSTARTTLS bool
	username          string
	password          string

	mu       sync.Mutex
	mailFrom string
	rcptTo   []string
	dataRaw  []byte
	authSeen bool
}

func newFakeSMTPServer(t *testing.T, advertiseSTARTTLS bool, cert tls.Certificate) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTPServer{
		ln:                ln,
		tlsConfig:         &tls.Config{Certificates: []tls.Certificate{cert}},
		advertiseSTARTTLS: advertiseSTARTTLS,
		username:          "user@example.com",
		password:          "s3cret",
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		s.handleConn(conn)
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return s
}

func (s *fakeSMTPServer) addr() (string, int) {
	tcpAddr := s.ln.Addr().(*net.TCPAddr)
	return tcpAddr.IP.String(), tcpAddr.Port
}

var addrRe = regexp.MustCompile(`<([^>]*)>`)

func extractAddr(line string) string {
	if m := addrRe.FindStringSubmatch(line); len(m) == 2 {
		return m[1]
	}
	return ""
}

func (s *fakeSMTPServer) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	var writer io.Writer = conn
	tlsActive := false

	send := func(msg string) { _, _ = writer.Write([]byte(msg + "\r\n")) }
	send("220 fake.smtp.test ESMTP ready")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"):
			send("250-fake.smtp.test greets you")
			switch {
			case tlsActive:
				send("250 AUTH PLAIN")
			case s.advertiseSTARTTLS:
				send("250 STARTTLS")
			default:
				send("250 8BITMIME")
			}
		case strings.HasPrefix(upper, "STARTTLS"):
			send("220 Ready to start TLS")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = bufio.NewReader(conn)
			writer = conn
			tlsActive = true
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			fields := strings.Fields(line)
			var b64 string
			if len(fields) >= 3 {
				b64 = fields[2]
			} else {
				send("334 ")
				cont, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				b64 = strings.TrimRight(cont, "\r\n")
			}
			decoded, err := base64.StdEncoding.DecodeString(b64)
			ok := err == nil
			if ok {
				parts := strings.Split(string(decoded), "\x00")
				ok = len(parts) == 3 && parts[1] == s.username && parts[2] == s.password
			}
			s.mu.Lock()
			s.authSeen = ok
			s.mu.Unlock()
			if ok {
				send("235 Authentication successful")
			} else {
				send("535 Authentication failed")
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFrom = extractAddr(line)
			s.mu.Unlock()
			send("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcptTo = append(s.rcptTo, extractAddr(line))
			s.mu.Unlock()
			send("250 OK")
		case strings.HasPrefix(upper, "DATA"):
			send("354 End data with <CR><LF>.<CR><LF>")
			var buf bytes.Buffer
			for {
				l, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
				buf.WriteString(l)
			}
			s.mu.Lock()
			s.dataRaw = buf.Bytes()
			s.mu.Unlock()
			send("250 OK: queued")
		case strings.HasPrefix(upper, "QUIT"):
			send("221 Bye")
			return
		case strings.HasPrefix(upper, "NOOP"):
			send("250 OK")
		default:
			send("502 command not implemented")
		}
	}
}

func TestSendMail_STARTTLS_PlaintextAndMultipart(t *testing.T) {
	cert := generateSelfSignedCert(t)
	srv := newFakeSMTPServer(t, true, cert)
	host, port := srv.addr()

	cfg := SMTPConfig{
		Host:               host,
		Port:               port,
		Username:           srv.username,
		Password:           srv.password,
		InsecureSkipVerify: true,
	}

	err := SendMail(cfg, OutgoingMessage{
		From:     "sender@openlocalcrm.local",
		To:       []string{"kunde@example.com"},
		Subject:  "Ihr Angebot",
		TextBody: "Hallo Kunde,\r\nvielen Dank fuer Ihr Interesse.",
	})
	if err != nil {
		t.Fatalf("SendMail (plaintext) failed: %v", err)
	}

	srv.mu.Lock()
	if !srv.authSeen {
		t.Error("expected AUTH to succeed via STARTTLS-secured connection")
	}
	if srv.mailFrom != "sender@openlocalcrm.local" {
		t.Errorf("mailFrom = %q, want sender@openlocalcrm.local", srv.mailFrom)
	}
	if len(srv.rcptTo) != 1 || srv.rcptTo[0] != "kunde@example.com" {
		t.Errorf("rcptTo = %v, want [kunde@example.com]", srv.rcptTo)
	}
	data := string(srv.dataRaw)
	srv.mu.Unlock()

	if !strings.Contains(data, "Subject: =?utf-8?q?Ihr_Angebot?=") && !strings.Contains(data, "Subject: Ihr Angebot") {
		t.Errorf("data missing subject header, got:\n%s", data)
	}
	if !strings.Contains(data, "vielen Dank") {
		t.Errorf("data missing text body, got:\n%s", data)
	}

	// Second send, this time multipart (text + html), against a fresh server.
	srv2 := newFakeSMTPServer(t, true, cert)
	host2, port2 := srv2.addr()
	cfg2 := cfg
	cfg2.Host = host2
	cfg2.Port = port2

	err = SendMail(cfg2, OutgoingMessage{
		From:     "sender@openlocalcrm.local",
		To:       []string{"kunde@example.com"},
		Cc:       []string{"vertrieb@openlocalcrm.local"},
		Subject:  "Ihr Angebot (HTML)",
		TextBody: "Hallo Kunde (Text)",
		HTMLBody: "<p>Hallo Kunde (HTML)</p>",
	})
	if err != nil {
		t.Fatalf("SendMail (multipart) failed: %v", err)
	}

	srv2.mu.Lock()
	data2 := string(srv2.dataRaw)
	rcpt2 := append([]string{}, srv2.rcptTo...)
	srv2.mu.Unlock()

	if len(rcpt2) != 2 {
		t.Errorf("expected 2 recipients (to+cc), got %v", rcpt2)
	}
	if !strings.Contains(data2, "multipart/alternative") {
		t.Errorf("expected multipart/alternative body, got:\n%s", data2)
	}
	if !strings.Contains(data2, "Hallo Kunde (Text)") || !strings.Contains(data2, "Hallo Kunde (HTML)") {
		t.Errorf("expected both text and html parts, got:\n%s", data2)
	}
}

func TestSendMail_RefusesPlaintextAuthWithoutSTARTTLS(t *testing.T) {
	cert := generateSelfSignedCert(t)
	srv := newFakeSMTPServer(t, false, cert) // does not advertise STARTTLS
	host, port := srv.addr()

	cfg := SMTPConfig{
		Host:     host,
		Port:     port,
		Username: srv.username,
		Password: srv.password,
	}

	err := SendMail(cfg, OutgoingMessage{
		From:     "sender@openlocalcrm.local",
		To:       []string{"kunde@example.com"},
		Subject:  "Test",
		TextBody: "Test",
	})
	if err == nil {
		t.Fatal("expected SendMail to refuse plaintext auth, got nil error")
	}
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Errorf("expected error about STARTTLS, got: %v", err)
	}

	srv.mu.Lock()
	authSeen := srv.authSeen
	mailFrom := srv.mailFrom
	srv.mu.Unlock()
	if authSeen {
		t.Error("credentials must never be sent when STARTTLS is unavailable")
	}
	if mailFrom != "" {
		t.Error("SendMail must not proceed to MAIL FROM when it refused to authenticate")
	}
}

func TestSendMail_ValidatesInput(t *testing.T) {
	if err := SendMail(SMTPConfig{Host: "127.0.0.1"}, OutgoingMessage{To: []string{"a@example.com"}}); err != ErrNoSender {
		t.Errorf("expected ErrNoSender, got %v", err)
	}
	if err := SendMail(SMTPConfig{Host: "127.0.0.1"}, OutgoingMessage{From: "a@example.com"}); err != ErrNoRecipients {
		t.Errorf("expected ErrNoRecipients, got %v", err)
	}
}

func TestFetchNewMessages_RequiresHost(t *testing.T) {
	if _, err := FetchNewMessages(IMAPConfig{}, 0); err == nil {
		t.Fatal("expected FetchNewMessages to require a host")
	}
}

func TestTestAuth_SucceedsAndFailsHonestly(t *testing.T) {
	cert := generateSelfSignedCert(t)

	srv := newFakeSMTPServer(t, true, cert)
	host, port := srv.addr()
	cfg := SMTPConfig{
		Host:               host,
		Port:               port,
		Username:           srv.username,
		Password:           srv.password,
		InsecureSkipVerify: true,
	}
	if err := TestAuth(cfg); err != nil {
		t.Fatalf("TestAuth failed against a working server: %v", err)
	}

	badCfg := SMTPConfig{Host: "127.0.0.1", Port: 1}
	if err := TestAuth(badCfg); err == nil {
		t.Fatal("expected TestAuth to fail honestly against an unreachable host")
	}
}

func TestTestLogin_SucceedsAndFailsHonestly(t *testing.T) {
	cert := generateSelfSignedCert(t)
	host, port := startFakeIMAPServer(t, "inbox@openlocalcrm.local", "s3cret", cert, nil)

	cfg := IMAPConfig{
		Host:               host,
		Port:               port,
		Username:           "inbox@openlocalcrm.local",
		Password:           "s3cret",
		InsecureSkipVerify: true,
	}
	if err := TestLogin(cfg); err != nil {
		t.Fatalf("TestLogin failed against a working server: %v", err)
	}

	badCfg := cfg
	badCfg.Password = "wrong"
	if err := TestLogin(badCfg); err == nil {
		t.Fatal("expected TestLogin to fail honestly with wrong credentials")
	}
}

func TestSendMail_ImplicitTLS(t *testing.T) {
	cert := generateSelfSignedCert(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv := &fakeSMTPServer{ln: ln, username: "user@example.com", password: "s3cret"}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		srv.handleConnAlreadySecure(conn)
	}()

	host, port := srv.addr()
	cfg := SMTPConfig{
		Host:               host,
		Port:               port,
		Username:           srv.username,
		Password:           srv.password,
		ImplicitTLS:        true,
		InsecureSkipVerify: true,
	}

	err = SendMail(cfg, OutgoingMessage{
		From:     "sender@openlocalcrm.local",
		To:       []string{"kunde@example.com"},
		Subject:  "SMTPS Test",
		TextBody: "via SMTPS",
	})
	if err != nil {
		t.Fatalf("SendMail (implicit TLS) failed: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.authSeen {
		t.Error("expected AUTH to succeed over implicit TLS")
	}
}

// handleConnAlreadySecure serves a connection that is already TLS-wrapped
// (SMTPS), so EHLO immediately advertises AUTH without STARTTLS.
func (s *fakeSMTPServer) handleConnAlreadySecure(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	reader := bufio.NewReader(conn)
	send := func(msg string) { _, _ = conn.Write([]byte(msg + "\r\n")) }
	send("220 fake.smtps.test ESMTP ready")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			send("250-fake.smtps.test greets you")
			send("250 AUTH PLAIN")
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			fields := strings.Fields(line)
			var b64 string
			if len(fields) >= 3 {
				b64 = fields[2]
			}
			decoded, err := base64.StdEncoding.DecodeString(b64)
			ok := err == nil
			if ok {
				parts := strings.Split(string(decoded), "\x00")
				ok = len(parts) == 3 && parts[1] == s.username && parts[2] == s.password
			}
			s.mu.Lock()
			s.authSeen = ok
			s.mu.Unlock()
			if ok {
				send("235 Authentication successful")
			} else {
				send("535 Authentication failed")
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			send("250 OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			send("250 OK")
		case strings.HasPrefix(upper, "DATA"):
			send("354 End data with <CR><LF>.<CR><LF>")
			for {
				l, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" || l == ".\n" {
					break
				}
			}
			send("250 OK: queued")
		case strings.HasPrefix(upper, "QUIT"):
			send("221 Bye")
			return
		default:
			send("502 command not implemented")
		}
	}
}

// --- Fake IMAP server ---------------------------------------------------

func startFakeIMAPServer(t *testing.T, username, password string, cert tls.Certificate, fixtures [][]byte) (host string, port int) {
	t.Helper()

	memServer := imapmemserver.New()
	user := imapmemserver.NewUser(username, password)
	memServer.AddUser(user)
	if err := user.Create("INBOX", &imap.CreateOptions{}); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	for _, raw := range fixtures {
		if _, err := user.Append("INBOX", bytes.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatalf("append fixture message: %v", err)
		}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	imapSrv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return memServer.NewSession(), nil, nil
		},
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	})
	go func() { _ = imapSrv.Serve(ln) }()
	t.Cleanup(func() {
		_ = imapSrv.Close()
		_ = ln.Close()
	})

	tcpAddr := ln.Addr().(*net.TCPAddr)
	return tcpAddr.IP.String(), tcpAddr.Port
}

func fixtureMessage(messageID, from, subject, body string, date time.Time) []byte {
	return []byte(fmt.Sprintf(
		"From: %s\r\nTo: inbox@openlocalcrm.local\r\nSubject: %s\r\nDate: %s\r\nMessage-Id: <%s>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n",
		from, subject, date.Format(time.RFC1123Z), messageID, body,
	))
}

func TestFetchNewMessages_IncrementalUIDFetch(t *testing.T) {
	cert := generateSelfSignedCert(t)
	now := time.Now().UTC()
	fixtures := [][]byte{
		fixtureMessage("msg-1@example.com", "Kunde Eins <eins@example.com>", "Anfrage 1", "Erste Nachricht", now),
		fixtureMessage("msg-2@example.com", "Kunde Zwei <zwei@example.com>", "Anfrage 2", "Zweite Nachricht", now.Add(time.Minute)),
	}
	host, port := startFakeIMAPServer(t, "inbox@openlocalcrm.local", "s3cret", cert, fixtures)

	cfg := IMAPConfig{
		Host:               host,
		Port:               port,
		Username:           "inbox@openlocalcrm.local",
		Password:           "s3cret",
		InsecureSkipVerify: true,
	}

	result, err := FetchNewMessages(cfg, 0)
	if err != nil {
		t.Fatalf("FetchNewMessages failed: %v", err)
	}
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].SenderEmail != "eins@example.com" {
		t.Errorf("sender = %q, want eins@example.com", result.Messages[0].SenderEmail)
	}
	if result.Messages[0].Subject != "Anfrage 1" {
		t.Errorf("subject = %q, want Anfrage 1", result.Messages[0].Subject)
	}
	if !strings.Contains(result.Messages[0].TextBody, "Erste Nachricht") {
		t.Errorf("body = %q, want to contain Erste Nachricht", result.Messages[0].TextBody)
	}
	if result.LastUID != 2 {
		t.Errorf("LastUID = %d, want 2", result.LastUID)
	}

	// Incremental: fetching again since the last UID must return nothing new.
	again, err := FetchNewMessages(cfg, result.LastUID)
	if err != nil {
		t.Fatalf("second FetchNewMessages failed: %v", err)
	}
	if len(again.Messages) != 0 {
		t.Errorf("expected no new messages on second fetch, got %d", len(again.Messages))
	}
}

func TestFetchNewMessages_LoginFailure(t *testing.T) {
	cert := generateSelfSignedCert(t)
	host, port := startFakeIMAPServer(t, "inbox@openlocalcrm.local", "s3cret", cert, nil)

	cfg := IMAPConfig{
		Host:               host,
		Port:               port,
		Username:           "inbox@openlocalcrm.local",
		Password:           "wrong-password",
		InsecureSkipVerify: true,
	}

	_, err := FetchNewMessages(cfg, 0)
	if err == nil {
		t.Fatal("expected FetchNewMessages to fail with wrong credentials")
	}
}
