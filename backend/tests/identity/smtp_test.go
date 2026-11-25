package identity_test

import (
	"encoding/base64"
	"io"
	"net/mail"
	"strings"
	"testing"

	"github.com/google/uuid"

	identitymail "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/mail"
)

func TestSMTPMessageAndConfiguration(t *testing.T) {
	cfg := identitymail.SMTPConfig{Host: "smtp.example.invalid", Port: 587, TLSMode: "starttls", Username: "user", Password: "test-only", FromEmail: "noreply@example.invalid", FromName: "织光", PublicOrigin: "https://glowloom.example.invalid"}
	if _, err := identitymail.NewSender(cfg); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*identitymail.SMTPConfig){
		func(c *identitymail.SMTPConfig) { c.TLSMode = "none" },
		func(c *identitymail.SMTPConfig) { c.FromEmail = "safe@example.invalid\r\nBcc: other@example.invalid" },
		func(c *identitymail.SMTPConfig) { c.Host = "" },
		func(c *identitymail.SMTPConfig) { c.PublicOrigin = "https://example.invalid/path?token=bad" },
	} {
		bad := cfg
		mutate(&bad)
		if _, err := identitymail.NewSender(bad); err == nil {
			t.Fatal("unsafe SMTP configuration accepted")
		}
	}
	sender, err := identitymail.NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	message, err := sender.Message(id, "password_reset", "creator@example.invalid", []byte(`{"token":"proof-token"}`))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(message)))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, parsed.Body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "/reset-password#token=proof-token") || strings.Contains(string(body), "?token=") {
		t.Fatal("email proof must remain in URL fragment")
	}
	if parsed.Header.Get("Message-Id") == "" {
		t.Fatal("SMTP message needs stable event identity")
	}
	if _, err := sender.Message(id, "registration_code", "victim@example.invalid\r\nBcc:other@example.invalid", []byte(`{"code":"123456"}`)); err == nil {
		t.Fatal("recipient header injection accepted")
	}
}
