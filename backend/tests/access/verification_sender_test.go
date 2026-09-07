package access_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/access/identity/adapter/verification"
)

func TestSMTPSenderReportsDeliveryOnlyAfterTransportAccepts(t *testing.T) {
	var delivered verification.Message
	sender := verification.NewSMTPSender(testSMTPConfig(), func(_ context.Context, message verification.Message) error {
		delivered = message
		return nil
	})

	sent, err := sender.Send(context.Background(), "creator@example.test", "123456")
	if err != nil {
		t.Fatal(err)
	}
	if !sent {
		t.Fatal("Send reported that an accepted SMTP delivery was not sent")
	}
	if delivered.To != "creator@example.test" || !strings.Contains(delivered.Body, "123456") ||
		!strings.Contains(delivered.Subject, "Lanverse") {
		t.Fatalf("unexpected verification message: %#v", delivered)
	}
}

func TestSMTPSenderDoesNotClaimDeliveryWhenDisabledOrRejected(t *testing.T) {
	transportCalls := 0
	disabledConfig := testSMTPConfig()
	disabledConfig.Enabled = false
	disabled := verification.NewSMTPSender(disabledConfig, func(context.Context, verification.Message) error {
		transportCalls++
		return nil
	})

	sent, err := disabled.Send(context.Background(), "creator@example.test", "123456")
	if err != nil || sent || transportCalls != 0 {
		t.Fatalf("disabled sender result = sent:%v err:%v calls:%d", sent, err, transportCalls)
	}

	rejected := verification.NewSMTPSender(testSMTPConfig(), func(context.Context, verification.Message) error {
		return errors.New("provider rejected credentials")
	})
	sent, err = rejected.Send(context.Background(), "creator@example.test", "123456")
	if err == nil || sent {
		t.Fatalf("rejected sender result = sent:%v err:%v", sent, err)
	}
}

func TestConfiguredSenderNeverClaimsEmailDelivery(t *testing.T) {
	sent, err := (verification.ConfiguredSender{}).Send(context.Background(), "creator@example.test", "123456")
	if err != nil || sent {
		t.Fatalf("fixed-code sender result = sent:%v err:%v", sent, err)
	}
}

func testSMTPConfig() verification.SMTPConfig {
	return verification.SMTPConfig{
		Enabled:   true,
		Host:      "smtp.example.test",
		Port:      465,
		TLSMode:   "tls",
		Username:  "sender@example.test",
		Password:  "application-password",
		FromEmail: "sender@example.test",
		FromName:  "Lanverse",
	}
}
