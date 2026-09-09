package domain

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

const (
	ProductionWorldGateInputSchemaVersion = "production-world-human-gate-input-production"
	ProductionWorldGateKey                = "bible_continuity"
	productionWorldSubjectType            = "production_world"
	productionWorldEffectPlanKey          = "bible_continuity"
	confirmProductionWorldStep            = "confirm_production_world"
)

type ProductionWorldCandidateRevisionRef struct {
	CandidateRevisionID   string `json:"candidate_revision_id"`
	CandidateRevision     int64  `json:"candidate_revision"`
	CandidateRevisionHash string `json:"candidate_revision_hash"`
	CandidateContentHash  string `json:"candidate_content_hash"`
}

type ProductionWorldCandidateProjectionRef struct {
	Candidate      agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"candidate"`
	ProjectionKind string                                               `json:"projection_kind"`
	ProjectionHash string                                               `json:"projection_hash"`
}

type ProductionWorldBusinessKeyRoot struct {
	Partition string `json:"partition"`
	Root      string `json:"root"`
}

type ProductionWorldExpectedHead struct {
	OwnerKind     string `json:"owner_kind"`
	VersionFamily string `json:"version_family"`
	ScopeKind     string `json:"scope_kind"`
	ScopeKey      string `json:"scope_key"`
	Revision      int64  `json:"revision"`
	ContentHash   string `json:"content_hash"`
}

type ProductionWorldGateSubject struct {
	SourceVersion               agentcontract.ScriptSourceVersionIdentity            `json:"source_version"`
	StructureIdentitySetVersion worlddomain.ProductionWorldOwnerVersionRef           `json:"structure_identity_set_version"`
	ProductionWorldCandidate    ProductionWorldCandidateRevisionRef                  `json:"production_world_candidate"`
	SceneOccurrenceCandidate    agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_occurrence_candidate"`
	InteractionCandidate        ProductionWorldCandidateProjectionRef                `json:"interaction_candidate"`
	ContinuityCandidate         ProductionWorldCandidateProjectionRef                `json:"continuity_candidate"`
	ExpectedBusinessKeyRoots    []ProductionWorldBusinessKeyRoot                     `json:"expected_business_key_roots"`
	PlanningEpisodeScopes       []worlddomain.ProductionWorldPlanningEpisodeScope    `json:"planning_episode_scopes"`
	ScopeClosureRoot            string                                               `json:"scope_closure_root"`
	RepairTargets               []ProductionWorldRepairTargetSet                     `json:"repair_targets"`
	ExpectedHeads               []ProductionWorldExpectedHead                        `json:"expected_heads"`
	ReadSetRoot                 string                                               `json:"read_set_root"`
}

type ProductionWorldAtomicEffectStep struct {
	StepKey       string                        `json:"step_key"`
	OwnerKinds    []string                      `json:"owner_kinds"`
	OwnerCommand  string                        `json:"owner_command"`
	ExpectedHeads []ProductionWorldExpectedHead `json:"expected_heads"`
	ReadSetRoot   string                        `json:"read_set_root"`
}

type ProductionWorldEffectPlan struct {
	PlanKey    string                          `json:"plan_key"`
	AtomicStep ProductionWorldAtomicEffectStep `json:"atomic_step"`
}

type ProductionWorldGateInputDraft struct {
	WorkspaceID, ProjectID, WorkflowRunID, NodeRunID string
	CandidateRevisionID                              string
	CandidateRevision                                int64
	CandidateRevisionHash                            string
	Candidate                                        worlddomain.ProductionWorldCandidate
	AllowedDecisions                                 []string
	ExpectedHeads                                    []ProductionWorldExpectedHead
}

type ProductionWorldGateInput struct {
	SchemaVersion    string                     `json:"schema_version"`
	GateKey          string                     `json:"gate_key"`
	GateInstanceKey  string                     `json:"gate_instance_key"`
	WorkspaceID      string                     `json:"workspace_id"`
	ProjectID        string                     `json:"project_id"`
	WorkflowRunID    string                     `json:"workflow_run_id"`
	NodeRunID        string                     `json:"node_run_id"`
	SubjectType      string                     `json:"subject_type"`
	Subject          ProductionWorldGateSubject `json:"subject"`
	SubjectHash      string                     `json:"subject_hash"`
	AllowedDecisions []string                   `json:"allowed_decisions"`
	EffectPlan       ProductionWorldEffectPlan  `json:"effect_plan"`
	EffectPlanHash   string                     `json:"effect_plan_hash"`
	InputHash        string                     `json:"input_hash"`
}

