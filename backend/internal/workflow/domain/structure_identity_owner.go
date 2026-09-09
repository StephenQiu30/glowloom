package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

const StructureIdentityOwnerMaterialSchema = "structure-identity-owner-material-production"

type StructureIdentityOwnerCandidate struct {
	Identity  agentcontract.SceneAnalysisCandidateRevisionIdentity `json:"identity"`
	Release   agentcontract.SceneAnalysisReleaseIdentity           `json:"release"`
	Candidate json.RawMessage                                      `json:"candidate"`
}

type StructureIdentityOwnerMaterial struct {
	SchemaVersion string                            `json:"schema_version"`
	GateInputID   string                            `json:"gate_input_id"`
	GateInput     StructureIdentityGateInput        `json:"gate_input"`
	SpanIndexID   string                            `json:"span_index_id"`
	SpanIndexHash string                            `json:"span_index_hash"`
	Candidates    []StructureIdentityOwnerCandidate `json:"candidates"`
}

func DecodeStructureIdentityOwnerMaterial(raw json.RawMessage) (StructureIdentityOwnerMaterial, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value StructureIdentityOwnerMaterial
	if err := decoder.Decode(&value); err != nil {
		return StructureIdentityOwnerMaterial{}, errors.New("invalid Structure Identity owner material")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return StructureIdentityOwnerMaterial{}, errors.New("invalid Structure Identity owner material")
	}
	if value.SchemaVersion != StructureIdentityOwnerMaterialSchema || value.GateInputID == "" ||
		value.SpanIndexID == "" || !nodeOutputContentHashPattern.MatchString(value.SpanIndexHash) ||
		len(value.Candidates) != 4 {
		return StructureIdentityOwnerMaterial{}, errors.New("incomplete Structure Identity owner material")
	}
	gateJSON, err := json.Marshal(value.GateInput)
	if err != nil {
		return StructureIdentityOwnerMaterial{}, err
	}
	_, canonicalGate, err := DecodeStructureIdentityGateInput(gateJSON)
	if err != nil || string(canonicalGate) != string(gateJSON) {
		return StructureIdentityOwnerMaterial{}, errors.New("Structure Identity owner Gate input has drifted")
	}
	expected := map[string]agentcontract.SceneAnalysisCandidateRevisionIdentity{
		"propose_script_spans": value.GateInput.Subject.SpanCandidate,
		"extract_scene_facts":  value.GateInput.Subject.SceneFactCandidate,
		"resolve_identities":   value.GateInput.Subject.IdentityCandidate,
		"review_candidate":     value.GateInput.Subject.ReviewCandidate,
	}
	for _, candidate := range value.Candidates {
		frozen, exists := expected[candidate.Identity.StageKey]
		if !exists || candidate.Identity != frozen || candidate.Release.Validate() != nil ||
			len(candidate.Candidate) == 0 || candidate.Candidate[0] != '{' {
			return StructureIdentityOwnerMaterial{}, errors.New("Structure Identity owner Candidate lineage has drifted")
		}
		delete(expected, candidate.Identity.StageKey)
	}
	if len(expected) != 0 {
		return StructureIdentityOwnerMaterial{}, errors.New("Structure Identity owner Candidate set is incomplete")
	}
	return value, nil
}
