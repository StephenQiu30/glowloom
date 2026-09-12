package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ReferenceCallReceiptInput struct {
	WorkspaceID     string                   `json:"workspace_id"`
	ProjectID       string                   `json:"project_id"`
	Call            ReferenceProviderCall    `json:"call"`
	SubmissionToken string                   `json:"submission_token"`
	Slot            ReferenceOutputSlot      `json:"slot"`
	ObservedAt      time.Time                `json:"observed_at"`
	Disposition     string                   `json:"disposition"`
	ReasonCode      string                   `json:"reason_code"`
	Output          *ProviderOutput          `json:"output,omitempty"`
	Usage           ProviderUsageObservation `json:"usage"`
}

// ReferenceCallReceipt is the immutable observation of one synchronous Submit.
// An outcome_unknown receipt is not a remote terminal result or permission to retry.
type ReferenceCallReceipt struct {
	ContractID string `json:"contract_id"`
	ReferenceCallReceiptInput
	ContentHash string `json:"content_hash"`
}

func BuildReferenceCallReceipt(input ReferenceCallReceiptInput) (ReferenceCallReceipt, error) {
	for _, id := range []string{input.WorkspaceID, input.ProjectID, input.SubmissionToken} {
		if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: input.Call.CallKey}).Valid() {
			return ReferenceCallReceipt{}, errors.New("invalid Reference receipt scope or token")
		}
	}
	raw, err := json.Marshal(input.Call)
	if err != nil {
		return ReferenceCallReceipt{}, err
	}
	if _, err = DecodeReferenceProviderCall(raw); err != nil {
		return ReferenceCallReceipt{}, err
	}
	if input.Slot.SlotKey != input.Call.SlotKey || input.Slot.ViewRole != input.Call.SlotKey || !input.Slot.Required || input.ObservedAt.IsZero() {
		return ReferenceCallReceipt{}, errors.New("invalid Reference receipt slot or time")
	}
	input.Slot, err = normalizeReferenceOutputSlot(input.Slot)
	if err != nil {
		return ReferenceCallReceipt{}, err
	}
	input.ObservedAt = input.ObservedAt.UTC().Truncate(time.Microsecond)
	if input.Disposition == "staged" {
		if input.Output == nil || input.ReasonCode != "" {
			return ReferenceCallReceipt{}, errors.New("Reference staged receipt needs one output")
		}
		output := *input.Output
		expectedKey := "staging/reference/" + input.WorkspaceID + "/" + input.ProjectID + "/" + input.Call.ExecutionRef.ID + "/" + input.Call.CallKey + "/" + input.SubmissionToken + "/image.png"
		if output.OutputKey != "image" || output.StagingObjectKey != expectedKey || !targetHashPattern.MatchString(output.SHA256) || output.MediaType != "image/png" || !slices.Contains(input.Slot.AllowedMediaTypes, output.MediaType) || output.Width != input.Slot.MinWidth || output.Height != input.Slot.MinHeight || output.Bytes < 1 || output.Bytes > input.Slot.MaxBytes || input.Usage.ImageCount != 1 || input.Usage.InputTokens < 0 || input.Usage.OutputTokens < 0 || input.Usage.TotalTokens < 0 || input.Usage.VideoDurationMS != 0 {
			return ReferenceCallReceipt{}, errors.New("Reference staged evidence differs from output contract")
		}
		input.Output = &output
	} else {
		if input.Output != nil || input.Usage != (ProviderUsageObservation{}) {
			return ReferenceCallReceipt{}, errors.New("non-success Reference receipt has output")
		}
		var reasons []string
		switch input.Disposition {
		case "output_rejected":
			reasons = []string{"response_byte_limit", "invalid_response_json", "provider_output_cardinality_mismatch", "invalid_image_encoding", "image_dimensions_mismatch", "invalid_png_contents", "invalid_usage_observation"}
		case "outcome_unknown":
			reasons = []string{"transport_failed", "unexpected_http_status", "response_read_failed", "dispatch_deadline_reached", "staging_failed"}
		case "not_sent":
			reasons = []string{"submit_not_attempted"}
		default:
			return ReferenceCallReceipt{}, errors.New("unsupported Reference receipt disposition")
		}
		if !slices.Contains(reasons, input.ReasonCode) {
			return ReferenceCallReceipt{}, errors.New("unsupported Reference receipt reason")
		}
	}
	value := ReferenceCallReceipt{ContractID: "reference-call-submit-receipt", ReferenceCallReceiptInput: input}
	raw, err = json.Marshal(value)
	if err != nil {
		return ReferenceCallReceipt{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	return value, err
}

func DecodeReferenceCallReceipt(raw json.RawMessage) (ReferenceCallReceipt, error) {
	var value ReferenceCallReceipt
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceCallReceipt{}, err
	}
	expected, err := BuildReferenceCallReceipt(value.ReferenceCallReceiptInput)
	if err != nil || !reflect.DeepEqual(value, expected) {
		return ReferenceCallReceipt{}, errors.New("Reference receipt content has drifted")
	}
	return value, nil
}

func RecordReferenceCallReceipt(before ReferenceCallState, receipt ReferenceCallReceipt) (ReferenceCallState, bool, error) {
	if err := validateReferenceCallState(before); err != nil {
		return ReferenceCallState{}, false, err
	}
	expected, err := BuildReferenceCallReceipt(receipt.ReferenceCallReceiptInput)
	if err != nil || !reflect.DeepEqual(expected, receipt) {
		return ReferenceCallState{}, false, errors.New("Reference receipt content has drifted")
	}
	if before.Dispatch == nil || before.CallKey != receipt.Call.CallKey || before.Dispatch.SubmissionToken != receipt.SubmissionToken {
		return ReferenceCallState{}, false, errors.New("Reference receipt differs from dispatch identity")
	}
	if before.Receipt != nil {
		if !reflect.DeepEqual(*before.Receipt, receipt) {
			return ReferenceCallState{}, false, errors.New("Reference receipt cannot be replaced")
		}
		return before, false, nil
	}
	if before.Status != ProviderCallDispatching && before.Status != ProviderCallOutcomeUnknown {
		return ReferenceCallState{}, false, errors.New("Reference call cannot accept receipt")
	}
	before.Receipt, before.Revision, before.OutcomeUnknownAt = &expected, before.Revision+1, nil
	switch receipt.Disposition {
	case "staged":
		before.Status = ProviderCallSucceeded
	case "outcome_unknown":
		before.Status, before.OutcomeUnknownAt = ProviderCallOutcomeUnknown, &expected.ObservedAt
	default:
		before.Status = ProviderCallFailed
	}
	after, err := buildReferenceCallState(before)
	if err != nil {
		return ReferenceCallState{}, false, err
	}
	return after, true, nil
}
