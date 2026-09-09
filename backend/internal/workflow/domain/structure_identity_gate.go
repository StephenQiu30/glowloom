package domain

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const (
	StructureIdentityGateInputSchemaVersion = "structure-identity-human-gate-input-production"
	StructureIdentityGateKey                = "structure_identity"
	structureIdentitySubjectType            = "structure_identity"
	structureIdentityEffectPlanKey          = "structure_identity"
	confirmProjectEpisodeLifecycleStep      = "confirm_project_episode_lifecycle"
	confirmStructureIdentitySetStep         = "confirm_structure_identity_set"
)

type StructureIdentityGateSubject struct {
	SourceVersion      agentcontract.ScriptSourceVersionIdentity            `json:"source_version"`
	SpanCandidate      agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"span_candidate"`
	SceneFactCandidate agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_fact_candidate"`
	IdentityCandidate  agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"identity_candidate"`
	ReviewCandidate    agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"review_candidate"`
}

type HumanGateEvidenceRef struct {
	SourceVersionID string `json:"source_version_id"`
	SourceStart     int    `json:"source_start"`
	SourceEnd       int    `json:"source_end"`
	TextHash        string `json:"text_hash"`
}

type HumanGateImpactSummary struct {
	AffectedScopeKeys   []string `json:"affected_scope_keys"`
	PreservedFamilies   []string `json:"preserved_families"`
	InvalidatedFamilies []string `json:"invalidated_families"`
}

type StructureIdentityAllowedChange struct {
	Operation         string   `json:"operation"`
	TargetKeys        []string `json:"target_keys"`
	AffectedScopeKeys []string `json:"affected_scope_keys"`
}

type StructureIdentityRepairOption struct {
	IssueKey       string                           `json:"issue_key"`
	Code           string                           `json:"code"`
	Severity       string                           `json:"severity"`
	Scope          string                           `json:"scope"`
	Summary        string                           `json:"summary"`
	EvidenceRefs   []HumanGateEvidenceRef           `json:"evidence_refs"`
	AllowedChanges []StructureIdentityAllowedChange `json:"allowed_changes"`
}

type StructureIdentityChangeRequest struct {
	IssueRefs    []string                       `json:"issue_refs"`
	EvidenceRefs []HumanGateEvidenceRef         `json:"evidence_refs"`
	ChangeSpec   StructureIdentityAllowedChange `json:"change_spec"`
	ReasonCode   string                         `json:"reason_code"`
	UserNote     *string                        `json:"user_note,omitempty"`
}

type HumanGateExpectedHead struct {
	OwnerKind   string `json:"owner_kind"`
	LogicalID   string `json:"logical_id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash,omitempty"`
}

type StructureIdentityEffectStep struct {
	StepKey           string                `json:"step_key"`
	Position          int                   `json:"position"`
	OwnerKind         string                `json:"owner_kind"`
	OwnerCommand      string                `json:"owner_command"`
	ExpectedHead      HumanGateExpectedHead `json:"expected_head"`
	ReadSetHash       string                `json:"read_set_hash"`
	DependsOnStepKeys []string              `json:"depends_on_step_keys"`
	RecoveryTarget    string                `json:"recovery_target"`
}

type StructureIdentityEffectPlan struct {
	PlanKey string                        `json:"plan_key"`
	Steps   []StructureIdentityEffectStep `json:"steps"`
}

type StructureIdentityGateInputDraft struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	Subject                                          StructureIdentityGateSubject
	EvidenceRefs                                     []HumanGateEvidenceRef
	Impact                                           HumanGateImpactSummary
	RepairOptions                                    []StructureIdentityRepairOption
	AllowedDecisions                                 []string
	ExpectedProjectHead, ExpectedBibleHead           HumanGateExpectedHead
}

