package application

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ErrInvalidFingerprint means the frozen request cannot safely identify a result.
var ErrInvalidFingerprint = errors.New("invalid operation input fingerprint")

// FingerprintInput contains the effective request after target expansion and
// parameter validation. Price and project are deliberately absent: the caller
// scopes reuse by project and price does not change the generated output.
type FingerprintInput struct {
	Capability            string
	Mode                  string
	ModelProfileVersionID uuid.UUID
	ProviderModelID       string
	OutputCount           int32
	Params                json.RawMessage
	ParamSchema           json.RawMessage
	Inputs                []FingerprintPart
	FinalPrompt           string
}

// FingerprintPart describes one frozen input. Asset IDs preserve the identity
// of direct media and mask inputs; digests detect changes to their contents.
type FingerprintPart struct {
	SeqNo        int32      `json:"seq_no"`
	Role         string     `json:"role"`
	RefType      string     `json:"ref_type"`
	RefID        *uuid.UUID `json:"ref_id"`
	RefVersion   string     `json:"ref_version"`
	MediaAssetID *uuid.UUID `json:"media_asset_id,omitempty"`
	MaskAssetID  *uuid.UUID `json:"mask_asset_id,omitempty"`
	MediaSHA256  string     `json:"media_sha256"`
	MaskSHA256   string     `json:"mask_sha256"`
	TextValue    string     `json:"text_value"`
}

type fingerprintParam struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type fingerprintField struct {
	Field    string          `json:"field"`
	Type     string          `json:"type"`
	Default  json.RawMessage `json:"default"`
	ForModes []string        `json:"for_modes"`
}

// InputHash returns a deterministic SHA-256 fingerprint and whether a prior
// completed result is eligible for reuse. A model's active seed field without
// an effective value makes the request random, even though its hash is stable.
func InputHash(input FingerprintInput) (string, bool, error) {
	if strings.TrimSpace(input.Capability) == "" || strings.TrimSpace(input.Mode) == "" ||
		input.ModelProfileVersionID == uuid.Nil || strings.TrimSpace(input.ProviderModelID) == "" ||
		input.OutputCount < 1 || input.OutputCount > 8 || len(input.Params) > 64*1024 ||
		len(input.ParamSchema) > 64*1024 {
		return "", false, ErrInvalidFingerprint
	}
	params, reusable, err := normalizeFingerprintParams(input.Params, input.ParamSchema, input.Mode)
	if err != nil {
		return "", false, err
	}
	parts, err := normalizeFingerprintParts(input.Inputs)
	if err != nil {
		return "", false, err
	}
	payload, err := json.Marshal(struct {
		Version               int                         `json:"version"`
		Capability            string                      `json:"capability"`
		Mode                  string                      `json:"mode"`
		ModelProfileVersionID uuid.UUID                   `json:"model_profile_version_id"`
		ProviderModelID       string                      `json:"provider_model_id"`
		OutputCount           int32                       `json:"output_count"`
		Params                map[string]fingerprintParam `json:"params"`
		Inputs                []FingerprintPart           `json:"inputs"`
		FinalPrompt           string                      `json:"prompt"`
	}{
		Version: 1, Capability: input.Capability, Mode: input.Mode,
		ModelProfileVersionID: input.ModelProfileVersionID, ProviderModelID: input.ProviderModelID,
		OutputCount: input.OutputCount, Params: params, Inputs: parts, FinalPrompt: input.FinalPrompt,
	})
	if err != nil {
		return "", false, fmt.Errorf("encode operation fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), reusable, nil
}

func normalizeFingerprintParams(rawParams, rawSchema json.RawMessage, mode string) (map[string]fingerprintParam, bool, error) {
	provided, err := fingerprintObject(rawParams)
	if err != nil {
		return nil, false, err
	}
	trimmedSchema := bytes.TrimSpace(rawSchema)
	if len(trimmedSchema) < 2 || trimmedSchema[0] != '[' {
		return nil, false, ErrInvalidFingerprint
	}
	var fields []fingerprintField
	if err := json.Unmarshal(trimmedSchema, &fields); err != nil {
		return nil, false, ErrInvalidFingerprint
	}
	seen := make(map[string]bool, len(fields))
	normalized := make(map[string]fingerprintParam, len(provided))
	reusable := true
	for _, field := range fields {
		if field.Field == "" || seen[field.Field] {
			return nil, false, ErrInvalidFingerprint
		}
		switch field.Type {
		case "string", "boolean", "integer", "number":
		default:
			return nil, false, ErrInvalidFingerprint
		}
		seen[field.Field] = true
		active := len(field.ForModes) == 0 || slices.Contains(field.ForModes, mode)
		value, present := provided[field.Field]
		if !active {
			if present {
				return nil, false, ErrInvalidFingerprint
			}
			continue
		}
		var defaultValue fingerprintParam
		hasDefault := len(field.Default) != 0
		if hasDefault {
			defaultValue, err = normalizeFingerprintValue(field.Default, field.Type)
			if err != nil {
				return nil, false, err
			}
		}
		if field.Field == "seed" && !present {
			reusable = false
			// Keep random requests distinct from an explicit value equal to
			// the schema default, which normalization otherwise removes.
			normalized[field.Field] = fingerprintParam{Kind: "random"}
		}
		if !present {
			continue
		}
		candidate, err := normalizeFingerprintValue(value, field.Type)
		if err != nil {
			return nil, false, err
		}
		if !hasDefault || candidate != defaultValue {
			normalized[field.Field] = candidate
		}
		delete(provided, field.Field)
	}
	if len(provided) != 0 {
		return nil, false, ErrInvalidFingerprint
	}
	return normalized, reusable, nil
}

func fingerprintObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidFingerprint
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidFingerprint
		}
		name, ok := nameToken.(string)
		if !ok || name == "" {
			return nil, ErrInvalidFingerprint
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidFingerprint
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, ErrInvalidFingerprint
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return nil, ErrInvalidFingerprint
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidFingerprint
	}
	return fields, nil
}

