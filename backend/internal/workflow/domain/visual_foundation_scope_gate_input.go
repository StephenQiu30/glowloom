package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"

	"github.com/google/uuid"

	platformcanonical "github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
)

const (
	VisualFoundationScopeGateInputSchemaVersion = "visual-foundation-scope-human-gate-input-production"
	VisualFoundationScopeGateKey                = "visual_foundation_scope"
	visualFoundationScopeSubjectType            = "visual_foundation_scope"
	visualFoundationScopeEffectPlanKey          = "visual_foundation_scope"
	confirmVisualFoundationAndReferencePlanStep = "confirm_visual_foundation_and_reference_plan"
	imageGenerationCapabilityUnavailable        = "image_generation_capability_unavailable"
)

type VisualFoundationScopeImageGenerationCapability struct {
	Available   bool   `json:"available"`
	ReadSetHash string `json:"read_set_hash"`
}

type VisualFoundationScopeSemanticBlocker struct {
	Code              string   `json:"code"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type VisualFoundationScopeAtomicEffectStep struct {
	StepKey                        string                  `json:"step_key"`
	OwnerKinds                     []string                `json:"owner_kinds"`
	OwnerCommand                   string                  `json:"owner_command"`
	ExpectedHeads                  []HumanGateExpectedHead `json:"expected_heads"`
	ReadSetRoot                    string                  `json:"read_set_root"`
	ExpectedReferenceTargetKeyRoot string                  `json:"expected_reference_target_key_root"`
}

type VisualFoundationScopeEffectPlan struct {
	PlanKey    string                                `json:"plan_key"`
	AtomicStep VisualFoundationScopeAtomicEffectStep `json:"atomic_step"`
}

type VisualFoundationScopeGateInputDraft struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	Subject                                          VisualFoundationScopeSubject
	PresetRelease                                    presetdomain.Release
	ImageGenerationCapability                        VisualFoundationScopeImageGenerationCapability
	ExpectedPresetHead, ExpectedReferenceHead        HumanGateExpectedHead
}

type VisualFoundationScopeGateInput struct {
	SchemaVersion                string                                         `json:"schema_version"`
	GateKey                      string                                         `json:"gate_key"`
	GateInstanceKey              string                                         `json:"gate_instance_key"`
	WorkspaceID                  string                                         `json:"workspace_id"`
	ProjectID                    string                                         `json:"project_id"`
	WorkflowRunID                string                                         `json:"workflow_run_id"`
	NodeRunID                    string                                         `json:"node_run_id"`
	SubjectType                  string                                         `json:"subject_type"`
	Subject                      VisualFoundationScopeSubject                   `json:"subject"`
	SubjectHash                  string                                         `json:"subject_hash"`
	PresetCapabilities           []presetdomain.Capability                      `json:"preset_capabilities"`
	PresetCapabilityManifestRoot string                                         `json:"preset_capability_manifest_root"`
	ImageGenerationCapability    VisualFoundationScopeImageGenerationCapability `json:"image_generation_capability"`
	SemanticBlockers             []VisualFoundationScopeSemanticBlocker         `json:"semantic_blockers"`
	AllowedDecisions             []string                                       `json:"allowed_decisions"`
	ReadSetRoot                  string                                         `json:"read_set_root"`
	EffectPlan                   VisualFoundationScopeEffectPlan                `json:"effect_plan"`
	EffectPlanHash               string                                         `json:"effect_plan_hash"`
	InputHash                    string                                         `json:"input_hash"`
}

func NewVisualFoundationScopeGateInput(
	draft VisualFoundationScopeGateInputDraft,
) (VisualFoundationScopeGateInput, json.RawMessage, error) {
	if _, err := validateVisualFoundationScopeReleaseFromSubject(draft.Subject, draft.PresetRelease); err != nil {
		return VisualFoundationScopeGateInput{}, nil, err
	}
	value := VisualFoundationScopeGateInput{
		SchemaVersion: VisualFoundationScopeGateInputSchemaVersion,
		GateKey:       VisualFoundationScopeGateKey,
		WorkspaceID:   draft.WorkspaceID,
		ProjectID:     draft.ProjectID,
		WorkflowRunID: draft.WorkflowRunID,
		NodeRunID:     draft.NodeRunID,
		SubjectType:   visualFoundationScopeSubjectType,
		Subject:       draft.Subject,
		PresetCapabilities: cloneVisualFoundationScopeCapabilities(
			draft.PresetRelease.CapabilityManifest,
		),
		PresetCapabilityManifestRoot: draft.Subject.PresetCapabilityManifestRoot,
		ImageGenerationCapability:    draft.ImageGenerationCapability,
		EffectPlan: VisualFoundationScopeEffectPlan{
			AtomicStep: VisualFoundationScopeAtomicEffectStep{
				ExpectedHeads: []HumanGateExpectedHead{draft.ExpectedPresetHead, draft.ExpectedReferenceHead},
			},
		},
	}
	if err := completeVisualFoundationScopeGateInput(&value, false); err != nil {
		return VisualFoundationScopeGateInput{}, nil, err
	}
	return encodeVisualFoundationScopeGateInput(value)
}

func DecodeVisualFoundationScopeGateInput(
	raw json.RawMessage,
) (VisualFoundationScopeGateInput, json.RawMessage, error) {
	var value VisualFoundationScopeGateInput
	if err := decodeVisualFoundationScopeStrict(raw, &value); err != nil {
		return VisualFoundationScopeGateInput{}, nil, errors.New("invalid Gate 3 Visual Foundation Scope input")
	}
	original := value
	if err := completeVisualFoundationScopeGateInput(&value, true); err != nil || !reflect.DeepEqual(original, value) {
		return VisualFoundationScopeGateInput{}, nil, errors.New("Gate 3 Visual Foundation Scope input has drifted")
	}
	return encodeVisualFoundationScopeGateInput(value)
}

func completeVisualFoundationScopeGateInput(value *VisualFoundationScopeGateInput, verify bool) error {
	if value.SchemaVersion != VisualFoundationScopeGateInputSchemaVersion ||
		value.GateKey != VisualFoundationScopeGateKey || value.SubjectType != visualFoundationScopeSubjectType ||
		validateVisualFoundationScopeSubject(value.Subject) != nil ||
		value.WorkspaceID != value.Subject.ConfirmedProductionWorld.WorkspaceID ||
		value.ProjectID != value.Subject.ConfirmedProductionWorld.ProjectID {
		return errors.New("invalid Gate 3 Visual Foundation Scope input identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Gate 3 workflow identity")
		}
	}
	if presetdomain.ValidateCapabilityManifest(value.PresetCapabilities) != nil {
		return errors.New("invalid Gate 3 Preset capability manifest")
	}
	capabilityRoot, err := visualFoundationScopeHash(value.PresetCapabilities)
	if err != nil || capabilityRoot != value.Subject.PresetCapabilityManifestRoot ||
		capabilityRoot != value.PresetCapabilityManifestRoot {
		return errors.New("Gate 3 Preset capability manifest has drifted")
	}
	if !nodeOutputContentHashPattern.MatchString(value.ImageGenerationCapability.ReadSetHash) {
		return errors.New("invalid Gate 3 image generation capability read set")
	}

	expectedHeads := value.EffectPlan.AtomicStep.ExpectedHeads
	if validateVisualFoundationScopeExpectedHeads(expectedHeads, value.ProjectID) != nil {
		return errors.New("invalid Gate 3 expected Heads")
	}
	subjectHash, err := visualFoundationScopeHash(value.Subject)
	if err != nil {
		return err
	}
	candidateRoot, err := visualFoundationScopeHash([]VisualFoundationScopeCandidateRevisionRef{
		value.Subject.VisualFoundationCandidate,
		value.Subject.ReferencePlanCandidate,
	})
	if err != nil {
		return err
	}
	blockers := visualFoundationScopeSemanticBlockers(*value)
	allowedDecisions := []string{"approved", "changes_requested", "rejected"}
	if len(blockers) > 0 {
		allowedDecisions = []string{"changes_requested", "rejected"}
	}
	readSetRoot, err := visualFoundationScopeHash(struct {
		SubjectReadSetRoot         string                  `json:"subject_read_set_root"`
		PresetCapabilityRoot       string                  `json:"preset_capability_root"`
		ImageCapabilityReadSetHash string                  `json:"image_capability_read_set_hash"`
		ExpectedHeads              []HumanGateExpectedHead `json:"expected_heads"`
	}{
		value.Subject.ReadSetRoot,
		capabilityRoot,
		value.ImageGenerationCapability.ReadSetHash,
		expectedHeads,
	})
	if err != nil {
		return err
	}
	plan := VisualFoundationScopeEffectPlan{
		PlanKey: visualFoundationScopeEffectPlanKey,
		AtomicStep: VisualFoundationScopeAtomicEffectStep{
			StepKey:                        confirmVisualFoundationAndReferencePlanStep,
			OwnerKinds:                     []string{"preset", "production/reference"},
			OwnerCommand:                   confirmVisualFoundationAndReferencePlanStep,
			ExpectedHeads:                  append([]HumanGateExpectedHead(nil), expectedHeads...),
			ReadSetRoot:                    readSetRoot,
			ExpectedReferenceTargetKeyRoot: value.Subject.ExpectedReferenceTargetSet.ExpectedTargetKeyRoot,
		},
	}
	planHash, err := visualFoundationScopeHash(plan)
	if err != nil {
		return err
	}
	instanceKey := VisualFoundationScopeGateKey + ":" + value.ProjectID + ":" + candidateRoot
	if verify && (value.GateInstanceKey != instanceKey || value.SubjectHash != subjectHash ||
		!reflect.DeepEqual(value.SemanticBlockers, blockers) ||
		!slices.Equal(value.AllowedDecisions, allowedDecisions) || value.ReadSetRoot != readSetRoot ||
		!reflect.DeepEqual(value.EffectPlan, plan) || value.EffectPlanHash != planHash) {
		return errors.New("Gate 3 readiness or effect plan has drifted")
	}
	value.GateInstanceKey = instanceKey
	value.SubjectHash = subjectHash
	value.SemanticBlockers = blockers
	value.AllowedDecisions = allowedDecisions
	value.ReadSetRoot = readSetRoot
	value.EffectPlan = plan
	value.EffectPlanHash = planHash
	originalInputHash := value.InputHash
	value.InputHash = ""
	inputHash, err := visualFoundationScopeHash(*value)
	if err != nil {
		return err
	}
	if verify && originalInputHash != inputHash {
		return errors.New("Gate 3 input content has drifted")
	}
	value.InputHash = inputHash
	return nil
}

func validateVisualFoundationScopeReleaseFromSubject(
	subject VisualFoundationScopeSubject,
	release presetdomain.Release,
) (string, error) {
	root, err := validateVisualFoundationScopeReleaseRef(subject.PresetRelease, release)
	if err != nil || root != subject.PresetCapabilityManifestRoot {
		return "", errors.New("Gate 3 Preset release has drifted")
	}
	return root, nil
}

func visualFoundationScopeSemanticBlockers(
	value VisualFoundationScopeGateInput,
) []VisualFoundationScopeSemanticBlocker {
	if value.ImageGenerationCapability.Available {
		return []VisualFoundationScopeSemanticBlocker{}
	}
	return []VisualFoundationScopeSemanticBlocker{{
		Code:              imageGenerationCapabilityUnavailable,
		AffectedScopeKeys: append([]string(nil), value.Subject.ConfirmedProductionWorld.P1ScopeKeys...),
	}}
}

func validateVisualFoundationScopeExpectedHeads(values []HumanGateExpectedHead, projectID string) error {
	if len(values) != 2 || values[0].OwnerKind != "preset" || values[0].LogicalID != projectID ||
		values[1].OwnerKind != "production/reference" {
		return errors.New("invalid Gate 3 expected Heads")
	}
	for _, value := range values {
		if _, err := uuid.Parse(value.LogicalID); err != nil || value.Revision < 0 ||
			(value.Revision == 0 && value.ContentHash != "") ||
			(value.Revision > 0 && !nodeOutputContentHashPattern.MatchString(value.ContentHash)) {
			return errors.New("invalid Gate 3 expected Head")
		}
	}
	return nil
}

func cloneVisualFoundationScopeCapabilities(values []presetdomain.Capability) []presetdomain.Capability {
	result := make([]presetdomain.Capability, len(values))
	for index, value := range values {
		result[index] = presetdomain.Capability{
			TargetKind: value.TargetKind,
			ViewRoles:  append([]string(nil), value.ViewRoles...),
		}
	}
	return result
}

func encodeVisualFoundationScopeGateInput(
	value VisualFoundationScopeGateInput,
) (VisualFoundationScopeGateInput, json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return VisualFoundationScopeGateInput{}, nil, err
	}
	canonical, err := visualFoundationScopeCanonicalJSON(raw)
	if err != nil {
		return VisualFoundationScopeGateInput{}, nil, err
	}
	return value, canonical, nil
}

func visualFoundationScopeCanonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	canonical, err := platformcanonical.JSON(raw)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
