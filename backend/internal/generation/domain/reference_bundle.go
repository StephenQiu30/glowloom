package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

// ReferenceBundleFacts must be loaded from authorized, exact Owner facts. This
// compiler proves immutable content, not current authorization or publication.
type ReferenceBundleFacts struct {
	WorkspaceID, ProjectID         string
	TargetRef                      GenerationRevisionRef
	GenerationRound                int64
	TargetKind, DependencyRootHash string
	Output                         ReferenceOutputContract
	Job                            ReferenceProviderJob
	Calls                          []ReferenceProviderCall
	States                         []ReferenceCallState
	Media                          []ReferenceStagedMedia
}

type ReferenceBundleSlot struct {
	SlotKey       string                 `json:"slot_key"`
	ViewRole      string                 `json:"view_role"`
	CallRef       ReferenceProviderCall  `json:"provider_call_ref"`
	CallStateHash string                 `json:"call_state_hash"`
	CallStatus    string                 `json:"call_status"`
	MediaRef      *GenerationRevisionRef `json:"staged_media_ref,omitempty"`
	MediaSHA256   string                 `json:"media_sha256"`
	QCRef         GenerationActionRef    `json:"deterministic_qc_result_ref"`
}

type ReferenceDeterministicQC struct {
	ID          string                `json:"qc_result_id"`
	Scope       string                `json:"scope"`
	InputRoot   string                `json:"input_root"`
	PolicyRef   GenerationContractRef `json:"policy_ref"`
	Status      string                `json:"status"`
	Issues      []string              `json:"issues"`
	ContentHash string                `json:"content_hash"`
}

type ReferenceCandidateBundleInput struct {
	ID                   string                `json:"bundle_input_id"`
	TargetRef            GenerationRevisionRef `json:"generation_target_ref"`
	ExecutionRef         GenerationRevisionRef `json:"execution_ref"`
	GenerationRound      int64                 `json:"generation_round"`
	CandidateBundleIndex int                   `json:"candidate_bundle_index"`
	OutputContractRef    GenerationContractRef `json:"output_contract_ref"`
	Slots                []ReferenceBundleSlot `json:"slots"`
	SlotSetRoot          string                `json:"slot_set_root"`
	BundleCompleteness   string                `json:"bundle_completeness"`
	BundleQCRef          GenerationActionRef   `json:"bundle_deterministic_qc_result_ref"`
	DependencyRootHash   string                `json:"dependency_root_hash"`
	CreatedAt            time.Time             `json:"created_at"`
	ContentHash          string                `json:"content_hash"`
}

type ReferenceBundleEvaluation struct {
	Input     ReferenceCandidateBundleInput `json:"input"`
	SlotQC    []ReferenceDeterministicQC    `json:"slot_qc_results"`
	BundleQC  ReferenceDeterministicQC      `json:"bundle_qc_result"`
	Admission ReferenceBundleAdmission      `json:"admission"`
}

// ReferenceBundleAdmission describes material readiness, not actor authorization. Internal
// inspection does not change QC, rights observations, or formal-use eligibility.
type ReferenceBundleAdmission struct {
	PolicyRef              GenerationContractRef `json:"policy_ref"`
	InternalReviewReady    bool                  `json:"internal_review_ready"`
	SelectionReady         bool                  `json:"selection_ready"`
	PublicationReady       bool                  `json:"publication_ready"`
	InternalReviewBlockers []string              `json:"internal_review_blockers"`
	FormalUseBlockers      []string              `json:"formal_use_blockers"`
}

// ValidateInternalReview checks the exact material policy for a complete,
// technically valid bundle. It does not authorize a caller or formal use.
func (admission ReferenceBundleAdmission) ValidateInternalReview() error {
	expected := referenceBundleAdmission(ReferenceCandidateBundleInput{BundleCompleteness: "complete"}, ReferenceDeterministicQC{Issues: []string{"rights_not_assessed"}})
	if !reflect.DeepEqual(admission, expected) {
		return errors.New("Reference Bundle is not eligible for internal review")
	}
	return nil
}

