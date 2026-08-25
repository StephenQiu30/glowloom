package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	keyPattern     = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	valuePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
)

type compiledDefinition struct {
	definition NodeDefinition
	schema     *jsonschema.Schema
}

func NewCatalog(key, version string, definitions []NodeDefinition) (Catalog, error) {
	if !keyPattern.MatchString(key) || !versionPattern.MatchString(version) || len(definitions) == 0 {
		return Catalog{}, errors.New("invalid node catalog identity")
	}
	if len(definitions) > 500 {
		return Catalog{}, errors.New("node catalog exceeds definition limit")
	}

	normalized := make([]NodeDefinition, len(definitions))
	compiled := make(map[string]compiledDefinition, len(definitions))
	for index, value := range definitions {
		definition, schema, err := normalizeDefinition(value)
		if err != nil {
			return Catalog{}, fmt.Errorf("node definition %q: %w", value.Key, err)
		}
		identity := definitionIdentity(definition.Key, definition.Version)
		if _, exists := compiled[identity]; exists {
			return Catalog{}, fmt.Errorf("duplicate node definition %s", identity)
		}
		normalized[index] = definition
		compiled[identity] = compiledDefinition{definition: definition, schema: schema}
	}
	slices.SortFunc(normalized, func(left, right NodeDefinition) int {
		return strings.Compare(definitionIdentity(left.Key, left.Version), definitionIdentity(right.Key, right.Version))
	})
	contentHash, err := hashValue(struct {
		Key, Version string
		Definitions  []NodeDefinition
	}{key, version, normalized})
	if err != nil {
		return Catalog{}, err
	}
	executable := make([]NodeDefinition, 0, len(normalized))
	for _, definition := range normalized {
		if definition.Executable {
			executable = append(executable, definition)
		}
	}
	executionHash, err := hashValue(struct {
		Key, Version string
		Definitions  []NodeDefinition
	}{key, version, executable})
	if err != nil {
		return Catalog{}, err
	}
	return Catalog{
		Key: key, Version: version, Definitions: normalized,
		ContentHash: contentHash, ExecutionHash: executionHash, compiled: compiled,
	}, nil
}

func normalizeDefinition(value NodeDefinition) (NodeDefinition, *jsonschema.Schema, error) {
	if !keyPattern.MatchString(value.Key) || !versionPattern.MatchString(value.Version) || strings.TrimSpace(value.Name) == "" ||
		!keyPattern.MatchString(value.Category) || strings.TrimSpace(value.Executor) == "" ||
		!oneOf(value.CachePolicy, "never", "by_inputs") || !oneOf(value.RiskLevel, "low", "external_ai", "human_gate") {
		return NodeDefinition{}, nil, errors.New("invalid identity or execution policy")
	}
	inputs, err := normalizePorts(value.InputPorts)
	if err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("input ports: %w", err)
	}
	outputs, err := normalizePorts(value.OutputPorts)
	if err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("output ports: %w", err)
	}
	configSchema, err := canonicalJSON(value.ConfigSchema, true)
	if err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("config schema: %w", err)
	}
	parsedSchema, err := jsonschema.UnmarshalJSON(bytes.NewReader(configSchema))
	if err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("decode config schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	resource := "urn:lanverse:authoring:node:" + value.Key + ":" + value.Version
	if err = compiler.AddResource(resource, parsedSchema); err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("register config schema: %w", err)
	}
	compiledSchema, err := compiler.Compile(resource)
	if err != nil {
		return NodeDefinition{}, nil, fmt.Errorf("compile config schema: %w", err)
	}

	normalized := NodeDefinition{
		Key: value.Key, Version: value.Version, Name: strings.TrimSpace(value.Name), Category: value.Category,
		Executor: strings.TrimSpace(value.Executor), InputPorts: inputs, OutputPorts: outputs,
		ConfigSchema: configSchema, CachePolicy: value.CachePolicy, RiskLevel: value.RiskLevel, Executable: value.Executable,
	}
	normalized.ContentHash, err = hashValue(normalized)
	if err != nil {
		return NodeDefinition{}, nil, err
	}
	return normalized, compiledSchema, nil
}

func normalizePorts(values []PortDefinition) ([]PortDefinition, error) {
	normalized := append([]PortDefinition(nil), values...)
	seen := make(map[string]struct{}, len(normalized))
	for _, port := range normalized {
		if !keyPattern.MatchString(port.Key) || !valuePattern.MatchString(port.ValueType) {
			return nil, errors.New("invalid port identity")
		}
		if _, exists := seen[port.Key]; exists {
			return nil, fmt.Errorf("duplicate port %s", port.Key)
		}
		seen[port.Key] = struct{}{}
	}
	slices.SortFunc(normalized, func(left, right PortDefinition) int { return strings.Compare(left.Key, right.Key) })
	return normalized, nil
}

func canonicalJSON(raw json.RawMessage, requireObject bool) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("multiple JSON values are not allowed")
		}
		return nil, err
	}
	if requireObject {
		if _, ok := value.(map[string]any); !ok {
			return nil, errors.New("JSON value must be an object")
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func definitionIdentity(key, version string) string { return key + "@" + version }

func oneOf(value string, allowed ...string) bool {
	return slices.Contains(allowed, value)
}