type StructureIdentityGateInput struct {
	SchemaVersion    string                          `json:"schema_version"`
	GateKey          string                          `json:"gate_key"`
	GateInstanceKey  string                          `json:"gate_instance_key"`
	WorkspaceID      string                          `json:"workspace_id"`
	ProjectID        string                          `json:"project_id"`
	WorkflowRunID    string                          `json:"workflow_run_id"`
	NodeRunID        string                          `json:"node_run_id"`
	SubjectType      string                          `json:"subject_type"`
	Subject          StructureIdentityGateSubject    `json:"subject"`
	SubjectHash      string                          `json:"subject_hash"`
	EvidenceRefs     []HumanGateEvidenceRef          `json:"evidence_refs"`
	Impact           HumanGateImpactSummary          `json:"impact_summary"`
	RepairOptions    []StructureIdentityRepairOption `json:"repair_options"`
	AllowedDecisions []string                        `json:"allowed_decisions"`
	EffectPlan       StructureIdentityEffectPlan     `json:"effect_plan"`
	EffectPlanHash   string                          `json:"effect_plan_hash"`
	InputHash        string                          `json:"input_hash"`
}

func NewStructureIdentityGateInput(
	draft StructureIdentityGateInputDraft,
) (StructureIdentityGateInput, json.RawMessage, error) {
	value := StructureIdentityGateInput{
		SchemaVersion: StructureIdentityGateInputSchemaVersion,
		GateKey:       StructureIdentityGateKey,
		WorkspaceID:   strings.TrimSpace(draft.WorkspaceID),
		ProjectID:     strings.TrimSpace(draft.ProjectID),
		WorkflowRunID: strings.TrimSpace(draft.WorkflowRunID),
		NodeRunID:     strings.TrimSpace(draft.NodeRunID),
		SubjectType:   structureIdentitySubjectType,
		Subject:       draft.Subject,
		EvidenceRefs:  append([]HumanGateEvidenceRef(nil), draft.EvidenceRefs...),
		Impact: HumanGateImpactSummary{
			AffectedScopeKeys:   append([]string(nil), draft.Impact.AffectedScopeKeys...),
			PreservedFamilies:   append([]string(nil), draft.Impact.PreservedFamilies...),
			InvalidatedFamilies: append([]string(nil), draft.Impact.InvalidatedFamilies...),
		},
		RepairOptions:    cloneStructureIdentityRepairOptions(draft.RepairOptions),
		AllowedDecisions: append([]string(nil), draft.AllowedDecisions...),
	}
	value.Subject.SourceVersion.CreatedAt = value.Subject.SourceVersion.CreatedAt.UTC()
	if err := normalizeStructureIdentityGateInput(&value); err != nil {
		return StructureIdentityGateInput{}, nil, err
	}
	value.GateInstanceKey = StructureIdentityGateKey + ":" + value.ProjectID + ":" +
		value.Subject.ReviewCandidate.CandidateRevisionHash
	value.SubjectHash = hashStructureIdentityGateMaterial(value.Subject)
	value.EffectPlan = newStructureIdentityEffectPlan(
		value.SubjectHash,
		draft.ExpectedProjectHead,
		draft.ExpectedBibleHead,
	)
	if err := validateStructureIdentityEffectPlan(value.EffectPlan, value.ProjectID, value.SubjectHash); err != nil {
		return StructureIdentityGateInput{}, nil, err
	}
	value.EffectPlanHash = hashStructureIdentityGateMaterial(value.EffectPlan)
	value.InputHash = hashStructureIdentityGateMaterial(structureIdentityGateInputHashMaterial(value))
	encoded, err := json.Marshal(value)
	if err != nil {
		return StructureIdentityGateInput{}, nil, err
	}
	return value, encoded, nil
}

