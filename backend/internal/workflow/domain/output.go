package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"

	"github.com/google/uuid"
)

const NodeOutputSchemaVersion = "node-output-v1"

var (
	nodeOutputPortPattern             = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)
	nodeOutputValueTypePattern        = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
	nodeOutputReferenceVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)
	nodeOutputContentHashPattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type NodeOutputBinding struct {
	Port             string `json:"port"`
	ValueType        string `json:"value_type"`
	ReferenceID      string `json:"reference_id"`
	ReferenceVersion string `json:"reference_version"`
	ContentHash      string `json:"content_hash"`
}

type NodeOutputSnapshot struct {
	SchemaVersion string              `json:"schema_version"`
	Bindings      []NodeOutputBinding `json:"bindings"`
}

func BuildNodeOutput(value NodeOutputSnapshot) (NodeOutputSnapshot, json.RawMessage, string, error) {
	if value.SchemaVersion != NodeOutputSchemaVersion || len(value.Bindings) == 0 {
		return NodeOutputSnapshot{}, nil, "", errors.New("invalid node output snapshot")
	}
	bindings := append([]NodeOutputBinding(nil), value.Bindings...)
	for index := range bindings {
		binding := &bindings[index]
		referenceID, err := uuid.Parse(binding.ReferenceID)
		if !nodeOutputPortPattern.MatchString(binding.Port) ||
			!nodeOutputValueTypePattern.MatchString(binding.ValueType) || err != nil ||
			!nodeOutputReferenceVersionPattern.MatchString(binding.ReferenceVersion) ||
			!nodeOutputContentHashPattern.MatchString(binding.ContentHash) {
			return NodeOutputSnapshot{}, nil, "", errors.New("invalid node output binding")
		}
		binding.ReferenceID = referenceID.String()
	}
	slices.SortFunc(bindings, func(left, right NodeOutputBinding) int {
		return nodeOutputPortCompare(left.Port, right.Port)
	})
	for index := 1; index < len(bindings); index++ {
		if bindings[index-1].Port == bindings[index].Port {
			return NodeOutputSnapshot{}, nil, "", errors.New("duplicate node output port")
		}
	}
	normalized := NodeOutputSnapshot{SchemaVersion: NodeOutputSchemaVersion, Bindings: bindings}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return NodeOutputSnapshot{}, nil, "", err
	}
	return normalized, encoded, sha256Hex(encoded), nil
}

func ParseNodeOutput(raw json.RawMessage) (NodeOutputSnapshot, json.RawMessage, string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value NodeOutputSnapshot
	if err := decoder.Decode(&value); err != nil {
		return NodeOutputSnapshot{}, nil, "", errors.New("invalid node output snapshot")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return NodeOutputSnapshot{}, nil, "", errors.New("invalid node output snapshot")
	}
	return BuildNodeOutput(value)
}

func CanonicalNodeOutput(raw json.RawMessage) (json.RawMessage, string, error) {
	_, canonical, outputHash, err := ParseNodeOutput(raw)
	return canonical, outputHash, err
}

func nodeOutputPortCompare(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
