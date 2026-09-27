package application

// NewIdentityActionParser allows only the account-action summary fields
// declared by DES-12. Other action families must register a reviewed policy
// before their producers are enabled.
func NewIdentityActionParser() *Parser {
	return NewParser(map[string][]string{
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
	})
}