func referenceBundleAdmission(input ReferenceCandidateBundleInput, qc ReferenceDeterministicQC) ReferenceBundleAdmission {
	const policy = `{"formal_use_requires":["rights_assessment","vision_review"],"internal_review_requires":["complete_bundle","technical_qc"],"rights_not_assessed":"internal_review_only"}`
	hash, _ := canonical.Hash([]byte(policy))
	result := ReferenceBundleAdmission{
		PolicyRef:              GenerationContractRef{ContractID: "reference-bundle-purpose-admission", ContentHash: hash},
		InternalReviewBlockers: []string{},
		FormalUseBlockers:      append([]string{}, qc.Issues...),
	}
	for _, issue := range qc.Issues {
		if issue != "rights_not_assessed" {
			result.InternalReviewBlockers = append(result.InternalReviewBlockers, issue)
		}
	}
	if input.BundleCompleteness != "complete" {
		result.InternalReviewBlockers = append(result.InternalReviewBlockers, "bundle_incomplete")
		result.FormalUseBlockers = append(result.FormalUseBlockers, "bundle_incomplete")
	}
	// Bundle Inputs precede Vision Candidates. A successful internal review and
	// separate rights evidence must be consumed by the later Selection Owner.
	result.FormalUseBlockers = append(result.FormalUseBlockers, "vision_review_required")
	slices.Sort(result.InternalReviewBlockers)
	slices.Sort(result.FormalUseBlockers)
	result.InternalReviewReady = len(result.InternalReviewBlockers) == 0
	return result
}

type ReferenceBundleInputCollection struct {
	ContractID   string                      `json:"contract_id"`
	TargetRef    GenerationRevisionRef       `json:"generation_target_ref"`
	ExecutionRef GenerationRevisionRef       `json:"execution_ref"`
	JobHash      string                      `json:"job_hash"`
	Bundles      []ReferenceBundleEvaluation `json:"bundles"`
	ContentHash  string                      `json:"content_hash"`
}

func referenceBundleQCPolicy() GenerationContractRef {
	const policy = `{"bundle_checks":["complete_slots","same_execution","unique_images"],"rights_not_assessed":"blocked","slot_checks":["receipt_identity","validated_png","dimensions","byte_budget","rights_observation"]}`
	hash, _ := canonical.Hash([]byte(policy))
	return GenerationContractRef{ContractID: "reference-bundle-deterministic-qc", ContentHash: hash}
}

