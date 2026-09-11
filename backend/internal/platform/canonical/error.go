package canonical

import (
	"errors"
	"fmt"
)

const (
	errorInvalidJSON            = "invalid_json"
	errorDuplicateKey           = "duplicate_key"
	errorDuplicateNormalizedKey = "duplicate_normalized_key"
	errorInvalidUnicode         = "invalid_unicode"
	errorIntegerRequired        = "integer_required"
	errorIntegerOutOfRange      = "integer_out_of_range"
	errorNestingLimit           = "nesting_limit"
	errorUnsupportedValue       = "unsupported_value"
)

type contractError struct {
	code  string
	cause error
}

func (e *contractError) Error() string { return e.cause.Error() }
func (e *contractError) Unwrap() error { return e.cause }

// ErrorCode returns the stable cross-runtime failure code for a Production
// Canonical JSON error. Non-canonical errors deliberately return an empty code.
func ErrorCode(err error) string {
	var target *contractError
	if errors.As(err, &target) {
		return target.code
	}
	return ""
}

func canonicalError(code, message string) error {
	return &contractError{code: code, cause: errors.New(message)}
}

func wrapCanonicalError(code string, err error) error {
	if err == nil || ErrorCode(err) != "" {
		return err
	}
	return &contractError{code: code, cause: fmt.Errorf("Production Canonical JSON: %w", err)}
}
