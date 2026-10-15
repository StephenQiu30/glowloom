// Package mail submits identity notifications through the existing SMTP service.
package mail

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/domain"
)

// ErrSMTPTransport hides transport details that may contain credentials.
var ErrSMTPTransport = errors.New("SMTP submission failed")

// SMTPConfig contains server-side delivery settings and the public recovery origin.
type SMTPConfig struct {
	Host                                                           string
	Port                                                           int
	TLSMode, Username, Password, FromEmail, FromName, PublicOrigin string
}

// Sender submits identity messages over authenticated encrypted SMTP.
type Sender struct{ config SMTPConfig }

// NewSender constructs Sender with explicit caller-supplied dependencies.
func NewSender(cfg SMTPConfig) (*Sender, error) {
	from, err := domain.NormalizeEmail(cfg.FromEmail)
	if err != nil || cfg.Host == "" || strings.ContainsAny(cfg.Host, "/\\\r\n \t") || cfg.Port < 1 || cfg.Port > 65535 || (cfg.TLSMode != "tls" && cfg.TLSMode != "starttls") || (cfg.Username == "") != (cfg.Password == "") {
		return nil, errors.New("invalid SMTP configuration")
	}
	for _, r := range cfg.FromName {
		if unicode.IsControl(r) {
			return nil, errors.New("invalid SMTP sender name")
		}
	}
	origin, err := url.Parse(cfg.PublicOrigin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return nil, errors.New("invalid identity mail origin")
	}
	localHTTP := origin.Scheme == "http" && (origin.Hostname() == "localhost" || net.ParseIP(origin.Hostname()).IsLoopback())
	if origin.Scheme != "https" && !localHTTP {
		return nil, errors.New("invalid identity mail origin")
	}
	cfg.FromEmail = from
	cfg.PublicOrigin = strings.TrimRight(cfg.PublicOrigin, "/")
	return &Sender{config: cfg}, nil
}

// Message renders only known templates; links carry proofs in the fragment.
func (s *Sender) Message(event uuid.UUID, template, recipient string, payload []byte) ([]byte, error) {
	to, err := domain.NormalizeEmail(recipient)
	if err != nil {
		return nil, err
	}
	var data struct{ Code, Token string }
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, errors.New("invalid identity mail payload")
	}
	var subject, body string
	switch template {
	case "registration_code", "migration_bind", "email_old", "email_new":
		if len(data.Code) != 6 || strings.Trim(data.Code, "0123456789") != "" {
			return nil, errors.New("invalid verification code")
		}
		subject = "织光 · 验证邮箱"
		body = "你的邮箱验证码是：" + data.Code + "\n\n验证码 10 分钟内有效，请勿向他人透露。若非本人操作，请忽略此邮件。"
	case "deletion_confirm":
		if len(data.Code) != 6 || strings.Trim(data.Code, "0123456789") != "" {
			return nil, errors.New("invalid verification code")
		}
		subject = "织光 · 确认注销账号"
		body = "你的注销验证码是：" + data.Code + "\n\n验证码 10 分钟内有效。完成确认后账号将停止登录和访问，资料与作品保留。请勿向他人透露验证码；若非本人操作，请忽略此邮件。"
	case "password_reset":
		if data.Token == "" || len(data.Token) > 512 {
			return nil, errors.New("invalid recovery token")
		}
		subject = "织光 · 重置密码"
		body = "请打开以下链接设置新密码，链接 30 分钟内有效：\n\n" + s.config.PublicOrigin + "/reset-password#token=" + url.QueryEscape(data.Token) + "\n\n若非本人操作，请忽略此邮件。"
	case "team_governance", "account_governance":
		subject = "织光 · 账号安全状态已更新"
		body = "你的织光账号或团队管理权限已更新，请登录账号查看当前状态。若非预期操作，请联系平台管理员。"
	case "authority_proposal":
		subject = "织光 · 待确认的管理职责"
		body = "你收到一项管理职责提案。请登录织光，在账号设置中查看并明确接受或拒绝；接受前不会变更权限。提案 48 小时内有效。"
	case "account_invitation", "team_invitation":
		if data.Token == "" || len(data.Token) > 512 {
			return nil, errors.New("invalid invitation token")
		}
		subject = "织光 · 创作邀请"
		body = "你收到一项创作邀请。请使用本邮箱登录后查看并主动接受，邀请 48 小时内有效：\n\n" + s.config.PublicOrigin + "/invitations#token=" + url.QueryEscape(data.Token)
	case "email_changed":
		subject = "织光 · 登录邮箱已更新"
		body = "你的织光账号登录邮箱已更新，原有登录会话已失效。请使用新邮箱重新登录；若非本人操作，请联系平台管理员。"
	case "password_changed":
		subject = "织光 · 密码已更新"
		body = "你的织光账号密码已更新，原有登录会话已失效。若非本人操作，请通过登录页找回密码。"
	default:
		return nil, errors.New("unsupported identity mail template")
	}
	from := (&stdmail.Address{Name: s.config.FromName, Address: s.config.FromEmail}).String()
	_, host, _ := strings.Cut(s.config.FromEmail, "@")
	messageID := "<" + event.String() + "@" + host + ">"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var out strings.Builder
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMessage-ID: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n", from, to, mime.QEncoding.Encode("UTF-8", subject), messageID, time.Now().UTC().Format(time.RFC1123Z))
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		out.WriteString(encoded[:n])
		out.WriteString("\r\n")
		encoded = encoded[n:]
	}
	return []byte(out.String()), nil
}

// Send never downgrades encryption. A DATA acknowledgement means submitted only.
func (s *Sender) Send(ctx context.Context, event uuid.UUID, template, recipient string, payload []byte) (string, error) {
	message, err := s.Message(event, template, recipient, payload)
	if err != nil {
		return "", err
	}
	to, err := domain.NormalizeEmail(recipient)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	tlsConfig := &tls.Config{ServerName: s.config.Host, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	if s.config.TLSMode == "tls" {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return "", ErrSMTPTransport
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return "", ErrSMTPTransport
	}
	client, err := smtp.NewClient(conn, s.config.Host)
	if err != nil {
		return "", ErrSMTPTransport
	}
	defer func() { _ = client.Close() }()
	if s.config.TLSMode == "starttls" {
		if err := client.StartTLS(tlsConfig); err != nil {
			return "", ErrSMTPTransport
		}
	}
	if s.config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)); err != nil {
			return "", ErrSMTPTransport
		}
	}
	if err := client.Mail(s.config.FromEmail); err != nil {
		return "", ErrSMTPTransport
	}
	if err := client.Rcpt(to); err != nil {
		return "", ErrSMTPTransport
	}
	writer, err := client.Data()
	if err != nil {
		return "", ErrSMTPTransport
	}
	if _, err := writer.Write(message); err != nil {
		return "", application.ErrMailOutcomeUnknown
	}
	if err := writer.Close(); err != nil {
		return "", application.ErrMailOutcomeUnknown
	}
	// QUIT failure after the successful DATA response cannot undo acceptance.
	_ = client.Quit()
	return event.String(), nil
}
