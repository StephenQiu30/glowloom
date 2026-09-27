package application

// NewIdentityActionParser allows only the account-action summary fields
// declared by DES-12. Other action families must register a reviewed policy
// before their producers are enabled.
func NewIdentityActionParser() *Parser {
	return NewParser(identityActionFields())
}

// NewRecordedActionParser includes every action currently produced by the app.
func NewRecordedActionParser() *Parser {
	fields := identityActionFields()
	fields["credential.set"] = []string{"last4"}
	fields["credential.tested"] = []string{"last4"}
	fields["credential.disabled"] = []string{"last4"}
	fields["provider.created"] = []string{"key", "adapter_key", "region", "status", "concurrency_limit", "rate_limit_per_min"}
	fields["provider.updated"] = []string{"status", "concurrency_limit", "rate_limit_per_min"}
	fields["model.created"] = []string{"key", "provider_id", "capability", "display_name", "status"}
	fields["model.version_published"] = []string{"version_id", "version_no", "provider_model_id", "revision"}
	fields["price.published"] = []string{"version_id", "version_no", "unit", "currency", "effective_from", "revision"}
	return NewParser(fields)
}

func identityActionFields() map[string][]string {
	return map[string][]string{
		"auth.login_succeeded":  nil,
		"auth.login_failed":     {"reason"},
		"auth.locked":           {"retry_after_s"},
		"auth.logout":           nil,
		"user.created":          {"role", "status", "must_change_password"},
		"user.updated":          {"display_name", "role"},
		"user.disabled":         {"status"},
		"user.enabled":          {"status"},
		"user.password_reset":   {"must_change_password"},
		"user.password_changed": {"must_change_password"},
	}
}
