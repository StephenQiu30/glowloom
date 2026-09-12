package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ReferenceCallDispatch struct {
	SubmissionToken        string    `json:"submission_token"`
	DispatchedBy           string    `json:"dispatched_by"`
	MembershipTokenVersion int       `json:"membership_token_version"`
	DispatchedAt           time.Time `json:"dispatched_at"`
	DeadlineAt             time.Time `json:"deadline_at"`
}

type ReferenceCallState struct {
	CallKey          string                 `json:"call_key"`
	Status           string                 `json:"status"`
	Revision         int64                  `json:"revision"`
	Dispatch         *ReferenceCallDispatch `json:"dispatch"`
	OutcomeUnknownAt *time.Time             `json:"outcome_unknown_at"`
	Receipt          *ReferenceCallReceipt  `json:"receipt,omitempty"`
	ContentHash      string                 `json:"content_hash"`
}

func NewReferenceCallState(callKey string) (ReferenceCallState, error) {
	return buildReferenceCallState(ReferenceCallState{CallKey: callKey, Status: ProviderCallPending, Revision: 1})
}

func buildReferenceCallState(value ReferenceCallState) (ReferenceCallState, error) {
	if !targetHashPattern.MatchString(value.CallKey) {
		return ReferenceCallState{}, errors.New("invalid Reference call state identity")
	}
	switch value.Status {
	case ProviderCallPending:
		if value.Revision != 1 || value.Dispatch != nil || value.OutcomeUnknownAt != nil || value.Receipt != nil {
			return ReferenceCallState{}, errors.New("invalid pending Reference call state")
		}
	case ProviderCallDispatching, ProviderCallOutcomeUnknown, ProviderCallSucceeded, ProviderCallFailed:
		if value.Dispatch == nil {
			return ReferenceCallState{}, errors.New("Reference call has no dispatch boundary")
		}
		dispatch := *value.Dispatch
		for _, id := range []string{dispatch.SubmissionToken, dispatch.DispatchedBy} {
			if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: value.CallKey}).Valid() {
				return ReferenceCallState{}, errors.New("invalid Reference call dispatch actor or token")
			}
		}
		if dispatch.MembershipTokenVersion < 1 || dispatch.DispatchedAt.IsZero() || !dispatch.DeadlineAt.After(dispatch.DispatchedAt) {
			return ReferenceCallState{}, errors.New("invalid Reference call dispatch deadline")
		}
		dispatch.DispatchedAt = dispatch.DispatchedAt.UTC().Truncate(time.Microsecond)
		dispatch.DeadlineAt = dispatch.DeadlineAt.UTC().Truncate(time.Microsecond)
		if !dispatch.DeadlineAt.After(dispatch.DispatchedAt) {
			return ReferenceCallState{}, errors.New("Reference dispatch deadline has insufficient precision")
		}
		value.Dispatch = &dispatch
		if value.Status == ProviderCallDispatching {
			if value.Revision != 2 || value.OutcomeUnknownAt != nil || value.Receipt != nil {
				return ReferenceCallState{}, errors.New("invalid dispatching Reference call state")
			}
		} else if value.Receipt == nil {
			if value.Status != ProviderCallOutcomeUnknown || value.Revision != 3 || value.OutcomeUnknownAt == nil || value.OutcomeUnknownAt.Before(dispatch.DeadlineAt) {
				return ReferenceCallState{}, errors.New("invalid outcome-unknown Reference call state")
			}
			observed := value.OutcomeUnknownAt.UTC().Truncate(time.Microsecond)
			value.OutcomeUnknownAt = &observed
		} else {
			receipt, err := BuildReferenceCallReceipt(value.Receipt.ReferenceCallReceiptInput)
			if err != nil || !reflect.DeepEqual(receipt, *value.Receipt) || receipt.Call.CallKey != value.CallKey || receipt.SubmissionToken != dispatch.SubmissionToken || receipt.ObservedAt.Before(dispatch.DispatchedAt) || value.Revision < 3 || value.Revision > 4 {
				return ReferenceCallState{}, errors.New("Reference receipt differs from call state")
			}
			expectedStatus := ProviderCallFailed
			switch receipt.Disposition {
			case "staged":
				expectedStatus = ProviderCallSucceeded
				if receipt.ObservedAt.After(dispatch.DeadlineAt) {
					return ReferenceCallState{}, errors.New("Reference staged observation exceeds dispatch deadline")
				}
			case "outcome_unknown":
				expectedStatus = ProviderCallOutcomeUnknown
			}
			if value.Status != expectedStatus || (expectedStatus != ProviderCallOutcomeUnknown && value.OutcomeUnknownAt != nil) || (expectedStatus == ProviderCallOutcomeUnknown && (value.OutcomeUnknownAt == nil || !value.OutcomeUnknownAt.Equal(receipt.ObservedAt))) {
				return ReferenceCallState{}, errors.New("Reference receipt disposition differs from state")
			}
			if expectedStatus == ProviderCallOutcomeUnknown {
				observed := receipt.ObservedAt
				value.OutcomeUnknownAt = &observed
			}
			value.Receipt = &receipt
		}
	default:
		return ReferenceCallState{}, errors.New("unsupported Reference call state")
	}
	value.ContentHash = ""
	raw, err := json.Marshal(value)
	if err != nil {
		return ReferenceCallState{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceCallState{}, err
	}
	return value, nil
}

