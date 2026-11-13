package application

import (
	"context"
	"encoding/json"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ProviderOutcome is the normalized result of one provider call.
type ProviderOutcome string

// ProviderTaskState is the normalized lifecycle of a remote task.
type ProviderTaskState string

// Provider outcomes distinguish accepted requests from classified failures.
const (
	ProviderOutcomeAccepted     ProviderOutcome = "accepted"
	ProviderOutcomeCompleted    ProviderOutcome = "completed"
	ProviderOutcomeRejected     ProviderOutcome = "rejected"
	ProviderOutcomeNotSubmitted ProviderOutcome = "not_submitted"
	ProviderOutcomeUnknown      ProviderOutcome = "unknown"
	ProviderOutcomeCancelled    ProviderOutcome = "cancelled"
	ProviderOutcomeNotCancelled ProviderOutcome = "not_cancelled"
)

// Provider task states record queued, running and terminal remote progress.
const (
	ProviderTaskQueued    ProviderTaskState = "queued"
	ProviderTaskPending   ProviderTaskState = "pending"
	ProviderTaskRunning   ProviderTaskState = "running"
	ProviderTaskSucceeded ProviderTaskState = "succeeded"
	ProviderTaskFailed    ProviderTaskState = "failed"
	ProviderTaskCancelled ProviderTaskState = "cancelled"
	ProviderTaskExpired   ProviderTaskState = "expired"
	ProviderTaskNotFound  ProviderTaskState = "not_found"
	ProviderTaskUnknown   ProviderTaskState = "unknown"
)

// ProviderReference keeps authorized inputs inside the credential-owning worker.
type ProviderReference struct {
	Role      string
	URL       string
	MediaKind string `json:"media_kind,omitempty"`
	// ImageBytes is a bounded, digest-verified alternative for local object URLs.
	// It must never enter workflow history or other serialized task metadata.
	ImageBytes []byte `json:"-"`
}

// GenerationRequest carries validated inputs and immutable provider settings.
type GenerationRequest struct {
	// StrictSpeechTerminal selects the official SSE endpoint and requires an explicit terminal frame.
	StrictSpeechTerminal bool `json:"strict_speech_terminal,omitempty"`
	// RequireSpeechTerminalUsage is the character_count_v1 receipt contract.
	RequireSpeechTerminalUsage bool   `json:"require_speech_terminal_usage,omitempty"`
	ProviderRequestID          string `json:"provider_request_id,omitempty"`
	Config                     catalogdomain.ProviderConfig
	ModelID                    string
	Capability                 string
	Mode                       string
	Prompt                     string
	Params                     map[string]any
	References                 []ProviderReference
	OutputCount                int
	EndpointTag                string
}

// GeneratedImage stays inside the worker; bytes never enter workflow history.
type GeneratedImage struct {
	Bytes     []byte
	MediaType string
}

// GeneratedAudio stays in private worker storage, outside workflow history.
type GeneratedAudio struct {
	Bytes     []byte
	MediaType string
}

// GenerationResult distinguishes transport ambiguity from confirmed rejection.
type GenerationResult struct {
	HTTPStatus           int            `json:"http_status,omitempty"`
	Subtitle             *AudioSubtitle `json:"subtitle,omitempty"`
	ProviderTerminalCode int            `json:"provider_terminal_code,omitempty"`
	ProviderLogID        string         `json:"provider_log_id,omitempty"`
	Outcome              ProviderOutcome
	TaskID               string
	State                ProviderTaskState
	Text                 string
	Images               []GeneratedImage
	Audio                []GeneratedAudio
	URLs                 []string
	Usage                json.RawMessage
	ErrorCode            string
}

// GenerationProvider performs exactly one provider request per call. The caller
// owns durable dispatch admission, credential loading and output persistence.
type GenerationProvider interface {
	Submit(context.Context, GenerationRequest, string) (GenerationResult, error)
	Query(context.Context, catalogdomain.ProviderConfig, string, string) (GenerationResult, error)
	Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (GenerationResult, error)
}

// DialNotSent records a failed TCP dial before any connection was obtained.
const DialNotSent = "dial_not_sent"
