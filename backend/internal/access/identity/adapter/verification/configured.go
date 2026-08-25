package verification

import "context"

// ConfiguredSender is only enabled when the runtime explicitly supplies a
// verification code (for local development or isolated end-to-end tests).
// Production delivery is intentionally reported as unavailable until an SMTP
// adapter is configured; the API never claims that an email was sent when it was not.
type ConfiguredSender struct{ Enabled bool }

func (sender ConfiguredSender) Send(context.Context, string, string) (bool, error) {
	return sender.Enabled, nil
}
