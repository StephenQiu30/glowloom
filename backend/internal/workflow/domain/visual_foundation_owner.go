package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	"github.com/google/uuid"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	presetdomain "github.com/StephenQiu30/lanverse/backend/internal/preset/domain"
	storygraphdomain "github.com/StephenQiu30/lanverse/backend/internal/storygraph/domain"
)

const VisualFoundationOwnerMaterialSchema = "visual-foundation-owner-material-production"

// VisualFoundationOwnerMaterial freezes the exact Backend facts required to
// atomically publish both Gate 3 Owner collections after approval.
type VisualFoundationOwnerMaterial struct {
	SchemaVersion            string                                         `json:"schema_version"`
	GateInputID              string                                         `json:"gate_input_id"`
	GateInput                VisualFoundationScopeGateInput                 `json:"gate_input"`
	ConfirmedProductionWorld storygraphdomain.ReferencePlanWorldReadSet     `json:"confirmed_production_world"`
	Selection                presetdomain.ProjectSelection                  `json:"selection"`
	Release                  presetdomain.Release                           `json:"release"`
	VisualCandidate          VisualFoundationScopeCandidateRevisionMaterial `json:"visual_candidate"`
	ReferencePlanInput       agentcontract.ReferencePlanInput               `json:"reference_plan_input"`
	ReferenceCandidate       VisualFoundationScopeCandidateRevisionMaterial `json:"reference_candidate"`
}

func DecodeVisualFoundationOwnerMaterial(raw json.RawMessage) (VisualFoundationOwnerMaterial, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value VisualFoundationOwnerMaterial
	if err := decoder.Decode(&value); err != nil {
		return VisualFoundationOwnerMaterial{}, errors.New("invalid Visual Foundation owner material")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return VisualFoundationOwnerMaterial{}, errors.New("invalid Visual Foundation owner material")
	}
	if value.SchemaVersion != VisualFoundationOwnerMaterialSchema {
		return VisualFoundationOwnerMaterial{}, errors.New("invalid Visual Foundation owner material schema")
	}
	if parsed, err := uuid.Parse(value.GateInputID); err != nil || parsed == uuid.Nil {
		return VisualFoundationOwnerMaterial{}, errors.New("invalid Visual Foundation owner Gate identity")
	}
	gateJSON, err := json.Marshal(value.GateInput)
	if err != nil {
		return VisualFoundationOwnerMaterial{}, err
	}
	gate, _, err := DecodeVisualFoundationScopeGateInput(gateJSON)
	if err != nil {
		return VisualFoundationOwnerMaterial{}, errors.New("Visual Foundation owner Gate input is invalid")
	}
	rebuiltSubject, _, err := NewVisualFoundationScopeSubject(VisualFoundationScopeSubjectDraft{
		ConfirmedProductionWorld:   value.ConfirmedProductionWorld,
		ProjectPresetSelection:     value.Selection,
		PresetRelease:              value.Release,
		VisualFoundationCandidate:  value.VisualCandidate,
		ReferencePlanInput:         value.ReferencePlanInput,
		ReferencePlanCandidate:     value.ReferenceCandidate,
		ExpectedReferenceTargetSet: gate.Subject.ExpectedReferenceTargetSet,
	})
	if err != nil || !reflect.DeepEqual(rebuiltSubject, gate.Subject) {
		return VisualFoundationOwnerMaterial{}, errors.New("Visual Foundation owner read set has drifted")
	}
	return value, nil
}
