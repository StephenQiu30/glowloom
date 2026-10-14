package domain

import (
	"errors"
	"strings"
)

// ErrInvalidEmail means the address is outside the supported ASCII mailbox syntax.
var ErrInvalidEmail = errors.New("invalid email address")

// NormalizeEmail preserves dots and tags; aliases are never merged implicitly.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if len(email) > 254 || strings.Count(email, "@") != 1 {
		return "", ErrInvalidEmail
	}
	local, host, _ := strings.Cut(email, "@")
	if len(local) == 0 || len(local) > 64 || local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return "", ErrInvalidEmail
	}
	for _, c := range local {
		if !asciiAlphanumeric(c) && !strings.ContainsRune(".!#$%&'*+-/=?^_`{|}~", c) {
			return "", ErrInvalidEmail
		}
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrInvalidEmail
		}
		for _, c := range label {
			if !asciiAlphanumeric(c) && c != '-' {
				return "", ErrInvalidEmail
			}
		}
	}
	return email, nil
}

func asciiAlphanumeric(c rune) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
}
