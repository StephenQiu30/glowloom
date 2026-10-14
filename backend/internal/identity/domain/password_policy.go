package domain

import "unicode/utf8"

// ValidateNewPassword checks length without trimming, truncating, or changing it.
// Blocking known compromised passwords is an additional application dependency.
func ValidateNewPassword(raw string) error {
	length := utf8.RuneCountInString(raw)
	if !utf8.ValidString(raw) || length < 15 || length > 128 {
		return ErrWeakPassword
	}
	return nil
}
