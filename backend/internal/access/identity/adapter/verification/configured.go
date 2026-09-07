package verification

import "context"

// ConfiguredSender supports a fixed verification code for local development or
// isolated end-to-end tests. It never reports an email delivery because it does
// not contact an external provider.
type ConfiguredSender struct{}

func (ConfiguredSender) Send(context.Context, string, string) (bool, error) {
	return false, nil
}
