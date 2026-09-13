package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

type ReferenceCandidateSetMember struct {
	BundleIndex int                   `json:"candidate_bundle_index"`
	BundleRef   GenerationRevisionRef `json:"candidate_bundle_ref"`
}

type ReferenceCandidateSetSlot struct {
	CallRef   ReferenceProviderCall `json:"provider_call_ref"`
	StateHash string                `json:"call_state_hash"`
	QCRef     *GenerationActionRef  `json:"deterministic_qc_ref"`
	Issues    []string              `json:"issues"`
}

// A complete evidence set is not an eligibility, selection, or rights grant.
type ReferenceCandidateSet struct {
	ContractID                string                        `json:"contract_id"`
	ID                        string                        `json:"candidate_set_id"`
	WorkspaceID               string                        `json:"workspace_id"`
	ProjectID                 string                        `json:"project_id"`
	TargetRef                 GenerationRevisionRef         `json:"generation_target_ref"`
	ExecutionRef              GenerationRevisionRef         `json:"execution_ref"`
	GenerationRound           int64                         `json:"generation_round"`
	ExpectedBundleCount       int                           `json:"expected_bundle_count"`
	ExecutionProgressHash     string                        `json:"execution_progress_hash"`
	BundleRefs                []ReferenceCandidateSetMember `json:"ordered_candidate_bundle_refs"`
	FailedSlots               []ReferenceCandidateSetSlot   `json:"failed_or_unknown_slot_refs"`
	GenerationCompletionState string                        `json:"generation_completion_state"`
	DependencyRootHash        string                        `json:"dependency_root_hash"`
	CreatedAt                 time.Time                     `json:"created_at"`
	ContentHash               string                        `json:"content_hash"`
}

