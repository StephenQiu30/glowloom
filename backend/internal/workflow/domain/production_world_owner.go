package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/google/uuid"

	worlddomain "github.com/StephenQiu30/lanverse/backend/internal/production/world/domain"
)

const ProductionWorldOwnerMaterialSchema = "production-world-owner-material-production"

// ProductionWorldOwnerMaterial is the exact immutable input handed from Workflow
// to the Backend coordinator after Gate 2 approval.
type ProductionWorldOwnerMaterial struct {
	SchemaVersion string                               `json:"schema_version"`
	GateInputID   string                               `json:"gate_input_id"`
	GateInput     ProductionWorldGateInput             `json:"gate_input"`
	Candidate     worlddomain.ProductionWorldCandidate `json:"candidate"`
}

func DecodeProductionWorldOwnerMaterial(raw json.RawMessage) (ProductionWorldOwnerMaterial, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value ProductionWorldOwnerMaterial
	if err := decoder.Decode(&value); err != nil {
		return ProductionWorldOwnerMaterial{}, errors.New("invalid Production World owner material")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ProductionWorldOwnerMaterial{}, errors.New("invalid Production World owner material")
	}
	if value.SchemaVersion != ProductionWorldOwnerMaterialSchema {
		return ProductionWorldOwnerMaterial{}, errors.New("invalid Production World owner material schema")
	}
	if _, err := uuid.Parse(value.GateInputID); err != nil {
		return ProductionWorldOwnerMaterial{}, errors.New("invalid Production World owner Gate identity")
	}
	gateJSON, err := json.Marshal(value.GateInput)
	if err != nil {
		return ProductionWorldOwnerMaterial{}, err
	}
	gate, canonicalGate, err := DecodeProductionWorldGateInput(gateJSON)
	if err != nil || string(canonicalGate) != string(gateJSON) {
		return ProductionWorldOwnerMaterial{}, errors.New("Production World owner Gate input has drifted")
	}
	candidateJSON, err := json.Marshal(value.Candidate)
	if err != nil {
		return ProductionWorldOwnerMaterial{}, err
	}
	candidate, canonicalCandidate, err := worlddomain.DecodeProductionWorldCandidate(candidateJSON)
	if err != nil || string(canonicalCandidate) != string(candidateJSON) {
		return ProductionWorldOwnerMaterial{}, errors.New("Production World owner Candidate has drifted")
	}
	if candidate.WorkspaceID != gate.WorkspaceID || candidate.ProjectID != gate.ProjectID ||
		candidate.SourceVersion != gate.Subject.SourceVersion ||
		candidate.StructureIdentitySetVersion != gate.Subject.StructureIdentitySetVersion ||
		candidate.UpstreamCandidates.SceneOccurrence != gate.Subject.SceneOccurrenceCandidate ||
		candidate.UpstreamCandidates.InteractionContinuity != gate.Subject.InteractionCandidate.Candidate ||
		candidate.InteractionProjectionHash != gate.Subject.InteractionCandidate.ProjectionHash ||
		candidate.ContinuityProjectionHash != gate.Subject.ContinuityCandidate.ProjectionHash ||
		candidate.ContentHash != gate.Subject.ProductionWorldCandidate.CandidateContentHash {
		return ProductionWorldOwnerMaterial{}, errors.New("Production World owner read set has drifted")
	}
	return value, nil
}