func DecodeStructureIdentityGateInput(
	raw json.RawMessage,
) (StructureIdentityGateInput, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value StructureIdentityGateInput
	if err := decoder.Decode(&value); err != nil {
		return StructureIdentityGateInput{}, nil, errors.New("invalid structure identity Gate input")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return StructureIdentityGateInput{}, nil, errors.New("invalid structure identity Gate input")
	}
	originalSubjectHash, originalPlanHash, originalInputHash := value.SubjectHash, value.EffectPlanHash, value.InputHash
	originalInstanceKey := value.GateInstanceKey
	value.Subject.SourceVersion.CreatedAt = value.Subject.SourceVersion.CreatedAt.UTC()
	if err := normalizeStructureIdentityGateInput(&value); err != nil ||
		validateStructureIdentityEffectPlan(value.EffectPlan, value.ProjectID, originalSubjectHash) != nil {
		return StructureIdentityGateInput{}, nil, errors.New("invalid structure identity Gate input")
	}
	value.SubjectHash = hashStructureIdentityGateMaterial(value.Subject)
	value.EffectPlanHash = hashStructureIdentityGateMaterial(value.EffectPlan)
	value.GateInstanceKey = StructureIdentityGateKey + ":" + value.ProjectID + ":" +
		value.Subject.ReviewCandidate.CandidateRevisionHash
	value.InputHash = hashStructureIdentityGateMaterial(structureIdentityGateInputHashMaterial(value))
	if originalSubjectHash != value.SubjectHash || originalPlanHash != value.EffectPlanHash ||
		originalInputHash != value.InputHash || originalInstanceKey != value.GateInstanceKey {
		return StructureIdentityGateInput{}, nil, errors.New("structure identity Gate input hash mismatch")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return StructureIdentityGateInput{}, nil, err
	}
	return value, encoded, nil
}

func normalizeStructureIdentityGateInput(value *StructureIdentityGateInput) error {
	if value.SchemaVersion != StructureIdentityGateInputSchemaVersion || value.GateKey != StructureIdentityGateKey ||
		value.SubjectType != structureIdentitySubjectType {
		return errors.New("invalid structure identity Gate input identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity Gate workflow identity")
		}
	}
	if err := validateStructureIdentityGateSubject(value.Subject); err != nil {
		return err
	}
	slices.SortFunc(value.EvidenceRefs, compareHumanGateEvidenceRef)
	if len(value.EvidenceRefs) == 0 {
		return errors.New("structure identity Gate has no evidence")
	}
	for index, evidence := range value.EvidenceRefs {
		if evidence.SourceVersionID != value.Subject.SourceVersion.VersionID || evidence.SourceStart < 0 ||
			evidence.SourceEnd <= evidence.SourceStart || !nodeOutputContentHashPattern.MatchString(evidence.TextHash) ||
			(index > 0 && compareHumanGateEvidenceRef(value.EvidenceRefs[index-1], evidence) == 0) {
			return errors.New("invalid structure identity Gate evidence")
		}
	}
	if err := normalizeHumanGateImpact(&value.Impact); err != nil {
		return err
	}
	for index := range value.AllowedDecisions {
		value.AllowedDecisions[index] = strings.ToLower(strings.TrimSpace(value.AllowedDecisions[index]))
	}
	slices.Sort(value.AllowedDecisions)
	if !validStructureIdentityDecisions(value.AllowedDecisions) {
		return errors.New("invalid structure identity Gate decisions")
	}
	if err := normalizeStructureIdentityRepairOptions(value); err != nil {
		return err
	}
	return nil
}

func validateStructureIdentityGateSubject(value StructureIdentityGateSubject) error {
	if value.SourceVersion.Validate() != nil {
		return errors.New("invalid structure identity Gate source")
	}
	for _, identifier := range []string{value.SourceVersion.LogicalID, value.SourceVersion.VersionID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity Gate source")
		}
	}
	expected := []struct {
		stage string
		ref   agentcontract.SceneAnalysisCandidateRevisionIdentity
	}{
		{"propose_script_spans", value.SpanCandidate},
		{"extract_scene_facts", value.SceneFactCandidate},
		{"resolve_identities", value.IdentityCandidate},
		{"review_candidate", value.ReviewCandidate},
	}
	ids := make(map[string]struct{}, len(expected))
	invocations := make(map[string]struct{}, len(expected))
	for _, item := range expected {
		if item.ref.Validate() != nil || item.ref.StageKey != item.stage || item.ref.ShardKey != "script:full" {
			return errors.New("invalid structure identity Gate Candidate identity")
		}
		if _, duplicate := ids[item.ref.CandidateRevisionID]; duplicate {
			return errors.New("duplicate structure identity Gate Candidate")
		}
		if _, duplicate := invocations[item.ref.SourceInvocationID]; duplicate {
			return errors.New("duplicate structure identity Gate invocation")
		}
		ids[item.ref.CandidateRevisionID] = struct{}{}
		invocations[item.ref.SourceInvocationID] = struct{}{}
	}
	return nil
}