func BuildReferenceBundleInputs(facts ReferenceBundleFacts) (ReferenceBundleInputCollection, error) {
	fail := func() (ReferenceBundleInputCollection, error) {
		return ReferenceBundleInputCollection{}, errors.New("Reference Bundle facts are incomplete or inconsistent")
	}
	for _, id := range []string{facts.WorkspaceID, facts.ProjectID} {
		if !(GenerationRevisionRef{ID: id, Revision: 1, ContentHash: facts.DependencyRootHash}).Valid() {
			return fail()
		}
	}
	if !facts.TargetRef.Valid() || facts.GenerationRound != 1 || !targetHashPattern.MatchString(facts.DependencyRootHash) {
		return fail()
	}
	raw, err := json.Marshal(facts.Output)
	if err != nil {
		return fail()
	}
	output, err := DecodeReferenceOutputContract(raw, facts.TargetKind)
	if err != nil {
		return fail()
	}
	progress, err := BuildReferenceJobProgress(facts.Job, facts.Calls, facts.States)
	if err != nil || !progress.Terminal || len(facts.Calls) != len(output.Slots)*output.CandidateBundleCount {
		return fail()
	}
	states := make(map[string]ReferenceCallState, len(facts.States))
	for _, state := range facts.States {
		states[state.CallKey] = state
	}
	media := make(map[string]ReferenceStagedMedia, len(facts.Media))
	for _, value := range facts.Media {
		raw, err := json.Marshal(value)
		if err != nil {
			return fail()
		}
		if _, err = DecodeReferenceStagedMedia(raw); err != nil || value.State == "quarantined" {
			return fail()
		}
		if _, exists := media[value.Call.CallKey]; exists {
			return fail()
		}
		media[value.Call.CallKey] = value
	}
	calls := slices.Clone(facts.Calls)
	slices.SortFunc(calls, func(a, b ReferenceProviderCall) int {
		if a.BundleIndex != b.BundleIndex {
			return a.BundleIndex - b.BundleIndex
		}
		return strings.Compare(a.SlotKey, b.SlotKey)
	})
	result := ReferenceBundleInputCollection{ContractID: "reference-bundle-input-collection", TargetRef: facts.TargetRef, ExecutionRef: facts.Job.ExecutionRef, JobHash: facts.Job.ContentHash, Bundles: make([]ReferenceBundleEvaluation, 0, output.CandidateBundleCount)}
	usedMedia := 0
	for index := range output.CandidateBundleCount {
		input := ReferenceCandidateBundleInput{TargetRef: facts.TargetRef, ExecutionRef: facts.Job.ExecutionRef, GenerationRound: facts.GenerationRound, CandidateBundleIndex: index, OutputContractRef: GenerationContractRef{ContractID: output.ContractID, ContentHash: output.ContentHash}, DependencyRootHash: facts.DependencyRootHash, Slots: make([]ReferenceBundleSlot, 0, len(output.Slots)), BundleCompleteness: "complete"}
		slotQC := make([]ReferenceDeterministicQC, 0, len(output.Slots))
		for offset, slot := range output.Slots {
			call := calls[index*len(output.Slots)+offset]
			state := states[call.CallKey]
			if call.BundleIndex != index || call.SlotKey != slot.SlotKey || state.Receipt == nil || state.Receipt.Call != call || state.Receipt.WorkspaceID != facts.WorkspaceID || state.Receipt.ProjectID != facts.ProjectID || !reflect.DeepEqual(state.Receipt.Slot, slot) {
				return fail()
			}
			member := ReferenceBundleSlot{SlotKey: slot.SlotKey, ViewRole: slot.ViewRole, CallRef: call, CallStateHash: state.ContentHash, CallStatus: state.Status}
			issues := []string{}
			if state.Receipt.ObservedAt.After(input.CreatedAt) {
				input.CreatedAt = state.Receipt.ObservedAt
			}
			switch state.Status {
			case ProviderCallFailed:
				if _, exists := media[call.CallKey]; exists {
					return fail()
				}
				issues = append(issues, "provider_explicit_failure")
				input.BundleCompleteness = "partial_explicit_failure"
			case ProviderCallSucceeded:
				value, exists := media[call.CallKey]
				if !exists || value.Call != call || value.WorkspaceID != facts.WorkspaceID || value.ProjectID != facts.ProjectID {
					return fail()
				}
				initial, err := InitialReferenceStagedMedia(value)
				if err != nil {
					return fail()
				}
				expected, err := NewReferenceStagedMedia(*state.Receipt, value.ObjectStoreRef)
				if err != nil || !reflect.DeepEqual(initial, expected) {
					return fail()
				}
				usedMedia++
				member.MediaRef = &GenerationRevisionRef{ID: value.ID, Revision: value.Revision, ContentHash: value.ContentHash}
				member.MediaSHA256 = value.SHA256
				if value.ValidatedAt.After(input.CreatedAt) {
					input.CreatedAt = *value.ValidatedAt
				}
				issues = append(issues, "rights_not_assessed")
				if value.State == "rejected" {
					issues = append(issues, "media_rejected")
				}
				a, b, _ := strings.Cut(slot.AspectRatio, ":")
				w, _ := strconv.Atoi(a)
				h, _ := strconv.Atoi(b)
				if !slices.Contains(slot.AllowedMediaTypes, value.MediaType) || value.Width < slot.MinWidth || value.Height < slot.MinHeight || value.ByteSize > slot.MaxBytes || int64(value.Width)*int64(h) != int64(value.Height)*int64(w) {
					issues = append(issues, "media_policy_failed")
				}
			default:
				return fail()
			}
			root, err := referenceSlotInputRoot(input, member)
			if err != nil {
				return fail()
			}
			qc, err := buildReferenceQC("slot", root, issues)
			if err != nil {
				return fail()
			}
			member.QCRef = GenerationActionRef{ID: qc.ID, ContentHash: qc.ContentHash}
			input.Slots = append(input.Slots, member)
			slotQC = append(slotQC, qc)
		}
		input.SlotSetRoot, err = referenceBundleHash(input.Slots)
		if err != nil {
			return fail()
		}
		issues := referenceBundleIssues(input, slotQC)
		qc, err := buildReferenceQC("bundle", input.SlotSetRoot, issues)
		if err != nil {
			return fail()
		}
		input.BundleQCRef = GenerationActionRef{ID: qc.ID, ContentHash: qc.ContentHash}
		input.ID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("reference-bundle-input:"+qc.ID+":"+facts.TargetRef.ContentHash+":"+facts.DependencyRootHash)).String()
		input.ContentHash, err = referenceBundleHash(input)
		if err != nil {
			return fail()
		}
		result.Bundles = append(result.Bundles, ReferenceBundleEvaluation{Input: input, SlotQC: slotQC, BundleQC: qc, Admission: referenceBundleAdmission(input, qc)})
	}
	if usedMedia != len(media) {
		return fail()
	}
	result.ContentHash, err = referenceBundleHash(result)
	return result, err
}