func normalizeFingerprintValue(raw json.RawMessage, fieldType string) (fingerprintParam, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fingerprintParam{}, ErrInvalidFingerprint
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fingerprintParam{}, ErrInvalidFingerprint
	}
	switch fieldType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return fingerprintParam{}, ErrInvalidFingerprint
		}
		return fingerprintParam{Kind: fieldType, Value: text}, nil
	case "boolean":
		flag, ok := value.(bool)
		if !ok {
			return fingerprintParam{}, ErrInvalidFingerprint
		}
		return fingerprintParam{Kind: fieldType, Value: strconv.FormatBool(flag)}, nil
	case "integer", "number":
		number, ok := value.(json.Number)
		if !ok {
			return fingerprintParam{}, ErrInvalidFingerprint
		}
		exact, err := exactFingerprintNumber(number.String())
		if err != nil || fieldType == "integer" && !exact.IsInt() {
			return fingerprintParam{}, ErrInvalidFingerprint
		}
		return fingerprintParam{Kind: fieldType, Value: exact.RatString()}, nil
	default:
		return fingerprintParam{}, ErrInvalidFingerprint
	}
}

func exactFingerprintNumber(value string) (*big.Rat, error) {
	if len(value) > 512 {
		return nil, ErrInvalidFingerprint
	}
	exponent := 0
	if at := strings.IndexAny(value, "eE"); at >= 0 {
		parsed, err := strconv.Atoi(value[at+1:])
		if err != nil || parsed < -512 || parsed > 512 {
			return nil, ErrInvalidFingerprint
		}
		exponent, value = parsed, value[:at]
	}
	negative := strings.HasPrefix(value, "-")
	value = strings.TrimPrefix(value, "-")
	decimalPlaces := 0
	if at := strings.IndexByte(value, '.'); at >= 0 {
		decimalPlaces = len(value) - at - 1
		value = value[:at] + value[at+1:]
	}
	integer, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return nil, ErrInvalidFingerprint
	}
	if negative {
		integer.Neg(integer)
	}
	scale := exponent - decimalPlaces
	if scale >= 0 {
		integer.Mul(integer, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
		return new(big.Rat).SetInt(integer), nil
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil)
	return new(big.Rat).SetFrac(integer, denominator), nil
}

func normalizeFingerprintParts(parts []FingerprintPart) ([]FingerprintPart, error) {
	if len(parts) == 0 || len(parts) > 256 {
		return nil, ErrInvalidFingerprint
	}
	ordered := slices.Clone(parts)
	slices.SortFunc(ordered, func(a, b FingerprintPart) int {
		return cmp.Compare(a.SeqNo, b.SeqNo)
	})
	for index := range ordered {
		part := &ordered[index]
		if part.SeqNo < 0 || index > 0 && part.SeqNo == ordered[index-1].SeqNo ||
			strings.TrimSpace(part.Role) == "" || strings.TrimSpace(part.RefType) == "" ||
			part.RefID != nil && *part.RefID == uuid.Nil ||
			part.MediaAssetID != nil && (*part.MediaAssetID == uuid.Nil || part.MediaSHA256 == "") ||
			part.MaskAssetID != nil && (*part.MaskAssetID == uuid.Nil || part.MaskSHA256 == "") ||
			(part.RefID == nil && part.TextValue == "" && part.MediaSHA256 == "" && part.MaskSHA256 == "") {
			return nil, ErrInvalidFingerprint
		}
		for _, digest := range []*string{&part.MediaSHA256, &part.MaskSHA256} {
			if *digest == "" {
				continue
			}
			if len(*digest) != 64 {
				return nil, ErrInvalidFingerprint
			}
			decoded, err := hex.DecodeString(*digest)
			if err != nil || len(decoded) != sha256.Size {
				return nil, ErrInvalidFingerprint
			}
			*digest = strings.ToLower(*digest)
		}
		part.SeqNo = int32(index)
	}
	return ordered, nil
}