func normalizeHumanGateImpact(value *HumanGateImpactSummary) error {
	collections := []*[]string{&value.AffectedScopeKeys, &value.PreservedFamilies, &value.InvalidatedFamilies}
	for _, collection := range collections {
		for index := range *collection {
			(*collection)[index] = strings.TrimSpace((*collection)[index])
		}
		slices.Sort(*collection)
		if len(*collection) == 0 {
			return errors.New("structure identity Gate impact is incomplete")
		}
		for index, item := range *collection {
			if item == "" || (index > 0 && (*collection)[index-1] == item) {
				return errors.New("invalid structure identity Gate impact")
			}
		}
	}
	for _, scope := range value.AffectedScopeKeys {
		kind, identifier, found := strings.Cut(scope, ":")
		if !found || kind != "scene" {
			return errors.New("invalid structure identity Gate affected scope")
		}
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid structure identity Gate affected scope")
		}
	}
	preserved := map[string]struct{}{
		"identity_resolution": {}, "scene_facts": {}, "script_source": {}, "script_spans": {},
	}
	invalidated := map[string]struct{}{
		"production_world": {}, "reference_selection": {}, "storyboard": {}, "visual_foundation": {},
	}
	for _, family := range value.PreservedFamilies {
		if _, exists := preserved[family]; !exists || slices.Contains(value.InvalidatedFamilies, family) {
			return errors.New("structure identity Gate impact overlaps")
		}
	}
	for _, family := range value.InvalidatedFamilies {
		if _, exists := invalidated[family]; !exists {
			return errors.New("invalid structure identity Gate invalidated family")
		}
	}
	return nil
}