func referenceSlotInputRoot(input ReferenceCandidateBundleInput, slot ReferenceBundleSlot) (string, error) {
	slot.QCRef = GenerationActionRef{}
	return referenceBundleHash(struct {
		Target       GenerationRevisionRef `json:"target"`
		Execution    GenerationRevisionRef `json:"execution"`
		Output       GenerationContractRef `json:"output"`
		Dependencies string                `json:"dependencies"`
		Slot         ReferenceBundleSlot   `json:"slot"`
	}{input.TargetRef, input.ExecutionRef, input.OutputContractRef, input.DependencyRootHash, slot})
}

func referenceBundleIssues(input ReferenceCandidateBundleInput, slots []ReferenceDeterministicQC) []string {
	issues := []string{}
	seen := map[string]bool{}
	for i, slot := range input.Slots {
		issues = append(issues, slots[i].Issues...)
		if slot.MediaSHA256 != "" {
			if seen[slot.MediaSHA256] {
				issues = append(issues, "duplicate_image")
			}
			seen[slot.MediaSHA256] = true
		}
	}
	return issues
}

func buildReferenceQC(scope, root string, issues []string) (ReferenceDeterministicQC, error) {
	issues = slices.Clone(issues)
	slices.Sort(issues)
	issues = slices.Compact(issues)
	if issues == nil {
		issues = []string{}
	}
	policy := referenceBundleQCPolicy()
	qc := ReferenceDeterministicQC{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("reference-qc:"+scope+":"+root+":"+policy.ContentHash)).String(), Scope: scope, InputRoot: root, PolicyRef: policy, Status: "passed", Issues: issues}
	for _, issue := range issues {
		switch issue {
		case "rights_not_assessed":
			if qc.Status == "passed" {
				qc.Status = "blocked"
			}
		case "provider_explicit_failure", "media_rejected", "media_policy_failed", "duplicate_image":
			qc.Status = "failed"
		default:
			return ReferenceDeterministicQC{}, errors.New("unsupported deterministic QC issue")
		}
	}
	var err error
	qc.ContentHash, err = referenceBundleHash(qc)
	return qc, err
}

func referenceBundleHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return canonical.Hash(raw)
}