func NewProductionWorldGateInput(
	draft ProductionWorldGateInputDraft,
) (ProductionWorldGateInput, json.RawMessage, error) {
	candidateJSON, err := json.Marshal(draft.Candidate)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	candidate, _, err := worlddomain.DecodeProductionWorldCandidate(candidateJSON)
	if err != nil || candidate.WorkspaceID != draft.WorkspaceID || candidate.ProjectID != draft.ProjectID {
		return ProductionWorldGateInput{}, nil, errors.New("invalid Gate 2 Production World Candidate")
	}
	candidateRef := ProductionWorldCandidateRevisionRef{
		CandidateRevisionID: draft.CandidateRevisionID, CandidateRevision: draft.CandidateRevision,
		CandidateRevisionHash: draft.CandidateRevisionHash, CandidateContentHash: candidate.ContentHash,
	}
	views, err := productionWorldReviewViews(candidate)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	repairTargets, err := productionWorldRepairTargetSets(views)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	value := ProductionWorldGateInput{
		SchemaVersion: ProductionWorldGateInputSchemaVersion, GateKey: ProductionWorldGateKey,
		WorkspaceID: draft.WorkspaceID, ProjectID: draft.ProjectID,
		WorkflowRunID: draft.WorkflowRunID, NodeRunID: draft.NodeRunID,
		SubjectType: productionWorldSubjectType,
		Subject: ProductionWorldGateSubject{
			SourceVersion:               candidate.SourceVersion,
			StructureIdentitySetVersion: candidate.StructureIdentitySetVersion,
			ProductionWorldCandidate:    candidateRef,
			SceneOccurrenceCandidate:    candidate.UpstreamCandidates.SceneOccurrence,
			InteractionCandidate: ProductionWorldCandidateProjectionRef{
				Candidate:      candidate.UpstreamCandidates.InteractionContinuity,
				ProjectionKind: "interaction", ProjectionHash: candidate.InteractionProjectionHash,
			},
			ContinuityCandidate: ProductionWorldCandidateProjectionRef{
				Candidate:      candidate.UpstreamCandidates.InteractionContinuity,
				ProjectionKind: "continuity", ProjectionHash: candidate.ContinuityProjectionHash,
			},
			ExpectedBusinessKeyRoots: productionWorldBusinessKeyRoots(candidate.SharedProof.ExpectedBusinessKeyRoots),
			PlanningEpisodeScopes:    append([]worlddomain.ProductionWorldPlanningEpisodeScope(nil), candidate.SharedProof.PlanningEpisodeScopes...),
			ScopeClosureRoot:         candidate.SharedProof.ScopeClosureRoot,
			RepairTargets:            repairTargets,
			ExpectedHeads:            append([]ProductionWorldExpectedHead(nil), draft.ExpectedHeads...),
		},
		AllowedDecisions: append([]string(nil), draft.AllowedDecisions...),
	}
	if err = completeProductionWorldGateInput(&value, false); err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	return value, encoded, nil
}

func DecodeProductionWorldGateInput(
	raw json.RawMessage,
) (ProductionWorldGateInput, json.RawMessage, error) {
	var value ProductionWorldGateInput
	if err := decodeProductionWorldJSON(raw, &value); err != nil {
		return ProductionWorldGateInput{}, nil, errors.New("invalid Gate 2 input")
	}
	original := value
	if err := completeProductionWorldGateInput(&value, true); err != nil || !reflect.DeepEqual(original, value) {
		return ProductionWorldGateInput{}, nil, errors.New("Gate 2 input hash mismatch")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ProductionWorldGateInput{}, nil, err
	}
	return value, encoded, nil
}

