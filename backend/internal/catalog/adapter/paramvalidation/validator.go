// Package paramvalidation checks published model parameters against the
// catalog's shared JSON Schema and the version's supported modes.
package paramvalidation

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

// ErrInvalidConfiguration means a version cannot be published as configured.
var ErrInvalidConfiguration = errors.New("invalid model configuration")

//go:embed schema.json
var schemaFile embed.FS

// Validator keeps the compiled, immutable publication schema.
type Validator struct {
	schema *jsonschema.Schema
}

// NewValidator compiles the versioned Draft 2020-12 schema at construction.
func NewValidator() (*Validator, error) {
	content, err := schemaFile.ReadFile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("read model configuration schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("decode model configuration schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const location = "https://lanverse.local/catalog/model-version.schema.json"
	if err := compiler.AddResource(location, document); err != nil {
		return nil, fmt.Errorf("add model configuration schema: %w", err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return nil, fmt.Errorf("compile model configuration schema: %w", err)
	}
	return &Validator{schema: schema}, nil
}

// Validate checks a full version before it enters an append-only publication.
func (v *Validator) Validate(version domain.ModelVersion) error {
	if v == nil || v.schema == nil || version.Validate() != nil ||
		len(version.Limits) > 64*1024 || len(version.ParamSchema) > 64*1024 ||
		!uniqueJSONKeys(version.Limits) || !uniqueJSONKeys(version.ParamSchema) {
		return ErrInvalidConfiguration
	}
	payload, err := json.Marshal(struct {
		Modes       []string        `json:"modes"`
		Limits      json.RawMessage `json:"limits"`
		ParamSchema json.RawMessage `json:"param_schema"`
	}{Modes: version.Modes, Limits: version.Limits, ParamSchema: version.ParamSchema})
	if err != nil {
		return ErrInvalidConfiguration
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil || v.schema.Validate(value) != nil {
		return ErrInvalidConfiguration
	}
	root := value.(map[string]any)
	return validateFields(root["param_schema"].([]any), version.Modes)
}

func validateFields(fields []any, modes []string) error {
	allowedModes := make(map[string]bool, len(modes))
	for _, mode := range modes {
		allowedModes[mode] = true
	}
	seen := make(map[string]bool, len(fields))
	for _, entry := range fields {
		field := entry.(map[string]any)
		name := field["field"].(string)
		if seen[name] || strings.TrimSpace(field["label"].(string)) == "" {
			return fmt.Errorf("%w: duplicate field or empty label", ErrInvalidConfiguration)
		}
		seen[name] = true
		if selected, ok := field["for_modes"].([]any); ok {
			for _, mode := range selected {
				if !allowedModes[mode.(string)] {
					return fmt.Errorf("%w: unsupported field mode", ErrInvalidConfiguration)
				}
			}
		}
		if err := validateFieldValues(field); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldValues(field map[string]any) error {
	fieldType := field["type"].(string)
	component := field["component"].(string)
	options, hasOptions := field["enum"].([]any)
	_, hasMin := field["min"]
	_, hasMax := field["max"]
	_, hasStep := field["step"]
	if (hasMin || hasMax || hasStep) && fieldType != "integer" && fieldType != "number" {
		return fmt.Errorf("%w: numeric limits require a numeric field", ErrInvalidConfiguration)
	}
	for _, key := range []string{"min", "max", "step"} {
		if value, exists := field[key]; exists {
			if _, ok := number(value); !ok {
				return fmt.Errorf("%w: numeric limit exceeds supported range", ErrInvalidConfiguration)
			}
		}
	}
	switch component {
	case "switch":
		if fieldType != "boolean" || hasOptions {
			return fmt.Errorf("%w: switch requires a boolean field", ErrInvalidConfiguration)
		}
	case "textarea":
		if fieldType != "string" || hasOptions {
			return fmt.Errorf("%w: textarea requires a free text field", ErrInvalidConfiguration)
		}
	case "slider":
		if (fieldType != "integer" && fieldType != "number") || !hasMin || !hasMax || hasOptions {
			return fmt.Errorf("%w: slider requires a numeric range", ErrInvalidConfiguration)
		}
	case "select", "segmented", "voice":
		if !hasOptions || fieldType == "boolean" || (component == "voice" && fieldType != "string") {
			return fmt.Errorf("%w: choice component requires typed options", ErrInvalidConfiguration)
		}
	case "input":
		if fieldType == "boolean" || hasOptions {
			return fmt.Errorf("%w: input requires a free text or numeric field", ErrInvalidConfiguration)
		}
	}
	if hasMin && hasMax {
		minimum, _ := number(field["min"])
		maximum, _ := number(field["max"])
		if minimum.Cmp(maximum) > 0 {
			return fmt.Errorf("%w: minimum exceeds maximum", ErrInvalidConfiguration)
		}
	}
	if fieldType == "integer" {
		for _, key := range []string{"min", "max", "step"} {
			if value, ok := field[key]; ok && !isInteger(value) {
				return fmt.Errorf("%w: integer field has fractional bound", ErrInvalidConfiguration)
			}
		}
	}
	for _, option := range options {
		if !matchesType(option, fieldType) || !withinRange(option, field) ||
			(fieldType == "string" && strings.TrimSpace(option.(string)) == "") {
			return fmt.Errorf("%w: option type or range mismatch", ErrInvalidConfiguration)
		}
	}
	if defaultValue, ok := field["default"]; ok {
		if !matchesType(defaultValue, fieldType) ||
			(field["required"] == true && fieldType == "string" && strings.TrimSpace(defaultValue.(string)) == "") {
			return fmt.Errorf("%w: default type mismatch", ErrInvalidConfiguration)
		}
		if hasOptions && !containsOption(options, defaultValue) {
			return fmt.Errorf("%w: default is outside options", ErrInvalidConfiguration)
		}
		if !withinRange(defaultValue, field) {
			return fmt.Errorf("%w: default is outside range", ErrInvalidConfiguration)
		}
		if component == "slider" && hasStep && !onStep(defaultValue, field) {
			return fmt.Errorf("%w: slider default misses step", ErrInvalidConfiguration)
		}
	}
	return nil
}

func withinRange(value any, field map[string]any) bool {
	numeric, ok := number(value)
	if !ok {
		return true
	}
	if minValue, exists := field["min"]; exists {
		minimum, _ := number(minValue)
		if numeric.Cmp(minimum) < 0 {
			return false
		}
	}
	if maxValue, exists := field["max"]; exists {
		maximum, _ := number(maxValue)
		if numeric.Cmp(maximum) > 0 {
			return false
		}
	}
	return true
}

func onStep(value any, field map[string]any) bool {
	numeric, valueOK := number(value)
	minimum, minOK := number(field["min"])
	step, stepOK := number(field["step"])
	if !valueOK || !minOK || !stepOK {
		return false
	}
	delta := new(big.Rat).Sub(numeric, minimum)
	return new(big.Rat).Quo(delta, step).IsInt()
}

func matchesType(value any, fieldType string) bool {
	switch fieldType {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := number(value)
		return ok
	case "integer":
		return isInteger(value)
	default:
		return false
	}
}

func isInteger(value any) bool {
	numeric, ok := number(value)
	return ok && numeric.IsInt()
}

func number(value any) (*big.Rat, bool) {
	encoded, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	numeric, ok := new(big.Rat).SetString(encoded.String())
	if !ok || numeric.Cmp(big.NewRat(-9007199254740991, 1)) < 0 ||
		numeric.Cmp(big.NewRat(9007199254740991, 1)) > 0 {
		return nil, false
	}
	return numeric, true
}

func containsOption(options []any, value any) bool {
	for _, option := range options {
		if option == value {
			return true
		}
		first, firstNumeric := number(option)
		second, secondNumeric := number(value)
		if firstNumeric && secondNumeric && first.Cmp(second) == 0 {
			return true
		}
	}
	return false
}

func uniqueJSONKeys(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(decoder, 0); err != nil {
		return false
	}
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return ErrInvalidConfiguration
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return ErrInvalidConfiguration
			}
			seen[key] = true
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	default:
		return ErrInvalidConfiguration
	}
}