func validStructureIdentityDecisions(values []string) bool {
	if len(values) < 1 || len(values) > 3 || !slices.Contains(values, "rejected") {
		return false
	}
	for index, value := range values {
		if (value != "approved" && value != "changes_requested" && value != "rejected") ||
			(index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

var structureIdentityIssueKeyPattern = regexp.MustCompile(`^issue_[a-z0-9_]{1,80}$`)
var structureIdentityIssueCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,80}$`)

func normalizeStructureIdentityRepairOptions(value *StructureIdentityGateInput) error {
	slices.SortFunc(value.RepairOptions, func(left, right StructureIdentityRepairOption) int {
		return cmp.Compare(left.IssueKey, right.IssueKey)
	})
	for index := range value.RepairOptions {
		option := &value.RepairOptions[index]
		option.Scope = strings.TrimSpace(option.Scope)
		option.Summary = strings.TrimSpace(option.Summary)
		slices.SortFunc(option.EvidenceRefs, compareHumanGateEvidenceRef)
		if !structureIdentityIssueKeyPattern.MatchString(option.IssueKey) ||
			!structureIdentityIssueCodePattern.MatchString(option.Code) ||
			(option.Severity != "warning" && option.Severity != "blocking") ||
			option.Scope == "" || option.Summary == "" || len(option.EvidenceRefs) == 0 ||
			len(option.AllowedChanges) == 0 ||
			(index > 0 && value.RepairOptions[index-1].IssueKey == option.IssueKey) {
			return errors.New("invalid structure identity Gate repair option")
		}
		for evidenceIndex, evidence := range option.EvidenceRefs {
			if !slices.Contains(value.EvidenceRefs, evidence) ||
				(evidenceIndex > 0 && compareHumanGateEvidenceRef(option.EvidenceRefs[evidenceIndex-1], evidence) == 0) {
				return errors.New("structure identity Gate repair evidence is outside the frozen evidence")
			}
		}
		for changeIndex := range option.AllowedChanges {
			change := &option.AllowedChanges[changeIndex]
			change.Operation = strings.TrimSpace(change.Operation)
			for targetIndex := range change.TargetKeys {
				change.TargetKeys[targetIndex] = strings.TrimSpace(change.TargetKeys[targetIndex])
			}
			for scopeIndex := range change.AffectedScopeKeys {
				change.AffectedScopeKeys[scopeIndex] = strings.TrimSpace(change.AffectedScopeKeys[scopeIndex])
			}
			slices.Sort(change.TargetKeys)
			slices.Sort(change.AffectedScopeKeys)
		}
		slices.SortFunc(option.AllowedChanges, compareStructureIdentityAllowedChange)
		for changeIndex := range option.AllowedChanges {
			change := &option.AllowedChanges[changeIndex]
			if !validStructureIdentityRepairOperation(change.Operation) ||
				!validUniqueNonemptyStrings(change.TargetKeys) || !validUniqueNonemptyStrings(change.AffectedScopeKeys) ||
				!allStringsContained(value.Impact.AffectedScopeKeys, change.AffectedScopeKeys) ||
				(changeIndex > 0 && compareStructureIdentityAllowedChange(option.AllowedChanges[changeIndex-1], *change) == 0) {
				return errors.New("invalid structure identity Gate allowed change")
			}
		}
	}
	if slices.Contains(value.AllowedDecisions, "changes_requested") != (len(value.RepairOptions) > 0) {
		return errors.New("structure identity Gate changes decision does not match repair options")
	}
	return nil
}

func validStructureIdentityRepairOperation(value string) bool {
	return slices.Contains([]string{
		"inspect_source", "adjust_episode_boundary", "adjust_scene_boundary", "separate_identity",
		"merge_identity", "resolve_mention", "reject_mention",
	}, value)
}

func ValidateStructureIdentityChangeRequest(
	gate StructureIdentityGateInput,
	request StructureIdentityChangeRequest,
) error {
	if len(request.IssueRefs) == 0 || !slices.IsSorted(request.IssueRefs) ||
		!validUniqueNonemptyStrings(request.IssueRefs) || len(request.EvidenceRefs) == 0 ||
		!slices.IsSortedFunc(request.EvidenceRefs, compareHumanGateEvidenceRef) {
		return errors.New("invalid structure identity change request")
	}
	for index, evidence := range request.EvidenceRefs {
		if index > 0 && compareHumanGateEvidenceRef(request.EvidenceRefs[index-1], evidence) == 0 {
			return errors.New("invalid structure identity change request evidence")
		}
	}
	if !validStructureIdentityRepairOperation(request.ChangeSpec.Operation) ||
		!slices.IsSorted(request.ChangeSpec.TargetKeys) ||
		!validUniqueNonemptyStrings(request.ChangeSpec.TargetKeys) ||
		!slices.IsSorted(request.ChangeSpec.AffectedScopeKeys) ||
		!validUniqueNonemptyStrings(request.ChangeSpec.AffectedScopeKeys) ||
		!validStructureIdentityReason(request.ChangeSpec.Operation, request.ReasonCode) {
		return errors.New("invalid structure identity change spec")
	}
	if request.UserNote != nil {
		note := strings.TrimSpace(*request.UserNote)
		if note == "" || note != *request.UserNote || len([]rune(note)) > 1000 {
			return errors.New("invalid structure identity change request note")
		}
	}

	options := make(map[string]StructureIdentityRepairOption, len(gate.RepairOptions))
	for _, option := range gate.RepairOptions {
		options[option.IssueKey] = option
	}
	expectedEvidence := make(map[HumanGateEvidenceRef]struct{})
	for _, issueRef := range request.IssueRefs {
		option, exists := options[issueRef]
		if !exists || !slices.ContainsFunc(option.AllowedChanges, func(change StructureIdentityAllowedChange) bool {
			return compareStructureIdentityAllowedChange(change, request.ChangeSpec) == 0
		}) {
			return errors.New("structure identity change request is outside the frozen repair options")
		}
		for _, evidence := range option.EvidenceRefs {
			expectedEvidence[evidence] = struct{}{}
		}
	}
	if len(expectedEvidence) != len(request.EvidenceRefs) {
		return errors.New("structure identity change request evidence has drifted")
	}
	for _, evidence := range request.EvidenceRefs {
		if _, exists := expectedEvidence[evidence]; !exists {
			return errors.New("structure identity change request evidence has drifted")
		}
	}
	return nil
}

func validStructureIdentityReason(operation, reason string) bool {
	reason = strings.TrimSpace(reason)
	switch operation {
	case "inspect_source":
		return reason == "source_interpretation_incorrect" || reason == "insufficient_evidence"
	case "adjust_episode_boundary", "adjust_scene_boundary":
		return reason == "structure_boundary_incorrect"
	case "separate_identity", "merge_identity", "resolve_mention", "reject_mention":
		return reason == "identity_resolution_incorrect"
	default:
		return false
	}
}

func validUniqueNonemptyStrings(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for index, value := range values {
		if value == "" || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func allStringsContained(universe, values []string) bool {
	for _, value := range values {
		if !slices.Contains(universe, value) {
			return false
		}
	}
	return true
}

func compareStructureIdentityAllowedChange(left, right StructureIdentityAllowedChange) int {
	if result := cmp.Compare(left.Operation, right.Operation); result != 0 {
		return result
	}
	if result := cmp.Compare(strings.Join(left.TargetKeys, "\x00"), strings.Join(right.TargetKeys, "\x00")); result != 0 {
		return result
	}
	return cmp.Compare(strings.Join(left.AffectedScopeKeys, "\x00"), strings.Join(right.AffectedScopeKeys, "\x00"))
}

func cloneStructureIdentityRepairOptions(values []StructureIdentityRepairOption) []StructureIdentityRepairOption {
	result := make([]StructureIdentityRepairOption, len(values))
	for index, option := range values {
		result[index] = option
		result[index].EvidenceRefs = append([]HumanGateEvidenceRef(nil), option.EvidenceRefs...)
		result[index].AllowedChanges = make([]StructureIdentityAllowedChange, len(option.AllowedChanges))
		for changeIndex, change := range option.AllowedChanges {
			result[index].AllowedChanges[changeIndex] = StructureIdentityAllowedChange{
				Operation:         change.Operation,
				TargetKeys:        append([]string(nil), change.TargetKeys...),
				AffectedScopeKeys: append([]string(nil), change.AffectedScopeKeys...),
			}
		}
	}
	return result
}

func newStructureIdentityEffectPlan(
	readSetHash string,
	expectedProjectHead, expectedBibleHead HumanGateExpectedHead,
) StructureIdentityEffectPlan {
	return StructureIdentityEffectPlan{
		PlanKey: structureIdentityEffectPlanKey,
		Steps: []StructureIdentityEffectStep{
			{
				StepKey: confirmProjectEpisodeLifecycleStep, Position: 1,
				OwnerKind: "production/project", OwnerCommand: "confirm_project_episode_lifecycle",
				ExpectedHead: expectedProjectHead, ReadSetHash: readSetHash,
				DependsOnStepKeys: []string{}, RecoveryTarget: "project_episode_lifecycle_receipt",
			},
			{
				StepKey: confirmStructureIdentitySetStep, Position: 2,
				OwnerKind: "production/bible", OwnerCommand: "confirm_structure_identity_set",
				ExpectedHead: expectedBibleHead, ReadSetHash: readSetHash,
				DependsOnStepKeys: []string{confirmProjectEpisodeLifecycleStep},
				RecoveryTarget:    "structure_identity_set_receipt",
			},
		},
	}
}

func validateStructureIdentityEffectPlan(
	value StructureIdentityEffectPlan,
	projectID, readSetHash string,
) error {
	if value.PlanKey != structureIdentityEffectPlanKey || len(value.Steps) != 2 ||
		!nodeOutputContentHashPattern.MatchString(readSetHash) {
		return errors.New("invalid structure identity Gate effect plan")
	}
	expected := newStructureIdentityEffectPlan(readSetHash, value.Steps[0].ExpectedHead, value.Steps[1].ExpectedHead)
	for index := range value.Steps {
		step := &value.Steps[index]
		step.ExpectedHead.OwnerKind = strings.TrimSpace(step.ExpectedHead.OwnerKind)
		step.ExpectedHead.LogicalID = strings.TrimSpace(step.ExpectedHead.LogicalID)
		if step.ExpectedHead.LogicalID != projectID || validateHumanGateExpectedHead(step.ExpectedHead) != nil {
			return errors.New("invalid structure identity Gate expected Head")
		}
		expected.Steps[index].ExpectedHead = step.ExpectedHead
	}
	if !reflect.DeepEqual(value, expected) {
		return errors.New("structure identity Gate effect steps drifted")
	}
	return nil
}

func validateHumanGateExpectedHead(value HumanGateExpectedHead) error {
	if (value.OwnerKind != "production/project" && value.OwnerKind != "production/bible") || value.Revision < 0 {
		return errors.New("invalid human Gate expected Head")
	}
	if _, err := uuid.Parse(value.LogicalID); err != nil {
		return errors.New("invalid human Gate expected Head")
	}
	if (value.Revision == 0 && value.ContentHash != "") ||
		(value.Revision > 0 && !nodeOutputContentHashPattern.MatchString(value.ContentHash)) {
		return errors.New("invalid human Gate expected Head")
	}
	return nil
}

func compareHumanGateEvidenceRef(left, right HumanGateEvidenceRef) int {
	if result := cmp.Compare(left.SourceVersionID, right.SourceVersionID); result != 0 {
		return result
	}
	if result := cmp.Compare(left.SourceStart, right.SourceStart); result != 0 {
		return result
	}
	if result := cmp.Compare(left.SourceEnd, right.SourceEnd); result != 0 {
		return result
	}
	return cmp.Compare(left.TextHash, right.TextHash)
}

func hashStructureIdentityGateMaterial(value any) string {
	encoded, _ := json.Marshal(value)
	return sha256Hex(encoded)
}

func structureIdentityGateInputHashMaterial(value StructureIdentityGateInput) any {
	return struct {
		SchemaVersion    string                          `json:"schema_version"`
		GateKey          string                          `json:"gate_key"`
		GateInstanceKey  string                          `json:"gate_instance_key"`
		WorkspaceID      string                          `json:"workspace_id"`
		ProjectID        string                          `json:"project_id"`
		WorkflowRunID    string                          `json:"workflow_run_id"`
		NodeRunID        string                          `json:"node_run_id"`
		SubjectType      string                          `json:"subject_type"`
		Subject          StructureIdentityGateSubject    `json:"subject"`
		SubjectHash      string                          `json:"subject_hash"`
		EvidenceRefs     []HumanGateEvidenceRef          `json:"evidence_refs"`
		Impact           HumanGateImpactSummary          `json:"impact_summary"`
		RepairOptions    []StructureIdentityRepairOption `json:"repair_options"`
		AllowedDecisions []string                        `json:"allowed_decisions"`
		EffectPlan       StructureIdentityEffectPlan     `json:"effect_plan"`
		EffectPlanHash   string                          `json:"effect_plan_hash"`
	}{
		value.SchemaVersion, value.GateKey, value.GateInstanceKey,
		value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID,
		value.SubjectType, value.Subject, value.SubjectHash, value.EvidenceRefs,
		value.Impact, value.RepairOptions, value.AllowedDecisions, value.EffectPlan, value.EffectPlanHash,
	}
}
