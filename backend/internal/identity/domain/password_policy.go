package domain

import "unicode/utf8"

const (
	// MinPasswordLength is the minimum number of Unicode code points for a new password.
	MinPasswordLength = 8
	// MaxPasswordLength bounds new passwords without truncation.
	MaxPasswordLength = 128
)

// ValidateNewPassword checks length without trimming, truncating, or changing it.
// Blocking known compromised passwords is an additional application dependency.
func ValidateNewPassword(raw string) error {
	length := utf8.RuneCountInString(raw)
	if !utf8.ValidString(raw) || length < MinPasswordLength || length > MaxPasswordLength {
		return ErrWeakPassword
	}
	return nil
}
