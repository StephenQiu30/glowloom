package application

import "errors"

// Catalog errors are declared at the application boundary for HTTP consumers.
var (
	ErrProviderKeyExists      = errors.New("provider key already exists")
	ErrProviderNotFound       = errors.New("provider not found")
	ErrProviderUnavailable    = errors.New("provider unavailable")
	ErrCredentialNotFound     = errors.New("active credential not found")
	ErrModelKeyExists         = errors.New("model key already exists")
	ErrModelNotFound          = errors.New("model not found")
	ErrModelSourceUnavailable = errors.New("model provider or capability unavailable")
	ErrModelVersionConflict   = errors.New("model version conflict")
	ErrPriceVersionConflict   = errors.New("price version conflict")
)
