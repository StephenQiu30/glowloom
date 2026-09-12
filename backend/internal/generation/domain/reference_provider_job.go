package domain

import (
	"cmp"
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type ReferenceProviderCallInput struct {
	BundleIndex         int    `json:"bundle_index"`
	SlotKey             string `json:"slot_key"`
	CompiledRequestHash string `json:"compiled_request_hash"`
}

// ReferenceProviderCall is the immutable invocation identity, not a send right.
type ReferenceProviderCall struct {
	ContractID   string                `json:"contract_id"`
	ExecutionRef GenerationRevisionRef `json:"execution_ref"`
	ReferenceProviderCallInput
	CallKey string `json:"call_key"`
}

// ReferenceProviderJob freezes membership only. Runtime status is derived from
// its calls, never inferred from this manifest's existence.
type ReferenceProviderJob struct {
	ContractID   string                `json:"contract_id"`
	ExecutionRef GenerationRevisionRef `json:"execution_ref"`
	CallKeys     []string              `json:"call_keys"`
	CallSetRoot  string                `json:"call_set_root"`
	ContentHash  string                `json:"content_hash"`
}

func buildReferenceProviderCall(execution GenerationRevisionRef, input ReferenceProviderCallInput) (ReferenceProviderCall, error) {
	if !execution.Valid() || execution.Revision != 1 || input.BundleIndex < 0 || input.BundleIndex >= DefaultReferenceGenerationLimits().MaxBundles || !targetNamePattern.MatchString(input.SlotKey) || !targetHashPattern.MatchString(input.CompiledRequestHash) {
		return ReferenceProviderCall{}, errors.New("invalid Reference Provider call identity")
	}
	raw, err := json.Marshal([]any{execution, input.BundleIndex, input.SlotKey, input.CompiledRequestHash})
	if err != nil {
		return ReferenceProviderCall{}, err
	}
	key, err := canonical.Hash(raw)
	if err != nil {
		return ReferenceProviderCall{}, err
	}
	return ReferenceProviderCall{ContractID: "reference-provider-call", ExecutionRef: execution, ReferenceProviderCallInput: input, CallKey: key}, nil
}

func BuildReferenceProviderJob(execution GenerationRevisionRef, inputs []ReferenceProviderCallInput) (ReferenceProviderJob, []ReferenceProviderCall, error) {
	limits := DefaultReferenceGenerationLimits()
	if len(inputs) == 0 || len(inputs) > limits.MaxBundles*limits.MaxSlots {
		return ReferenceProviderJob{}, nil, errors.New("invalid Reference Provider call count")
	}
	ordered := slices.Clone(inputs)
	slices.SortFunc(ordered, func(a, b ReferenceProviderCallInput) int {
		if order := cmp.Compare(a.BundleIndex, b.BundleIndex); order != 0 {
			return order
		}
		return cmp.Compare(a.SlotKey, b.SlotKey)
	})
	calls := make([]ReferenceProviderCall, 0, len(ordered))
	keys := make([]string, 0, len(ordered))
	slots := make([][]string, limits.MaxBundles)
	for _, input := range ordered {
		call, err := buildReferenceProviderCall(execution, input)
		if err != nil {
			return ReferenceProviderJob{}, nil, err
		}
		group := slots[input.BundleIndex]
		if len(group) >= limits.MaxSlots || slices.Contains(group, input.SlotKey) {
			return ReferenceProviderJob{}, nil, errors.New("duplicate or excessive Reference Provider slots")
		}
		slots[input.BundleIndex] = append(group, input.SlotKey)
		calls, keys = append(calls, call), append(keys, call.CallKey)
	}
	for bundle := 0; bundle <= ordered[len(ordered)-1].BundleIndex; bundle++ {
		if len(slots[bundle]) == 0 || !slices.Equal(slots[0], slots[bundle]) {
			return ReferenceProviderJob{}, nil, errors.New("Reference Provider bundles have incomplete slot coverage")
		}
	}
	job, err := buildReferenceProviderJobManifest(execution, keys)
	if err != nil {
		return ReferenceProviderJob{}, nil, err
	}
	return job, calls, nil
}

func buildReferenceProviderJobManifest(execution GenerationRevisionRef, keys []string) (ReferenceProviderJob, error) {
	limits := DefaultReferenceGenerationLimits()
	if !execution.Valid() || execution.Revision != 1 || len(keys) == 0 || len(keys) > limits.MaxBundles*limits.MaxSlots {
		return ReferenceProviderJob{}, errors.New("invalid Reference Provider job identity")
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if !targetHashPattern.MatchString(key) || seen[key] {
			return ReferenceProviderJob{}, errors.New("invalid Reference Provider call set")
		}
		seen[key] = true
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		return ReferenceProviderJob{}, err
	}
	root, err := canonical.Hash(raw)
	if err != nil {
		return ReferenceProviderJob{}, err
	}
	value := ReferenceProviderJob{ContractID: "reference-provider-job", ExecutionRef: execution, CallKeys: slices.Clone(keys), CallSetRoot: root}
	raw, err = json.Marshal(value)
	if err != nil {
		return ReferenceProviderJob{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceProviderJob{}, err
	}
	return value, nil
}

func DecodeReferenceProviderCall(raw json.RawMessage) (ReferenceProviderCall, error) {
	var value ReferenceProviderCall
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceProviderCall{}, err
	}
	expected, err := buildReferenceProviderCall(value.ExecutionRef, value.ReferenceProviderCallInput)
	if err != nil || !reflect.DeepEqual(expected, value) {
		return ReferenceProviderCall{}, errors.New("Reference Provider call identity has drifted")
	}
	return value, nil
}

func DecodeReferenceProviderJob(raw json.RawMessage) (ReferenceProviderJob, error) {
	var value ReferenceProviderJob
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceProviderJob{}, err
	}
	expected, err := buildReferenceProviderJobManifest(value.ExecutionRef, value.CallKeys)
	if err != nil || !reflect.DeepEqual(expected, value) {
		return ReferenceProviderJob{}, errors.New("Reference Provider job identity has drifted")
	}
	return value, nil
}