func completeProductionWorldGateInput(value *ProductionWorldGateInput, verify bool) error {
	if value.SchemaVersion != ProductionWorldGateInputSchemaVersion || value.GateKey != ProductionWorldGateKey ||
		value.SubjectType != productionWorldSubjectType {
		return errors.New("invalid Gate 2 identity")
	}
	for _, identifier := range []string{value.WorkspaceID, value.ProjectID, value.WorkflowRunID, value.NodeRunID} {
		if _, err := uuid.Parse(identifier); err != nil {
			return errors.New("invalid Gate 2 workflow identity")
		}
	}
	if value.Subject.SourceVersion.Validate() != nil ||
		value.Subject.StructureIdentitySetVersion.OwnerKind != "production/bible" ||
		value.Subject.StructureIdentitySetVersion.LogicalID != value.ProjectID ||
		value.Subject.StructureIdentitySetVersion.Revision < 1 ||
		!nodeOutputContentHashPattern.MatchString(value.Subject.StructureIdentitySetVersion.ContentHash) ||
		validateProductionWorldCandidateRevisionRef(value.Subject.ProductionWorldCandidate) != nil ||
		value.Subject.SceneOccurrenceCandidate.Validate() != nil ||
		value.Subject.SceneOccurrenceCandidate.StageKey != "bind_scene_occurrences" ||
		validateProductionWorldProjection(value.Subject.InteractionCandidate, "interaction") != nil ||
		validateProductionWorldProjection(value.Subject.ContinuityCandidate, "continuity") != nil ||
		value.Subject.InteractionCandidate.Candidate != value.Subject.ContinuityCandidate.Candidate ||
		!nodeOutputContentHashPattern.MatchString(value.Subject.ScopeClosureRoot) ||
		validateProductionWorldRepairTargetSets(value.Subject.RepairTargets) != nil ||
		len(value.Subject.ExpectedBusinessKeyRoots) != 3 ||
		validateProductionWorldPlanningEpisodeScopes(value.Subject.PlanningEpisodeScopes, value.Subject.ScopeClosureRoot) != nil {
		return errors.New("invalid Gate 2 Subject")
	}
	if _, err := uuid.Parse(value.Subject.StructureIdentitySetVersion.VersionID); err != nil {
		return errors.New("invalid Gate 2 formal version")
	}
	for index, expected := range []string{"asset", "bible", "planning"} {
		root := value.Subject.ExpectedBusinessKeyRoots[index]
		if root.Partition != expected || !nodeOutputContentHashPattern.MatchString(root.Root) {
			return errors.New("invalid Gate 2 expected business key roots")
		}
	}
	for index := range value.AllowedDecisions {
		value.AllowedDecisions[index] = strings.ToLower(strings.TrimSpace(value.AllowedDecisions[index]))
	}
	slices.Sort(value.AllowedDecisions)
	if !slices.Equal(value.AllowedDecisions, []string{"approved", "rejected"}) {
		return errors.New("Gate 2 requires exact typed decisions")
	}
	slices.SortFunc(value.Subject.ExpectedHeads, compareProductionWorldExpectedHead)
	if err := validateProductionWorldExpectedHeads(value.Subject.ExpectedHeads, value.ProjectID, value.Subject.PlanningEpisodeScopes); err != nil {
		return err
	}
	readSetRoot := hashProductionWorldMaterial(productionWorldReadSetMaterial(value.Subject))
	if verify && value.Subject.ReadSetRoot != readSetRoot {
		return errors.New("Gate 2 read set drifted")
	}
	value.Subject.ReadSetRoot = readSetRoot
	instanceKey := ProductionWorldGateKey + ":" + value.ProjectID + ":" + value.Subject.ProductionWorldCandidate.CandidateRevisionHash
	subjectHash := hashProductionWorldMaterial(value.Subject)
	plan := ProductionWorldEffectPlan{
		PlanKey: productionWorldEffectPlanKey,
		AtomicStep: ProductionWorldAtomicEffectStep{
			StepKey:       confirmProductionWorldStep,
			OwnerKinds:    []string{"asset", "production/bible", "production/planning"},
			OwnerCommand:  "confirm_production_world",
			ExpectedHeads: append([]ProductionWorldExpectedHead(nil), value.Subject.ExpectedHeads...),
			ReadSetRoot:   readSetRoot,
		},
	}
	planHash := hashProductionWorldMaterial(plan)
	if verify && (value.GateInstanceKey != instanceKey || value.SubjectHash != subjectHash ||
		!reflect.DeepEqual(value.EffectPlan, plan) || value.EffectPlanHash != planHash) {
		return errors.New("Gate 2 effect plan drifted")
	}
	value.GateInstanceKey, value.SubjectHash = instanceKey, subjectHash
	value.EffectPlan, value.EffectPlanHash = plan, planHash
	originalInputHash := value.InputHash
	value.InputHash = ""
	inputHash := hashProductionWorldMaterial(productionWorldGateInputHashMaterial(*value))
	if verify && originalInputHash != inputHash {
		return errors.New("Gate 2 input content drifted")
	}
	value.InputHash = inputHash
	return nil
}