func validateReferenceCallState(value ReferenceCallState) error {
	expected, err := buildReferenceCallState(value)
	if err != nil || !reflect.DeepEqual(expected, value) {
		return errors.New("Reference call state has drifted")
	}
	return nil
}

func DecodeReferenceCallState(raw json.RawMessage) (ReferenceCallState, error) {
	var value ReferenceCallState
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceCallState{}, err
	}
	if err := validateReferenceCallState(value); err != nil {
		return ReferenceCallState{}, err
	}
	return value, nil
}

// ClaimReferenceCall proposes a transition. Only the committing application
// transaction may turn its boolean into an actual send right.
func ClaimReferenceCall(value ReferenceCallState, dispatch ReferenceCallDispatch) (ReferenceCallState, bool, error) {
	if err := validateReferenceCallState(value); err != nil {
		return ReferenceCallState{}, false, err
	}
	if value.Status != ProviderCallPending {
		return value, false, nil
	}
	value.Status, value.Revision, value.Dispatch = ProviderCallDispatching, 2, &dispatch
	claimed, err := buildReferenceCallState(value)
	if err != nil {
		return ReferenceCallState{}, false, err
	}
	return claimed, true, nil
}

func ExpireReferenceCall(value ReferenceCallState, now time.Time) (ReferenceCallState, bool, error) {
	if err := validateReferenceCallState(value); err != nil {
		return ReferenceCallState{}, false, err
	}
	if value.Status == ProviderCallPending || now.IsZero() {
		return ReferenceCallState{}, false, errors.New("Reference call has not crossed the send boundary")
	}
	if value.Status == ProviderCallOutcomeUnknown || value.Receipt != nil {
		return value, false, nil
	}
	if now.Before(value.Dispatch.DispatchedAt) {
		return ReferenceCallState{}, false, errors.New("Reference call observation predates dispatch")
	}
	if now.Before(value.Dispatch.DeadlineAt) {
		return value, false, nil
	}
	value.Status, value.Revision, value.OutcomeUnknownAt = ProviderCallOutcomeUnknown, 3, &now
	expired, err := buildReferenceCallState(value)
	if err != nil {
		return ReferenceCallState{}, false, err
	}
	return expired, true, nil
}

func ValidateReferenceCallTransition(before, after ReferenceCallState) error {
	var expected ReferenceCallState
	var changed bool
	var err error
	switch {
	case after.Receipt != nil:
		expected, changed, err = RecordReferenceCallReceipt(before, *after.Receipt)
	case before.Status == ProviderCallPending && after.Dispatch != nil:
		expected, changed, err = ClaimReferenceCall(before, *after.Dispatch)
	case before.Status == ProviderCallDispatching && after.OutcomeUnknownAt != nil:
		expected, changed, err = ExpireReferenceCall(before, *after.OutcomeUnknownAt)
	default:
		return errors.New("invalid Reference call state transition")
	}
	if err != nil || !changed || !reflect.DeepEqual(expected, after) {
		return errors.New("Reference call state transition has drifted")
	}
	return nil
}
