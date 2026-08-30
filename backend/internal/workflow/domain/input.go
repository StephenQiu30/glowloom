package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/google/uuid"

	authoring "github.com/StephenQiu30/lanverse/backend/internal/authoring/domain"
)

const (
	NodeInputSchemaVersion    = "node-input"
	NodeInputSourceNodeOutput = "node_output"
	NodeInputSourceVariable   = "variable"
)

type NodeInputBinding struct {
	Port             string          `json:"port"`
	ValueType        string          `json:"value_type"`
	SourceKind       string          `json:"source_kind"`
	SourceNodeID     string          `json:"source_node_id,omitempty"`
	SourcePort       string          `json:"source_port,omitempty"`
	Variable         string          `json:"variable,omitempty"`
	ReferenceID      string          `json:"reference_id,omitempty"`
	ReferenceVersion string          `json:"reference_version,omitempty"`
	ContentHash      string          `json:"content_hash,omitempty"`
	Value            json.RawMessage `json:"value,omitempty"`
}

type NodeInputSnapshot struct {
	SchemaVersion string                      `json:"schema_version"`
	Config        json.RawMessage             `json:"config"`
	Bindings      []NodeInputBinding          `json:"bindings"`
	FrozenInputs  []authoring.FrozenReference `json:"frozen_inputs"`
}

func BuildNodeInput(value NodeInputSnapshot) (NodeInputSnapshot, json.RawMessage, string, error) {
	if value.SchemaVersion != NodeInputSchemaVersion || len(value.FrozenInputs) == 0 {
		return NodeInputSnapshot{}, nil, "", errors.New("invalid node input snapshot")
	}
	config, err := canonicalWorkflowJSON(value.Config, true)
	if err != nil {
		return NodeInputSnapshot{}, nil, "", errors.New("invalid node input config")
	}
	bindings := append([]NodeInputBinding(nil), value.Bindings...)
	for index := range bindings {
		binding := &bindings[index]
		if !nodeOutputPortPattern.MatchString(binding.Port) || !nodeOutputValueTypePattern.MatchString(binding.ValueType) {
			return NodeInputSnapshot{}, nil, "", errors.New("invalid node input binding")
		}
		switch binding.SourceKind {
		case NodeInputSourceNodeOutput:
			referenceID, parseErr := uuid.Parse(binding.ReferenceID)
			if !nodeOutputPortPattern.MatchString(binding.SourceNodeID) ||
				!nodeOutputPortPattern.MatchString(binding.SourcePort) || parseErr != nil ||
				!nodeOutputReferenceVersionPattern.MatchString(binding.ReferenceVersion) ||
				!nodeOutputContentHashPattern.MatchString(binding.ContentHash) || binding.Variable != "" || len(binding.Value) != 0 {
				return NodeInputSnapshot{}, nil, "", errors.New("invalid node output input binding")
			}
			binding.ReferenceID = referenceID.String()
		case NodeInputSourceVariable:
			if !nodeOutputPortPattern.MatchString(binding.Variable) || binding.SourceNodeID != "" || binding.SourcePort != "" ||
				binding.ReferenceID != "" || binding.ReferenceVersion != "" || binding.ContentHash != "" || len(binding.Value) == 0 {
				return NodeInputSnapshot{}, nil, "", errors.New("invalid variable input binding")
			}
			canonical, canonicalErr := canonicalWorkflowJSON(binding.Value, false)
			if canonicalErr != nil {
				return NodeInputSnapshot{}, nil, "", errors.New("invalid variable input value")
			}
			binding.Value = canonical
		default:
			return NodeInputSnapshot{}, nil, "", errors.New("invalid node input source kind")
		}
	}
	slices.SortFunc(bindings, func(left, right NodeInputBinding) int { return strings.Compare(left.Port, right.Port) })
	for index := 1; index < len(bindings); index++ {
		if bindings[index-1].Port == bindings[index].Port {
			return NodeInputSnapshot{}, nil, "", errors.New("duplicate node input port")
		}
	}
	if len(bindings) == 0 {
		bindings = nil
	}

	frozen := append([]authoring.FrozenReference(nil), value.FrozenInputs...)
	for index := range frozen {
		reference := &frozen[index]
		parsedID, parseErr := uuid.Parse(reference.ID)
		if !nodeOutputPortPattern.MatchString(reference.Kind) || parseErr != nil ||
			!nodeOutputReferenceVersionPattern.MatchString(reference.Version) ||
			!nodeOutputContentHashPattern.MatchString(reference.Hash) {
			return NodeInputSnapshot{}, nil, "", errors.New("invalid frozen node input")
		}
		reference.ID = parsedID.String()
	}
	slices.SortFunc(frozen, func(left, right authoring.FrozenReference) int {
		return strings.Compare(left.Kind+":"+left.ID+":"+left.Version, right.Kind+":"+right.ID+":"+right.Version)
	})
	for index := range frozen {
		reference := &frozen[index]
		if index > 0 && frozen[index-1].Kind == reference.Kind && frozen[index-1].ID == reference.ID &&
			frozen[index-1].Version == reference.Version {
			return NodeInputSnapshot{}, nil, "", errors.New("duplicate frozen node input")
		}
	}

	normalized := NodeInputSnapshot{
		SchemaVersion: NodeInputSchemaVersion, Config: config, Bindings: bindings, FrozenInputs: frozen,
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return NodeInputSnapshot{}, nil, "", err
	}
	return normalized, encoded, sha256Hex(encoded), nil
}

func ParseNodeInput(raw json.RawMessage) (NodeInputSnapshot, json.RawMessage, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value NodeInputSnapshot
	if err := decoder.Decode(&value); err != nil {
		return NodeInputSnapshot{}, nil, "", errors.New("invalid node input snapshot")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return NodeInputSnapshot{}, nil, "", errors.New("invalid node input snapshot")
	}
	return BuildNodeInput(value)
}

func ValidateNodeOutputPorts(output NodeOutputSnapshot, expected []authoring.PortDefinition) error {
	normalized, _, _, err := BuildNodeOutput(output)
	if err != nil || len(expected) == 0 {
		return errors.New("invalid node output ports")
	}
	expectedByPort := make(map[string]authoring.PortDefinition, len(expected))
	for _, port := range expected {
		if !nodeOutputPortPattern.MatchString(port.Key) || !nodeOutputValueTypePattern.MatchString(port.ValueType) {
			return errors.New("invalid expected node output port")
		}
		if _, exists := expectedByPort[port.Key]; exists {
			return errors.New("duplicate expected node output port")
		}
		expectedByPort[port.Key] = port
	}
	observed := make(map[string]struct{}, len(normalized.Bindings))
	for _, binding := range normalized.Bindings {
		port, exists := expectedByPort[binding.Port]
		if !exists || port.ValueType != binding.ValueType {
			return errors.New("node output port has drifted")
		}
		observed[binding.Port] = struct{}{}
	}
	for _, port := range expected {
		if _, exists := observed[port.Key]; port.Required && !exists {
			return errors.New("required node output port is missing")
		}
	}
	return nil
}

func canonicalWorkflowJSON(raw json.RawMessage, requireObject bool) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON value")
	}
	if requireObject {
		if _, object := value.(map[string]any); !object {
			return nil, errors.New("JSON value must be an object")
		}
	}
	return json.Marshal(value)
}