func validateProductionWorldCandidateRevisionRef(value ProductionWorldCandidateRevisionRef) error {
	if value.CandidateRevision < 1 || !nodeOutputContentHashPattern.MatchString(value.CandidateRevisionHash) ||
		!nodeOutputContentHashPattern.MatchString(value.CandidateContentHash) {
		return errors.New("invalid Production World Candidate revision")
	}
	if _, err := uuid.Parse(value.CandidateRevisionID); err != nil {
		return errors.New("invalid Production World Candidate revision")
	}
	return nil
}

func validateProductionWorldProjection(value ProductionWorldCandidateProjectionRef, kind string) error {
	if value.Candidate.Validate() != nil || value.Candidate.StageKey != "reconcile_interaction_continuity" ||
		value.Candidate.ShardKey != "script:full" || value.ProjectionKind != kind ||
		!nodeOutputContentHashPattern.MatchString(value.ProjectionHash) {
		return errors.New("invalid Production World Candidate projection")
	}
	return nil
}

func validateProductionWorldPlanningEpisodeScopes(
	values []worlddomain.ProductionWorldPlanningEpisodeScope,
	scopeClosureRoot string,
) error {
	if len(values) == 0 {
		return errors.New("Gate 2 Planning Episode scope set is empty")
	}
	allScenes := make([]string, 0)
	previousScope := ""
	for _, value := range values {
		if value.ScopeKey <= previousScope || value.ScopeKey != "episode:"+value.EpisodeID ||
			len(value.SceneScopeKeys) == 0 || !slices.IsSorted(value.SceneScopeKeys) {
			return errors.New("Gate 2 Planning Episode scope set is invalid")
		}
		if _, err := uuid.Parse(value.EpisodeID); err != nil {
			return errors.New("Gate 2 Planning Episode identity is invalid")
		}
		for index, scene := range value.SceneScopeKeys {
			if strings.TrimSpace(scene) == "" || (index > 0 && value.SceneScopeKeys[index-1] == scene) {
				return errors.New("Gate 2 Planning Episode Scene set is invalid")
			}
			allScenes = append(allScenes, scene)
		}
		previousScope = value.ScopeKey
	}
	slices.Sort(allScenes)
	if !uniqueNonempty(allScenes) || hashProductionWorldMaterial(allScenes) != scopeClosureRoot {
		return errors.New("Gate 2 Planning Episode scope closure has drifted")
	}
	return nil
}

func compareProductionWorldExpectedHead(left, right ProductionWorldExpectedHead) int {
	if result := cmp.Compare(left.OwnerKind, right.OwnerKind); result != 0 {
		return result
	}
	if result := cmp.Compare(left.VersionFamily, right.VersionFamily); result != 0 {
		return result
	}
	return cmp.Compare(left.ScopeKey, right.ScopeKey)
}

func validateProductionWorldExpectedHeads(
	values []ProductionWorldExpectedHead,
	projectID string,
	episodes []worlddomain.ProductionWorldPlanningEpisodeScope,
) error {
	expected := []ProductionWorldExpectedHead{
		{OwnerKind: "asset", VersionFamily: "asset_identity_state_set", ScopeKind: "project", ScopeKey: "project:" + projectID},
		{OwnerKind: "production/bible", VersionFamily: "bible_production_world_set", ScopeKind: "project", ScopeKey: "project:" + projectID},
	}
	for _, episode := range episodes {
		expected = append(expected, ProductionWorldExpectedHead{
			OwnerKind: "production/planning", VersionFamily: "planning_scene_set",
			ScopeKind: "episode", ScopeKey: episode.ScopeKey,
		})
	}
	expected = append(expected, ProductionWorldExpectedHead{
		OwnerKind: "production/planning", VersionFamily: "planning_structure_rebase_set",
		ScopeKind: "project", ScopeKey: "project:" + projectID,
	})
	if len(values) != len(expected) {
		return errors.New("Gate 2 expected Head set is incomplete")
	}
	for index, value := range values {
		identity := expected[index]
		if value.OwnerKind != identity.OwnerKind || value.VersionFamily != identity.VersionFamily ||
			value.ScopeKind != identity.ScopeKind || value.ScopeKey != identity.ScopeKey || value.Revision < 0 ||
			(value.Revision == 0 && value.ContentHash != "") ||
			(value.Revision > 0 && !nodeOutputContentHashPattern.MatchString(value.ContentHash)) {
			return errors.New("invalid Gate 2 expected Head")
		}
	}
	return nil
}

