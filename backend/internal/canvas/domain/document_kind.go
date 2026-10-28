package domain

// ParseDocumentKind admits only explicit creation-container classifications.
// Empty legacy values retain the ordinary canvas classification.
func ParseDocumentKind(value string) (string, error) {
	switch value {
	case "", "canvas":
		return "canvas", nil
	case "short_drama":
		return value, nil
	default:
		return "", ErrInvalidCommand
	}
}
