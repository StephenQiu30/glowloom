package verification

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/mail"
	stdsmtp "net/smtp"
	"strings"
	"time"
)

const smtpOperationTimeout = 10 * time.Second

var errDeliveryUnavailable = errors.New("verification email delivery unavailable")

// SMTPConfig contains the connection and sender identity used for registration emails.
type SMTPConfig struct {
	Enabled   bool
	Host      string
	Port      int
	TLSMode   string
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

// Message is the bounded email payload handed to the SMTP transport.
type Message struct {
	From    string
	To      string
	Subject string
	Body    string
}

// SendFunc is an SMTP transport seam used by the sender and its tests.
type SendFunc func(context.Context, Message) error

// SMTPSender delivers registration verification codes through SMTP.
type SMTPSender struct {
	config SMTPConfig
	send   SendFunc
}

// NewSMTPSender creates an SMTP-backed verification sender.
func NewSMTPSender(config SMTPConfig, send ...SendFunc) *SMTPSender {
	sender := &SMTPSender{config: config}
	if len(send) > 0 && send[0] != nil {
		sender.send = send[0]
	} else {
		sender.send = sender.sendSMTP
	}
	return sender
}

func (sender *SMTPSender) Send(ctx context.Context, email, code string) (bool, error) {
	if sender == nil || !sender.config.Enabled {
		return false, nil
	}
	if !sender.config.valid() || !validRecipient(email) || len(code) != 6 {
		return false, errDeliveryUnavailable
	}
	message := Message{
		From:    (&mail.Address{Name: sender.config.FromName, Address: sender.config.FromEmail}).String(),
		To:      email,
		Subject: mime.QEncoding.Encode("UTF-8", "Lanverse 注册验证码"),
		Body:    fmt.Sprintf("你的 Lanverse 注册验证码是：%s\r\n\r\n验证码将在 10 分钟后失效。如非本人操作，请忽略此邮件。\r\n", code),
	}
	if err := sender.send(ctx, message); err != nil {
		return false, errDeliveryUnavailable
	}
	return true, nil
}

func (sender *SMTPSender) sendSMTP(ctx context.Context, message Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	address := net.JoinHostPort(sender.config.Host, fmt.Sprintf("%d", sender.config.Port))
	dialer := &net.Dialer{Timeout: smtpOperationTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	defer func() { _ = connection.Close() }()
	deadline := time.Now().Add(smtpOperationTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return err
	}
	if sender.config.TLSMode == "tls" {
		tlsConnection := tls.Client(connection, sender.tlsConfig())
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return err
		}
		connection = tlsConnection
	}

	client, err := stdsmtp.NewClient(connection, sender.config.Host)
	if err != nil {
		return err
	}
	defer func() { _ = client.Quit() }()
	if sender.config.TLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP STARTTLS unavailable")
		}
		if err := client.StartTLS(sender.tlsConfig()); err != nil {
			return err
		}
	}
	if sender.config.Username != "" {
		if err := client.Auth(stdsmtp.PlainAuth("", sender.config.Username, sender.config.Password, sender.config.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(sender.config.FromEmail); err != nil {
		return err
	}
	if err := client.Rcpt(message.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	payload := strings.Join([]string{
		"From: " + message.From,
		"To: " + (&mail.Address{Address: message.To}).String(),
		"Subject: " + message.Subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		message.Body,
	}, "\r\n")
	if _, err := io.WriteString(writer, payload); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func (sender *SMTPSender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: sender.config.Host, MinVersion: tls.VersionTLS12}
}

func (config SMTPConfig) valid() bool {
	if strings.TrimSpace(config.Host) == "" || config.Port < 1 || config.Port > 65535 ||
		strings.TrimSpace(config.FromEmail) == "" {
		return false
	}
	return config.TLSMode == "tls" || config.TLSMode == "starttls"
}

func validRecipient(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Name == "" && address.Address == value
}
