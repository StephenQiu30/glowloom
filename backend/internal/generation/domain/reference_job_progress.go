package domain

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// ReferenceJobProgress is a read projection, never a dispatch or selection right.
type ReferenceJobProgress struct {
	ExecutionRef   GenerationRevisionRef   `json:"execution_ref"`
	JobHash        string                  `json:"job_hash"`
	CallSetRoot    string                  `json:"call_set_root"`
	Status         string                  `json:"status"`
	Terminal       bool                    `json:"terminal"`
	Total          int                     `json:"total"`
	Pending        int                     `json:"pending"`
	Dispatching    int                     `json:"dispatching"`
	Succeeded      int                     `json:"succeeded"`
	Failed         int                     `json:"failed"`
	OutcomeUnknown int                     `json:"outcome_unknown"`
	Calls          []ReferenceCallProgress `json:"calls"`
	ContentHash    string                  `json:"content_hash"`
}

type ReferenceCallProgress struct {
	CallKey     string `json:"call_key"`
	BundleIndex int    `json:"bundle_index"`
	SlotKey     string `json:"slot_key"`
	Status      string `json:"status"`
	Revision    int64  `json:"revision"`
	StateHash   string `json:"state_hash"`
}

func BuildReferenceJobProgress(job ReferenceProviderJob, calls []ReferenceProviderCall, states []ReferenceCallState) (ReferenceJobProgress, error) {
	if len(calls) != len(job.CallKeys) || len(states) != len(calls) {
		return ReferenceJobProgress{}, errors.New("Reference job progress requires the full frozen call set")
	}
	inputs := make([]ReferenceProviderCallInput, len(calls))
	byKey := make(map[string]ReferenceProviderCall, len(calls))
	for i, call := range calls {
		inputs[i] = call.ReferenceProviderCallInput
		byKey[call.CallKey] = call
	}
	expected, ordered, err := BuildReferenceProviderJob(job.ExecutionRef, inputs)
	if err != nil || !reflect.DeepEqual(expected, job) || len(byKey) != len(calls) {
		return ReferenceJobProgress{}, errors.New("Reference job progress membership has drifted")
	}
	stateByKey := make(map[string]ReferenceCallState, len(states))
	for _, state := range states {
		call, exists := byKey[state.CallKey]
		if !exists || validateReferenceCallState(state) != nil || (state.Receipt != nil && state.Receipt.Call != call) {
			return ReferenceJobProgress{}, errors.New("Reference job progress call state has drifted")
		}
		if _, duplicate := stateByKey[state.CallKey]; duplicate {
			return ReferenceJobProgress{}, errors.New("Reference job progress contains duplicate states")
		}
		stateByKey[state.CallKey] = state
	}
	result := ReferenceJobProgress{ExecutionRef: job.ExecutionRef, JobHash: job.ContentHash, CallSetRoot: job.CallSetRoot, Total: len(calls), Calls: make([]ReferenceCallProgress, 0, len(calls))}
	for _, call := range ordered {
		if byKey[call.CallKey] != call {
			return ReferenceJobProgress{}, errors.New("Reference job progress call identity has drifted")
		}
		state := stateByKey[call.CallKey]
		result.Calls = append(result.Calls, ReferenceCallProgress{CallKey: call.CallKey, BundleIndex: call.BundleIndex, SlotKey: call.SlotKey, Status: state.Status, Revision: state.Revision, StateHash: state.ContentHash})
		switch state.Status {
		case ProviderCallPending:
			result.Pending++
		case ProviderCallDispatching:
			result.Dispatching++
		case ProviderCallOutcomeUnknown:
			result.OutcomeUnknown++
		case ProviderCallSucceeded:
			result.Succeeded++
		case ProviderCallFailed:
			result.Failed++
		}
	}
	switch {
	case result.OutcomeUnknown > 0:
		result.Status = ProviderJobOutcomeUnknown
	case result.Pending == result.Total:
		result.Status = ProviderJobPending
	case result.Pending+result.Dispatching > 0:
		result.Status = ProviderJobRunning
	case result.Succeeded == result.Total:
		result.Status, result.Terminal = ProviderJobSucceeded, true
	case result.Failed == result.Total:
		result.Status, result.Terminal = ProviderJobFailed, true
	default:
		result.Status, result.Terminal = ProviderJobPartialSucceeded, true
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return ReferenceJobProgress{}, err
	}
	result.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceJobProgress{}, err
	}
	return result, nil
}