func BuildReferenceCandidateSet(facts ReferenceBundleFacts, bundles []ReferenceCandidateBundle) (ReferenceCandidateSet, error) {
	output, err := BuildReferenceOutputContract(facts.TargetKind, facts.Output.CandidateBundleCount, facts.Output.Slots)
	if err != nil || !reflect.DeepEqual(output, facts.Output) || len(bundles) > 4 {
		return ReferenceCandidateSet{}, errors.New("invalid candidate Set output contract")
	}
	progress, err := BuildReferenceJobProgress(facts.Job, facts.Calls, facts.States)
	if err != nil {
		return ReferenceCandidateSet{}, err
	}
	if !progress.Terminal && progress.OutcomeUnknown == 0 {
		return ReferenceCandidateSet{}, errors.New("candidate Set transmission is still unresolved")
	}
	if len(facts.Calls) != output.CandidateBundleCount*len(output.Slots) {
		return ReferenceCandidateSet{}, errors.New("candidate Set call coverage is incomplete")
	}
	byKey := make(map[string]ReferenceProviderCall, len(facts.Calls))
	type slotIdentity struct {
		bundleIndex int
		slotKey     string
	}
	seenSlots := map[slotIdentity]bool{}
	for _, call := range facts.Calls {
		if call.BundleIndex >= output.CandidateBundleCount || !slices.ContainsFunc(output.Slots, func(slot ReferenceOutputSlot) bool { return slot.SlotKey == call.SlotKey }) {
			return ReferenceCandidateSet{}, errors.New("candidate Set call escaped output contract")
		}
		key := slotIdentity{call.BundleIndex, call.SlotKey}
		if seenSlots[key] {
			return ReferenceCandidateSet{}, errors.New("candidate Set has duplicate slots")
		}
		seenSlots[key] = true
		byKey[call.CallKey] = call
	}
	v := ReferenceCandidateSet{ContractID: "generation-candidate-set-production", WorkspaceID: facts.WorkspaceID, ProjectID: facts.ProjectID, TargetRef: facts.TargetRef, ExecutionRef: facts.Job.ExecutionRef, GenerationRound: facts.GenerationRound, ExpectedBundleCount: output.CandidateBundleCount, ExecutionProgressHash: progress.ContentHash, BundleRefs: []ReferenceCandidateSetMember{}, FailedSlots: []ReferenceCandidateSetSlot{}, DependencyRootHash: facts.DependencyRootHash}
	for _, state := range facts.States {
		call := byKey[state.CallKey]
		if state.Dispatch != nil && state.Dispatch.DispatchedAt.After(v.CreatedAt) {
			v.CreatedAt = state.Dispatch.DispatchedAt
		}
		if state.OutcomeUnknownAt != nil && state.OutcomeUnknownAt.After(v.CreatedAt) {
			v.CreatedAt = *state.OutcomeUnknownAt
		}
		if state.Receipt != nil {
			r := state.Receipt
			if r.WorkspaceID != facts.WorkspaceID || r.ProjectID != facts.ProjectID || !slices.ContainsFunc(output.Slots, func(slot ReferenceOutputSlot) bool { return reflect.DeepEqual(slot, r.Slot) }) {
				return ReferenceCandidateSet{}, errors.New("candidate Set receipt scope drifted")
			}
			if r.ObservedAt.After(v.CreatedAt) {
				v.CreatedAt = r.ObservedAt
			}
		}
		if progress.OutcomeUnknown > 0 && (state.Status == ProviderCallOutcomeUnknown || state.Status == ProviderCallFailed) {
			issue := "provider_explicit_failure"
			if state.Status == ProviderCallOutcomeUnknown {
				issue = "outcome_unknown"
			}
			v.FailedSlots = append(v.FailedSlots, ReferenceCandidateSetSlot{CallRef: call, StateHash: state.ContentHash, Issues: []string{issue}})
		}
	}
	if progress.OutcomeUnknown > 0 {
		if len(bundles) != 0 {
			return ReferenceCandidateSet{}, errors.New("unknown candidate Set is reconciliation only")
		}
		v.GenerationCompletionState = "outcome_unknown"
	} else {
		inputs, err := BuildReferenceBundleInputs(facts)
		if err != nil {
			return ReferenceCandidateSet{}, err
		}
		byIndex := map[int]ReferenceCandidateBundle{}
		for _, bundle := range bundles {
			raw, _ := json.Marshal(bundle)
			if _, err := DecodeReferenceCandidateBundle(raw); err != nil {
				return ReferenceCandidateSet{}, err
			}
			if _, exists := byIndex[bundle.CandidateBundleIndex]; exists {
				return ReferenceCandidateSet{}, errors.New("candidate Set has multiple reviews for one group")
			}
			if bundle.WorkspaceID != v.WorkspaceID || bundle.ProjectID != v.ProjectID || bundle.TargetRef != v.TargetRef || bundle.ExecutionRef != v.ExecutionRef || bundle.GenerationRound != v.GenerationRound || bundle.DependencyRootHash != v.DependencyRootHash {
				return ReferenceCandidateSet{}, errors.New("candidate Set Bundle scope drifted")
			}
			byIndex[bundle.CandidateBundleIndex] = bundle
		}
		for index, evaluation := range inputs.Bundles {
			input := evaluation.Input
			if input.CreatedAt.After(v.CreatedAt) {
				v.CreatedAt = input.CreatedAt
			}
			bundle, exists := byIndex[index]
			if evaluation.Admission.InternalReviewReady {
				if !exists || bundle.BundleInputRef != (GenerationActionRef{ID: input.ID, ContentHash: input.ContentHash}) {
					return ReferenceCandidateSet{}, errors.New("candidate Set is missing an exact reviewed Bundle")
				}
				v.BundleRefs = append(v.BundleRefs, ReferenceCandidateSetMember{BundleIndex: index, BundleRef: GenerationRevisionRef{ID: bundle.ID, Revision: 1, ContentHash: bundle.ContentHash}})
				if bundle.CreatedAt.After(v.CreatedAt) {
					v.CreatedAt = bundle.CreatedAt
				}
				delete(byIndex, index)
				continue
			}
			if exists {
				return ReferenceCandidateSet{}, errors.New("candidate Set reviewed a technically failed group")
			}
			digests := map[string]int{}
			for _, slot := range input.Slots {
				if slot.MediaSHA256 != "" {
					digests[slot.MediaSHA256]++
				}
			}
			for position, slot := range input.Slots {
				issues := []string{}
				for _, issue := range evaluation.SlotQC[position].Issues {
					if issue != "rights_not_assessed" {
						issues = append(issues, issue)
					}
				}
				qc := slot.QCRef
				if digests[slot.MediaSHA256] > 1 {
					issues = append(issues, "duplicate_image")
					qc = input.BundleQCRef
				}
				if len(issues) > 0 {
					slices.Sort(issues)
					v.FailedSlots = append(v.FailedSlots, ReferenceCandidateSetSlot{CallRef: slot.CallRef, StateHash: slot.CallStateHash, QCRef: &qc, Issues: slices.Compact(issues)})
				}
			}
		}
		if len(byIndex) != 0 {
			return ReferenceCandidateSet{}, errors.New("candidate Set contains unrelated groups")
		}
		v.GenerationCompletionState = "complete"
		if len(v.FailedSlots) > 0 {
			v.GenerationCompletionState = "partial_explicit_failure"
		}
	}
	slices.SortFunc(v.FailedSlots, func(a, b ReferenceCandidateSetSlot) int {
		if a.CallRef.BundleIndex != b.CallRef.BundleIndex {
			return a.CallRef.BundleIndex - b.CallRef.BundleIndex
		}
		return strings.Compare(a.CallRef.SlotKey, b.CallRef.SlotKey)
	})
	v.CreatedAt = v.CreatedAt.UTC().Truncate(time.Microsecond)
	return buildReferenceCandidateSet(v)
}

