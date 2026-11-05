package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
)

var (
	// ErrInvalidOperationInput means the operation task request is malformed.
	ErrInvalidOperationInput = errors.New("invalid operation workflow input")
	// ErrUnsupportedOperation means the configured operation has no safe adapter path.
	ErrUnsupportedOperation = errors.New("unsupported operation workflow capability")
	// ErrInvalidProviderResult means a provider returned inconsistent facts.
	ErrInvalidProviderResult = errors.New("invalid provider activity result")
)

// SettlementInput asks the flow activity to enter a terminal state and settle
// its reservation in one database transaction.
type SettlementInput struct {
	OperationID      string        `json:"operation_id"`
	From             domain.Status `json:"from"`
	To               domain.Status `json:"to"`
	ActualCostMicros int64         `json:"actual_cost_micros"`
	FailureCode      string        `json:"failure_code,omitempty"`
	Retryable        bool          `json:"retryable"`
	Reason           string        `json:"reason,omitempty"`
}

// OperationFinalizer settles completed operations and their required business facts.
type OperationFinalizer interface {
	ResolveRevokedOperation(context.Context, string) error
	FinalizeOperation(context.Context, SettlementInput) error
	CompleteFromReuse(context.Context, string) error
	CancelQueuedBatchOperation(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}

func makeMockSubmitInput(loaded application.WorkflowOperation) (ProviderSubmitInput, error) {
	var params map[string]any
	if err := json.Unmarshal(loaded.Operation.Params, &params); err != nil {
		return ProviderSubmitInput{}, fmt.Errorf("decode frozen mock params: %w", err)
	}
	inputs := make([]ProviderInput, 0, len(loaded.Inputs))
	for _, item := range loaded.Inputs {
		if item.MediaAssetID != nil || item.MaskAssetID != nil || item.RefID != nil {
			return ProviderSubmitInput{}, ErrUnsupportedOperation // signed input URLs pending.
		}
		inputs = append(inputs, ProviderInput{Role: item.Role, Text: item.TextValue})
	}
	return ProviderSubmitInput{
		OperationID: loaded.Operation.ID.String(), ProviderRequestKey: loaded.ProviderRequestKey,
		AdapterKey: loaded.Provider.AdapterKey, ProviderModelID: loaded.Provider.ProviderModelID,
		Capability: loaded.Operation.Capability, Mode: loaded.Operation.Mode,
		Params: params, Inputs: inputs, OutputCount: int(loaded.Operation.OutputCount),
	}, nil
}

func mockModerationStatus(raw json.RawMessage) (mediadomain.ModerationStatus, error) {
	var params struct {
		Status mediadomain.ModerationStatus `json:"mock_moderation_status"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", ErrInvalidOperationInput
	}
	if params.Status != mediadomain.ModerationPassed && params.Status != mediadomain.ModerationRejected {
		return "", ErrInvalidOperationInput
	}
	return params.Status, nil
}

func providerFailureCode(providerErr *ProviderError) string {
	code := "failed"
	if providerErr != nil && strings.TrimSpace(providerErr.Code) != "" {
		code = strings.ToLower(strings.TrimSpace(providerErr.Code))
	}
	var normalized strings.Builder
	for _, char := range code {
		if normalized.Len() >= 64 {
			break
		}
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			normalized.WriteRune(char)
		} else {
			normalized.WriteByte('_')
		}
	}
	return "provider:" + normalized.String()
}