func productionWorldReadSetMaterial(value ProductionWorldGateSubject) any {
	return struct {
		SourceVersion               agentcontract.ScriptSourceVersionIdentity            `json:"source_version"`
		StructureIdentitySetVersion worlddomain.ProductionWorldOwnerVersionRef           `json:"structure_identity_set_version"`
		ProductionWorldCandidate    ProductionWorldCandidateRevisionRef                  `json:"production_world_candidate"`
		SceneOccurrenceCandidate    agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"scene_occurrence_candidate"`
		InteractionCandidate        ProductionWorldCandidateProjectionRef                `json:"interaction_candidate"`
		ContinuityCandidate         ProductionWorldCandidateProjectionRef                `json:"continuity_candidate"`
		ExpectedBusinessKeyRoots    []ProductionWorldBusinessKeyRoot                     `json:"expected_business_key_roots"`
		PlanningEpisodeScopes       []worlddomain.ProductionWorldPlanningEpisodeScope    `json:"planning_episode_scopes"`
		ScopeClosureRoot            string                                               `json:"scope_closure_root"`
		RepairTargets               []ProductionWorldRepairTargetSet                     `json:"repair_targets"`
		ExpectedHeads               []ProductionWorldExpectedHead                        `json:"expected_heads"`
	}{
		SourceVersion: value.SourceVersion, StructureIdentitySetVersion: value.StructureIdentitySetVersion,
		ProductionWorldCandidate: value.ProductionWorldCandidate, SceneOccurrenceCandidate: value.SceneOccurrenceCandidate,
		InteractionCandidate: value.InteractionCandidate, ContinuityCandidate: value.ContinuityCandidate,
		ExpectedBusinessKeyRoots: value.ExpectedBusinessKeyRoots, PlanningEpisodeScopes: value.PlanningEpisodeScopes,
		ScopeClosureRoot: value.ScopeClosureRoot, RepairTargets: value.RepairTargets, ExpectedHeads: value.ExpectedHeads,
	}
}

func cloneProductionWorldRepairTargetSets(values []ProductionWorldRepairTargetSet) []ProductionWorldRepairTargetSet {
	result := make([]ProductionWorldRepairTargetSet, len(values))
	for index, value := range values {
		targets := make([]string, len(value.TargetKeys))
		copy(targets, value.TargetKeys)
		result[index] = ProductionWorldRepairTargetSet{
			Operation: value.Operation, TargetKeys: targets,
		}
	}
	return result
}

func productionWorldBusinessKeyRoots(
	sets []worlddomain.ProductionWorldExpectedBusinessKeySet,
) []ProductionWorldBusinessKeyRoot {
	result := make([]ProductionWorldBusinessKeyRoot, len(sets))
	for index, set := range sets {
		result[index] = ProductionWorldBusinessKeyRoot{Partition: set.Partition, Root: set.Root}
	}
	return result
}

func productionWorldGateInputHashMaterial(value ProductionWorldGateInput) any {
	return struct {
		SchemaVersion    string                     `json:"schema_version"`
		GateKey          string                     `json:"gate_key"`
		GateInstanceKey  string                     `json:"gate_instance_key"`
		WorkspaceID      string                     `json:"workspace_id"`
		ProjectID        string                     `json:"project_id"`
		WorkflowRunID    string                     `json:"workflow_run_id"`
		NodeRunID        string                     `json:"node_run_id"`
		SubjectType      string                     `json:"subject_type"`
		Subject          ProductionWorldGateSubject `json:"subject"`
		SubjectHash      string                     `json:"subject_hash"`
		AllowedDecisions []string                   `json:"allowed_decisions"`
		EffectPlan       ProductionWorldEffectPlan  `json:"effect_plan"`
		EffectPlanHash   string                     `json:"effect_plan_hash"`
	}{
		value.SchemaVersion, value.GateKey, value.GateInstanceKey, value.WorkspaceID, value.ProjectID,
		value.WorkflowRunID, value.NodeRunID, value.SubjectType, value.Subject, value.SubjectHash,
		value.AllowedDecisions, value.EffectPlan, value.EffectPlanHash,
	}
}

func decodeProductionWorldJSON(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func uniqueNonempty(values []string) bool {
	if len(values) == 0 || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || (index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func hashProductionWorldMaterial(value any) string {
	encoded, _ := json.Marshal(value)
	return sha256Hex(encoded)
}
