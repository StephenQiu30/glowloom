package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrVoiceExecutionUnknown fences any possibly charged incomplete execution.
	ErrVoiceExecutionUnknown = errors.New("subject voice execution unknown")
	// ErrVoiceProviderRejected is an explicit supplier rejection, never a retry signal.
	ErrVoiceProviderRejected = errors.New("subject voice provider rejected")
)

// VoiceSynthesisInput is private worker intent with a stable provider request ID.
type VoiceSynthesisInput struct {
	Config     json.RawMessage
	RequestID  uuid.UUID
	ResourceID string
	Text       string
	VoiceKey   string
	SpeechRate int
}

// VoiceSynthesisResult contains actual complete original bytes only after a
// successful terminal frame. It must be staged before database acknowledgement.
type VoiceSynthesisResult struct {
	Audio         []byte `json:"-"`
	MIMEType      string
	ProviderLogID string
	Usage         json.RawMessage
	TerminalCode  int
}

// VoiceSynthesizer is the worker's paid protocol boundary. Implementations must
// never retry a submission or put credentials into workflow payloads or errors.
type VoiceSynthesizer interface {
	Synthesize(context.Context, VoiceSynthesisInput, string) (VoiceSynthesisResult, error)
}