func buildReferenceCandidateSet(v ReferenceCandidateSet) (ReferenceCandidateSet, error) {
	for _, id := range []string{v.WorkspaceID, v.ProjectID} {
		if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: v.DependencyRootHash}).Valid() {
			return ReferenceCandidateSet{}, errors.New("invalid candidate Set scope")
		}
	}
	if v.ContractID != "generation-candidate-set-production" || !v.TargetRef.Valid() || !v.ExecutionRef.Valid() || v.ExecutionRef.Revision != 1 || v.GenerationRound != 1 || v.ExpectedBundleCount < 1 || v.ExpectedBundleCount > 4 || !targetHashPattern.MatchString(v.ExecutionProgressHash) || v.CreatedAt.IsZero() || v.BundleRefs == nil || v.FailedSlots == nil || len(v.FailedSlots) > 16 {
		return ReferenceCandidateSet{}, errors.New("invalid candidate Set identity")
	}
	covered := map[int]bool{}
	seenRefs := map[string]bool{}
	failed := map[int]bool{}
	unknown := false
	for i, member := range v.BundleRefs {
		if member.BundleIndex < 0 || member.BundleIndex >= v.ExpectedBundleCount || !member.BundleRef.Valid() || member.BundleRef.Revision != 1 || seenRefs[member.BundleRef.ID] || i > 0 && v.BundleRefs[i-1].BundleIndex >= member.BundleIndex {
			return ReferenceCandidateSet{}, errors.New("invalid candidate Set member")
		}
		covered[member.BundleIndex] = true
		seenRefs[member.BundleRef.ID] = true
	}
	for i, slot := range v.FailedSlots {
		raw, _ := json.Marshal(slot.CallRef)
		if _, err := DecodeReferenceProviderCall(raw); err != nil {
			return ReferenceCandidateSet{}, err
		}
		if slot.CallRef.ExecutionRef != v.ExecutionRef || slot.CallRef.BundleIndex >= v.ExpectedBundleCount || covered[slot.CallRef.BundleIndex] || !targetHashPattern.MatchString(slot.StateHash) || len(slot.Issues) == 0 || len(slot.Issues) > 4 || !slices.IsSorted(slot.Issues) || len(slices.Compact(slices.Clone(slot.Issues))) != len(slot.Issues) {
			return ReferenceCandidateSet{}, errors.New("invalid candidate Set failed slot")
		}
		if i > 0 {
			previous := v.FailedSlots[i-1].CallRef
			if previous.BundleIndex > slot.CallRef.BundleIndex || previous.BundleIndex == slot.CallRef.BundleIndex && previous.SlotKey >= slot.CallRef.SlotKey {
				return ReferenceCandidateSet{}, errors.New("candidate Set failed slots are not ordered")
			}
		}
		for _, issue := range slot.Issues {
			if !slices.Contains([]string{"provider_explicit_failure", "media_rejected", "media_policy_failed", "duplicate_image", "outcome_unknown"}, issue) {
				return ReferenceCandidateSet{}, errors.New("unsupported candidate Set issue")
			}
			unknown = unknown || issue == "outcome_unknown"
		}
		if slot.QCRef != nil && !slot.QCRef.Valid() || v.GenerationCompletionState != "outcome_unknown" && slot.QCRef == nil {
			return ReferenceCandidateSet{}, errors.New("candidate Set failure requires exact QC")
		}
		if v.GenerationCompletionState == "outcome_unknown" && (slot.QCRef != nil || len(slot.Issues) != 1 || slot.Issues[0] != "outcome_unknown" && slot.Issues[0] != "provider_explicit_failure") {
			return ReferenceCandidateSet{}, errors.New("unknown candidate Set cannot infer media diagnostics")
		}
		failed[slot.CallRef.BundleIndex] = true
	}
	switch v.GenerationCompletionState {
	case "complete":
		if len(v.BundleRefs) != v.ExpectedBundleCount || len(v.FailedSlots) != 0 {
			return ReferenceCandidateSet{}, errors.New("candidate Set is not complete")
		}
	case "partial_explicit_failure":
		if unknown || len(failed) == 0 || len(covered)+len(failed) != v.ExpectedBundleCount {
			return ReferenceCandidateSet{}, errors.New("candidate Set explicit coverage is incomplete")
		}
	case "outcome_unknown":
		if !unknown || len(v.BundleRefs) != 0 {
			return ReferenceCandidateSet{}, errors.New("candidate Set unknown state is unsafe")
		}
	default:
		return ReferenceCandidateSet{}, errors.New("unsupported candidate Set completion state")
	}
	v.ID, v.ContentHash = "", ""
	hash, err := referenceBundleHash(v)
	if err != nil {
		return ReferenceCandidateSet{}, err
	}
	v.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("reference-candidate-set:"+hash)).String()
	v.ContentHash, err = referenceBundleHash(v)
	return v, err
}

func DecodeReferenceCandidateSet(raw json.RawMessage) (ReferenceCandidateSet, error) {
	var v ReferenceCandidateSet
	if err := canonical.Decode(raw, &v); err != nil {
		return ReferenceCandidateSet{}, err
	}
	expected, err := buildReferenceCandidateSet(v)
	if err != nil || expected.ID != v.ID || expected.ContentHash != v.ContentHash {
		return ReferenceCandidateSet{}, errors.New("candidate Set content drifted")
	}
	typed, _ := json.Marshal(expected)
	left, err := canonical.JSON(raw)
	if err != nil {
		return ReferenceCandidateSet{}, err
	}
	right, err := canonical.JSON(typed)
	if err != nil || !bytes.Equal(left, right) {
		return ReferenceCandidateSet{}, errors.New("candidate Set wire shape is incomplete")
	}
	return v, nil
}
