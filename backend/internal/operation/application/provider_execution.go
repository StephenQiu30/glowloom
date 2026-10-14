package application

import (
	"context"
	"encoding/json"

	catalogdomain "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

// ProviderReference is an already-authorized temporary input URL.
type ProviderReference struct {
	Role string
	URL  string
}

// GenerationRequest carries validated inputs and immutable provider settings.
type GenerationRequest struct {
	Config      catalogdomain.ProviderConfig
	ModelID     string
	Capability  string
	Mode        string
	Prompt      string
	Params      map[string]any
	References  []ProviderReference
	OutputCount int
	EndpointTag string
}

// GeneratedImage stays inside the worker; bytes never enter workflow history.
type GeneratedImage struct {
	Bytes     []byte
	MediaType string
}

// GenerationResult distinguishes transport ambiguity from confirmed rejection.
type GenerationResult struct {
	Outcome   string
	TaskID    string
	State     string
	Text      string
	Images    []GeneratedImage
	URLs      []string
	Usage     json.RawMessage
	ErrorCode string
}

// GenerationProvider performs exactly one provider request per call. The caller
// owns durable dispatch admission, credential loading and output persistence.
type GenerationProvider interface {
	Submit(context.Context, GenerationRequest, string) (GenerationResult, error)
	Query(context.Context, catalogdomain.ProviderConfig, string, string) (GenerationResult, error)
	Cancel(context.Context, catalogdomain.ProviderConfig, string, string) (GenerationResult, error)
}