// DecodeReferenceBundleInputs validates content closure, not Owner authorization.
func DecodeReferenceBundleInputs(raw json.RawMessage) (ReferenceBundleInputCollection, error) {
	var value ReferenceBundleInputCollection
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceBundleInputCollection{}, err
	}
	fail := func() (ReferenceBundleInputCollection, error) {
		return ReferenceBundleInputCollection{}, errors.New("Reference Bundle input content has drifted")
	}
	if value.ContractID != "reference-bundle-input-collection" || !value.TargetRef.Valid() || !value.ExecutionRef.Valid() || !targetHashPattern.MatchString(value.JobHash) || len(value.Bundles) < 1 || len(value.Bundles) > 4 {
		return fail()
	}
	for index, bundle := range value.Bundles {
		input := bundle.Input
		if index > 0 && (input.OutputContractRef != value.Bundles[0].Input.OutputContractRef || input.DependencyRootHash != value.Bundles[0].Input.DependencyRootHash || len(input.Slots) != len(value.Bundles[0].Input.Slots)) {
			return fail()
		}
		if input.TargetRef != value.TargetRef || input.ExecutionRef != value.ExecutionRef || input.GenerationRound != 1 || input.CandidateBundleIndex != index || !input.OutputContractRef.Valid() || input.OutputContractRef.ContractID != ReferenceOutputContractID || !targetHashPattern.MatchString(input.DependencyRootHash) || input.CreatedAt.IsZero() || len(input.Slots) < 1 || len(input.Slots) != len(bundle.SlotQC) {
			return fail()
		}
		completeness := "complete"
		for i, slot := range input.Slots {
			if index > 0 && slot.SlotKey != value.Bundles[0].Input.Slots[i].SlotKey {
				return fail()
			}
			if slot.SlotKey != slot.ViewRole || slot.CallRef.SlotKey != slot.SlotKey || slot.CallRef.BundleIndex != index || slot.CallRef.ExecutionRef != input.ExecutionRef || !targetHashPattern.MatchString(slot.CallStateHash) || (i > 0 && input.Slots[i-1].SlotKey >= slot.SlotKey) {
				return fail()
			}
			callRaw, _ := json.Marshal(slot.CallRef)
			if _, err := DecodeReferenceProviderCall(callRaw); err != nil {
				return fail()
			}
			if slot.CallStatus == ProviderCallFailed {
				completeness = "partial_explicit_failure"
				if slot.MediaRef != nil || slot.MediaSHA256 != "" || !slices.Contains(bundle.SlotQC[i].Issues, "provider_explicit_failure") {
					return fail()
				}
			} else if slot.CallStatus != ProviderCallSucceeded || slot.MediaRef == nil || !slot.MediaRef.Valid() || slot.MediaRef.Revision != 2 || !targetHashPattern.MatchString(slot.MediaSHA256) || !slices.Contains(bundle.SlotQC[i].Issues, "rights_not_assessed") {
				return fail()
			}
			root, err := referenceSlotInputRoot(input, slot)
			if err != nil {
				return fail()
			}
			qc, err := buildReferenceQC("slot", root, bundle.SlotQC[i].Issues)
			if err != nil || !reflect.DeepEqual(qc, bundle.SlotQC[i]) || slot.QCRef != (GenerationActionRef{ID: qc.ID, ContentHash: qc.ContentHash}) {
				return fail()
			}
		}
		root, err := referenceBundleHash(input.Slots)
		if err != nil || root != input.SlotSetRoot || completeness != input.BundleCompleteness {
			return fail()
		}
		qc, err := buildReferenceQC("bundle", root, referenceBundleIssues(input, bundle.SlotQC))
		if err != nil || !reflect.DeepEqual(qc, bundle.BundleQC) || input.BundleQCRef != (GenerationActionRef{ID: qc.ID, ContentHash: qc.ContentHash}) {
			return fail()
		}
		if !reflect.DeepEqual(bundle.Admission, referenceBundleAdmission(input, qc)) {
			return fail()
		}
		if input.ID != uuid.NewSHA1(uuid.NameSpaceOID, []byte("reference-bundle-input:"+qc.ID+":"+input.TargetRef.ContentHash+":"+input.DependencyRootHash)).String() {
			return fail()
		}
		hash := input.ContentHash
		input.ContentHash = ""
		expected, err := referenceBundleHash(input)
		if err != nil || expected != hash {
			return fail()
		}
	}
	hash := value.ContentHash
	value.ContentHash = ""
	expected, err := referenceBundleHash(value)
	if err != nil || expected != hash {
		return fail()
	}
	value.ContentHash = hash
	// Reject omitted or null zero-valued fields instead of silently treating
	// an absent purpose decision as an explicit false decision.
	wireHash, err := canonical.Hash(raw)
	if err != nil {
		return fail()
	}
	encodedHash, err := referenceBundleHash(value)
	if err != nil || wireHash != encodedHash {
		return fail()
	}
	return value, nil
}
