package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
)

const (
	ProductionWorldRepairReviseEntity      = "revise_production_entity"
	ProductionWorldRepairRebindOccurrence  = "rebind_scene_occurrence"
	ProductionWorldRepairReviseInteraction = "revise_interaction"
	ProductionWorldRepairReviseContinuity  = "revise_continuity"
)

type ProductionWorldRepairChange struct {
	Operation  string   `json:"operation"`
	TargetKeys []string `json:"target_keys"`
}

type ProductionWorldRepairClosure struct {
	SceneScopeKeys  []string `json:"scene_scope_keys"`
	EntityKeys      []string `json:"entity_keys"`
	StateKeys       []string `json:"state_keys"`
	OccurrenceKeys  []string `json:"occurrence_keys"`
	InteractionKeys []string `json:"interaction_keys"`
	ContinuityKeys  []string `json:"continuity_keys"`
	LedgerKeys      []string `json:"ledger_keys"`
}

type ProductionWorldRepairBaseCandidate struct {
	Identity             SceneAnalysisCandidateRevisionIdentity `json:"identity"`
	CandidateType        string                                 `json:"candidate_type"`
	CandidateContentHash string                                 `json:"candidate_content_hash"`
	Candidate            json.RawMessage                        `json:"candidate"`
}

type ProductionWorldRepairDirective struct {
	ReviewDecisionID    string                             `json:"review_decision_id"`
	DecisionPayloadHash string                             `json:"decision_payload_hash"`
	IssueRefs           []string                           `json:"issue_refs"`
	EvidenceRefs        []StructureIdentityRepairEvidence  `json:"evidence_refs"`
	ChangeSpec          ProductionWorldRepairChange        `json:"change_spec"`
	Closure             ProductionWorldRepairClosure       `json:"closure"`
	ReasonCode          string                             `json:"reason_code"`
	BaseCandidate       ProductionWorldRepairBaseCandidate `json:"base_candidate"`
}

func (value ProductionWorldRepairDirective) ValidateFor(stageKey string) error {
	if _, err := uuid.Parse(value.ReviewDecisionID); err != nil ||
		!hashPattern.MatchString(value.DecisionPayloadHash) || value.IssueRefs == nil ||
		len(value.EvidenceRefs) == 0 || !validSortedProductionWorldKeys(value.ChangeSpec.TargetKeys, false) ||
		!validProductionWorldRepairReason(value.ChangeSpec.Operation, value.ReasonCode) {
		return errors.New("invalid Production World repair directive")
	}
	for index, issue := range value.IssueRefs {
		if strings.TrimSpace(issue) == "" || (index > 0 && value.IssueRefs[index-1] >= issue) {
			return errors.New("invalid Production World repair issue")
		}
	}
	for index, evidence := range value.EvidenceRefs {
		if _, err := uuid.Parse(evidence.SourceVersionID); err != nil || evidence.SourceStart < 0 ||
			evidence.SourceEnd <= evidence.SourceStart || !hashPattern.MatchString(evidence.TextHash) ||
			(index > 0 && compareStructureIdentityRepairEvidence(value.EvidenceRefs[index-1], evidence) >= 0) {
			return errors.New("invalid Production World repair evidence")
		}
	}
	if err := value.Closure.validate(); err != nil || !productionWorldTargetsInsideClosure(value.ChangeSpec, value.Closure) {
		return errors.New("invalid Production World repair closure")
	}
	rootStage := productionWorldRepairRootStage(value.ChangeSpec.Operation)
	if rootStage == "" || !productionWorldRepairStageContains(stageKey, rootStage) {
		return errors.New("Production World repair operation targets another stage")
	}
	if err := value.BaseCandidate.validateFor(stageKey); err != nil {
		return err
	}
	return nil
}

func (value ProductionWorldRepairClosure) validate() error {
	collections := [][]string{
		value.SceneScopeKeys, value.EntityKeys, value.StateKeys, value.OccurrenceKeys,
		value.InteractionKeys, value.ContinuityKeys, value.LedgerKeys,
	}
	for _, collection := range collections {
		if !validSortedProductionWorldKeys(collection, true) {
			return errors.New("invalid Production World repair closure keys")
		}
	}
	return nil
}

func (value ProductionWorldRepairBaseCandidate) validateFor(stageKey string) error {
	expectedType := map[string]string{
		"derive_production_entities":       "production_entity_fragment_candidate",
		"bind_scene_occurrences":           "scene_binding_fragment_candidate",
		"reconcile_interaction_continuity": "continuity_fragment_candidate",
	}[stageKey]
	if expectedType == "" || value.Identity.Validate() != nil || value.Identity.StageKey != stageKey ||
		value.Identity.ShardKey != "script:full" || value.CandidateType != expectedType ||
		!hashPattern.MatchString(value.CandidateContentHash) || len(value.Candidate) == 0 {
		return errors.New("invalid Production World repair base Candidate")
	}
	decoder := json.NewDecoder(bytes.NewReader(value.Candidate))
	decoder.UseNumber()
	var body map[string]any
	if decoder.Decode(&body) != nil || body == nil {
		return errors.New("invalid Production World repair base Candidate")
	}
	contentHash, err := CanonicalHash(value.Candidate)
	if err != nil || contentHash != value.CandidateContentHash {
		return errors.New("Production World repair base Candidate content has drifted")
	}
	return nil
}

func validSortedProductionWorldKeys(values []string, allowEmpty bool) bool {
	if values == nil || (!allowEmpty && len(values) == 0) || !slices.IsSorted(values) {
		return false
	}
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) ||
			(index > 0 && values[index-1] == value) {
			return false
		}
	}
	return true
}

func validProductionWorldRepairReason(operation, reason string) bool {
	return map[string]string{
		ProductionWorldRepairReviseEntity:      "production_entity_incorrect",
		ProductionWorldRepairRebindOccurrence:  "scene_occurrence_incorrect",
		ProductionWorldRepairReviseInteraction: "interaction_incorrect",
		ProductionWorldRepairReviseContinuity:  "continuity_incorrect",
	}[operation] == reason
}

func productionWorldTargetsInsideClosure(
	change ProductionWorldRepairChange,
	closure ProductionWorldRepairClosure,
) bool {
	allowed := []string(nil)
	switch change.Operation {
	case ProductionWorldRepairReviseEntity:
		allowed = append(append([]string(nil), closure.EntityKeys...), closure.StateKeys...)
	case ProductionWorldRepairRebindOccurrence:
		allowed = append(append([]string(nil), closure.SceneScopeKeys...), closure.OccurrenceKeys...)
	case ProductionWorldRepairReviseInteraction:
		allowed = closure.InteractionKeys
	case ProductionWorldRepairReviseContinuity:
		allowed = closure.ContinuityKeys
	default:
		return false
	}
	for _, target := range change.TargetKeys {
		if !slices.Contains(allowed, target) {
			return false
		}
	}
	return true
}

func productionWorldRepairRootStage(operation string) string {
	switch operation {
	case ProductionWorldRepairReviseEntity:
		return "derive_production_entities"
	case ProductionWorldRepairRebindOccurrence:
		return "bind_scene_occurrences"
	case ProductionWorldRepairReviseInteraction, ProductionWorldRepairReviseContinuity:
		return "reconcile_interaction_continuity"
	default:
		return ""
	}
}

func productionWorldRepairStageContains(stageKey, rootStage string) bool {
	order := map[string]int{
		"derive_production_entities": 1, "bind_scene_occurrences": 2, "reconcile_interaction_continuity": 3,
	}
	return order[rootStage] > 0 && order[stageKey] >= order[rootStage]
}
